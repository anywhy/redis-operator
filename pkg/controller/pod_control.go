package controller

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/label"
)

// PodControlInterface defines the interface that RedisController uses to create, update, and delete Pods,
type PodControlInterface interface {
	CreatePod(*v1alpha1.RedisCluster, *corev1.Pod) error
	UpdatePod(*v1alpha1.RedisCluster, *corev1.Pod) (*corev1.Pod, error)
	DeletePod(*v1alpha1.RedisCluster, *corev1.Pod) error
}

type realPodControl struct {
	kubeCli   kubernetes.Interface
	podLister corelisters.PodLister
	recorder  record.EventRecorder
}

// NewRealPodControl creates a new PodControlInterface
func NewRealPodControl(
	kubeCli kubernetes.Interface,
	podLister corelisters.PodLister,
	recorder record.EventRecorder,
) PodControlInterface {
	return &realPodControl{
		kubeCli:   kubeCli,
		podLister: podLister,
		recorder:  recorder,
	}
}

func (rpc *realPodControl) CreatePod(rc *v1alpha1.RedisCluster, pod *corev1.Pod) error {
	retPod, err := rpc.kubeCli.CoreV1().Pods(rc.Namespace).Create(pod)
	// sink already exists errors
	if apierrors.IsAlreadyExists(err) {
		if !metav1.IsControlledBy(retPod, rc) {
			err := fmt.Errorf(MessageResourceExists, "Pod", pod.Namespace, pod.Name)
			rpc.recordPodEvent("create", rc, pod, err)
		}
		return nil
	}
	rpc.recordPodEvent("create", rc, pod, err)
	return err
}

func (rpc *realPodControl) UpdatePod(rc *v1alpha1.RedisCluster, pod *corev1.Pod) (*corev1.Pod, error) {
	ns, rcName, labels := rc.GetNamespace(), rc.GetName(), pod.GetLabels()
	if labels == nil {
		return pod, fmt.Errorf("pod %s/%s has empty labels, Redis: %s", ns, pod.Name, rcName)
	}

	_, ok := labels[label.InstanceLabelKey]
	if !ok {
		return pod, fmt.Errorf("pod %s/%s doesn't have %s label, Redis: %s", ns,
			pod.Name, label.InstanceLabelKey, rcName)
	}
	
	var updatePod *corev1.Pod
	// don't wait due to limited number of clients, but backoff after the default number of steps
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var updateErr error
		updatePod, updateErr = rpc.kubeCli.CoreV1().Pods(ns).Update(pod)
		if updateErr == nil {
			glog.Infof("Redis: [%s/%s] updated successfully", ns, rcName)
			return nil
		}
		glog.Errorf("failed to update Redis: [%s/%s], error: %v", ns, rcName, updateErr)

		if updated, err := rpc.podLister.Pods(ns).Get(rcName); err == nil {
			// make a copy so we don't mutate the shared cache
			pod = updated.DeepCopy()
			pod.Labels = labels
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated Redis %s/%s from lister: %v", ns, rcName, err))
		}

		return updateErr
	})
	return updatePod, err
}

func (rpc *realPodControl) DeletePod(rc *v1alpha1.RedisCluster, pod *corev1.Pod) error {
	ns := rc.GetNamespace()
	podName := pod.GetName()
	rcName := rc.GetName()
	err := rpc.kubeCli.CoreV1().Pods(ns).Delete(podName, nil)
	if err != nil {
		glog.Errorf("failed to delete Pod: [%s/%s], Redis: %s, %v", ns, podName, rcName, err)
	} else {
		glog.V(4).Infof("delete Pod: [%s/%s] successfully, Redis: %s", ns, podName, rcName)
	}
	rpc.recordPodEvent("delete", rc, pod, err)
	return err
}

// recordPodEvent records an event for verb applied to a Pod in a Redis. If err is nil the generated event will
// have a reason of v1.EventTypeNormal. If err is not nil the generated event will have a reason of v1.EventTypeWarning.
func (rpc *realPodControl) recordPodEvent(verb string, rc *v1alpha1.RedisCluster, pod *corev1.Pod, err error) {
	if err == nil {
		reason := fmt.Sprintf("Successful%s", strings.Title(verb))
		message := fmt.Sprintf("%s Pod %s in Redis %s successful",
			strings.ToLower(verb), pod.Name, rc.Name)
		rpc.recorder.Event(rc, corev1.EventTypeNormal, reason, message)
	} else {
		reason := fmt.Sprintf("Failed%s", strings.Title(verb))
		message := fmt.Sprintf("%s Pod %s in Redis %s failed error: %s",
			strings.ToLower(verb), pod.Name, rc.Name, err)
		rpc.recorder.Event(rc, corev1.EventTypeWarning, reason, message)
	}
}

