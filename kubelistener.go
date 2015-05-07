/*
Part of the code licensed with Apache License, Version 2.0 to Google Inc.
*/

package main

import (
    "encoding/json"
    "fmt"
    "strconv"
    "os"
    "time"

    "github.com/glerchundi/kubelistener/log"
    "github.com/glerchundi/kubelistener/resource"

    "github.com/codegangsta/cli"
    kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
    kclient "github.com/GoogleCloudPlatform/kubernetes/pkg/client"
    kclientcmd "github.com/GoogleCloudPlatform/kubernetes/pkg/client/clientcmd"
    kclientcmdapi "github.com/GoogleCloudPlatform/kubernetes/pkg/client/clientcmd/api"
    klabels "github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
    kwatch "github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
)

type Event struct {
    Type   int         `json:"type"`
    Object interface{} `json:"object"`
}

func NewEvent(eventType kwatch.EventType, object interface{}) Event {
    var t int = -1
    switch eventType {
    case kwatch.Added:
        t = 0
    case kwatch.Modified:
        t = 1
    case kwatch.Deleted:
        t = 2
    }
    return Event{t,object}
}

type RunConfig struct {
    ConfigFile   string
    Resource     string
    Selector     string
    SyncInterval int
    LogLevel     string
}

func Run(cfg RunConfig) {
    // Configure logging.
    if cfg.LogLevel != "" {
        log.SetLevel(cfg.LogLevel)
    }

    // Dump cfg
    log.Debug(fmt.Sprintf("%#v", cfg))

    // Create Kubernetes client
    kubeClient, err := newKubeClient(cfg.ConfigFile)
    if err != nil {
        log.Fatal(err.Error())
    }

    // Create Kubernetes Label Selector
    selector := klabels.Everything()
    if cfg.Selector != "" {
        if s, err := klabels.Parse(cfg.Selector); err != nil {
            log.Warning(fmt.Sprintf("Unable to parse selector %s due to: %s", cfg.Selector, err.Error()))
        } else {
            selector = s
        }
    }

    // Get custom resource interface
    var resources resource.Resources = nil
    switch cfg.Resource {
    case "pods":
        resources = resource.NewPodResources(kubeClient.Pods(kapi.NamespaceAll))
    case "replicationcontrollers", "rc":
        resources = resource.NewReplicationControllerResources(kubeClient.ReplicationControllers(kapi.NamespaceAll))
    case "services":
        resources = resource.NewServiceResources(kubeClient.Services(kapi.NamespaceAll))
    default:
        log.Fatal(fmt.Sprintf("Unsupported resource type: %s", cfg.Resource))
    }

    // In case of error (non-fatal one), the watch will be aborted. At that point we just retry.
    for {
        // blocking loop
        mainLoop(resources, selector, cfg.SyncInterval)

        // if code flow fell here, means an "error" ocurred but it wasn't fatal
        time.Sleep(500 * time.Millisecond)
    }
}

func newKubeClient(kubeConfig string) (*kclient.Client, error) {
    var config *kclient.Config = nil
    if kubeConfig == "" {
        // No kubecfg file provided. Use kubernetes_ro service.
        masterUrl, err := getMasterUrl("KUBERNETES_RO_SERVICE_HOST", "KUBERNETES_RO_SERVICE_PORT")
        if err != nil {
            return nil, err
        }
        config = &kclient.Config{Host: masterUrl, Version: "v1beta1"}
    } else {
        masterUrl, err := getMasterUrl("KUBERNETES_SERVICE_HOST", "KUBERNETES_SERVICE_PORT")
        if err != nil {
            return nil, err
        }
        config, err = kclientcmd.NewNonInteractiveDeferredLoadingClientConfig(
            &kclientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfig},
            &kclientcmd.ConfigOverrides{ClusterInfo: kclientcmdapi.Cluster{Server: masterUrl}}).ClientConfig()
        if err != nil {
            return nil, err
        }
    }

    log.Info(fmt.Sprintf("Using %s for kubernetes master", config.Host))
    log.Info(fmt.Sprintf("Using kubernetes API %s", config.Version))

    return kclient.New(config)
}

