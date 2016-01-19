package main

import (
	"os"
	"strings"

	flag "github.com/spf13/pflag"
	"github.com/glerchundi/kubelistener/pkg"
)

const (
	cliName        = "kubelistener"
	cliDescription = "kubelistener listens to Kubernetes events and outputs them to specified locations."
)

func main() {
	// configuration
	cfg := pkg.NewConfig()

	// flags
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&cfg.KubeMasterURL, "kube-master-url", cfg.KubeMasterURL, "URL to reach kubernetes master.")
	fs.StringVar(&cfg.Namespace, "namespace", cfg.Namespace, "If present, the namespace scope.")
	fs.StringVar(&cfg.Resource, "resource", cfg.Resource, "Which resource to watch.")
	fs.StringVar(&cfg.Selector, "selector", cfg.Selector, "Filter resources by a user-provided selector.")
	fs.DurationVar(&cfg.ResyncInterval, "resync-interval", cfg.ResyncInterval, "Resync with kubernetes master every user-defined interval.")
	fs.StringVar(&cfg.AddEventsFile, "add-events-file", cfg.AddEventsFile, "File in which the events of type 'add' are printed.")
	fs.StringVar(&cfg.UpdateEventsFile, "update-events-file", cfg.UpdateEventsFile, "File in which the events of type 'update' are printed.")
	fs.StringVar(&cfg.DeleteEventsFile, "delete-events-file", cfg.DeleteEventsFile, "File in which the events of type 'delete' are printed.")
	fs.SetNormalizeFunc(
		func(f *flag.FlagSet, name string) flag.NormalizedName {
			if strings.Contains(name, "_") {
				return flag.NormalizedName(strings.Replace(name, "_", "-", -1))
			}
			return flag.NormalizedName(name)
		},
	)

	// parse
	fs.Parse(os.Args[1:])

	// set from env (if present)
	fs.VisitAll(func(f *flag.Flag) {
		if !f.Changed {
			key := strings.ToUpper(strings.Join(
				[]string{
					cliName,
					strings.Replace(f.Name, "-", "_", -1),
				},
				"_",
			))
			val := os.Getenv(key)
			if val != "" {
				fs.Set(f.Name, val)
			}
		}
	})

	// and then, run!
	kl := pkg.NewKubeListener(cfg)
	kl.Run()
}