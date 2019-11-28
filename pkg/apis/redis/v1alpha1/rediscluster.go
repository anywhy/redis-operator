package v1alpha1

func (mt MemberType) String() string {
	return string(mt)
}

// ReplicaUpgrading return replica cluster is Upgrading status
func (rc *RedisCluster) ReplicaUpgrading() bool {
	return rc.Status.Redis.Phase == UpgradePhase
}

// AllSentinelPodsStarted sentinel is ok
func (rc *RedisCluster) AllSentinelPodsStarted() bool {
	sentSet := rc.Status.Sentinel.StatefulSet
	return sentSet.Replicas == sentSet.ReadyReplicas
}

// ShoudEnableSentinel sentinel is enable
func (rc *RedisCluster) ShoudEnableSentinel() bool {
	return rc.Spec.Sentinel.Replicas > 0 && rc.Spec.Redis.Replicas > 1
}

// RedisRealReplicas redis real replicas
func (rc *RedisCluster) RedisRealReplicas() int32 {
	return rc.Spec.Redis.Replicas + int32(len(rc.Status.Redis.FailureMembers))
}

// RedisAllPodsStarted redis all pod started
func (rc *RedisCluster) RedisAllPodsStarted() bool {
	return rc.RedisRealReplicas() == rc.Status.Redis.StatefulSet.Replicas
}
