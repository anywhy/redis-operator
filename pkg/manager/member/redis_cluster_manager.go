package member

import "github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"

type redisClusterMemeberManager struct {
}

// FakeRedisClusterMemberManager replica member manager fake use test
type FakeRedisClusterMemberManager struct {
	err error
}

// NewFakeRedisClusterMemberManager new fake instance
func NewFakeRedisClusterMemberManager() *FakeRedisClusterMemberManager {
	return &FakeRedisClusterMemberManager{}
}

// SetSyncError sync err
func (frmm *FakeRedisClusterMemberManager) SetSyncError(err error) {
	frmm.err = err
}

// Sync sync info
func (frmm *FakeRedisClusterMemberManager) Sync(_ *v1alpha1.RedisCluster) error {
	if frmm.err != nil {
		return frmm.err
	}
	return nil
}
