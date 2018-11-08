package controller

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
)

// ServiceControlInterface manages Services used in TidbCluster
type ServiceControlInterface interface {
	CreateService(*v1alpha1.RedisCluster, *corev1.Service) error
	UpdateService(*v1alpha1.RedisCluster, *corev1.Service) (*corev1.Service, error)
	DeleteService(*v1alpha1.RedisCluster, *corev1.Service) error
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

func (sc *realServiceControl) CreateService(rc *v1alpha1.RedisCluster, svc *corev1.Service) error {
	rcSvc, err := sc.kubeCli.CoreV1().Services(rc.Namespace).Create(svc)
	if apierrors.IsAlreadyExists(err) {
		if !metav1.IsControlledBy(rcSvc, rc) {
			err := fmt.Errorf(MessageResourceExists, "Service", rcSvc.Namespace, rcSvc.Name)
			sc.recordServiceEvent("create", rc, rcSvc, err)
		}
		return nil
	}
	sc.recordServiceEvent("create", rc, svc, err)
	return err
}

func (sc *realServiceControl) UpdateService(rc *v1alpha1.RedisCluster, svc *corev1.Service) (*corev1.Service, error) {
	ns := rc.GetNamespace()
	rcName := rc.GetName()
	svcName := svc.GetName()
	svcSpec := svc.Spec.DeepCopy()

	var updateSvc *corev1.Service
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var updateErr error
		updateSvc, updateErr = sc.kubeCli.CoreV1().Services(ns).Update(svc)
		if updateErr == nil {
			glog.Infof("update Service: [%s/%s] successfully, RedisCluster: %s", ns, svcName, rcName)
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

func (sc *realServiceControl) DeleteService(rc *v1alpha1.RedisCluster, svc *corev1.Service) error {
	err := sc.kubeCli.CoreV1().Services(rc.Namespace).Delete(svc.Name, nil)
	sc.recordServiceEvent("delete", rc, svc, err)
	return err
}

func (sc *realServiceControl) recordServiceEvent(verb string, rc *v1alpha1.RedisCluster, svc *corev1.Service, err error) {
	rcName := rc.Name
	svcName := svc.Name
	if err == nil {
		reason := fmt.Sprintf("Successful%s", strings.Title(verb))
		msg := fmt.Sprintf("%s Service %s in RedisCluster %s successful",
			strings.ToLower(verb), svcName, rcName)
		sc.recorder.Event(rc, corev1.EventTypeNormal, reason, msg)
	} else {
		reason := fmt.Sprintf("Failed%s", strings.Title(verb))
		msg := fmt.Sprintf("%s Service %s in RedisCluster %s failed error: %s",
			strings.ToLower(verb), svcName, rcName, err)
		sc.recorder.Event(rc, corev1.EventTypeWarning, reason, msg)
	}
}

var _ ServiceControlInterface = &realServiceControl{}
