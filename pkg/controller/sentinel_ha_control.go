package controller

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/anywhy/redis-operator/pkg/util/math2"
	"github.com/golang/glog"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/util/redis"
)

// HAControlInterface is an interface that knows how to manage get redis master
type HAControlInterface interface {
	// Monitor .
	Watch(rc *v1alpha1.RedisCluster, callback func(master string) error)
}

// defaultHAControl is the default implementation of HAControlInterface.
type defaultHAControl struct {
	mutex     sync.Mutex
	haClients map[string]*haWatcher
}

// NewDefaultHAControl returns a defaultHAControl instance
func NewDefaultHAControl() HAControlInterface {
	return &defaultHAControl{haClients: make(map[string]*haWatcher)}
}

type haWatcher struct {
	mutex sync.Mutex

	monitor *redis.Sentinel
	wtached bool
}

func (w *haWatcher) unWatched() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.wtached = false
}

func (w *haWatcher) watchSentinels(sentinels []string, watchCallback func(master string) error) {
	// if wtached return
	if w.wtached {
		return
	}

	w.wtached = true
	go func(p *redis.Sentinel) {
		var trigger = make(chan struct{}, 1)
		delayUntil := func(deadline time.Time) {
			for !p.IsCanceled() {
				var d = deadline.Sub(time.Now())
				if d <= 0 {
					return
				}
				time.Sleep(math2.MinDuration(d, time.Second))
			}
		}
		go func() {
			defer close(trigger)
			defer w.unWatched()
			callback := func() {
				select {
				case trigger <- struct{}{}:
				default:
				}
			}
			for !p.IsCanceled() {
				timeout := time.Minute * 5
				retryAt := time.Now().Add(time.Second * 10)
				if !p.Subscribe(sentinels, timeout, callback) {
					delayUntil(retryAt)
				} else {
					callback()
				}
			}
		}()
		go func() {
			for range trigger {
				var success int
				for i := 0; i != 10 && !p.IsCanceled() && success != 2; i++ {
					timeout := time.Second * 5
					master, err := p.Master(sentinels, timeout)
					if err != nil {
						glog.Warningf("[%p] fetch replica cluster masters failed(%#v)", w, err)
					} else {
						if !p.IsCanceled() {
							if master != "" && (watchCallback(master) != nil) {
								continue
							}
						}
						success++
					}
					delayUntil(time.Now().Add(time.Second * 5))
				}
			}
		}()
	}(w.monitor)
}

func (hac *defaultHAControl) Watch(rc *v1alpha1.RedisCluster, callback func(master string) error) {
	hac.mutex.Lock()
	defer hac.mutex.Unlock()
	rcName, ns := rc.GetName(), rc.GetNamespace()
	key := haClientKey(ns, rcName)
	if _, ok := hac.haClients[key]; !ok {
		hac.haClients[key] = &haWatcher{
			monitor: redis.NewSentinel(rc.GetName(), rc.Spec.Sentinel.Password),
			wtached: false,
		}
	}

	replicas := rc.Spec.Sentinel.Replicas
	hac.haClients[key].watchSentinels(sentinelAddrs(ns, rcName, int(replicas)), callback)
}

// haClientKey returns the sentinel client key
func haClientKey(namespace, clusterName string) string {
	return fmt.Sprintf("%s.%s", clusterName, namespace)
}

// sentinelAddrs return all sentinel address
func sentinelAddrs(namespace, clusterName string, replicas int) []string {
	addrs := make([]string, replicas)
	for i := 0; i < replicas; i++ {
		addrs[i] = fmt.Sprintf("%s-sentinel-%s.%s-sentinel-peer.%s.svc:26379",
			clusterName, strconv.Itoa(i), clusterName, namespace)
	}
	return addrs
}
