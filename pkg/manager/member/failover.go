package member

import "github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"

// Failover implements the logic for redis's failover and recovery.
type Failover interface {
	Failover(*v1alpha1.RedisCluster)
	Recover(*v1alpha1.RedisCluster)
}
