package client

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/net/websocket"
	kapi "github.com/glerchundi/kubelistener/pkg/client/api/v1"
	kruntime "github.com/glerchundi/kubelistener/pkg/client/runtime"
)

type Client struct {
	// derived config
	tls *tls.Config
	headers http.Header
	baseURL string
	// user-provided configuration
	config *Config
}

type Config struct {
	MasterURL string
	Auth ClientAuth
	CaCertificate []byte
}

type ClientAuth interface {
}

type ClientCertificateAuth struct {
	ClientCertificate []byte
	ClientKey []byte
}

type TokenAuth struct {
	Token string
}

type UsernameAndPasswordAuth struct {
	Username string
	Password string
}

type Informer struct {
	// derived config
	httpClient *http.Client
	httpReq *http.Request
	wsConfig *websocket.Config
	// inherited config
	interval time.Duration
	// inter-routine comm.
	rc resourceCreator
	funnelChan chan interface{}
	// control flow channels
	stopChan <-chan struct{}
	doneChan chan bool
	errChan chan error
}

type resourceCreator interface {
	item() kruntime.Object
	list() kruntime.Object
}

type serviceCreator struct {}
func (*serviceCreator) item() kruntime.Object { return &kapi.Service{} }
func (*serviceCreator) list() kruntime.Object { return &kapi.ServiceList{} }

var resourceCreatorMap = map[string]resourceCreator {
	"services": &serviceCreator{},
}

func NewClient(config *Config) (*Client, error) {
	client := &Client{config: config}

	masterURL := config.MasterURL
	if masterURL == "" {
		glog.Warning("Master URL not set, discovering k8s service through env vars KUBERNETES_SERVICE{HOST,PORT}...")
		k8sSvcHost := os.Getenv("KUBERNETES_SERVICE_HOST")
		if k8sSvcHost == "" {
			return nil, fmt.Errorf("empty KUBERNETES_SERVICE_HOST environment variable")
		}

		k8sSvcPort := os.Getenv("KUBERNETES_SERVICE_PORT")
		if k8sSvcPort == "" {
			return nil, fmt.Errorf("empty KUBERNETES_SERVICE_PORT environment variable")
		}

		masterURL = fmt.Sprintf("https://%s:%s", k8sSvcHost, k8sSvcPort)
	}

	url, err := url.Parse(masterURL)
	if err != nil {
		return nil, err
	}

	scheme := strings.ToLower(url.Scheme)
	if scheme == "" || (scheme != "http" && scheme != "https") {
		return nil, fmt.Errorf("invalid url scheme: '%s'", scheme)
	}

	secure := scheme == "https"
	if secure && client.config.CaCertificate != nil {
		// Create CA certificate pool
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM([]byte(config.CaCertificate)); !ok {
			return nil, fmt.Errorf("unable to load CA certificate")
		}

		// Setup TLS config
		client.tls = &tls.Config{RootCAs: pool}
	}

	// Load authentication parameters depending on the type
	switch auth := client.config.Auth.(type) {
	case *ClientCertificateAuth:
		if !secure {
			return nil, fmt.Errorf("client certificate requires using a secure endpoint")
		}

		cert, err := tls.X509KeyPair(auth.ClientCertificate, auth.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("x509 client key pair could not be generated: %v", err)
		}
		client.tls.Certificates = []tls.Certificate{cert}
	case *TokenAuth:
		client.headers = http.Header {
			"Authorization": { fmt.Sprintf("Bearer %s", auth.Token) },
		}
	case *UsernameAndPasswordAuth:
		encodedAuth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprint("%s:%s", auth.Username, auth.Password)))
		client.headers = http.Header {
			"Authorization": { fmt.Sprintf("Basic %s", encodedAuth) },
		}
	default:
		return nil, fmt.Errorf("unknown auth type: %v", auth)
	}

	if client.tls != nil {
		client.tls.BuildNameToCertificate()
	}

	client.baseURL = fmt.Sprintf("%s/api/v1", url.Host)

	return client, nil
}

