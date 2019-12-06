package member

import (
	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	apps "k8s.io/api/apps/v1beta1"
)

type redisClusterScaler struct {
	generalScaler
}

// ScaleOut scales out the cluster
func (rcs *redisClusterScaler) ScaleOut(rc *v1alpha1.RedisCluster, newSet *apps.StatefulSet, oldSet *apps.StatefulSet) error {

	return nil
}

// ScaleIn scales in the cluster
func (rcs *redisClusterScaler) ScaleIn(rc *v1alpha1.RedisCluster, newSet *apps.StatefulSet, oldSet *apps.StatefulSet) error {

	return nil
}
