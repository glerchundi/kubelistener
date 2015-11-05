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
package main

import (
    "encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"github.com/golang/glog"
	flag "github.com/spf13/pflag"
	kapi "k8s.io/kubernetes/pkg/api"
	kcache "k8s.io/kubernetes/pkg/client/cache"
	kclient "k8s.io/kubernetes/pkg/client/unversioned"
	kclientcmd "k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	kframework "k8s.io/kubernetes/pkg/controller/framework"
	kfields "k8s.io/kubernetes/pkg/fields"
	kutil "k8s.io/kubernetes/pkg/util"
	//"strings"
	"strings"
)

var (
	argKubecfgFile      = flag.String("kubecfg-file", "", "Location of kubecfg file for access to kubernetes master service; --kube-master-url overrides the URL part of this; if neither this nor --kube-master-url are provided, defaults to service account tokens.")
	argKubeMasterURL    = flag.String("kube-master-url", "", "URL to reach kubernetes master. Env variables in this flag will be expanded.") 	
	argResource         = flag.String("resource", "services", "Which resource to watch.")
	argSelector         = flag.String("selector", "", "Filter resources by a user-provided selector.")
	argResyncInterval   = flag.Duration("resync-interval", 30 * time.Minute, "Resync with kubernetes master every user-defined interval.")
	argAddEventsFile    = flag.String("add-events-file", "/dev/stdout", "File in which the events of type 'add' are printed.")
	argUpdateEventsFile = flag.String("update-events-file", "/dev/stdout", "File in which the events of type 'update' are printed.")
	argDeleteEventsFile = flag.String("delete-events-file", "/dev/stdout", "File in which the events of type 'delete' are printed.")

	resources = map[string]func(*kclient.Client,kfields.Selector,*kubelistener)kcache.Store {
		"services":  watchServices,
		"endpoints": watchEndpoints,
		"pods":      watchPods,
	}
)

type kubelistener struct {
	// Resync period for the kube controller loop.
	resyncPeriod time.Duration
	// A cache that contains all the endpoints in the system.
	endpointsStore kcache.Store
	// A cache that contains all the services in the system.
	servicesStore kcache.Store
	// A cache that contains all the pods in the system.
	podsStore kcache.Store
	// Writer for 'add' events
	addWriter io.Writer
	// Writer for 'update' events
	updateWriter io.Writer
	// Writer for 'delete' events
	deleteWriter io.Writer
}

func (kl *kubelistener) handleServiceAdd(obj interface{}) {
	marshalAndPrint(kl.addWriter, obj)
}

func (kl *kubelistener) handleServiceUpdate(oldObj, newObj interface{}) {
	marshalAndPrint(kl.updateWriter, mergeUpdateObj(oldObj, newObj))
}

func (kl *kubelistener) handleServiceDelete(obj interface{}) {
	marshalAndPrint(kl.deleteWriter, obj)
}

func (kl *kubelistener) handleEndpointAdd(obj interface{}) {
	marshalAndPrint(kl.addWriter, obj)
}

func (kl *kubelistener) handlePodAdd(obj interface{}) {
	marshalAndPrint(kl.addWriter, obj)
}

func (kl *kubelistener) handlePodUpdate(oldObj interface{}, newObj interface{}) {
	marshalAndPrint(kl.updateWriter, mergeUpdateObj(oldObj, newObj))
}

func (kl *kubelistener) handlePodDelete(obj interface{}) {
	marshalAndPrint(kl.deleteWriter, obj)
}

// Returns a cache.ListWatch that gets all changes to services.
func createServiceLW(kubeClient *kclient.Client, selector kfields.Selector) *kcache.ListWatch {
	return kcache.NewListWatchFromClient(kubeClient, "services", kapi.NamespaceAll, selector)
}

// Returns a cache.ListWatch that gets all changes to endpoints.
func createEndpointsLW(kubeClient *kclient.Client, selector kfields.Selector) *kcache.ListWatch {
	return kcache.NewListWatchFromClient(kubeClient, "endpoints", kapi.NamespaceAll, selector)
}

// Returns a cache.ListWatch that gets all changes to pods.
func createEndpointsPodLW(kubeClient *kclient.Client, selector kfields.Selector) *kcache.ListWatch {
	return kcache.NewListWatchFromClient(kubeClient, "pods", kapi.NamespaceAll, selector)
}

func expandKubeMasterURL() (string, error) {
	parsedURL, err := url.Parse(os.ExpandEnv(*argKubeMasterURL))
	if err != nil {
		return "", fmt.Errorf("failed to parse --kube-master-url %s - %v", *argKubeMasterURL, err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" || parsedURL.Host == ":" {
		return "", fmt.Errorf("invalid --kube-master-url specified %s", *argKubeMasterURL)
	}
	return parsedURL.String(), nil
}

// TODO: evaluate using pkg/client/clientcmd
func newKubeClient() (*kclient.Client, error) {
	var (
		config    *kclient.Config
		err       error
		masterURL string
	)
	// If the user specified --kube_master_url, expand env vars and verify it.
	if *argKubeMasterURL != "" {
		masterURL, err = expandKubeMasterURL()
		if err != nil {
			return nil, err
		}
	}

	if masterURL != "" && *argKubecfgFile == "" {
		// Only --kube_master_url was provided.
		config = &kclient.Config{
			Host:    masterURL,
			Version: "v1",
		}
	} else {
		// We either have:
		//  1) --kube_master_url and --kubecfg_file
		//  2) just --kubecfg_file
		//  3) neither flag
		// In any case, the logic is the same.  If (3), this will automatically
		// fall back on the service account token.
		overrides := &kclientcmd.ConfigOverrides{}
		overrides.ClusterInfo.Server = masterURL                                     // might be "", but that is OK
		rules := &kclientcmd.ClientConfigLoadingRules{ExplicitPath: *argKubecfgFile} // might be "", but that is OK
		if config, err = kclientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig(); err != nil {
			return nil, err
		}
	}

	glog.Infof("Using %s for kubernetes master", config.Host)
	glog.Infof("Using kubernetes API %s", config.Version)
	return kclient.New(config)
}

func newWriter(filename string) (io.Writer, error) {
	return os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0600)
}

