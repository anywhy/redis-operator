package redis

import (
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/manager"
)

// ControlInterface implements the control logic for updating Rediss and their children StatefulSets.
// It is implemented as an interface to allow for extensions that provide different semantics.
// Currently, there is only one implementation.
type ControlInterface interface {
	// UpdateRedis implements the control logic for StatefulSet creation, update, and deletion
	UpdateRedis(*v1alpha1.Redis) error
}

type defaultRedisControl struct {
	rcControl                 controller.RedisControlInterface
	replicaMemberManager      manager.Manager
	sentinelMemberManager     manager.Manager
	redisClusterMemberManager manager.Manager
	reclaimPolicyManager      manager.Manager
	metaManager               manager.Manager
	recorder                  record.EventRecorder
}

// NewDefaultRedisControl returns a new instance of the default implementation ControlInterface that
// implements the documented semantics for Rediss.
func NewDefaultRedisControl(
	rcControl controller.RedisControlInterface,
	replicaMemberManager manager.Manager,
	sentinelMemberManager manager.Manager,
	redisClusterMemberManager manager.Manager,
	reclaimPolicyManager manager.Manager,
	metaManager manager.Manager,
	recorder record.EventRecorder) ControlInterface {
	return &defaultRedisControl{
		rcControl,
		replicaMemberManager,
		sentinelMemberManager,
		redisClusterMemberManager,
		reclaimPolicyManager,
		metaManager,
		recorder,
	}
}

// UpdateRedis executes the core logic loop for a Redis.
func (rcc *defaultRedisControl) UpdateRedis(rc *v1alpha1.Redis) error {
	var errs []error
	oldStatus := rc.Status.DeepCopy()

	err := rcc.updateRedis(rc)
	if err != nil {
		errs = append(errs, err)
	}

	if !apiequality.Semantic.DeepEqual(&rc.Status, oldStatus) {
		_, err := rcc.rcControl.UpdateRedis(rc.DeepCopy(), &rc.Status, oldStatus)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errorutils.NewAggregate(errs)
}

func (rcc *defaultRedisControl) updateRedis(rc *v1alpha1.Redis) error {
	if rc.Spec.Mode == v1alpha1.ReplicaCluster {
		return rcc.updateReplicaCluster(rc)
	}
	return rcc.updateRedisCluster(rc)
}

// update replica cluster
func (rcc *defaultRedisControl) updateReplicaCluster(rc *v1alpha1.Redis) error {
	// works that should do to making the redis replica cluster
	//  - create or update the master/slave service
	//  - create or update the master/slave headless service
	//  - create the replica statefulset
	//  - sync replica cluster status to Redis object
	//  - set one annotations to the master replica cluster member:
	// 		- label.ComponentLabelKey
	//   - upgrade the replica cluster
	//   - scale out/in the replica cluster
	if err := rcc.replicaMemberManager.Sync(rc); err != nil {
		return err
	}

	// syncing the labels from Pod to PVC and PV, these labels include:
	//   - label.ComponentLabelKey
	//   - label.NamespaceLabelKey
	return rcc.metaManager.Sync(rc)
}

// update redis cluster
func (rcc *defaultRedisControl) updateRedisCluster(rc *v1alpha1.Redis) error {
	return rcc.redisClusterMemberManager.Sync(rc)
}
