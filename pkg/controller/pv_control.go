package controller

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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

// PVControlInterface manages PVs used in Redis
type PVControlInterface interface {
	UpdatePV(*v1alpha1.RedisCluster, *corev1.PersistentVolume) (*corev1.PersistentVolume, error)
	PatchPVReclaimPolicy(*v1alpha1.RedisCluster, *corev1.PersistentVolume, corev1.PersistentVolumeReclaimPolicy) error
}

type realPVControl struct {
	kubeCli   kubernetes.Interface
	pvcLister corelisters.PersistentVolumeClaimLister
	pvLister  corelisters.PersistentVolumeLister
	recorder  record.EventRecorder
}

// NewRealPVControl creates a new PVControlInterface
func NewRealPVControl(
	kubeCli kubernetes.Interface,
	pvcLister corelisters.PersistentVolumeClaimLister,
	pvLister corelisters.PersistentVolumeLister,
	recorder record.EventRecorder,
) PVControlInterface {
	return &realPVControl{
		kubeCli:   kubeCli,
		pvcLister: pvcLister,
		pvLister:  pvLister,
		recorder:  recorder,
	}
}

// UpdatePV update a pv in a Redis.
func (rpc *realPVControl) UpdatePV(rc *v1alpha1.RedisCluster, pv *corev1.PersistentVolume) (*corev1.PersistentVolume, error) {
	ns, rcName := rc.GetNamespace(), rc.GetName()
	if pv.Labels == nil {
		pv.Labels = make(map[string]string)
	}
	if pv.Annotations == nil {
		pv.Annotations = make(map[string]string)
	}
	pvName := pv.GetName()
	pvcRef := pv.Spec.ClaimRef
	if pvcRef == nil {
		glog.Warningf("PV: [%s] doesn't have a ClaimRef, skipping, Redis: %s/%s", pvName, ns, rcName)
		return pv, nil
	}
	pvcName := pvcRef.Name
	pvc, err := rpc.pvcLister.PersistentVolumeClaims(ns).Get(pvcName)
	if err != nil {
		if !apierrs.IsNotFound(err) {
			return pv, err
		}
		glog.Warningf("PV: [%s]'s PVC: [%s/%s] doesn't exist, skipping. Redis: %s", pvName, ns, pvcName, rcName)
		return pv, nil
	}

	component := pvc.Labels[label.ComponentLabelKey]
	podName := pvc.Annotations[label.AnnPodNameKey]
	if pv.Labels[label.NamespaceLabelKey] == ns &&
		pv.Labels[label.ComponentLabelKey] == component &&
		pv.Labels[label.NameLabelKey] == pvc.Labels[label.NameLabelKey] &&
		pv.Labels[label.ManagedByLabelKey] == pvc.Labels[label.ManagedByLabelKey] &&
		pv.Labels[label.InstanceLabelKey] == pvc.Labels[label.InstanceLabelKey] &&
		pv.Annotations[label.AnnPodNameKey] == podName {
		glog.V(4).Infof("pv %s already has labels and annotations synced, skipping. Redis: %s/%s", pvName, ns, rcName)
		return pv, nil
	}

	pv.Labels[label.NamespaceLabelKey] = ns
	pv.Labels[label.ComponentLabelKey] = component
	pv.Labels[label.NameLabelKey] = pvc.Labels[label.NameLabelKey]
	pv.Labels[label.ManagedByLabelKey] = pvc.Labels[label.ManagedByLabelKey]
	pv.Labels[label.InstanceLabelKey] = pvc.Labels[label.InstanceLabelKey]

	setIfNotEmpty(pv.Annotations, label.AnnPodNameKey, podName)

	labels := pv.GetLabels()
	ann := pv.GetAnnotations()
	var updatePV *corev1.PersistentVolume
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var updateErr error
		updatePV, updateErr = rpc.kubeCli.CoreV1().PersistentVolumes().Update(pv)
		if updateErr == nil {
			glog.Infof("PV: [%s] updated successfully, Redis: %s/%s", pvName, ns, rcName)
			return nil
		}
		glog.Errorf("failed to update PV: [%s], Redis %s/%s, error: %v", pvName, ns, rcName, err)

		if updated, err := rpc.pvLister.Get(pvName); err == nil {
			// make a copy so we don't mutate the shared cache
			pv = updated.DeepCopy()
			pv.Labels = labels
			pv.Annotations = ann
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated PV %s/%s from lister: %v", ns, pvName, err))
		}
		return updateErr
	})

	rpc.recordPVEvent("update", rc, pvName, err)
	return updatePV, err
}

