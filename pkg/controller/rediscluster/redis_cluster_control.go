package rediscluster

import (
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/manager"
)

// ControlInterface implements the control logic for updating RedisClusters and their children StatefulSets.
// It is implemented as an interface to allow for extensions that provide different semantics.
// Currently, there is only one implementation.
type ControlInterface interface {
	// UpdateRedisCluster implements the control logic for StatefulSet creation, update, and deletion
	UpdateRedisCluster(*v1alpha1.RedisCluster) error
}

type defaultRedisClusterControl struct {
	rcControl                 controller.RedisClusterControlInterface
	msClusterMemberManager    manager.Manager
	sentinelMemberManager     manager.Manager
	shardClusterMemberManager manager.Manager
	reclaimPolicyManager      manager.Manager
	metaManager               manager.Manager
	recorder                  record.EventRecorder
}

// NewDefaultRedisClusterControl returns a new instance of the default implementation ControlInterface that
// implements the documented semantics for RedisClusters.
func NewDefaultRedisClusterControl(
	rcControl controller.RedisClusterControlInterface,
	msMemberManager manager.Manager,
	sentinelMemberManager manager.Manager,
	shardClusterMemberManager manager.Manager,
	reclaimPolicyManager manager.Manager,
	metaManager manager.Manager,
	recorder record.EventRecorder) ControlInterface {
	return &defaultRedisClusterControl{
		rcControl,
		msMemberManager,
		sentinelMemberManager,
		shardClusterMemberManager,
		reclaimPolicyManager,
		metaManager,
		recorder,
	}
}

// UpdateRedisCluster executes the core logic loop for a rediscluster.
func (rcc *defaultRedisClusterControl) UpdateRedisCluster(rc *v1alpha1.RedisCluster) error {
	var errs []error
	oldStatus := rc.Status.DeepCopy()

	err := rcc.updateRedisCluster(rc)
	if err != nil {
		errs = append(errs, err)
	}

	if !apiequality.Semantic.DeepEqual(&rc.Status, oldStatus) {
		_, err := rcc.rcControl.UpdateRedisCluster(rc.DeepCopy(), &rc.Status, oldStatus)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errorutils.NewAggregate(errs)
}

func (rcc *defaultRedisClusterControl) updateRedisCluster(rc *v1alpha1.RedisCluster) error {
	if rc.Spec.Mode == v1alpha1.MSCluster {
		return rcc.syncMSCluster(rc)
	}
	return rcc.syncShardCluster(rc)
}

// sync ms cluster
func (rcc *defaultRedisClusterControl) syncMSCluster(rc *v1alpha1.RedisCluster) error {
	return rcc.msClusterMemberManager.Sync(rc)
}

// sync shard cluster
func (rcc *defaultRedisClusterControl) syncShardCluster(rc *v1alpha1.RedisCluster) error {
	return rcc.shardClusterMemberManager.Sync(rc)
}
