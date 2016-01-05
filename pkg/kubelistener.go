/*
Copyright 2014 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// kubelistener listens to kubernetes events. This is useful for bridge them
// into your concrete system.
package kubelistener

import (
	"flag"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"
	"time"

	k8s "github.com/glerchundi/kubelistener/pkg/client"
	"github.com/glerchundi/kubelistener/pkg/util"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
)

type Listener interface {
	Event()
}

type Config struct {
	KubeMasterURL string
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
		Resource: "services",
		Selector: "",
		ResyncInterval: 30 * time.Minute,
		AddEventsFile: "/dev/stdout",
		UpdateEventsFile: "/dev/stdout",
		DeleteEventsFile: "/dev/stdout",
	}
}

type kubelistener struct {
	// Configuration
	config *Config
	// Writer for 'add' events
	addWriter io.Writer
	// Writer for 'update' events
	updateWriter io.Writer
	// Writer for 'delete' events
	deleteWriter io.Writer
}

func Run(config *Config) {
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
	kubeConfig := &k8s.Config{
		MasterURL: config.KubeMasterURL,
		Auth: &k8s.TokenAuth{ string(serviceAccountToken) },
		CaCertificate: caCertificate,
	}
	kubeClient, err := k8s.NewClient(kubeConfig)
	if err != nil {
		glog.Fatal(err)
	}

	// Flow control channels
	stopChan := make(<-chan struct {} )
	doneChan := make(chan bool)
	errChan := make(chan error, 10)

	// Create informer from client
	i, err := kubeClient.NewInformer(
		"default", config.Resource, config.Selector, config.ResyncInterval,
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