type updateObj struct {
	old interface{} `json:"old"`
	new interface{} `json:"new"`
}

func mergeUpdateObj(oldObj, newObj interface{}) interface{} {
	return updateObj{oldObj,newObj};
}

func marshalAndPrint(writer io.Writer, obj interface{}) {
    if data, err := json.Marshal(obj); err != nil {
		glog.Fatalf("Failed marshalling an event to json: %v", err)
    } else {
        fmt.Fprintln(writer, string(data))
    }
}

func watchServices(kubeClient *kclient.Client, selector kfields.Selector, kl *kubelistener) kcache.Store {
	glog.Info("Listening to services...")
	serviceStore, serviceController := kframework.NewInformer(
		createServiceLW(kubeClient, selector),
		&kapi.Service{},
		kl.resyncPeriod,
		kframework.ResourceEventHandlerFuncs{
			AddFunc:    kl.handleServiceAdd,
			UpdateFunc: kl.handleServiceUpdate,
			DeleteFunc: kl.handleServiceDelete,
		},
	)
	go serviceController.Run(kutil.NeverStop)
	return serviceStore
}

func watchEndpoints(kubeClient *kclient.Client, selector kfields.Selector, kl *kubelistener) kcache.Store {
	glog.Info("Listening to endpoints...")
	eStore, eController := kframework.NewInformer(
		createEndpointsLW(kubeClient, selector),
		&kapi.Endpoints{},
		kl.resyncPeriod,
		kframework.ResourceEventHandlerFuncs{
			AddFunc: kl.handleEndpointAdd,
			UpdateFunc: func(oldObj, newObj interface{}) {
				// TODO: Avoid unwanted updates.
				kl.handleEndpointAdd(newObj)
			},
		},
	)

	go eController.Run(kutil.NeverStop)
	return eStore
}

func watchPods(kubeClient *kclient.Client, selector kfields.Selector, kl *kubelistener) kcache.Store {
	glog.Info("Listening to pods...")
	eStore, eController := kframework.NewInformer(
		createEndpointsPodLW(kubeClient, selector),
		&kapi.Pod{},
		kl.resyncPeriod,
		kframework.ResourceEventHandlerFuncs{
			AddFunc: kl.handlePodAdd,
			UpdateFunc: func(oldObj, newObj interface{}) {
				kl.handlePodUpdate(oldObj, newObj)
			},
			DeleteFunc: kl.handlePodDelete,
		},
	)

	go eController.Run(kutil.NeverStop)
	return eStore
}

func main() {
	flag.Parse()

	// create kubernetes api client	
	kubeClient, err := newKubeClient()
	if err != nil {
		glog.Fatalf("Failed to create a kubernetes client: %v", err)
	}

    // if provided, parse selector
    selector := kfields.Everything()
    if *argSelector != "" {
        if s, err := kfields.ParseSelector(*argSelector); err != nil {
            glog.Warningf("Unable to parse selector '%s' due to: %s", *argSelector, err.Error())
        } else {
            selector = s
        }
    }
	
	// create kubelistener instance
	kl := kubelistener{resyncPeriod: *argResyncInterval}
	
	// open add events files
	if *argAddEventsFile == "" {
		glog.Warningf("Ignoring 'add' events because --add-events-file wasn't provided.")
	} else {
		if w, err := newWriter(*argAddEventsFile); err != nil {
			glog.Fatalf("Unable to open '%s' for writing due to: %s", *argAddEventsFile, err.Error())
		} else {
			kl.addWriter = w
		}
	}
	
	// open update events files
	if *argUpdateEventsFile == "" {
		glog.Warningf("Ignoring 'update' events because --update-events-file wasn't provided.")
	} else {
		if w, err := newWriter(*argUpdateEventsFile); err != nil {
			glog.Fatalf("Unable to open '%s' for writing due to: %s", *argUpdateEventsFile, err.Error())
		} else {
			kl.updateWriter = w
		}
	}
	
	// open delete events files
	if *argDeleteEventsFile == "" {
		glog.Warningf("Ignoring 'delete' events because --delete-events-file wasn't provided.")
	} else {
		if w, err := newWriter(*argDeleteEventsFile); err != nil {
			glog.Fatalf("Unable to open '%s' for writing due to: %s", *argDeleteEventsFile, err.Error())
		} else {
			kl.deleteWriter = w
		}
	}

	// choose which resources to watch
	if *argResource == "" {
		glog.Fatalf("Unable to start kubelistener because --resources to watch wasn't provided.")
	}

	var resource = strings.ToLower(*argResource)
	if resource == "all" {
		for _, watchFunc := range resources {
			watchFunc(kubeClient, selector, &kl)
		}
	} else if watchFunc, ok := resources[resource]; ok {
		watchFunc(kubeClient, selector, &kl)
	} else {
		glog.Fatal("Unknown resource to watch '%s':", resource)
	}

	select {}
}
