package meta

import (
	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/label"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newRedisClusterForMeta() *v1alpha1.Redis {
	return &v1alpha1.Redis{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Redis",
			APIVersion: "anywhy.github.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      controller.TestClusterName,
			Namespace: corev1.NamespaceDefault,
			UID:       types.UID("test"),
			Labels:    label.New().Instance(controller.TestClusterName),
		},
		Spec: v1alpha1.RedisSpec{
			Mode:            v1alpha1.ReplicaCluster,
			PVReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
		},
	}
}

func newPV() *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pv-1",
			Namespace: "",
			UID:       types.UID("test"),
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			ClaimRef: &corev1.ObjectReference{
				APIVersion: "v1",
				Kind:       "PersistentVolumeClaim",
				Name:       "pvc-1",
				Namespace:  corev1.NamespaceDefault,
				UID:        types.UID("test"),
			},
		},
	}
}

func newPVC(rc *v1alpha1.Redis) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-1",
			Namespace: corev1.NamespaceDefault,
			UID:       types.UID("test"),
			Labels: map[string]string{
				label.NameLabelKey:      controller.TestName,
				label.ComponentLabelKey: controller.TestComponentName,
				label.ManagedByLabelKey: controller.TestManagedByName,
				label.InstanceLabelKey:  rc.GetName(),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "pv-1",
		},
	}
}
