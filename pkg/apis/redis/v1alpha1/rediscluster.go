package v1alpha1

func (mt MemberType) String() string {
	return string(mt)
}

// RedisUpgrading return redis is Upgrading status
func (rc *RedisCluster) RedisUpgrading() bool {
	return rc.Status.Redis.Phase == UpgradePhase
}
