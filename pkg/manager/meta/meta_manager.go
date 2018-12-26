package meta

import (
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/label"
	"github.com/anywhy/redis-operator/pkg/manager"
)

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

func (mm *metaManager) Sync(rc *v1alpha1.Redis) error {
	if rc.Spec.Mode == v1alpha1.ReplicaCluster {
		return mm.syncReplicaCluster(rc)
	}
	return mm.syncRedisCluster(rc)
}

func (mm *metaManager) syncReplicaCluster(rc *v1alpha1.Redis) error {
	ns, labels := rc.GetNamespace(), rc.GetLabels()
	instanceName := labels[label.InstanceLabelKey]
	l, err := label.New().Instance(instanceName).Selector()
	if err != nil {
		return err
	}

	pods, err := mm.podLister.Pods(ns).List(l)
	if err != nil {
		return err
	}
	for _, pod := range pods {
		updatePod, err := mm.podControl.UpdatePod(rc, pod)
		if err != nil {
			return err
		}
		if updatePod.Labels[label.ComponentLabelKey] != label.MasterLabelKey &&
			updatePod.Labels[label.ComponentLabelKey] != label.SlaveLabelKey {
			continue
		}
	}

	return nil
}

func (mm *metaManager) syncRedisCluster(rc *v1alpha1.Redis) error {
	return nil
}
