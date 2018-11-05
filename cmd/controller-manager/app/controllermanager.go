package app

import (
	"fmt"
	"github.com/spf13/pflag"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"
	utilflag "k8s.io/kubernetes/pkg/util/flag"
	// kubeinformers "k8s.io/client-go/informers"

	"github.com/anywhy/redis-operator/cmd/controller-manager/app/options"
	"github.com/anywhy/redis-operator/version"
)

// NewRedisControllerManagerCommand creates a *cobra.Command object with default parameters
func NewRedisControllerManagerCommand() *cobra.Command {
	opts := options.NewRedisControllerManagerOptions()
	cmd := &cobra.Command{
		Use:  "redis-operator",
		Long: `redis-operator creates/configures/manages redis clusters atop Kubernetes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			utilflag.PrintFlags(cmd.Flags())
			pflag.Parse()
			return Run(opts, wait.NeverStop)
		},
	}
	opts.AddFlags(cmd.Flags())
	const usageFmt = "Usage:\n  %s\n\nFlags:\n%s"
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine(), cmd.Flags().FlagUsagesWrapped(2))
		return nil
	})
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine(), cmd.Flags().FlagUsagesWrapped(2))
	})
	cmd.MarkFlagFilename("config", "yaml", "yml", "json")

	return cmd
}

//Run starts the redis-operator controllers. This should never exit.
func Run(s *options.RedisControllerManagerOptions, stop <-chan struct{}) error {
	version.LogVersionInfo()
	return nil
}