var _ PodControlInterface = &realPodControl{}

var (
	// TestName name label
	TestName = "redis-cluster"
	// TestComponentName component label for instance
	TestComponentName = "redis"
	// TestClusterModeName component label cluster mode
	TestClusterModeName = "replica"
	// TestManagedByName controller by redis
	TestManagedByName = "redis-operator"
	// TestClusterName cluster name
	TestClusterName = "test"
	// TestPodName cluster pod name
	TestPodName = "test-pod"
	// TestNodeRoleName cluster node role
	TestNodeRoleName = "master"
)

// FakePodControl is a fake PodControlInterface
type FakePodControl struct {
	PodIndexer        cache.Indexer
	createPodTracker  requestTracker
	updatePodTracker  requestTracker
	deletePodTracker  requestTracker
	getClusterTracker requestTracker
}

// NewFakePodControl returns a FakePodControl
func NewFakePodControl(podInformer coreinformers.PodInformer) *FakePodControl {
	return &FakePodControl{
		podInformer.Informer().GetIndexer(),
		requestTracker{0, nil, 0},
		requestTracker{0, nil, 0},
		requestTracker{0, nil, 0},
		requestTracker{0, nil, 0},
	}
}

func (fpc *FakePodControl) setCreatePodError(err error, after int) {
	fpc.createPodTracker.err = err
	fpc.createPodTracker.after = after
}

// SetUpdatePodError sets the error attributes of updatePodTracker
func (fpc *FakePodControl) SetUpdatePodError(err error, after int) {
	fpc.updatePodTracker.err = err
	fpc.updatePodTracker.after = after
}

// SetDeletePodError sets the error attributes of deletePodTracker
func (fpc *FakePodControl) SetDeletePodError(err error, after int) {
	fpc.deletePodTracker.err = err
	fpc.deletePodTracker.after = after
}

// SetGetClusterError sets the error attributes of getClusterTracker
func (fpc *FakePodControl) SetGetClusterError(err error, after int) {
	fpc.getClusterTracker.err = err
	fpc.getClusterTracker.after = after
}

// CreatePod create pod
func (fpc *FakePodControl) CreatePod(_ *v1alpha1.RedisCluster, pod *corev1.Pod) error {
	defer fpc.createPodTracker.inc()
	if fpc.createPodTracker.errorReady() {
		defer fpc.createPodTracker.reset()
		return fpc.createPodTracker.err
	}

	return fpc.PodIndexer.Add(pod)
}

// DeletePod delete pod
func (fpc *FakePodControl) DeletePod(_ *v1alpha1.RedisCluster, pod *corev1.Pod) error {
	defer fpc.deletePodTracker.inc()
	if fpc.deletePodTracker.errorReady() {
		defer fpc.deletePodTracker.reset()
		return fpc.deletePodTracker.err
	}

	return fpc.PodIndexer.Delete(pod)
}

// UpdatePod update pod info
func (fpc *FakePodControl) UpdatePod(_ *v1alpha1.RedisCluster, pod *corev1.Pod) (*corev1.Pod, error) {
	defer fpc.updatePodTracker.inc()
	if fpc.updatePodTracker.errorReady() {
		defer fpc.updatePodTracker.reset()
		return nil, fpc.updatePodTracker.err
	}

	setIfNotEmpty(pod.Labels, label.NameLabelKey, TestName)
	setIfNotEmpty(pod.Labels, label.ComponentLabelKey, TestComponentName)
	setIfNotEmpty(pod.Labels, label.ManagedByLabelKey, TestManagedByName)
	setIfNotEmpty(pod.Labels, label.InstanceLabelKey, TestClusterName)
	setIfNotEmpty(pod.Labels, label.ClusterModeLabelKey, TestClusterModeName)
	setIfNotEmpty(pod.Labels, label.ClusterNodeRoleLabelKey, TestNodeRoleName)

	return pod, fpc.PodIndexer.Update(pod)
}

var _ PodControlInterface = &FakePodControl{}
