package member

import (
	apps "k8s.io/api/apps/v1beta1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
)

type redisScaler struct {
	generalScaler
}

// NewRedisScaler return redis scaler implement
func NewRedisScaler(
	pvcLister corelisters.PersistentVolumeClaimLister,
	pvcControl controller.PVCControlInterface) Scaler {
	return &redisScaler{generalScaler{
		pvcLister:  pvcLister,
		pvcControl: pvcControl}}
}

// ScaleOut scales out the cluster
func (rs *redisScaler) ScaleOut(rc *v1alpha1.RedisCluster, newSet *apps.StatefulSet, oldSet *apps.StatefulSet) error {
	if rc.RedisUpgrading() {
		resetReplicas(newSet, oldSet)
		return nil
	}

	increaseReplicas(newSet, oldSet)
	return nil
}

// ScaleIn scales in the cluster
func (rs *redisScaler) ScaleIn(rc *v1alpha1.RedisCluster, newSet *apps.StatefulSet, oldSet *apps.StatefulSet) error {
	if rc.RedisUpgrading() {
		resetReplicas(newSet, oldSet)
		return nil
	}

	decreaseReplicas(newSet, oldSet)
	return nil
}
