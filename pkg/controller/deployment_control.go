package controller

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	v1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	extlisters "k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
)

// DeploymentControlInterface defines the interface that RedisController uses to create, update, and delete deploy,
type DeploymentControlInterface interface {
	CreateDeployment(*v1alpha1.RedisCluster, *extv1.Deployment) error
	UpdateDeployment(*v1alpha1.RedisCluster, *extv1.Deployment) (*extv1.Deployment, error)
	DeleteDeployment(*v1alpha1.RedisCluster, *extv1.Deployment) error
}

// NewDeploymentControl returns a DeploymentControlInterface
func NewDeploymentControl(kubeCli kubernetes.Interface,
	deployLister extlisters.DeploymentLister,
	recorder record.EventRecorder) DeploymentControlInterface {
	return &realDeploymentControl{
		kubeCli,
		deployLister,
		recorder,
	}
}

type realDeploymentControl struct {
	kubeCli      kubernetes.Interface
	deployLister extlisters.DeploymentLister
	recorder     record.EventRecorder
}

// CreateDeployment create a Deployment in a Redis.
func (rdc *realDeploymentControl) CreateDeployment(rc *v1alpha1.RedisCluster, dep *extv1.Deployment) error {
	newDep, err := rdc.kubeCli.ExtensionsV1beta1().Deployments(rc.Namespace).Create(dep)
	if apierrors.IsAlreadyExists(err) {
		if !metav1.IsControlledBy(newDep, rc) {
			err := fmt.Errorf(MessageResourceExists, "Deployment", newDep.Namespace, newDep.Name)
			rdc.recordDeploymentEvent("create", rc, newDep, err)
		}
		return nil
	}

	rdc.recordDeploymentEvent("create", rc, dep, err)
	return err
}

// UpdateDeployment update a Deployment in a Redis.
func (rdc *realDeploymentControl) UpdateDeployment(rc *v1alpha1.RedisCluster, dep *extv1.Deployment) (*extv1.Deployment, error) {
	ns := rc.GetNamespace()
	rcName := rc.GetName()
	depName := dep.GetName()
	depSpec := dep.Spec.DeepCopy()
	var updatedDep *extv1.Deployment

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var updateErr error
		updatedDep, updateErr = rdc.kubeCli.ExtensionsV1beta1().Deployments(ns).Update(dep)
		if updateErr == nil {
			glog.Infof("Redis: [%s/%s]'s Deployment: [%s/%s] updated successfully", ns, rcName, ns, depName)
			return nil
		}
		glog.Errorf("failed to update Redis: [%s/%s]'s Deployment: [%s/%s], error: %v", ns, rcName, ns, depName, updateErr)

		if updated, err := rdc.deployLister.Deployments(ns).Get(depName); err == nil {
			// make a copy so we don't mutate the shared cache
			dep = updated.DeepCopy()
			dep.Spec = *depSpec
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated Deployment %s/%s from lister: %v", ns, depName, err))
		}
		return updateErr
	})

	rdc.recordDeploymentEvent("update", rc, dep, err)
	return updatedDep, err
}

// DeleteDeployment delete a Deployment in a Redis.
func (rdc *realDeploymentControl) DeleteDeployment(rc *v1alpha1.RedisCluster, dep *extv1.Deployment) error {
	err := rdc.kubeCli.ExtensionsV1beta1().Deployments(rc.Namespace).Delete(dep.Name, nil)
	rdc.recordDeploymentEvent("delete", rc, dep, err)
	return err
}

// recordDeploymentEvent records an event for verb applied to a deployment in a Redis. If err is nil the generated event will
// have a reason of v1.EventTypeNormal. If err is not nil the generated event will have a reason of v1.EventTypeWarning.
func (rdc *realDeploymentControl) recordDeploymentEvent(verb string, rc *v1alpha1.RedisCluster, dep *extv1.Deployment, err error) {
	deployName := dep.GetName()
	if err == nil {
		reason := fmt.Sprintf("Successful%s", strings.Title(verb))
		message := fmt.Sprintf("%s Deployment %s in Redis %s successful",
			strings.ToLower(verb), deployName, rc.Name)
		rdc.recorder.Event(rc, v1.EventTypeNormal, reason, message)
	} else {
		reason := fmt.Sprintf("Failed%s", strings.Title(verb))
		message := fmt.Sprintf("%s Deployment %s in Redis %s failed error: %s",
			strings.ToLower(verb), deployName, rc.Name, err)
		rdc.recorder.Event(rc, v1.EventTypeWarning, reason, message)
	}
}

var _ DeploymentControlInterface = &realDeploymentControl{}
