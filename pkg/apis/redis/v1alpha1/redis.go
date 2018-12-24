package v1alpha1

func (mt MemberType) String() string {
	return string(mt)
}

// RedisUpgrading return redis is Upgrading status
func (rc *Redis) RedisUpgrading() bool {
	return rc.Status.Replica.Phase == UpgradePhase
}
