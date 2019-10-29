package rediscluster

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"github.com/anywhy/redis-operator/pkg/controller"
	mm "github.com/anywhy/redis-operator/pkg/manager/member"
	"github.com/anywhy/redis-operator/pkg/manager/meta"
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = v1alpha1.SchemeGroupVersion.WithKind("Redis")

// Controller controls Rediss.
type Controller struct {
	// kubernetes client interface
	kubeClient kubernetes.Interface
	// operator client interface
	cli versioned.Interface
	// control returns an interface capable of syncing a redis cluster.
	// Abstracted out for testing.
	control ControlInterface

	// rcLister is able to list/get Rediss from a shared informer's store
	rcLister listers.RedisClusterLister
	// rcListerSynced returns true if the Rediss shared informer has synced at least once
	rcListerSynced cache.InformerSynced

	// setLister is able to list/get stateful sets from a shared informer's store
	setLister appslisters.StatefulSetLister
	// setListerSynced returns true if the statefulset shared informer has synced at least once
	setListerSynced cache.InformerSynced

	// Rediss that need to be synced.
	queue workqueue.RateLimitingInterface
}

// NewController creates a Redis controller.
func NewController(
	kubeCli kubernetes.Interface,
	cli versioned.Interface,
	informerFactory informers.SharedInformerFactory,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	autoFailover bool,
) *Controller {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&eventv1.EventSinkImpl{
		Interface: eventv1.New(kubeCli.CoreV1().RESTClient()).Events("")})
	recorder := eventBroadcaster.NewRecorder(v1alpha1.Scheme, corev1.EventSource{Component: "redises"})

	rcInformer := informerFactory.Anywhy().V1alpha1().RedisClusters()
	setInformer := kubeInformerFactory.Apps().V1beta1().StatefulSets()
	svcInformer := kubeInformerFactory.Core().V1().Services()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes()
	podInformer := kubeInformerFactory.Core().V1().Pods()
	// nodeInformer := kubeInformerFactory.Core().V1().Nodes()

	rcControl := controller.NewRealRedisControl(cli, rcInformer.Lister(), recorder)
	setControl := controller.NewRealStatefuSetControl(kubeCli, setInformer.Lister(), recorder)
	svcControl := controller.NewRealServiceControl(kubeCli, svcInformer.Lister(), recorder)
	podControl := controller.NewRealPodControl(kubeCli, podInformer.Lister(), recorder)
	pvcControl := controller.NewRealPVCControl(kubeCli, recorder, pvcInformer.Lister())
	pvControl := controller.NewRealPVControl(kubeCli, pvcInformer.Lister(), pvInformer.Lister(), recorder)

	replicaScaler := mm.NewReplicaScaler(pvcInformer.Lister(), pvcControl)
	replicaUpgrader := mm.NewReplicaUpgrader(podControl, podInformer.Lister())
	replicaFailover := mm.NewReplicaFailover(controller.NewDefaultHAControl(), podInformer.Lister(), podControl)

	rcc := &Controller{
		kubeClient: kubeCli,
		cli:        cli,
		control: NewDefaultRedisControl(
			rcControl,
			mm.NewReplicaMemberManager(
				setControl,
				svcControl,
				svcInformer.Lister(),
				podInformer.Lister(),
				podControl,
				setInformer.Lister(),
				replicaScaler,
				replicaUpgrader,
				autoFailover,
				replicaFailover),
			nil,
			meta.NewReclaimPolicyManager(
				pvcInformer.Lister(),
				pvInformer.Lister(),
				pvControl,
			),
			meta.NewMetaManager(
				pvcInformer.Lister(),
				pvcControl,
				pvInformer.Lister(),
				pvControl,
				podInformer.Lister(),
				podControl,
			),
			recorder,
		),
		queue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"redises",
		),
	}

	rcInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: rcc.enqueueRedis,
		UpdateFunc: func(old, cur interface{}) {
			rcc.enqueueRedis(cur)
		},
		DeleteFunc: rcc.enqueueRedis,
	})
	rcc.rcLister = rcInformer.Lister()
	rcc.rcListerSynced = rcInformer.Informer().HasSynced

	setInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: rcc.addStatefulSet,
		UpdateFunc: func(old, cur interface{}) {
			rcc.updateStatefuSet(old, cur)
		},
		DeleteFunc: rcc.deleteStatefulSet,
	})
	rcc.setLister = setInformer.Lister()
	rcc.setListerSynced = setInformer.Informer().HasSynced

	return rcc
}

