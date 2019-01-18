package v1alpha1

func (mt MemberType) String() string {
	return string(mt)
}

// ReplicaUpgrading return replica cluster is Upgrading status
func (rc *Redis) ReplicaUpgrading() bool {
	return rc.Status.Replica.Phase == UpgradePhase
}

// SentinelIsOk sentinel is ok
func (rc *Redis) SentinelIsOk() bool {
	sentSet := rc.Status.Sentinel.StatefulSet
	return sentSet.Replicas == sentSet.ReadyReplicas
}

// SentinelEnable sentinel is enable
func (rc *Redis) SentinelEnable() bool {
	return rc.Spec.Sentinel.Enable
}
