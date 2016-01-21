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

	"github.com/gorilla/websocket"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	log "github.com/glerchundi/logrus"
	kapi "github.com/glerchundi/kubelistener/pkg/client/api/v1"
	kruntime "github.com/glerchundi/kubelistener/pkg/client/runtime"
)

type Client struct {
	// derived config
	tls *tls.Config
	reqHeader http.Header
	baseURL string
	// user-provided configuration
	config *ClientConfig
}

type ClientConfig struct {
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
	wsURL string
	wsDialer *websocket.Dialer
	wsHeader http.Header
	// user-provided configuration
	config *InformerConfig
	// inter-routine comm.
	rc resourceCreator
	recvChan chan<- interface{}
	// control flow channels
	stopChan <-chan struct{}
	doneChan chan bool
	errChan chan error
}

type InformerConfig struct {
	Namespace string
	Resource string
	Selector string
	ResyncInterval time.Duration
}

type resourceCreator interface {
	item() kruntime.Object
	list() kruntime.Object
}

type podCreator struct {}
func (*podCreator) item() kruntime.Object { return &kapi.Pod{} }
func (*podCreator) list() kruntime.Object { return &kapi.PodList{} }

type replicationControllerCreator struct {}
func (*replicationControllerCreator) item() kruntime.Object { return &kapi.ReplicationController{} }
func (*replicationControllerCreator) list() kruntime.Object { return &kapi.ReplicationControllerList{} }

type serviceCreator struct {}
func (*serviceCreator) item() kruntime.Object { return &kapi.Service{} }
func (*serviceCreator) list() kruntime.Object { return &kapi.ServiceList{} }

var resourceCreatorMap = map[string]resourceCreator {
	"pods": &podCreator{},
	"replicationcontrollers": &replicationControllerCreator{},
	"services": &serviceCreator{},
}

func copyHeader(hIn http.Header) http.Header {
	hOut := make(http.Header, len(hIn))
	for k, vv := range hIn {
		vv2 := make([]string, len(vv))
		copy(vv2, vv)
		hOut[k] = vv2
	}
	return hOut
}

func NewClient(config *ClientConfig) (*Client, error) {
	client := &Client{config: config}

	masterURL := config.MasterURL
	if masterURL == "" {
		log.Warn("Master URL not set, discovering k8s service through env vars KUBERNETES_SERVICE{HOST,PORT}...")
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
		client.reqHeader = http.Header {
			"Authorization": { fmt.Sprintf("Bearer %s", auth.Token) },
		}
	case *UsernameAndPasswordAuth:
		encodedAuth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprint("%s:%s", auth.Username, auth.Password)))
		client.reqHeader = http.Header {
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

func (c *Client) NewInformer(config *InformerConfig, recvChan chan<- interface{},
                             stopChan <-chan struct{}, doneChan chan bool, errChan chan error) (*Informer, error) {
	// Check if a channel was provided
	if recvChan == nil {
		return nil, fmt.Errorf("no recv chan was provided")
	}

	// Use POD_NAMESPACE as default value or fallback to "default"
	namespace := config.Namespace
	if namespace == "" {
		namespace = os.Getenv("POD_NAMESPACE")
		if namespace == "" {
			namespace = "default"
		}
	}

	// HTTP Client
	httpURL := c.getResourcesURL("http", namespace, config.Resource, false)
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: c.tls,
		},
	}

	httpReq, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: GET %s : %v", httpURL, err)
	}
	httpReq.Header = copyHeader(c.reqHeader)

	// WebSocket Dialer
	wsURL := c.getResourcesURL("ws", namespace, config.Resource, true)
	wsDialer := &websocket.Dialer{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: c.tls,
	}
	wsHeader := copyHeader(c.reqHeader)
	wsHeader.Add("Origin", "http://localhost")

	resourceCreator, ok := resourceCreatorMap[config.Resource]
	if !ok {
		return nil, fmt.Errorf("'%s' is not a valid resource type", config.Resource)
	}

	// Return informer
	return &Informer{
		httpClient, httpReq,
		wsURL, wsDialer, wsHeader,
		config,
		resourceCreator,
		recvChan,
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
	const (
		// Time allowed to write a message to the peer.
		writeWait = 10 * time.Second
		// Time allowed to read the next pong message from the peer.
		pongWait = 10 * time.Second
		// Send pings to peer with this period. Must be less than pongWait.
		pingPeriod = (pongWait * 9) / 10
	)

	// write writes a message with the given message type and payload.
	writeFn := func(ws *websocket.Conn, mt int, payload []byte) error {
		ws.SetWriteDeadline(time.Now().Add(writeWait))
		return ws.WriteMessage(mt, payload)
	}

	for {
		ws, resp, err := i.wsDialer.Dial(i.wsURL, i.wsHeader)
		if err != nil {
			if err == websocket.ErrBadHandshake {
				err = fmt.Errorf("handshake failed with status %d", resp.StatusCode)
			}
			i.notifyError(err)
			continue
		}

		// TODO: Look which is the max resource limit in kubernetes (the json serialized one)
		//ws.SetReadLimit(maxResourceSize)
		ws.SetReadDeadline(time.Now().Add(pongWait))
		ws.SetPongHandler(func(string) error {
			ws.SetReadDeadline(time.Now().Add(pongWait)); return nil
		})

		// this routine pumps messages from the hub to the websocket connection.
		go func() {
			ticker := time.NewTicker(pingPeriod)
			defer func() {
				ticker.Stop()
				ws.Close()
			}()
			for {
				select {
				case <-ticker.C:
					if err := writeFn(ws, websocket.PingMessage, []byte{}); err != nil {
						return
					}
				}
			}
		}()

		L: for {
			select {
			case <-i.stopChan:
				break
			default:
				v := i.rc.item()
				we := &kapi.WatchEvent{Object:v}
				if err := ws.ReadJSON(&we); err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
						i.notifyError(err)
					}
					break L
				} else {
					// notify watch event
					i.notify(we)
				}
			}
		}
	}
}

func (i *Informer) list() {
	httpURL := i.httpReq.URL.String()
	for {
		res, err := ctxhttp.Do(context.Background(), i.httpClient, i.httpReq)
		if err != nil {
			i.errChan <- fmt.Errorf("failed to make request: GET %s: %v", httpURL, err)
			continue
		}

		body, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			i.notifyError(fmt.Errorf("failed to read request body for GET %s: %v", httpURL, err))
			continue
		}

		if res.StatusCode != http.StatusOK {
			i.notifyError(fmt.Errorf("http error %d GET %q: %s: %v", res.StatusCode, httpURL, string(body), err))
			continue
		}

		v := i.rc.list()
		if err := json.Unmarshal(body, &v); err != nil {
			i.notifyError(fmt.Errorf("failed to decode list of pod resources: %v", err))
			continue
		}

		// notify list
		i.notify(v)

		// wait until resync is required
		select {
		case <-i.stopChan:
			break
		case <-time.After(i.config.ResyncInterval):
			continue
		}
	}
}

func (i *Informer) notify(v interface{}) {
	// send but do not block for it
	select {
	case i.recvChan <- v:
	default:
		log.Warnf("unable to notify item, discarding it (%v)", v)
	}
}

func (i *Informer) notifyError(err error) {
	// send but do not block for it
	select {
	case i.errChan <- err:
	default:
		log.Warnf("unable to notify error, discarding it (%v)", err)
	}

	// Prevent errors from consuming all resources.
	time.Sleep(1 * time.Second)
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

	// wait until both finished
	wg.Wait()
}