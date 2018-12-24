package manager

import (
	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
)

// Manager implements the logic for syncing Redis.
type Manager interface {
	// Sync	implements the logic for syncing Redis.
	Sync(*v1alpha1.Redis) error
}
