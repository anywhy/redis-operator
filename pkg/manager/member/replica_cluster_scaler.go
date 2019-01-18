package member

import (
	apps "k8s.io/api/apps/v1beta1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
)

type replicaScaler struct {
	generalScaler
}

// NewReplicaScaler return redis scaler implement
func NewReplicaScaler(
	pvcLister corelisters.PersistentVolumeClaimLister,
	pvcControl controller.PVCControlInterface) Scaler {
	return &replicaScaler{generalScaler{
		pvcLister:  pvcLister,
		pvcControl: pvcControl}}
}

// ScaleOut scales out the cluster
func (rs *replicaScaler) ScaleOut(rc *v1alpha1.Redis, newSet *apps.StatefulSet, oldSet *apps.StatefulSet) error {
	if rc.ReplicaUpgrading() {
		resetReplicas(newSet, oldSet)
		return nil
	}

	increaseReplicas(newSet, oldSet)
	return nil
}

// ScaleIn scales in the cluster
func (rs *replicaScaler) ScaleIn(rc *v1alpha1.Redis, newSet *apps.StatefulSet, oldSet *apps.StatefulSet) error {
	if rc.ReplicaUpgrading() {
		resetReplicas(newSet, oldSet)
		return nil
	}

	decreaseReplicas(newSet, oldSet)
	return nil
}
