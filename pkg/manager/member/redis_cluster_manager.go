package member

import (
	"fmt"

	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/label"
	"github.com/anywhy/redis-operator/pkg/util"
)

type redisClusterMemeberManager struct {
}

func (rcm *redisClusterMemeberManager) getNewRedisClusterStatefulSet(rc *v1alpha1.RedisCluster) (*apps.StatefulSet, error) {
	ns, rcName := rc.GetNamespace(), rc.GetName()
	redisConfigMap := controller.RedisMemberName(rcName)

	podMount, podVolume := podinfoVolume()
	volMounts := []corev1.VolumeMount{
		podMount,
		{Name: "config", ReadOnly: true, MountPath: "/etc/redis"},
		{Name: "startup-script", ReadOnly: true, MountPath: "/usr/local/sbin"},
		{Name: v1alpha1.RedisMemberType.String(), MountPath: "/data"},
	}
	vols := []corev1.Volume{
		podVolume,
		{Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: redisConfigMap,
					},
					Items: []corev1.KeyToPath{{Key: "config-file", Path: "redis.conf"}},
				},
			},
		},
		{Name: "startup-script",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: redisConfigMap,
					},
					Items: []corev1.KeyToPath{{Key: "startup-script", Path: "server_start_script.sh"}},
				},
			},
		},
	}

	setName := controller.RedisMemberName(rcName)

	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	rediLabel := label.New().Instance(instanceName).Redis().RedisClusterMode()
	storageClassName := rc.Spec.Redis.StorageClassName
	if storageClassName == "" {
		storageClassName = controller.DefaultStorageClassName
	}

	pvc, err := rcm.volumeClaimTemplate(rc, &storageClassName)
	if err != nil {
		return nil, err
	}

	dnsPolicy := corev1.DNSClusterFirst // same as k8s defaults
	if rc.Spec.Redis.HostNetwork {
		dnsPolicy = corev1.DNSClusterFirstWithHostNet
	}

	rediSet := &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      setName,
			Labels:    rediLabel.Labels(),
			OwnerReferences: []metav1.OwnerReference{
				controller.GetOwnerRef(rc),
			},
		},
		Spec: apps.StatefulSetSpec{
			Replicas: func() *int32 { r := rc.Spec.Redis.Replicas; return &r }(),
			Selector: rediLabel.LabelSelector(),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      rediLabel.Labels(),
					Annotations: controller.AnnProm(2379),
				},
				Spec: corev1.PodSpec{
					SchedulerName: rc.Spec.SchedulerName,
					Affinity:      rc.Spec.Redis.Affinity,
					NodeSelector:  rc.Spec.Redis.NodeSelector,
					HostNetwork:   rc.Spec.Redis.HostNetwork,
					DNSPolicy:     dnsPolicy,
					Containers: []corev1.Container{
						{
							Name:            "redis",
							Image:           rc.Spec.Redis.Image,
							ImagePullPolicy: rc.Spec.Redis.ImagePullPolicy,
							Command:         []string{"/bin/bash", "/usr/local/sbin/server_start_script.sh"},
							Ports: []corev1.ContainerPort{
								{
									Name:          "redis",
									ContainerPort: int32(6379),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: volMounts,
							Resources:    util.ResourceRequirement(rc.Spec.Redis.ContainerSpec),
							Env: []corev1.EnvVar{
								{
									Name: "NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name:  "CLUSTER_NAME",
									Value: rc.GetName(),
								},
							},
						},
					},
					Volumes:       vols,
					RestartPolicy: corev1.RestartPolicyAlways,
					Tolerations:   rc.Spec.Redis.Tolerations,
				},
			},
			ServiceName:         controller.RedisMemberName(rcName),
			PodManagementPolicy: apps.ParallelPodManagement,
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				pvc,
			},
			UpdateStrategy: apps.StatefulSetUpdateStrategy{
				Type: apps.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &apps.RollingUpdateStatefulSetStrategy{
					Partition: controller.Int32Ptr(rc.RedisRealReplicas()),
				},
			},
		},
	}
	return rediSet, nil
}

func (rcm *redisClusterMemeberManager) volumeClaimTemplate(rc *v1alpha1.RedisCluster, storageClassName *string) (corev1.PersistentVolumeClaim, error) {
	var q, limit resource.Quantity
	var err error

	ns, rcName := rc.GetNamespace(), rc.GetName()
	var pvc corev1.PersistentVolumeClaim

	if rc.Spec.Redis.Requests != nil && rc.Spec.Redis.Requests.Storage != "" {
		size := rc.Spec.Redis.Requests.Storage
		q, err = resource.ParseQuantity(size)
		if err != nil {
			return pvc, fmt.Errorf("cant' get storage request size: %s for Redis: %s/%s", size, ns, rcName)
		}
	}

	if rc.Spec.Redis.Limits != nil && rc.Spec.Redis.Limits.Storage != "" {
		size := rc.Spec.Redis.Limits.Storage
		limit, err = resource.ParseQuantity(size)
		if err != nil {
			return pvc, fmt.Errorf("cant' get storage limit size: %s for Redis: %s/%s, %v", size, ns, rcName, err)
		}
	}

	pvc = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: v1alpha1.RedisMemberType.String()},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			StorageClassName: storageClassName,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: q,
				},
				Limits: corev1.ResourceList{
					corev1.ResourceStorage: limit,
				},
			},
		},
	}

	return pvc, nil
}

// FakeRedisClusterMemberManager cluster member manager fake use test
type FakeRedisClusterMemberManager struct {
	err error
}

// NewFakeRedisClusterMemberManager new fake instance
func NewFakeRedisClusterMemberManager() *FakeRedisClusterMemberManager {
	return &FakeRedisClusterMemberManager{}
}

// SetSyncError sync err
func (frmm *FakeRedisClusterMemberManager) SetSyncError(err error) {
	frmm.err = err
}

// Sync sync info
func (frmm *FakeRedisClusterMemberManager) Sync(_ *v1alpha1.RedisCluster) error {
	if frmm.err != nil {
		return frmm.err
	}
	return nil
}
