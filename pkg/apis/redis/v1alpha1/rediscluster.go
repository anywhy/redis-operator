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

// SentinelEnable sentinel is enable
func (rc *RedisCluster) SentinelEnable() bool {
	return rc.Spec.Sentinel.Enable
}