// Run runs the Redis controller.
func (rcc *Controller) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer rcc.queue.ShutDown()

	glog.Info("Starting Redis controller")
	defer glog.Info("Shutting down Redis controller")

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
		utilruntime.HandleError(fmt.Errorf("Redis: %v, sync failed %v, requeuing", key.(string), err))
		rcc.queue.AddRateLimited(key)
	} else {
		rcc.queue.Forget(key)
	}
	return true
}

// sync syncs the given Redis.
func (rcc *Controller) sync(key string) error {
	startTime := time.Now()
	defer func() {
		glog.V(4).Infof("Finished syncing Redis %q (%v)", key, time.Since(startTime))
	}()

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	rc, err := rcc.rcLister.RedisClusters(ns).Get(name)
	if errors.IsNotFound(err) {
		glog.Infof("Redis has been deleted %v", key)
		return nil
	}
	if err != nil {
		return err
	}

	return rcc.syncRedis(rc.DeepCopy())
}

func (rcc *Controller) syncRedis(rc *v1alpha1.RedisCluster) error {
	return rcc.control.UpdateRedis(rc)
}

// enqueueRedis enqueues the given Redis in the work queue.
func (rcc *Controller) enqueueRedis(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Cound't get key for object %+v: %v", obj, err))
		return
	}
	rcc.queue.Add(key)
}

// addStatefulSet adds the redisbcluster for the statefulset to the sync queue
func (rcc *Controller) addStatefulSet(obj interface{}) {
	set := obj.(*apps.StatefulSet)
	ns, setName := set.GetNamespace(), set.GetName()

	if set.DeletionTimestamp != nil {
		// on a restart of the controller manager, it's possible a new statefulset shows up in a state that
		// is already pending deletion. Prevent the statefulset from being a creation observation.
		rcc.deleteStatefulSet(set)
		return
	}

	// If it has a ControllerRef, that's all that matters.
	rc := rcc.resolveRedisFromSet(ns, set)
	if rc == nil {
		return
	}
	glog.Infof("StatefuSet %s/%s created, Redis: %s/%s", ns, setName, ns, rc.Name)
	rcc.enqueueRedis(rc)
}

// updateStatefuSet adds the Redis for the current and old statefulsets to the sync queue.
func (rcc *Controller) updateStatefuSet(old, cur interface{}) {
	curSet := cur.(*apps.StatefulSet)
	oldSet := old.(*apps.StatefulSet)
	ns, setName := curSet.GetNamespace(), curSet.GetName()
	if curSet.ResourceVersion == oldSet.ResourceVersion {
		// Periodic resync will send update events for all known statefulsets.
		// Two different versions of the same statefulset will always have different RVs.
		return
	}

	// If it has a ControllerRef, that's all that matters.
	rc := rcc.resolveRedisFromSet(ns, curSet)
	if rc == nil {
		return
	}
	glog.Infof("StatefulSet %s/%s updated, %+v -> %+v.", ns, setName, oldSet.Spec, curSet.Spec)
	rcc.enqueueRedis(rc)
}

// deleteStatefulSet enqueues the Redis for the statefulset accounting for deletion tombstones.
func (rcc *Controller) deleteStatefulSet(obj interface{}) {
	set, ok := obj.(*apps.StatefulSet)
	ns := set.GetNamespace()
	setName := set.GetName()

	// When a delete is dropped, the relist will notice a statefuset in the store not
	// in the list, leading to the insertion of a tombstone object which contains
	// the deleted key/value.
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %+v", obj))
			return
		}
		set, ok = tombstone.Obj.(*apps.StatefulSet)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a statefuset %+v", obj))
			return
		}
	}

	// If it has a Redis, that's all that matters.
	rc := rcc.resolveRedisFromSet(ns, set)
	if rc == nil {
		return
	}
	glog.Infof("StatefulSet %s/%s deleted through %v.", ns, setName, utilruntime.GetCaller())
	rcc.enqueueRedis(rc)
}

// resolveRedisFromSet returns the Redis by a StatefulSet,
// or nil if the StatefulSet could not be resolved to a matching Redis
// of the correct Kind.
func (rcc *Controller) resolveRedisFromSet(namespace string, set *apps.StatefulSet) *v1alpha1.RedisCluster {
	controllerRef := metav1.GetControllerOf(set)
	if controllerRef == nil {
		return nil
	}

	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != controllerKind.Kind {
		return nil
	}
	rc, err := rcc.rcLister.RedisClusters(namespace).Get(controllerRef.Name)
	if err != nil {
		return nil
	}
	if rc.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return rc
}
