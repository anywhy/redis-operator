package meta

import (
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/label"
	"github.com/anywhy/redis-operator/pkg/manager"
)

type reclaimPolicyManager struct {
	pvcLister corelisters.PersistentVolumeClaimLister
	pvLister  corelisters.PersistentVolumeLister
	pvControl controller.PVControlInterface
}

// NewReclaimPolicyManager returns a *reclaimPolicyManager
func NewReclaimPolicyManager(pvcLister corelisters.PersistentVolumeClaimLister,
	pvLister corelisters.PersistentVolumeLister,
	pvControl controller.PVControlInterface) manager.Manager {
	return &reclaimPolicyManager{
		pvcLister,
		pvLister,
		pvControl,
	}
}

func (rpm *reclaimPolicyManager) Sync(rc *v1alpha1.Redis) error {
	ns := rc.GetNamespace()
	instanceName := rc.GetLabels()[label.InstanceLabelKey]

	l, err := label.New().Instance(instanceName).Selector()
	if err != nil {
		return err
	}
	pvcs, err := rpm.pvcLister.PersistentVolumeClaims(ns).List(l)
	if err != nil {
		return err
	}

	for _, pvc := range pvcs {
		pv, err := rpm.pvLister.Get(pvc.Spec.VolumeName)
		if err != nil {
			return err
		}

		if pv.Spec.PersistentVolumeReclaimPolicy == rc.Spec.PVReclaimPolicy {
			continue
		}

		err = rpm.pvControl.PatchPVReclaimPolicy(rc, pv, rc.Spec.PVReclaimPolicy)
		if err != nil {
			return err
		}
	}

	return nil
}

var _ manager.Manager = &reclaimPolicyManager{}
