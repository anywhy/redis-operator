package controller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
)

const (
	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "%s %s/%s  already exists and is not managed by RedisCluster"
)

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = v1alpha1.SchemeGroupVersion.WithKind("RedisCluster")
	// DefaultStorageClassName is the default storageClassName
	DefaultStorageClassName string
)

// GetOwnerRef returns RedisCluster's OwnerReference
func GetOwnerRef(rc *v1alpha1.RedisCluster) metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return metav1.OwnerReference{
		APIVersion:         controllerKind.GroupVersion().String(),
		Kind:               controllerKind.Kind,
		Name:               rc.GetName(),
		UID:                rc.GetUID(),
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}
}
