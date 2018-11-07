package options

import (
	"time"

	"github.com/spf13/pflag"

	"github.com/anywhy/redis-operator/pkg/controller"
)

// RedisControllerManagerOptions redis controller options
type RedisControllerManagerOptions struct {
	Master     string
	Kubeconfig string

	Namespace string

	// PrintVersion bool
	Workers int

	// ConfigFile is the location of the scheduler server's configuration file.
	ConfigFile string

	LeaseDuration  time.Duration
	RenewDuration  time.Duration
	RetryPeriod    time.Duration
	ResyncDuration time.Duration
	WaitDuration   time.Duration
}

// NewRedisControllerManagerOptions new default redis options
func NewRedisControllerManagerOptions() *RedisControllerManagerOptions {
	return &RedisControllerManagerOptions{
		Workers:        2,
		LeaseDuration:  15 * time.Second,
		RenewDuration:  5 * time.Second,
		RetryPeriod:    3 * time.Second,
		ResyncDuration: 30 * time.Second,
		WaitDuration:   5 * time.Second,
	}
}

// AddFlags add operator options flags
func (s *RedisControllerManagerOptions) AddFlags(pflag *pflag.FlagSet) {
	pflag.StringVar(&s.Master, "master", s.Master, "k8s master addr")
	pflag.StringVar(&s.ConfigFile, "config", s.ConfigFile, "The path to the configuration file. Flags override values in this file.")
	// pflag.BoolVar(&s.PrintVersion, "version", false, "Show version and quit")
	pflag.StringVar(&s.Namespace, "namespace", s.Namespace, "The Operator deployment of k8s namespce")
	pflag.StringVar(&s.Kubeconfig, "kubeconfig", "", "Config k8s config file path")
	pflag.IntVar(&s.Workers, "workers", s.Workers, "start by threads number for worker")
	pflag.StringVar(&controller.DefaultStorageClassName, "default-storage-class-name", "standard", "Default storage class name")
}
