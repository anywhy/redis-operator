package member

import (
	apps "k8s.io/api/apps/v1beta1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
)

type redisClusterUpgrader struct {
	podControl controller.PodControlInterface
	podLister  corelisters.PodLister
}

func (rcu *redisClusterUpgrader) Upgrade(rc *v1alpha1.RedisCluster, oldSet *apps.StatefulSet, newSet *apps.StatefulSet) error {
	return nil
}
