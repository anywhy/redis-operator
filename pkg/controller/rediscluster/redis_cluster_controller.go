package rediscluster

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	//corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	eventv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/client/clientset/versioned"
	informers "github.com/anywhy/redis-operator/pkg/client/informers/externalversions"
	listers "github.com/anywhy/redis-operator/pkg/client/listers/redis/v1alpha1"
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = v1alpha1.SchemeGroupVersion.WithKind("RedisCluster")

// Controller controls redisclusters.
type Controller struct {
	// kubernetes client interface
	kubeClient kubernetes.Interface
	// operator client interface
	cli versioned.Interface
	// control returns an interface capable of syncing a redis cluster.
	// Abstracted out for testing.
	control ControlInterface

	// rcLister is able to list/get redisclusters from a shared informer's store
	rcLister listers.RedisLister
	// rcListerSynced returns true if the redisclusters shared informer has synced at least once
	rcListerSynced cache.InformerSynced

	// setLister is able to list/get stateful sets from a shared informer's store
	setLister appslisters.StatefulSetLister
	// setListerSynced returns true if the statefulset shared informer has synced at least once
	setListerSynced cache.InformerSynced

	// redisclusters that need to be synced.
	queue workqueue.RateLimitingInterface
}

// NewController creates a rediscluster controller.
func NewController(
	kubeCli kubernetes.Interface,
	cli versioned.Interface,
	informerFactory informers.SharedInformerFactory,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
) *Controller {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&eventv1.EventSinkImpl{
		Interface: eventv1.New(kubeCli.CoreV1().RESTClient()).Events("")})
	// recorder := eventBroadcaster.NewRecorder(v1alpha1.Scheme, corev1.EventSource{Component: "rediscluster"})

	// rcInfomer := informerFactory.Redis().V1alpha1().Redises()
	// setInformer := kubeInformerFactory.Apps().V1beta1().StatefulSets()
	// svcInformer := kubeInformerFactory.Core().V1().Services()
	// pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	// pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes()
	// podInformer := kubeInformerFactory.Core().V1().Pods()
	// nodeInformer := kubeInformerFactory.Core().V1().Nodes()

	rcc := &Controller{
		kubeClient: kubeCli,
		cli:        cli,
		queue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"rediscluster",
		),
	}

	return rcc
}

// Run runs the tidbcluster controller.
func (rcc *Controller) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer rcc.queue.ShutDown()

	glog.Info("Starting rediscluster controller")
	defer glog.Info("Shutting down rediscluster controller")

	if !cache.WaitForCacheSync(stopCh, rcc.rcListerSynced, rcc.setListerSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(rcc.worker, time.Second, stopCh)
	}

	<-stopCh
}

// worker runs a worker goroutine that invokes processNextWorkItem until the the controller's queue is closed
func (rcc *Controller) worker() {
	for rcc.processNextWorkItem() {
		// revive:disable:empty-block
	}
}

// processNextWorkItem dequeues items, processes them, and marks them done. It enforces that the syncHandler is never
// invoked concurrently with the same key.
func (rcc *Controller) processNextWorkItem() bool {
	key, quit := rcc.queue.Get()
	if quit {
		return false
	}
	defer rcc.queue.Done(key)
	if err := rcc.sync(key.(string)); err != nil {
		utilruntime.HandleError(fmt.Errorf("RedisCluster: %v, sync failed %v, requeuing", key.(string), err))
		rcc.queue.AddRateLimited(key)
	} else {
		rcc.queue.Forget(key)
	}
	return true
}

// sync syncs the given rediscluster.
func (rcc *Controller) sync(key string) error {
	startTime := time.Now()
	defer func() {
		glog.V(4).Infof("Finished syncing RedisCluster %q (%v)", key, time.Since(startTime))
	}()

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	redi, err := rcc.rcLister.Redises(ns).Get(name)
	if errors.IsNotFound(err) {
		glog.Infof("RedisCluster has been deleted %v", key)
		return nil
	}
	if err != nil {
		return err
	}

	return rcc.syncRedisCluster(redi.DeepCopy())
}

func (rcc *Controller) syncRedisCluster(redi *v1alpha1.Redis) error {
	return nil
}

// enqueueRedisCluster enqueues the given rediscluster in the work queue.
func (rcc *Controller) enqueueRedisCluster(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Cound't get key for object %+v: %v", obj, err))
		return
	}
	rcc.queue.Add(key)
}
