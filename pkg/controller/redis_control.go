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

// RedisControlInterface manages Redises
type RedisControlInterface interface {
	UpdateRedis(rc *v1alpha1.Redis, newStatus *v1alpha1.RedisStatus,
		oldStatus *v1alpha1.RedisStatus) (*v1alpha1.Redis, error)
}

type realRedisControl struct {
	cli      versioned.Interface
	rcLister listers.RedisLister
	recorder record.EventRecorder
}

// NewRealRedisControl creates a new RedisControlInterface
func NewRealRedisControl(cli versioned.Interface,
	rcLister listers.RedisLister,
	recorder record.EventRecorder) RedisControlInterface {
	return &realRedisControl{
		cli,
		rcLister,
		recorder,
	}
}

func (rrc *realRedisControl) UpdateRedis(rc *v1alpha1.Redis,
	newStatus *v1alpha1.RedisStatus, oldStatus *v1alpha1.RedisStatus) (*v1alpha1.Redis, error) {
	ns, rcName := rc.GetNamespace(), rc.GetName()
	status := rc.Status.DeepCopy()
	var updateRC *v1alpha1.Redis

	// don't wait due to limited number of clients, but backoff after the default number of steps
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var updateErr error
		updateRC, updateErr = rrc.cli.Redis().Redises(ns).Update(rc)
		if updateErr == nil {
			glog.Infof("Redis: [%s/%s] updated successfully", ns, rcName)
			return nil
		}
		glog.Errorf("failed to update Redis: [%s/%s], error: %v", ns, rcName, updateErr)

		if updated, err := rrc.rcLister.Redises(ns).Get(rcName); err == nil {
			// make a copy so we don't mutate the shared cache
			rc = updated.DeepCopy()
			rc.Status = *status
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated Redis %s/%s from lister: %v", ns, rcName, err))
		}

		return updateErr
	})
	if !apiequality.Semantic.DeepEqual(newStatus, oldStatus) {
		rrc.recordRedisEvent("update", rc, err)
	}
	return updateRC, err
}

func (rrc *realRedisControl) recordRedisEvent(verb string, rc *v1alpha1.Redis, err error) {
	rcName := rc.GetName()
	if err == nil {
		reason := fmt.Sprintf("Successful%s", strings.Title(verb))
		msg := fmt.Sprintf("%s Redis %s successful",
			strings.ToLower(verb), rcName)
		rrc.recorder.Event(rc, corev1.EventTypeNormal, reason, msg)
	} else {
		reason := fmt.Sprintf("Failed%s", strings.Title(verb))
		msg := fmt.Sprintf("%s Redis %s failed error: %s",
			strings.ToLower(verb), rcName, err)
		rrc.recorder.Event(rc, corev1.EventTypeWarning, reason, msg)
	}
}
