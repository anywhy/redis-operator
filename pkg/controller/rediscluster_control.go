package controller

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/client/clientset/versioned"
	listers "github.com/anywhy/redis-operator/pkg/client/listers/redis/v1alpha1"
)

// RedisClusterControlInterface manages RedisClusters
type RedisClusterControlInterface interface {
	UpdateRedisCluster(rc *v1alpha1.RedisCluster, newStatus *v1alpha1.RedisClusterStatus,
		oldStatus *v1alpha1.RedisClusterStatus) (*v1alpha1.RedisCluster, error)
}

type realRedisClusterControl struct {
	cli      versioned.Interface
	rcLister listers.RedisClusterLister
	recorder record.EventRecorder
}

// NewRealRedisClusterControl creates a new RedisClusterControlInterface
func NewRealRedisClusterControl(cli versioned.Interface,
	rcLister listers.RedisClusterLister,
	recorder record.EventRecorder) RedisClusterControlInterface {
	return &realRedisClusterControl{
		cli,
		rcLister,
		recorder,
	}
}

func (rrc *realRedisClusterControl) UpdateRedisCluster(rc *v1alpha1.RedisCluster,
	newStatus *v1alpha1.RedisClusterStatus, oldStatus *v1alpha1.RedisClusterStatus) (*v1alpha1.RedisCluster, error) {
	ns, rcName := rc.GetNamespace(), rc.GetName()
	status := rc.Status.DeepCopy()
	var updateRC *v1alpha1.RedisCluster

	// don't wait due to limited number of clients, but backoff after the default number of steps
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var updateErr error
		updateRC, updateErr = rrc.cli.Redis().RedisClusters(ns).Update(rc)
		if updateErr == nil {
			glog.Infof("RedisCluster: [%s/%s] updated successfully", ns, rcName)
			return nil
		}
		glog.Errorf("failed to update RedisCluster: [%s/%s], error: %v", ns, rcName, updateErr)

		if updated, err := rrc.rcLister.RedisClusters(ns).Get(rcName); err == nil {
			// make a copy so we don't mutate the shared cache
			rc = updated.DeepCopy()
			rc.Status = *status
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated RedisCluster %s/%s from lister: %v", ns, rcName, err))
		}

		return updateErr
	})
	if !apiequality.Semantic.DeepEqual(newStatus, oldStatus) {
		rrc.recordRedisClusterEvent("update", rc, err)
	}
	return updateRC, err
}

func (rrc *realRedisClusterControl) recordRedisClusterEvent(verb string, rc *v1alpha1.RedisCluster, err error) {
	rcName := rc.GetName()
	if err == nil {
		reason := fmt.Sprintf("Successful%s", strings.Title(verb))
		msg := fmt.Sprintf("%s RedisCluster %s successful",
			strings.ToLower(verb), rcName)
		rrc.recorder.Event(rc, corev1.EventTypeNormal, reason, msg)
	} else {
		reason := fmt.Sprintf("Failed%s", strings.Title(verb))
		msg := fmt.Sprintf("%s RedisCluster %s failed error: %s",
			strings.ToLower(verb), rcName, err)
		rrc.recorder.Event(rc, corev1.EventTypeWarning, reason, msg)
	}
}
