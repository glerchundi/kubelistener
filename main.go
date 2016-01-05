package main

import (
	"strings"
	"os"

	kubelistener "github.com/glerchundi/kubelistener/pkg"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

const (
	cliName        = "kubelistener"
	cliDescription = "kubelistener listens to Kubernetes events and outputs them to specified locations."
)

var (
	cfg = kubelistener.NewConfig()
)

func AddConfigFlags(fs *flag.FlagSet, c *kubelistener.Config) {
	fs.StringVar(&c.KubeMasterURL, "kube-master-url", c.KubeMasterURL, "URL to reach kubernetes master. Env variables in this flag will be expanded.")
	fs.StringVar(&c.Resource, "resource", c.Resource, "Which resource to watch.")
	fs.StringVar(&c.Selector, "selector", c.Selector, "Filter resources by a user-provided selector.")
	fs.DurationVar(&c.ResyncInterval, "resync-interval", c.ResyncInterval, "Resync with kubernetes master every user-defined interval.")
	fs.StringVar(&c.AddEventsFile, "add-events-file", c.AddEventsFile, "File in which the events of type 'add' are printed.")
	fs.StringVar(&c.UpdateEventsFile, "update-events-file", c.UpdateEventsFile, "File in which the events of type 'update' are printed.")
	fs.StringVar(&c.DeleteEventsFile, "delete-events-file", c.DeleteEventsFile, "File in which the events of type 'delete' are printed.")
}

func main() {
	// commands
	rootCmd := &cobra.Command{
		Use:   cliName,
		Short: cliDescription,
		Run:   run,
	}

	rootCmd.SetGlobalNormalizationFunc(
		func(f *flag.FlagSet, name string) flag.NormalizedName {
			if strings.Contains(name, "_") {
				return flag.NormalizedName(strings.Replace(name, "_", "-", -1))
			}
			return flag.NormalizedName(name)
		},
	)

	// flags
	AddConfigFlags(rootCmd.PersistentFlags(), cfg)

	// execute!
	rootCmd.Execute()
}

func run(cmd *cobra.Command, args []string) {
	// Set flags form env's (if not set explicitly)
	setFromEnvs := func(prefix string, flagSet *flag.FlagSet) {
		flagSet.VisitAll(func(f *flag.Flag) {
			if !f.Changed {
				key := strings.ToUpper(strings.Join(
					[]string{
						prefix,
						strings.Replace(f.Name, "-", "_", -1),
					},
					"_",
				))
				val := os.Getenv(key)
				if val != "" {
					flagSet.Set(f.Name, val)
				}
			}
		})
	}

	setFromEnvs(cliName, cmd.PersistentFlags())

	// and then, run!
	kubelistener.Run(cfg)
}
