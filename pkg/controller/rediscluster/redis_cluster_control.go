package rediscluster

import (
	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
)

// ControlInterface implements the control logic for updating RedisClusters and their children StatefulSets.
// It is implemented as an interface to allow for extensions that provide different semantics.
// Currently, there is only one implementation.
type ControlInterface interface {
	// UpdateRedisCluster implements the control logic for StatefulSet creation, update, and deletion
	UpdateRedisCluster(*v1alpha1.Redis) error
}
