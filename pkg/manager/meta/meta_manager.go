package meta

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/label"
	"github.com/anywhy/redis-operator/pkg/manager"
)

var errPVCNotFound = errors.New("PVC is not found")

type metaManager struct {
	pvcLister  corelisters.PersistentVolumeClaimLister
	pvcControl controller.PVCControlInterface
	pvLister   corelisters.PersistentVolumeLister
	pvControl  controller.PVControlInterface
	podLister  corelisters.PodLister
	podControl controller.PodControlInterface
}

// NewMetaManager returns a *metaManager
func NewMetaManager(
	pvcLister corelisters.PersistentVolumeClaimLister,
	pvcControl controller.PVCControlInterface,
	pvLister corelisters.PersistentVolumeLister,
	pvControl controller.PVControlInterface,
	podLister corelisters.PodLister,
	podControl controller.PodControlInterface,
) manager.Manager {
	return &metaManager{
		pvcLister:  pvcLister,
		pvcControl: pvcControl,
		pvLister:   pvLister,
		pvControl:  pvControl,
		podLister:  podLister,
		podControl: podControl,
	}
}

func (mm *metaManager) Sync(rc *v1alpha1.RedisCluster) error {
	ns, labels := rc.GetNamespace(), rc.GetLabels()
	instanceName := labels[label.InstanceLabelKey]
	l, err := label.New().Instance(instanceName).Redis().Selector()
	if err != nil {
		return err
	}

	pods, err := mm.podLister.Pods(ns).List(l)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		// update meta info for pvc
		pvc, err := mm.resolvePVCFromPod(pod)
		if err != nil {
			return err
		}
		_, err = mm.pvcControl.UpdatePVC(rc, pvc, pod)
		if err != nil {
			return err
		}

		if pvc.Spec.VolumeName != "" {
			// update meta info for pv
			pv, err := mm.pvLister.Get(pvc.Spec.VolumeName)
			if err != nil {
				return err
			}
			_, err = mm.pvControl.UpdatePV(rc, pv)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (mm *metaManager) resolvePVCFromPod(pod *corev1.Pod) (*corev1.PersistentVolumeClaim, error) {
	var pvcName string
	for _, vol := range pod.Spec.Volumes {
		switch vol.Name {
		case v1alpha1.RedisMemberType.String():
			if vol.PersistentVolumeClaim != nil {
				pvcName = vol.PersistentVolumeClaim.ClaimName
				break
			}
		default:
			continue
		}
	}
	if len(pvcName) == 0 {
		return nil, errPVCNotFound
	}

	pvc, err := mm.pvcLister.PersistentVolumeClaims(pod.Namespace).Get(pvcName)
	if err != nil {
		return nil, err
	}
	return pvc, nil
}

var _ manager.Manager = &metaManager{}

// FakeMetaManager meata manage fake use test
type FakeMetaManager struct {
	err error
}

// NewFakeMetaManager new instance
func NewFakeMetaManager() *FakeMetaManager {
	return &FakeMetaManager{}
}

// SetSyncError sync err
func (fmm *FakeMetaManager) SetSyncError(err error) {
	fmm.err = err
}

// Sync sync info
func (fmm *FakeMetaManager) Sync(_ *v1alpha1.RedisCluster) error {
	if fmm.err != nil {
		return fmm.err
	}
	return nil
}