func mainLoop(resources resource.Resources, selector klabels.Selector, syncInterval int) {
    // Func for getting whole list of resources
    getResourcesList := func(eventType kwatch.EventType) {
        rl, err := resources.List(selector)
        if err != nil {
            log.Info(fmt.Sprintf("Failed to list resources: %v", err))
            return
        }

        // marshal list of events
        for _, i := range rl {
            marshalAndPrint(NewEvent(eventType, i))
        }
    }

    // Create watcher channel
    watcher, err := resources.Watch(selector)
    if err != nil {
        log.Info(fmt.Sprintf("Failed to watch for resource changes: %v", err))
        return
    }
    defer watcher.Stop()

    // Create the onetime channel
    onetimeCh := make(chan bool, 1)
    onetimeCh <- true
    kubeCh := watcher.ResultChan()
    ticker := time.NewTicker(time.Duration(syncInterval) * time.Second)
    for {
        select {
        case <-onetimeCh:
            getResourcesList(kwatch.Added)
        case <-ticker.C:
            getResourcesList(kwatch.Modified)
        case event, ok := <-kubeCh:
            if !ok {
                log.Info("watchLoop channel closed")
                return
            }

            // check for error
            if event.Type == kwatch.Error {
                if status, ok := event.Object.(*kapi.Status); ok {
                    log.Error(fmt.Sprintf("Error during watch: %#v", status))
                }
                log.Fatal(fmt.Sprintf("Received unexpected error: %#v", event.Object))
            }

            // marshal and print to stdout
            marshalAndPrint(NewEvent(event.Type, event.Object))
        }
    }
}

func marshalAndPrint(event interface{}) {
    if data, err := json.Marshal(event); err != nil {
        log.Error(err.Error())
    } else {
        fmt.Fprintln(os.Stdout, string(data))
    }
}

func getMasterUrl(envHost, envPort string) (string, error) {
    masterHost := os.Getenv(envHost)
    if masterHost == "" {
        return "", fmt.Errorf("%s is not defined", envHost)
    }
    masterPort := os.Getenv(envPort)
    if masterPort == "" {
        return "", fmt.Errorf("%s is not defined", envPort)
    }
    if masterHost == "" {
        return "", fmt.Errorf("%s defined but empty", envHost)
    }
    masterPortValue, err := strconv.Atoi(masterPort)
    if err != nil {
        return "", fmt.Errorf("Unable to parse %s to int", masterPort)
    }
    if masterPortValue < 1 || masterPortValue > 65535 {
        return "", fmt.Errorf("%s defined but doesn't fall in a valid range [1,65535]", envPort)
    }
    return fmt.Sprintf("http://%s:%d", masterHost, masterPortValue), nil
}

func main() {
    app := cli.NewApp()
    app.Name = "kubelistener"
    app.Version = "0.1.0"
    app.Usage = "listen to kubernetes resource events and outputs them to stdout (logging data to stderr)"
    app.Flags = []cli.Flag{
        cli.StringFlag{
            Name: "resource",
            Value: "services",
        },
        cli.StringFlag{
            Name: "selector",
        },
        cli.IntFlag{
            Name: "sync-interval",
            Value: 60,
        },
        cli.StringFlag{
            Name: "log-level",
        },
    }
    app.Action = func(c *cli.Context) {
        Run(RunConfig{
            ConfigFile: c.GlobalString("kubeconfig"),
            Resource: c.GlobalString("resource"),
            Selector: c.GlobalString("selector"),
            SyncInterval: c.GlobalInt("sync-interval"),
            LogLevel: c.GlobalString("log-level"),
        })
    }
    app.Run(os.Args)
}


