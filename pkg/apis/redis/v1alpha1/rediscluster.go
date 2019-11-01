package v1alpha1

func (mt MemberType) String() string {
	return string(mt)
}

// ReplicaUpgrading return replica cluster is Upgrading status
func (rc *RedisCluster) ReplicaUpgrading() bool {
	return rc.Status.Redis.Phase == UpgradePhase
}

// SentinelIsOk sentinel is ok
func (rc *RedisCluster) SentinelIsOk() bool {
	sentSet := rc.Status.Sentinel.StatefulSet
	return sentSet.Replicas == sentSet.ReadyReplicas
}

// IsEnableSentinel sentinel is enable
func (rc *RedisCluster) IsEnableSentinel() bool {
	return rc.Spec.Sentinel.Enable
}

// RedisRealReplicas redis real replicas
func (rc *RedisCluster) RedisRealReplicas() int32 {
	return rc.Spec.Redis.Replicas + int32(len(rc.Status.Redis.FailureMembers))
}

// RedisAllPodsStarted redis all pod started
func (rc *RedisCluster) RedisAllPodsStarted() bool {
	return rc.RedisRealReplicas() == rc.Status.Redis.StatefulSet.Replicas
}