// PatchPVReclaimPolicy update a pvclaim in a Redis.
func (rpc *realPVControl) PatchPVReclaimPolicy(rc *v1alpha1.RedisCluster, pv *corev1.PersistentVolume,
	reclaimPolicy corev1.PersistentVolumeReclaimPolicy) error {
	pvName := pv.GetName()
	patchBytes := []byte(fmt.Sprintf(`{"spec":{"persistentVolumeReclaimPolicy":"%s"}}`, reclaimPolicy))

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		_, err := rpc.kubeCli.CoreV1().PersistentVolumes().Patch(pvName, types.StrategicMergePatchType, patchBytes)
		return err
	})
	rpc.recordPVEvent("patch", rc, pvName, err)
	return err
}

func (rpc *realPVControl) recordPVEvent(verb string, tc *v1alpha1.RedisCluster, pvName string, err error) {
	tcName := tc.GetName()
	if err == nil {
		reason := fmt.Sprintf("Successful%s", strings.Title(verb))
		msg := fmt.Sprintf("%s PV %s in Redis %s successful",
			strings.ToLower(verb), pvName, tcName)
		rpc.recorder.Event(tc, corev1.EventTypeNormal, reason, msg)
	} else {
		reason := fmt.Sprintf("Failed%s", strings.Title(verb))
		msg := fmt.Sprintf("%s PV %s in Redis %s failed error: %s",
			strings.ToLower(verb), pvName, tcName, err)
		rpc.recorder.Event(tc, corev1.EventTypeWarning, reason, msg)
	}
}

var _ PVControlInterface = &realPVControl{}

// FakePVControl is a fake PVControlInterface
type FakePVControl struct {
	PVCLister       corelisters.PersistentVolumeClaimLister
	PVIndexer       cache.Indexer
	updatePVTracker requestTracker
}

// NewFakePVControl returns a FakePVControl
func NewFakePVControl(pvInformer coreinformers.PersistentVolumeInformer, pvcInformer coreinformers.PersistentVolumeClaimInformer) *FakePVControl {
	return &FakePVControl{
		pvcInformer.Lister(),
		pvInformer.Informer().GetIndexer(),
		requestTracker{0, nil, 0},
	}
}

// SetUpdatePVError sets the error attributes of updatePVTracker
func (fpc *FakePVControl) SetUpdatePVError(err error, after int) {
	fpc.updatePVTracker.err = err
	fpc.updatePVTracker.after = after
}

// PatchPVReclaimPolicy patchs the reclaim policy of PV
func (fpc *FakePVControl) PatchPVReclaimPolicy(_ *v1alpha1.RedisCluster, pv *corev1.PersistentVolume, reclaimPolicy corev1.PersistentVolumeReclaimPolicy) error {
	defer fpc.updatePVTracker.inc()
	if fpc.updatePVTracker.errorReady() {
		defer fpc.updatePVTracker.reset()
		return fpc.updatePVTracker.err
	}
	pv.Spec.PersistentVolumeReclaimPolicy = reclaimPolicy

	return fpc.PVIndexer.Update(pv)
}

// UpdatePV update a pv in a Redis.
func (fpc *FakePVControl) UpdatePV(rc *v1alpha1.RedisCluster, pv *corev1.PersistentVolume) (*corev1.PersistentVolume, error) {
	defer fpc.updatePVTracker.inc()
	if fpc.updatePVTracker.errorReady() {
		defer fpc.updatePVTracker.reset()
		return nil, fpc.updatePVTracker.err
	}

	ns := rc.GetNamespace()
	if pv.Labels == nil {
		pv.Labels = make(map[string]string)
	}
	if pv.Annotations == nil {
		pv.Annotations = make(map[string]string)
	}

	pvcName := pv.Spec.ClaimRef.Name
	pvc, err := fpc.PVCLister.PersistentVolumeClaims(ns).Get(pvcName)
	if err != nil {
		return nil, err
	}

	component := pvc.Labels[label.ComponentLabelKey]
	podName := pvc.Annotations[label.AnnPodNameKey]
	pv.Labels[label.NamespaceLabelKey] = ns
	pv.Labels[label.ComponentLabelKey] = component
	pv.Labels[label.NameLabelKey] = pvc.Labels[label.NameLabelKey]
	pv.Labels[label.ManagedByLabelKey] = pvc.Labels[label.ManagedByLabelKey]
	pv.Labels[label.InstanceLabelKey] = pvc.Labels[label.InstanceLabelKey]

	setIfNotEmpty(pv.Annotations, label.AnnPodNameKey, podName)

	return pv, fpc.PVIndexer.Update(pv)
}

var _ PVControlInterface = &FakePVControl{}
