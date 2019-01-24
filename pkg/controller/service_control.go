package controller

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	rcinformers "github.com/anywhy/redis-operator/pkg/client/informers/externalversions/redis/v1alpha1"
	v1listers "github.com/anywhy/redis-operator/pkg/client/listers/redis/v1alpha1"
)

// ServiceControlInterface manages Services used in Redis
type ServiceControlInterface interface {
	CreateService(*v1alpha1.Redis, *corev1.Service) error
	UpdateService(*v1alpha1.Redis, *corev1.Service) (*corev1.Service, error)
	DeleteService(*v1alpha1.Redis, *corev1.Service) error
}

type realServiceControl struct {
	kubeCli   kubernetes.Interface
	svcLister corelisters.ServiceLister
	recorder  record.EventRecorder
}

// NewRealServiceControl creates a new ServiceControlInterface
func NewRealServiceControl(kubeCli kubernetes.Interface, svcLister corelisters.ServiceLister,
	recorder record.EventRecorder) ServiceControlInterface {
	return &realServiceControl{
		kubeCli,
		svcLister,
		recorder,
	}
}

// CreateService create a Service in a Redis.
func (sc *realServiceControl) CreateService(rc *v1alpha1.Redis, svc *corev1.Service) error {
	_, err := sc.kubeCli.CoreV1().Services(rc.Namespace).Create(svc)
	if apierrors.IsAlreadyExists(err) {
		if !metav1.IsControlledBy(svc, rc) {
			err := fmt.Errorf(MessageResourceExists, "Service", svc.Namespace, svc.Name)
			sc.recordServiceEvent("create", rc, svc, err)
		}
		return nil
	}
	sc.recordServiceEvent("create", rc, svc, err)
	return err
}

// UpdateService update a Service in a Redis.
func (sc *realServiceControl) UpdateService(rc *v1alpha1.Redis, svc *corev1.Service) (*corev1.Service, error) {
	ns := rc.GetNamespace()
	rcName := rc.GetName()
	svcName := svc.GetName()
	svcSpec := svc.Spec.DeepCopy()

	var updateSvc *corev1.Service
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var updateErr error
		updateSvc, updateErr = sc.kubeCli.CoreV1().Services(ns).Update(svc)
		if updateErr == nil {
			glog.Infof("update Service: [%s/%s] successfully, Redis: %s", ns, svcName, rcName)
			return nil
		}

		if updated, err := sc.svcLister.Services(rc.Namespace).Get(svcName); err != nil {
			svc = updated.DeepCopy()
			svc.Spec = *svcSpec
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated Service %s/%s from lister: %v", ns, svcName, err))
		}

		return updateErr
	})
	sc.recordServiceEvent("update", rc, svc, err)
	return updateSvc, err
}

// DeleteService delete a Service in a Redis.
func (sc *realServiceControl) DeleteService(rc *v1alpha1.Redis, svc *corev1.Service) error {
	err := sc.kubeCli.CoreV1().Services(rc.Namespace).Delete(svc.Name, nil)
	sc.recordServiceEvent("delete", rc, svc, err)
	return err
}

// recordServiceEvent records an event for verb applied to a service in a Redis. If err is nil the generated event will
// have a reason of v1.EventTypeNormal. If err is not nil the generated event will have a reason of v1.EventTypeWarning.
func (sc *realServiceControl) recordServiceEvent(verb string, rc *v1alpha1.Redis, svc *corev1.Service, err error) {
	rcName := rc.Name
	svcName := svc.Name
	if err == nil {
		reason := fmt.Sprintf("Successful%s", strings.Title(verb))
		msg := fmt.Sprintf("%s Service %s in Redis %s successful",
			strings.ToLower(verb), svcName, rcName)
		sc.recorder.Event(rc, corev1.EventTypeNormal, reason, msg)
	} else {
		reason := fmt.Sprintf("Failed%s", strings.Title(verb))
		msg := fmt.Sprintf("%s Service %s in Redis %s failed error: %s",
			strings.ToLower(verb), svcName, rcName, err)
		sc.recorder.Event(rc, corev1.EventTypeWarning, reason, msg)
	}
}

var _ ServiceControlInterface = &realServiceControl{}

// FakeServiceControl is a fake ServiceControlInterface
type FakeServiceControl struct {
	SvcLister                corelisters.ServiceLister
	SvcIndexer               cache.Indexer
	RcLister                 v1listers.RedisLister
	rcIndexer                cache.Indexer
	createServiceTracker     requestTracker
	updateServiceTracker     requestTracker
	deleteStatefulSetTracker requestTracker
}

// NewFakeServiceControl returns a FakeServiceControl
func NewFakeServiceControl(svcInformer coreinformers.ServiceInformer, rcInformer rcinformers.RedisInformer) *FakeServiceControl {
	return &FakeServiceControl{
		svcInformer.Lister(),
		svcInformer.Informer().GetIndexer(),
		rcInformer.Lister(),
		rcInformer.Informer().GetIndexer(),
		requestTracker{0, nil, 0},
		requestTracker{0, nil, 0},
		requestTracker{0, nil, 0},
	}
}

// SetCreateServiceError sets the error attributes of createServiceTracker
func (ssc *FakeServiceControl) SetCreateServiceError(err error, after int) {
	ssc.createServiceTracker.err = err
	ssc.createServiceTracker.after = after
}

// SetUpdateServiceError sets the error attributes of updateServiceTracker
func (ssc *FakeServiceControl) SetUpdateServiceError(err error, after int) {
	ssc.updateServiceTracker.err = err
	ssc.updateServiceTracker.after = after
}

// SetDeleteServiceError sets the error attributes of deleteServiceTracker
func (ssc *FakeServiceControl) SetDeleteServiceError(err error, after int) {
	ssc.deleteStatefulSetTracker.err = err
	ssc.deleteStatefulSetTracker.after = after
}

// CreateService adds the service to SvcIndexer
func (ssc *FakeServiceControl) CreateService(_ *v1alpha1.Redis, svc *corev1.Service) error {
	defer ssc.createServiceTracker.inc()
	if ssc.createServiceTracker.errorReady() {
		defer ssc.createServiceTracker.reset()
		return ssc.createServiceTracker.err
	}

	return ssc.SvcIndexer.Add(svc)
}

// UpdateService updates the service of SvcIndexer
func (ssc *FakeServiceControl) UpdateService(_ *v1alpha1.Redis, svc *corev1.Service) (*corev1.Service, error) {
	defer ssc.updateServiceTracker.inc()
	if ssc.updateServiceTracker.errorReady() {
		defer ssc.updateServiceTracker.reset()
		return nil, ssc.updateServiceTracker.err
	}

	return svc, ssc.SvcIndexer.Update(svc)
}

// DeleteService deletes the service of SvcIndexer
func (ssc *FakeServiceControl) DeleteService(_ *v1alpha1.Redis, _ *corev1.Service) error {
	return nil
}

var _ ServiceControlInterface = &FakeServiceControl{}
