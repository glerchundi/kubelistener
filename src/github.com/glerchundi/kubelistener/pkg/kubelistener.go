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
    "encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"
	"strings"

	"github.com/glerchundi/kubelistener/pkg/util"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	kapi "k8s.io/kubernetes/pkg/api"
	kcache "k8s.io/kubernetes/pkg/client/cache"
	kclient "k8s.io/kubernetes/pkg/client/unversioned"
	kclientcmd "k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	kframework "k8s.io/kubernetes/pkg/controller/framework"
	kfields "k8s.io/kubernetes/pkg/fields"
	kutil "k8s.io/kubernetes/pkg/util"
)

type Config struct {
	KubeCfgFile string
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
		KubeCfgFile: "",
		KubeMasterURL: "",
		Resource: "services",
		Selector: "",
		ResyncInterval: 30 * time.Minute,
		AddEventsFile: "/dev/stdout",
		UpdateEventsFile: "/dev/stdout",
		DeleteEventsFile: "/dev/stdout",
	}
}

var (
	resources = map[string]func(*kclient.Client,kfields.Selector,*kubelistener)kcache.Store {
		"services":  watchServices,
		"endpoints": watchEndpoints,
		"pods":      watchPods,
	}
)

type kubelistener struct {
	// Configuration
	config *Config
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
	if service, ok := obj.(*kapi.Service); ok {
		marshalAndPrint(kl.addWriter, service)
	}
}

func (kl *kubelistener) handleServiceUpdate(oldObj, newObj interface{}) {
	oldService, okOld := oldObj.(*kapi.Service)
	newService, okNew := newObj.(*kapi.Service)
	if okOld && okNew {
		marshalAndPrint(kl.updateWriter, mergeUpdateObj(oldService, newService))
	} else if okOld {
		marshalAndPrint(kl.deleteWriter, oldService)
	} else if okNew {
		marshalAndPrint(kl.addWriter, newService)
	}
}

func (kl *kubelistener) handleServiceDelete(obj interface{}) {
	if service, ok := obj.(*kapi.Service); ok {
		marshalAndPrint(kl.deleteWriter, service)
	}
}

func (kl *kubelistener) handleEndpointAdd(obj interface{}) {
	if endpoint, ok := obj.(*kapi.Endpoints); ok {
		marshalAndPrint(kl.addWriter, endpoint)
	}
}

func (kl *kubelistener) handlePodAdd(obj interface{}) {
	if pod, ok := obj.(*kapi.Pod); ok {
		marshalAndPrint(kl.addWriter, pod)
	}
}

func (kl *kubelistener) handlePodUpdate(oldObj interface{}, newObj interface{}) {
	oldPod, okOld := oldObj.(*kapi.Pod)
	newPod, okNew := newObj.(*kapi.Pod)
	if okOld && okNew {
		if oldPod.Status.PodIP != newPod.Status.PodIP {
			marshalAndPrint(kl.updateWriter, mergeUpdateObj(oldPod, newPod))
		}
	} else if okOld {
		marshalAndPrint(kl.deleteWriter, oldPod)
	} else if okNew {
		marshalAndPrint(kl.addWriter, newPod)
	}
}

func (kl *kubelistener) handlePodDelete(obj interface{}) {
	if pod, ok := obj.(*kapi.Pod); ok {
		marshalAndPrint(kl.deleteWriter, pod)
	}
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

func expandKubeMasterURL(kubeMasterURL string) (string, error) {
	parsedURL, err := url.Parse(os.ExpandEnv(kubeMasterURL))
	if err != nil {
		return "", fmt.Errorf("failed to parse --kube-master-url %s - %v", kubeMasterURL, err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" || parsedURL.Host == ":" {
		return "", fmt.Errorf("invalid --kube-master-url specified %s", kubeMasterURL)
	}
	return parsedURL.String(), nil
}

// TODO: evaluate using pkg/client/clientcmd
func newKubeClient(kubeMasterURL, kubeCfgFile string) (*kclient.Client, error) {
	var (
		config    *kclient.Config
		err       error
		masterURL string
	)
	// If the user specified --kube_master_url, expand env vars and verify it.
	if kubeMasterURL != "" {
		masterURL, err = expandKubeMasterURL(kubeMasterURL)
		if err != nil {
			return nil, err
		}
	}

	if masterURL != "" && kubeCfgFile == "" {
		// Only --kube_master_url was provided.
		config = &kclient.Config{
			Host:    masterURL,
			Version: "v1",
		}
	} else {
		// We either have:
		//  1) --kube-master-url and --kubecfg_file
		//  2) just --kubecfg-file
		//  3) neither flag
		// In any case, the logic is the same.  If (3), this will automatically
		// fall back on the service account token.
		overrides := &kclientcmd.ConfigOverrides{}
		// might be "", but that is OK
		overrides.ClusterInfo.Server = masterURL
		rules := &kclientcmd.ClientConfigLoadingRules{ExplicitPath: kubeCfgFile} // might be "", but that is OK
		if config, err = kclientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig(); err != nil {
			return nil, err
		}
	}

	glog.Infof("Using %s for kubernetes master", config.Host)
	glog.Infof("Using kubernetes API '%s'", config.Version)
	return kclient.New(config)
}

func newWriter(filename string) (io.Writer, error) {
	return os.OpenFile(filename, os.O_APPEND | os.O_WRONLY, 0600)
}

type updateObj struct {
	Old interface{} `json:"old"`
	New interface{} `json:"new"`
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
		kl.config.ResyncInterval,
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
		kl.config.ResyncInterval,
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
		kl.config.ResyncInterval,
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

func Run(config *Config) {
	// initialize logs
	util.InitLogs()
	defer util.FlushLogs()

	// configure logging.
	logLevel := pflag.Lookup("log-level")
	flag.Set("v", logLevel.Value.String())

	// create kubernetes api client
	kubeClient, err := newKubeClient(config.KubeMasterURL, config.KubeCfgFile)
	if err != nil {
		glog.Fatalf("Failed to create a kubernetes client: %v", err)
	}

    // if provided, parse selector
    selector := kfields.Everything()
    if config.Selector != "" {
        if s, err := kfields.ParseSelector(config.Selector); err != nil {
            glog.Warningf("Unable to parse selector '%s' due to: %s", config.Selector, err.Error())
        } else {
            selector = s
        }
    }
	
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
			watchFunc(kubeClient, selector, &kl)
		}
	} else if watchFunc, ok := resources[resource]; ok {
		watchFunc(kubeClient, selector, &kl)
	} else {
		glog.Fatal("Unknown resource to watch '%s':", resource)
	}

	select {}
}
