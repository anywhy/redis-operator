package member

import (
	apps "k8s.io/api/apps/v1beta1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
)

// Upgrader implements the logic for upgrading the redis cluster.
type Upgrader interface {
	// Upgrade upgrade the cluster
	Upgrade(*v1alpha1.RedisCluster, *apps.StatefulSet, *apps.StatefulSet) error
}