func (c *Client) NewInformer(namespace, resource, selector string, resyncInterval time.Duration,
                             stopChan <-chan struct{}, doneChan chan bool, errChan chan error) (*Informer, error) {
	// HTTP Client
	httpURL := c.getResourcesURL("http", namespace, resource, false)
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: c.tls,
		},
	}

	httpReq, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: GET %s : %v", httpURL, err)
	}
	httpReq.Header = c.headers

	// WebSocket Client
	wsURL := c.getResourcesURL("ws", namespace, resource, true)
	wsConfig, err := websocket.NewConfig(wsURL, "http://localhost"); if err != nil {
		return nil, err
	}
	wsConfig.TlsConfig = c.tls
	wsConfig.Header = c.headers

	// Return informer
	return &Informer{
		httpClient, httpReq,
		wsConfig,
		resyncInterval,
		resourceCreatorMap[resource],
		make(chan interface{}, 100),
		stopChan, doneChan, errChan,
	}, nil
}

func (c *Client) getResourcesURL(schemePrefix, namespace, resource string, watch bool) string {
	// define scheme based on TLS
	scheme := schemePrefix
	if c.tls != nil {
		scheme = fmt.Sprintf("%ss", schemePrefix)
	}

	// Add watch prefix if needed
	watchPrefix := ""
	if watch {
		watchPrefix = "watch/"
	}

	// Return resources URL
	return fmt.Sprintf("%s://%s/%snamespaces/%s/%s", scheme, c.baseURL, watchPrefix, namespace, resource)
}

func (i *Informer) watch() {
	wsConn, err := websocket.DialConfig(i.wsConfig)
	if err != nil {
		i.errChan <- err
		return
	}

	for {
		select {
		case <-i.stopChan:
			break
		default:
			v := i.rc.item()
			we := &kapi.WatchEvent{Object:v}
			if err := websocket.JSON.Receive(wsConn, &we); err != nil {
				i.errChan <- err
				return
			}
			i.funnelChan <- we
		}
	}
}

func (i *Informer) list() {
	httpURL := i.httpReq.URL.String()
	for {
		res, err := ctxhttp.Do(context.Background(), i.httpClient, i.httpReq)
		if err != nil {
			i.errChan <- fmt.Errorf("failed to make request: GET %s: %v", httpURL, err)
			return
		}

		body, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			i.errChan <- fmt.Errorf("failed to read request body for GET %s: %v", httpURL, err)
			return
		}

		if res.StatusCode != http.StatusOK {
			i.errChan <- fmt.Errorf("http error %d GET %q: %s: %v", res.StatusCode, httpURL, string(body), err)
			return
		}

		v := i.rc.list()
		if err := json.Unmarshal(body, &v); err != nil {
			i.errChan <- fmt.Errorf("failed to decode list of pod resources: %v", err)
			return
		}

		i.funnelChan <- v

		select {
		case <-i.stopChan:
			break
		case <-time.After(i.interval):
			continue
		}
	}
}

func (i *Informer) process(v interface{}) {
	switch vv := v.(type) {
	case *kapi.Service:
		fmt.Printf("*SERVICE:     %v\n\n", vv)
	case *kapi.ServiceList:
		fmt.Printf("*SERVICELIST: %v\n\n", vv)
	case *kapi.WatchEvent:
		fmt.Printf("*WATCHEVENT:  %v\n", vv)
	default:
		fmt.Printf("DEFAULT:      %v\n\n", vv)
	}
}

func (i *Informer) Run() {
	defer close(i.doneChan)

	var wg sync.WaitGroup

	// watch through websocket endpoint
	wg.Add(1)
	go func() {
		defer wg.Done()
		i.watch()
	}()

	// list using http endpoint
	wg.Add(1)
	go func() {
		defer wg.Done()
		i.list()
	}()

	// serializer channel
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case v := <-i.funnelChan:
				i.process(v)
			}
		}
	}()

	// wait until both finished
	wg.Wait()
}