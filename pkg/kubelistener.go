package pkg

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"
	"time"

	kclient "github.com/glerchundi/kubelistener/pkg/client"
	kapi "github.com/glerchundi/kubelistener/pkg/client/api/v1"
	"github.com/glerchundi/kubelistener/pkg/util"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
)

type Config struct {
	KubeMasterURL string
	Namespace string
	Resource string
	Selector string
	ResyncInterval time.Duration
	AddEventsFile string
	UpdateEventsFile string
	DeleteEventsFile string
}

func NewConfig() *Config {
	return &Config{
		KubeMasterURL: "",
		Namespace: "",
		Resource: "services",
		Selector: "",
		ResyncInterval: 30 * time.Minute,
		AddEventsFile: "/dev/stdout",
		UpdateEventsFile: "/dev/stdout",
		DeleteEventsFile: "/dev/stdout",
	}
}

type KubeListener struct {
	// Configuration
	config *Config
	// Writer for 'add' events
	addWriter io.Writer
	// Writer for 'update' events
	updateWriter io.Writer
	// Writer for 'delete' events
	deleteWriter io.Writer
}

func NewKubeListener(config *Config) *KubeListener {
	return &KubeListener{config:config}
}

func process(v interface{}) {
	switch vv := v.(type) {
		case *kapi.Service:
		fmt.Printf("*SERVICE:     %v\n\n", vv)
		case *kapi.ServiceList:
		fmt.Printf("*SERVICELIST: %v\n\n", vv)
		case *kapi.WatchEvent:
		fmt.Printf("*WATCHEVENT:  %v\n", vv)
		fmt.Printf("*WATCHEVENT (VALUE): %v\n", vv.Object)
		default:
		fmt.Printf("DEFAULT:      %v\n\n", vv)
	}
}

func (kl *KubeListener) Run() {
	// Initialize logs
	util.InitLogs()
	defer util.FlushLogs()

	// Configure logging.
	logLevel := pflag.Lookup("log-level")
	flag.Set("v", logLevel.Value.String())

	// Get service account token
	serviceAccountToken, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		glog.Fatal(err)
	}

	// Get CA certificate data
	caCertificate, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		glog.Fatal(err)
	}

	// Create new k8s client
	kubeConfig := &kclient.ClientConfig{
		MasterURL: kl.config.KubeMasterURL,
		Auth: &kclient.TokenAuth{ string(serviceAccountToken) },
		CaCertificate: caCertificate,
	}
	kubeClient, err := kclient.NewClient(kubeConfig)
	if err != nil {
		glog.Fatal(err)
	}

	// Flow control channels
	stopChan := make(<-chan struct {} )
	doneChan := make(chan bool)
	errChan := make(chan error, 10)

	// Create informer from client
	informerConfig := &kclient.InformerConfig{
		Namespace: kl.config.Namespace,
		Resource: kl.config.Resource,
		Selector: kl.config.Selector,
		ResyncInterval: kl.config.ResyncInterval,
		Processor: process,
	}
	i, err := kubeClient.NewInformer(
		informerConfig,
		stopChan, doneChan, errChan,
	)

	go i.Run()

	// Wait for signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case err := <-errChan:
			glog.Error(err)
		case s := <-signalChan:
			glog.Infof("Captured %v. Exiting...", s)
			close(doneChan)
		case <-doneChan:
			os.Exit(0)
		}
	}

	/*
	// create kubelistener instance
	kl := kubelistener{config: config}
	
	// open add events files
	if config.AddEventsFile == "" {
		glog.Warningf("Ignoring 'add' events because --add-events-file wasn't provided.")
	} else {
		if w, err := newWriter(config.AddEventsFile); err != nil {
			glog.Fatalf("Unable to open '%s' for writing due to: %s", config.AddEventsFile, err.Error())
		} else {
			kl.addWriter = w
		}
	}
	
	// open update events files
	if config.UpdateEventsFile == "" {
		glog.Warningf("Ignoring 'update' events because --update-events-file wasn't provided.")
	} else {
		if w, err := newWriter(config.UpdateEventsFile); err != nil {
			glog.Fatalf("Unable to open '%s' for writing due to: %s", config.UpdateEventsFile, err.Error())
		} else {
			kl.updateWriter = w
		}
	}
	
	// open delete events files
	if config.DeleteEventsFile == "" {
		glog.Warningf("Ignoring 'delete' events because --delete-events-file wasn't provided.")
	} else {
		if w, err := newWriter(config.DeleteEventsFile); err != nil {
			glog.Fatalf("Unable to open '%s' for writing due to: %s", config.DeleteEventsFile, err.Error())
		} else {
			kl.deleteWriter = w
		}
	}

	// choose which resources to watch
	if config.Resource == "" {
		glog.Fatalf("Unable to start kubelistener because --resources to watch wasn't provided.")
	}

	var resource = strings.ToLower(config.Resource)
	if resource == "all" {
		for _, watchFunc := range resources {
			watchFunc(&kl)
		}
	} else if watchFunc, ok := resources[resource]; ok {
		watchFunc(&kl)
	} else {
		glog.Fatal("Unknown resource to watch '%s':", resource)
	}
	*/
}
