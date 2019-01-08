package member

import (
	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
)

type replicaFailover struct {
	haControl controller.HAControlInterface
}

// NewReplicaFailover return a replica cluster Failover
func NewReplicaFailover(haControl controller.HAControlInterface) Failover {
	return &replicaFailover{haControl}
}

func (rf *replicaFailover) Failover(rc *v1alpha1.Redis) error {
	return nil
}

func (rf *replicaFailover) Recover(*v1alpha1.Redis) {
	// Do nothing now
}
