package manager

import (
	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
)

// Manager implements the logic for syncing rediscluster.
type Manager interface {
	// Sync	implements the logic for syncing rediscluster.
	Sync(*v1alpha1.RedisCluster) error
}
