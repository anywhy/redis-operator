package app

import (
	"net/http"
	"os"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	utilflag "k8s.io/kubernetes/pkg/util/flag"

	"github.com/anywhy/redis-operator/cmd/controller-manager/app/options"
	"github.com/anywhy/redis-operator/pkg/client/clientset/versioned"
	informers "github.com/anywhy/redis-operator/pkg/client/informers/externalversions"
	"github.com/anywhy/redis-operator/pkg/controller/redis"
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
			return Run(opts, wait.NeverStop)
		},
	}

	opts.AddFlags(cmd.Flags())
	cmd.AddCommand(addVersionCmd())
	cmd.MarkFlagFilename("config", "yaml", "yml", "json")
	return cmd
}

//Run starts the redis-operator controllers. This should never exit.
func Run(s *options.RedisControllerManagerOptions, stop <-chan struct{}) error {
	version.LogVersionInfo()

	ns := os.Getenv("NAMESPACE")
	if s.Namespace != "" {
		ns = s.Namespace
	}
	if ns == "" {
		glog.Fatal("NAMESPACE environment variable not set")
	}

	hostName, err := os.Hostname()
	if err != nil {
		glog.Fatalf("failed to get hostname: %v", err)
	}
	cfg, err := clusterConfig(s)
	if err != nil {
		glog.Fatalf("failed to get config: %v", err)
	}
	kubeCli, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("failed to get kubernetes Clientset: %v", err)
	}
	cli, err := versioned.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("failed to create Clientset: %v", err)
	}

	informerFactory := informers.NewSharedInformerFactory(cli, s.ResyncDuration)
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeCli, s.ResyncDuration)
	rl := resourcelock.EndpointsLock{
		EndpointsMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "redis-controller-manager",
		},
		Client: kubeCli.CoreV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      hostName,
			EventRecorder: &record.FakeRecorder{},
		},
	}

	// redis-controller
	rcController := redis.NewController(kubeCli, cli, informerFactory, kubeInformerFactory)
	go informerFactory.Start(stop)
	go kubeInformerFactory.Start(stop)
	onStarted := func(stopCh <-chan struct{}) {
		rcController.Run(s.Workers, stopCh)
	}
	onStopped := func() {
		glog.Fatalf("leader election lost")
	}

	// leader election for multiple redis-manager
	go wait.Forever(func() {
		leaderelection.RunOrDie(leaderelection.LeaderElectionConfig{
			Lock:          &rl,
			LeaseDuration: s.LeaseDuration,
			RenewDeadline: s.RenewDuration,
			RetryPeriod:   s.RetryPeriod,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: onStarted,
				OnStoppedLeading: onStopped,
			},
		})
	}, s.WaitDuration)

	glog.Fatal(http.ListenAndServe(":9090", nil))
	return nil
}

// clusterConfig get k8s cluster config
func clusterConfig(s *options.RedisControllerManagerOptions) (*rest.Config, error) {
	if len(s.Master) > 0 || s.Kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags(s.Master, s.Kubeconfig)
	}
	return rest.InClusterConfig()
}

// versionCmd version config
func addVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "version",
		Long: "Show Redis-Operator version",
		Run: func(cmd *cobra.Command, args []string) {
			version.PrintVersionInfo()
		},
	}

	return cmd
}
