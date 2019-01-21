package redisc

import (
	"github.com/anywhy/redis-operator/pkg/util/redis"
)

// ClusterManager The Cluster Manager global structure
type ClusterManager struct {
}

// ClusterManagerNode redis cluster node
type ClusterManagerNode struct {
	Client *redis.Client

	Name  string
	IP    string
	Port  int
	Slots []int
}
