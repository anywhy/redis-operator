package controller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
)

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = v1alpha1.SchemeGroupVersion.WithKind("RedisCluster")
	// DefaultStorageClassName is the default storageClassName
	DefaultStorageClassName string
)

// GetOwnerRef returns RedisCluster's OwnerReference
func GetOwnerRef(redi *v1alpha1.Redis) metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return metav1.OwnerReference{
		APIVersion:         controllerKind.GroupVersion().String(),
		Kind:               controllerKind.Kind,
		Name:               redi.GetName(),
		UID:                redi.GetUID(),
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}
}
