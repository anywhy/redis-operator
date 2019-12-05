package controller

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
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

// PVCControlInterface manages PVCs used in Redis
type PVCControlInterface interface {
	UpdatePVC(*v1alpha1.RedisCluster, *corev1.PersistentVolumeClaim, *corev1.Pod) (*corev1.PersistentVolumeClaim, error)
	DeletePVC(*v1alpha1.RedisCluster, *corev1.PersistentVolumeClaim) error
}

type realPVCControl struct {
	kubeCli   kubernetes.Interface
	recorder  record.EventRecorder
	pvcLister corelisters.PersistentVolumeClaimLister
}

// NewRealPVCControl creates a new PVCControlInterface
func NewRealPVCControl(
	kubeCli kubernetes.Interface,
	recorder record.EventRecorder,
	pvcLister corelisters.PersistentVolumeClaimLister) PVCControlInterface {
	return &realPVCControl{
		kubeCli:   kubeCli,
		recorder:  recorder,
		pvcLister: pvcLister,
	}
}

// UpdatePVC update a pvc in a Redis.
func (rpc *realPVCControl) UpdatePVC(rc *v1alpha1.RedisCluster, pvc *corev1.PersistentVolumeClaim, pod *corev1.Pod) (*corev1.PersistentVolumeClaim, error) {
	ns, rcName := rc.GetNamespace(), rc.GetName()
	pvcName := pvc.GetName()

	if pvc.Annotations == nil {
		pvc.Annotations = make(map[string]string)
	}

	if pvc.Labels == nil {
		pvc.Labels = make(map[string]string)
	}

	if pod != nil {
		podName := pod.GetName()
		if pvc.Annotations[label.AnnPodNameKey] == podName {
			glog.V(4).Infof("pvc %s/%s already has labels and annotations synced, skipping, Redis: %s", ns, pvcName, rcName)
			return pvc, nil
		}

		// udpate labels and anno
		setIfNotEmpty(pvc.Annotations, label.AnnPodNameKey, podName)
	}

	labels := pvc.GetLabels()
	ann := pvc.GetAnnotations()
	var updatePVC *corev1.PersistentVolumeClaim
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var updateErr error
		updatePVC, updateErr = rpc.kubeCli.CoreV1().PersistentVolumeClaims(ns).Update(pvc)
		if updateErr == nil {
			glog.Infof("update PVC: [%s/%s] successfully, Redis: %s", ns, pvcName, rcName)
			return nil
		}
		glog.Errorf("failed to update PVC: [%s/%s], Redis: %s, error: %v", ns, pvcName, rcName, updateErr)

		if updated, err := rpc.pvcLister.PersistentVolumeClaims(ns).Get(pvcName); err == nil {
			// make a copy so we don't mutate the shared cache
			pvc = updated.DeepCopy()
			pvc.Labels = labels
			pvc.Annotations = ann
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated PVC %s/%s from lister: %v", ns, pvcName, err))
		}

		return updateErr
	})
	rpc.recordPVCEvent("update", rc, pvcName, err)
	return updatePVC, err
}

// DeletePVC delete a pvc in a Redis.
func (rpc *realPVCControl) DeletePVC(rc *v1alpha1.RedisCluster, pvc *corev1.PersistentVolumeClaim) error {
	err := rpc.kubeCli.CoreV1().PersistentVolumeClaims(rc.Namespace).Delete(pvc.Name, nil)
	if err != nil {
		glog.Errorf("failed to delete PVC: [%s/%s], Redis: %s, %v", rc.Namespace, pvc.Name, rc.Name, err)
	}
	glog.Infof("delete PVC: [%s/%s] successfully, Redis: %s", rc.Namespace, pvc.Name, rc.Name)
	rpc.recordPVCEvent("delete", rc, pvc.Name, err)
	return err
}

func (rpc *realPVCControl) recordPVCEvent(verb string, rc *v1alpha1.RedisCluster, pvcName string, err error) {
	rcName := rc.GetName()
	if err == nil {
		reason := fmt.Sprintf("Successful%s", strings.Title(verb))
		msg := fmt.Sprintf("%s PVC %s in Redis %s successful",
			strings.ToLower(verb), pvcName, rcName)
		rpc.recorder.Event(rc, corev1.EventTypeNormal, reason, msg)
	} else {
		reason := fmt.Sprintf("Failed%s", strings.Title(verb))
		msg := fmt.Sprintf("%s PVC %s in Redis %s failed error: %s",
			strings.ToLower(verb), pvcName, rcName, err)
		rpc.recorder.Event(rc, corev1.EventTypeWarning, reason, msg)
	}
}

var _ PVCControlInterface = &realPVCControl{}

// FakePVCControl is a fake PVCControlInterface
type FakePVCControl struct {
	PVCIndexer       cache.Indexer
	updatePVCTracker requestTracker
	deletePVCTracker requestTracker
}

// NewFakePVCControl returns a FakePVCControl
func NewFakePVCControl(pvcInformer coreinformers.PersistentVolumeClaimInformer) *FakePVCControl {
	return &FakePVCControl{
		pvcInformer.Informer().GetIndexer(),
		requestTracker{0, nil, 0},
		requestTracker{0, nil, 0},
	}
}

// SetUpdatePVCError sets the error attributes of updatePVCTracker
func (fpc *FakePVCControl) SetUpdatePVCError(err error, after int) {
	fpc.updatePVCTracker.err = err
	fpc.updatePVCTracker.after = after
}

// SetDeletePVCError sets the error attributes of deletePVCTracker
func (fpc *FakePVCControl) SetDeletePVCError(err error, after int) {
	fpc.deletePVCTracker.err = err
	fpc.deletePVCTracker.after = after
}

// DeletePVC deletes the pvc
func (fpc *FakePVCControl) DeletePVC(_ *v1alpha1.RedisCluster, pvc *corev1.PersistentVolumeClaim) error {
	defer fpc.deletePVCTracker.inc()
	if fpc.deletePVCTracker.errorReady() {
		defer fpc.deletePVCTracker.reset()
		return fpc.deletePVCTracker.err
	}

	return fpc.PVCIndexer.Delete(pvc)
}

// UpdatePVC updates the annotation, labels and spec of pvc
func (fpc *FakePVCControl) UpdatePVC(rc *v1alpha1.RedisCluster, pvc *corev1.PersistentVolumeClaim, pod *corev1.Pod) (*corev1.PersistentVolumeClaim, error) {
	defer fpc.updatePVCTracker.inc()
	if fpc.updatePVCTracker.errorReady() {
		defer fpc.updatePVCTracker.reset()
		return nil, fpc.updatePVCTracker.err
	}

	if pvc.Annotations == nil {
		pvc.Annotations = make(map[string]string)
	}
	if pvc.Labels == nil {
		pvc.Labels = make(map[string]string)
	}

	podName := pod.GetName()
	setIfNotEmpty(pvc.Annotations, label.AnnPodNameKey, podName)

	return pvc, fpc.PVCIndexer.Update(pvc)
}

var _ PVCControlInterface = &FakePVCControl{}
