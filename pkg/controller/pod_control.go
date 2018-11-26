package controller

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
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
	ns, rcName, labels := rc.GetNamespace(), rc.GetName(), rc.GetLabels()
	if labels == nil {
		return pod, fmt.Errorf("pod %s/%s has empty labels, RedisCluster: %s", ns, pod.Name, rcName)
	}

	_, ok := labels[label.InstanceLabelKey]
	if !ok {
		return pod, fmt.Errorf("pod %s/%s doesn't have %s label, RedisCluster: %s", ns,
			pod.Name, label.InstanceLabelKey, rcName)
	}

	if _, ok := labels[label.ComponentLabelKey]; !ok {
		labels[label.ComponentLabelKey] = label.SlaveLabelKey
	}

	var updatePod *corev1.Pod
	// don't wait due to limited number of clients, but backoff after the default number of steps
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var updateErr error
		updatePod, updateErr = rpc.kubeCli.CoreV1().Pods(ns).Update(pod)
		if updateErr == nil {
			glog.Infof("RedisCluster: [%s/%s] updated successfully", ns, rcName)
			return nil
		}
		glog.Errorf("failed to update RedisCluster: [%s/%s], error: %v", ns, rcName, updateErr)

		if updated, err := rpc.podLister.Pods(ns).Get(rcName); err == nil {
			// make a copy so we don't mutate the shared cache
			pod = updated.DeepCopy()
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated RedisCluster %s/%s from lister: %v", ns, rcName, err))
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
		glog.Errorf("failed to delete Pod: [%s/%s], RedisCluster: %s, %v", ns, podName, rcName, err)
	} else {
		glog.V(4).Infof("delete Pod: [%s/%s] successfully, RedisCluster: %s", ns, podName, rcName)
	}
	rpc.recordPodEvent("delete", rc, pod, err)
	return err
}

// recordPodEvent records an event for verb applied to a Pod in a Redis. If err is nil the generated event will
// have a reason of v1.EventTypeNormal. If err is not nil the generated event will have a reason of v1.EventTypeWarning.
func (rpc *realPodControl) recordPodEvent(verb string, rc *v1alpha1.RedisCluster, pod *corev1.Pod, err error) {
	if err == nil {
		reason := fmt.Sprintf("Successful%s", strings.Title(verb))
		message := fmt.Sprintf("%s Pod %s in RedisCluster %s successful",
			strings.ToLower(verb), pod.Name, rc.Name)
		rpc.recorder.Event(rc, corev1.EventTypeNormal, reason, message)
	} else {
		reason := fmt.Sprintf("Failed%s", strings.Title(verb))
		message := fmt.Sprintf("%s Pod %s in RedisCluster %s failed error: %s",
			strings.ToLower(verb), pod.Name, rc.Name, err)
		rpc.recorder.Event(rc, corev1.EventTypeWarning, reason, message)
	}
}

var _ PodControlInterface = &realPodControl{}
