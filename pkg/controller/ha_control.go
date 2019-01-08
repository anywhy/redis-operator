package controller

import (
	"fmt"
	"sync"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/util/redis"
)

// HAControlInterface is an interface that knows how to manage get redis master
type HAControlInterface interface {
	// GetHAClient provides redis sentinel of the replica redis cluster.
	GetHAClient(rc *v1alpha1.Redis) (*redis.Sentinel, error)
}

// defaultHAControl is the default implementation of HAControlInterface.
type defaultHAControl struct {
	mutex     sync.Mutex
	haClients map[string]*redis.Sentinel
}

func (hac *defaultHAControl) GetHAClient(rc *v1alpha1.Redis) (*redis.Sentinel, error) {
	hac.mutex.Lock()
	defer hac.mutex.Unlock()
	rcName, ns := rc.GetName(), rc.GetNamespace()
	
	key := haClientKey(ns, rcName)
	if _, ok := hac.haClients[key]; !ok {
		sentinel, err := redis.NewSentinel(rc.Spec.Sentinel.MasterName, haClientAddr(ns, rcName),
			rc.Spec.Sentinel.Password)
		if err != nil {
			return nil, err
		}
		hac.haClients[key] = sentinel
	}

	return hac.haClients[key], nil
}

// haClientKey returns the sentinel client key
func haClientKey(namespace, clusterName string) string {
	return fmt.Sprintf("%s.%s", clusterName, namespace)
}

func haClientAddr(namespace, clusterName string) string {
	return fmt.Sprintf("%s-master-peer.%s.svc:6379", clusterName, namespace)
}
