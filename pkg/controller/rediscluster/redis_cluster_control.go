package rediscluster

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
	UpdateRedisCluster(*v1alpha1.RedisCluster) error
}

type defaultRedisControl struct {
	rcControl                 controller.RedisClusterControlInterface
	replicaMemberManager      manager.Manager
	replicaHAMemberManager    manager.Manager
	redisClusterMemberManager manager.Manager
	reclaimPolicyManager      manager.Manager
	metaManager               manager.Manager
	recorder                  record.EventRecorder
}

// NewDefaultRedisControl returns a new instance of the default implementation ControlInterface that
// implements the documented semantics for Rediss.
func NewDefaultRedisControl(
	rcControl controller.RedisClusterControlInterface,
	replicaMemberManager manager.Manager,
	replicaHAMemberManager manager.Manager,
	redisClusterMemberManager manager.Manager,
	reclaimPolicyManager manager.Manager,
	metaManager manager.Manager,
	recorder record.EventRecorder) ControlInterface {
	return &defaultRedisControl{
		rcControl,
		replicaMemberManager,
		replicaHAMemberManager,
		redisClusterMemberManager,
		reclaimPolicyManager,
		metaManager,
		recorder,
	}
}

// UpdateRedis executes the core logic loop for a Redis.
func (rcc *defaultRedisControl) UpdateRedisCluster(rc *v1alpha1.RedisCluster) error {
	var errs []error
	oldStatus := rc.Status.DeepCopy()

	err := rcc.updateRedis(rc)
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

func (rcc *defaultRedisControl) updateRedis(rc *v1alpha1.RedisCluster) error {
	// syncing all PVs managed by operator's reclaim policy to Retain
	if err := rcc.reclaimPolicyManager.Sync(rc); err != nil {
		return err
	}

	// sync replica cluster, member/service/label
	if rc.Spec.Mode == v1alpha1.Replica {
		return rcc.updateReplicaRedisCluster(rc)
	}

	// sync redis cluster, member/service/label
	return rcc.updateRedisCluster(rc)
}

// update replica cluster
func (rcc *defaultRedisControl) updateReplicaRedisCluster(rc *v1alpha1.RedisCluster) error {
	// works that should do to making the redis replica cluster
	//  - create or update the master/slave service
	//  - create or update the master/slave headless service
	//  - create the replica statefulset
	//  - sync replica cluster status to Redis object
	//  - build replica cluster member and set master role node, slave role node:
	// 		- label.ClusterNodeRoleLabelKey
	//   - upgrade the replica cluster
	//   - scale out/in the replica cluster
	if err := rcc.replicaMemberManager.Sync(rc); err != nil {
		return err
	}

	// if sentienl is enable create sentinel cluster
	if rc.ShoudEnableSentinel() && rc.RedisAllPodsStarted() {
		if err := rcc.replicaHAMemberManager.Sync(rc); err != nil {
			return err
		}
	}

	// syncing the labels from Pod to PVC and PV, these labels include:
	//   - label.ComponentLabelKey
	//   - label.NamespaceLabelKey
	return rcc.metaManager.Sync(rc)
}

// update redis cluster
func (rcc *defaultRedisControl) updateRedisCluster(rc *v1alpha1.RedisCluster) error {
	return rcc.redisClusterMemberManager.Sync(rc)
}
