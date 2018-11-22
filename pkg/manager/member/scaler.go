package member

import (
	"fmt"

	apps "k8s.io/api/apps/v1beta1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
)

// Scaler implements the logic for scaling out or scaling in the cluster.
type Scaler interface {
	// ScaleOut scales out the cluster
	ScaleOut(rc *v1alpha1.RedisCluster, newSet *apps.StatefulSet, oldSet *apps.StatefulSet) error
	// ScaleIn scales in the cluster
	ScaleIn(rc *v1alpha1.RedisCluster, newSet *apps.StatefulSet, oldSet *apps.StatefulSet) error
}

type generalScaler struct {
	pvcLister  corelisters.PersistentVolumeClaimLister
	pvcControl controller.PVCControlInterface
}

func resetReplicas(newSet *apps.StatefulSet, oldSet *apps.StatefulSet) {
	*newSet.Spec.Replicas = *oldSet.Spec.Replicas
}
func increaseReplicas(newSet *apps.StatefulSet, oldSet *apps.StatefulSet) {
	*newSet.Spec.Replicas = *oldSet.Spec.Replicas + 1
}
func decreaseReplicas(newSet *apps.StatefulSet, oldSet *apps.StatefulSet) {
	*newSet.Spec.Replicas = *oldSet.Spec.Replicas - 1
}

func ordinalPodName(memberType v1alpha1.MemberType, rcName string, ordinal int32) string {
	return fmt.Sprintf("%s-%s-%d", rcName, memberType, ordinal)
}
