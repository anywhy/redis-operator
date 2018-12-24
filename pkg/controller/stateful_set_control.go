package controller

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1beta1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
)

// StatefulSetControlInterface defines the interface that uses to create, update, and delete StatefulSets,
type StatefulSetControlInterface interface {
	// CreateStatefulSet creates a StatefulSet in a Redis.
	CreateStatefulSet(*v1alpha1.Redis, *apps.StatefulSet) error
	// UpdateStatefulSet updates a StatefulSet in a Redis.
	UpdateStatefulSet(*v1alpha1.Redis, *apps.StatefulSet) (*apps.StatefulSet, error)
	// DeleteStatefulSet deletes a StatefulSet in a Redis.
	DeleteStatefulSet(*v1alpha1.Redis, *apps.StatefulSet) error
}

type realStatefulSetControl struct {
	kubeCli   kubernetes.Interface
	setLister appslisters.StatefulSetLister
	recorder  record.EventRecorder
}

// NewRealStatefuSetControl returns a StatefulSetControlInterface
func NewRealStatefuSetControl(kubeCli kubernetes.Interface, setLister appslisters.StatefulSetLister,
	recorder record.EventRecorder) StatefulSetControlInterface {
	return &realStatefulSetControl{kubeCli, setLister, recorder}
}

// CreateStatefulSet create a StatefulSet in a Redis.
func (sc *realStatefulSetControl) CreateStatefulSet(rc *v1alpha1.Redis, set *apps.StatefulSet) error {
	newSS, err := sc.kubeCli.AppsV1beta1().StatefulSets(rc.Namespace).Create(set)
	// sink already exists errors
	if apierrors.IsAlreadyExists(err) {
		if !metav1.IsControlledBy(set, rc) {
			err := fmt.Errorf(MessageResourceExists, "StatefulSet", newSS.Namespace, newSS.Name)
			sc.recordStatefulSetEvent("create", rc, newSS, err)
		}
		return nil
	}
	sc.recordStatefulSetEvent("create", rc, set, err)
	return err
}

// UpdateStatefulSet update a StatefulSet in a Redis.
func (sc *realStatefulSetControl) UpdateStatefulSet(rc *v1alpha1.Redis, set *apps.StatefulSet) (*apps.StatefulSet, error) {
	ns := rc.GetNamespace()
	rcName := rc.GetName()
	setName := set.GetName()
	setSpec := set.Spec.DeepCopy()
	var updatedSS *apps.StatefulSet

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var updateErr error
		updatedSS, updateErr = sc.kubeCli.AppsV1beta1().StatefulSets(ns).Update(set)
		if updateErr == nil {
			glog.Infof("Redis: [%s/%s]'s StatefulSet: [%s/%s] updated successfully", ns, rcName, ns, setName)
			return nil
		}
		glog.Errorf("failed to update Redis: [%s/%s]'s StatefulSet: [%s/%s], error: %v", ns, rcName, ns, setName, updateErr)

		if updated, err := sc.setLister.StatefulSets(ns).Get(setName); err == nil {
			// make a copy so we don't mutate the shared cache
			set = updated.DeepCopy()
			set.Spec = *setSpec
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated StatefulSet %s/%s from lister: %v", ns, setName, err))
		}
		return updateErr
	})

	sc.recordStatefulSetEvent("update", rc, set, err)
	return updatedSS, err
}

// DeleteStatefulSet delete a StatefulSet in a Redis.
func (sc *realStatefulSetControl) DeleteStatefulSet(rc *v1alpha1.Redis, set *apps.StatefulSet) error {
	err := sc.kubeCli.AppsV1beta1().StatefulSets(rc.Namespace).Delete(set.Name, nil)
	sc.recordStatefulSetEvent("delete", rc, set, err)
	return err
}

func (sc *realStatefulSetControl) recordStatefulSetEvent(verb string, rc *v1alpha1.Redis, set *apps.StatefulSet, err error) {
	rcName := rc.Name
	setName := set.Name
	if err == nil {
		reason := fmt.Sprintf("Successful%s", strings.Title(verb))
		message := fmt.Sprintf("%s StatefulSet %s in Redis %s successful",
			strings.ToLower(verb), setName, rcName)
		sc.recorder.Event(rc, corev1.EventTypeNormal, reason, message)
	} else {
		reason := fmt.Sprintf("Failed%s", strings.Title(verb))
		message := fmt.Sprintf("%s StatefulSet %s in Redis %s failed error: %s",
			strings.ToLower(verb), setName, rcName, err)
		sc.recorder.Event(rc, corev1.EventTypeWarning, reason, message)
	}
}

var _ StatefulSetControlInterface = &realStatefulSetControl{}
