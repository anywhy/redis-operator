package v1alpha1

func (mt MemberType) String() string {
	return string(mt)
}

// RedisUpgrading return redis is Upgrading status
func (rc *Redis) RedisUpgrading() bool {
	return rc.Status.Phase == UpgradePhase
}

// SentinelIsOk sentinel is ok
func (rc *Redis) SentinelIsOk() bool {
	sentSet := rc.Status.Sentinel.StatefulSet
	return sentSet.Replicas == sentSet.ReadyReplicas
}
