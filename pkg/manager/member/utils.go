package member

import (
	"github.com/golang/glog"
	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/json"
)

const (
	// LastAppliedConfigAnnotation is annotation key of last applied configuration
	LastAppliedConfigAnnotation = "anywhy.github/last-applied-configuration"
	// ImagePullBackOff is the pod state of image pull failed
	ImagePullBackOff = "ImagePullBackOff"
	// ErrImagePull is the pod state of image pull failed
	ErrImagePull = "ErrImagePull"
)

// setServiceLastAppliedConfigAnnotation set last applied config info to Service's annotation
func setServiceLastAppliedConfigAnnotation(svc *corev1.Service) error {
	svcApply, err := encode(svc.Spec)
	if err != nil {
		return err
	}
	if svc.Annotations == nil {
		svc.Annotations = map[string]string{}
	}
	svc.Annotations[LastAppliedConfigAnnotation] = svcApply
	return nil
}

// serviceEqual compares the new Service's spec with old Service's last applied config
func serviceEqual(new, old *corev1.Service) (bool, error) {
	oldSpec := corev1.ServiceSpec{}
	if lastAppliedConfig, ok := old.Annotations[LastAppliedConfigAnnotation]; ok {
		err := json.Unmarshal([]byte(lastAppliedConfig), &oldSpec)
		if err != nil {
			glog.Errorf("unmarshal ServiceSpec: [%s/%s]'s applied config failed,error: %v", old.GetNamespace(), old.GetName(), err)
			return false, err
		}
		return apiequality.Semantic.DeepEqual(oldSpec, new.Spec), nil
	}
	return false, nil
}

func encode(obj interface{}) (string, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// setDeploymentLastAppliedConfigAnnotation set last applied config info to Deployment's annotation
func setDeploymentLastAppliedConfigAnnotation(dep *extv1.Deployment) error {
	setApply, err := encode(dep.Spec)
	if err != nil {
		return err
	}
	if dep.Annotations == nil {
		dep.Annotations = map[string]string{}
	}
	dep.Annotations[LastAppliedConfigAnnotation] = setApply

	templateApply, err := encode(dep.Spec.Template.Spec)
	if err != nil {
		return err
	}
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}
	dep.Spec.Template.Annotations[LastAppliedConfigAnnotation] = templateApply
	return nil
}

// templateEqual compares the new podTemplateSpec's spec with old podTemplateSpec's last applied config
func templateEqual(new corev1.PodTemplateSpec, old corev1.PodTemplateSpec) bool {
	oldConfig := corev1.PodSpec{}
	if lastAppliedConfig, ok := old.Annotations[LastAppliedConfigAnnotation]; ok {
		err := json.Unmarshal([]byte(lastAppliedConfig), &oldConfig)
		if err != nil {
			glog.Errorf("unmarshal PodTemplate: [%s/%s]'s applied config failed,error: %v", old.GetNamespace(), old.GetName(), err)
			return false
		}
		return apiequality.Semantic.DeepEqual(oldConfig, new.Spec)
	}
	return false
}

// statefulSetIsUpgrading confirms whether the statefulSet is upgrading phase
func statefulSetIsUpgrading(set *apps.StatefulSet) bool {
	if set.Status.ObservedGeneration == nil {
		return false
	}
	if set.Status.CurrentRevision != set.Status.UpdateRevision {
		return true
	}
	if set.Generation > *set.Status.ObservedGeneration && *set.Spec.Replicas == set.Status.Replicas {
		return true
	}
	return false
}

// deploymentIsUpgrading confirms whether the deployment is upgrading phase
func deploymentIsUpgrading(dep *extv1.Deployment) bool {
	if &dep.Status.ObservedGeneration == nil {
		return false
	}
	if dep.Status.UpdatedReplicas > 0 {
		return true
	}
	if dep.Generation > dep.Status.ObservedGeneration && *dep.Spec.Replicas == dep.Status.Replicas {
		return true
	}
	return false
}
