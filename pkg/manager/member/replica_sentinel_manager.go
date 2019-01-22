package member

import (
	"github.com/golang/glog"
	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appslisters "k8s.io/client-go/listers/apps/v1beta1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/label"
	"github.com/anywhy/redis-operator/pkg/manager"
	"github.com/anywhy/redis-operator/pkg/util"
)

type sentinelMemberManager struct {
	setControl controller.StatefulSetControlInterface
	svcControl controller.ServiceControlInterface
	svcLister  corelisters.ServiceLister
	setLister  appslisters.StatefulSetLister
	podLister  corelisters.PodLister
}

// NewSentinelMemberManager new redis sentinel manager
func NewSentinelMemberManager(
	setControl controller.StatefulSetControlInterface,
	svcControl controller.ServiceControlInterface,
	svcLister corelisters.ServiceLister,
	podLister corelisters.PodLister,
	setLister appslisters.StatefulSetLister) manager.Manager {
	return &sentinelMemberManager{
		setControl: setControl,
		svcControl: svcControl,
		svcLister:  svcLister,
		podLister:  podLister,
		setLister:  setLister,
	}
}

// Sync	implements sentinel logic for syncing Redis.
func (smm *sentinelMemberManager) Sync(rc *v1alpha1.Redis) error {
	if rc.Spec.Mode == v1alpha1.ReplicaCluster {
		// Sync sentinel service
		if err := smm.syncSentinelServiceForRedis(rc); err != nil {
			return err
		}
		return smm.syncSentinelStatefulSetForRedis(rc)
	}
	return nil
}

func (smm *sentinelMemberManager) syncSentinelStatefulSetForRedis(rc *v1alpha1.Redis) error {
	ns, rcName := rc.Namespace, rc.Name

	newSentiSet, err := smm.getNewSentinelStatefulSet(rc)
	if err != nil {
		return err
	}
	oldSentiSet, err := smm.setLister.StatefulSets(ns).Get(controller.SentinelMemberName(rcName))
	if apierrors.IsNotFound(err) {
		err := setStatefulSetLastAppliedConfigAnnotation(newSentiSet)
		if err != nil {
			return err
		}

		err = smm.setControl.CreateStatefulSet(rc, newSentiSet)
		if err != nil {
			return err
		}
		rc.Status.Sentinel.StatefulSet = &apps.StatefulSetStatus{}
		return controller.RequeueErrorf("Redis: [%s/%s], waiting for replica sentinel running", ns, rcName)
	}

	if err = smm.syncRedisStatus(rc, oldSentiSet); err != nil {
		glog.Errorf("failed to sync Redis: [%s/%s]'s status, error: %v", ns, rcName, err)
	}

	// stop sentinel
	if rc.ReplicaUpgrading() {
		replicas := int32(0)
		set := *oldSentiSet
		set.Spec.Replicas = &replicas
		if err := setStatefulSetLastAppliedConfigAnnotation(&set); err != nil {
			return err
		}

		_, err = smm.setControl.UpdateStatefulSet(rc, &set)
		return err
	}

	// update
	if !statefulSetEqual(*newSentiSet, *oldSentiSet) {
		set := *oldSentiSet
		set.Spec.Template = newSentiSet.Spec.Template
		*set.Spec.Replicas = *newSentiSet.Spec.Replicas
		set.Spec.UpdateStrategy = newSentiSet.Spec.UpdateStrategy
		if err := setStatefulSetLastAppliedConfigAnnotation(&set); err != nil {
			return err
		}

		_, err = smm.setControl.UpdateStatefulSet(rc, &set)
		return err
	}

	return nil
}

func (smm *sentinelMemberManager) syncSentinelServiceForRedis(rc *v1alpha1.Redis) error {
	svcList := []ServiceConfig{
		// {
		// 	Name:       "sentinel",
		// 	Port:       16379,
		// 	SvcLabel:   func(l label.Label) label.Label { return l.Sentinel() },
		// 	MemberName: controller.SentinelMemberName,
		// 	Headless:   false,
		// },
		{
			Name:       "peer",
			Port:       16379,
			SvcLabel:   func(l label.Label) label.Label { return l.Sentinel() },
			MemberName: controller.SentinelPeerMemberName,
			Headless:   true,
		},
	}

	for _, svc := range svcList {
		if err := smm.syncSentinelService(rc, svc); err != nil {
			return err
		}
	}

	return nil
}

func (smm *sentinelMemberManager) syncSentinelService(rc *v1alpha1.Redis, svcConfig ServiceConfig) error {
	ns, rcName := rc.GetNamespace(), rc.GetName()

	newSvc := smm.getNewSentinelServiceForRedis(rc, svcConfig)
	oldSvc, err := smm.svcLister.Services(ns).Get(svcConfig.MemberName(rcName))
	if err != nil {
		if apierrors.IsNotFound(err) {
			err := setServiceLastAppliedConfigAnnotation(newSvc)
			if err != nil {
				return err
			}
			return smm.svcControl.CreateService(rc, newSvc)
		}
		return err
	}

	if ok, err := serviceEqual(newSvc, oldSvc); err != nil {
		return err
	} else if !ok {
		svc := *oldSvc
		svc.Spec = newSvc.Spec
		svc.Spec.ClusterIP = oldSvc.Spec.ClusterIP
		if err = setServiceLastAppliedConfigAnnotation(&svc); err != nil {
			return err
		}

		_, err := smm.svcControl.UpdateService(rc, &svc)
		return err
	}

	return nil
}

func (smm *sentinelMemberManager) syncRedisStatus(rc *v1alpha1.Redis, set *apps.StatefulSet) error {
	rc.Status.Sentinel.StatefulSet = &set.Status
	upgrading, err := smm.sentinelIsUpgrading(set, rc)
	if err != nil {
		return err
	}
	if upgrading {
		rc.Status.Sentinel.Phase = v1alpha1.UpgradePhase
	} else {
		rc.Status.Sentinel.Phase = v1alpha1.NormalPhase
	}

	return nil
}

func (smm *sentinelMemberManager) getNewSentinelServiceForRedis(rc *v1alpha1.Redis, svcConfig ServiceConfig) *corev1.Service {
	ns, rcName := rc.Namespace, rc.Name

	svcName := svcConfig.MemberName(rcName)
	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	rediLabel := svcConfig.SvcLabel(label.New().Instance(instanceName)).Labels()
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            svcName,
			Namespace:       ns,
			Labels:          rediLabel,
			OwnerReferences: []metav1.OwnerReference{controller.GetOwnerRef(rc)},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       svcConfig.Name,
					Port:       svcConfig.Port,
					TargetPort: intstr.FromInt(int(svcConfig.Port)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: rediLabel,
		},
	}

	if svcConfig.Headless {
		svc.Spec.ClusterIP = "None"
	} else {
		svc.Spec.Type = controller.GetServiceType(rc.Spec.Services, v1alpha1.SentinelMemberType.String())
	}

	return svc
}

func (smm *sentinelMemberManager) sentinelIsUpgrading(set *apps.StatefulSet, rc *v1alpha1.Redis) (bool, error) {
	if statefulSetIsUpgrading(set) {
		return true, nil
	}

	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	selector, err := label.New().Instance(instanceName).Sentinel().Selector()
	if err != nil {
		return false, err
	}
	sentiPods, err := smm.podLister.Pods(rc.GetNamespace()).List(selector)
	if err != nil {
		return false, err
	}
	for _, pod := range sentiPods {
		revisionHash, exist := pod.Labels[apps.ControllerRevisionHashLabelKey]
		if !exist {
			return false, nil
		}
		if revisionHash != rc.Status.Sentinel.StatefulSet.UpdateRevision {
			return true, nil
		}
	}

	return false, nil
}

const sentinelCmd = `
redis-server /etc/redis/sentinel.conf --sentinel
`

func (smm *sentinelMemberManager) getNewSentinelStatefulSet(rc *v1alpha1.Redis) (*apps.StatefulSet, error) {
	ns, rcName := rc.GetNamespace(), rc.GetName()
	redisConfigMap := controller.RedisMemberName(rcName)

	podMount, podVolume := podinfoVolume()
	volMounts := []corev1.VolumeMount{
		podMount,
		{Name: v1alpha1.SentinelMemberType.String(), MountPath: "/data"},
		{Name: "configmap", MountPath: "/configmap"},
		{Name: "sentinelconfig", MountPath: "/etc/redis"},
	}
	vols := []corev1.Volume{
		podVolume,
		{Name: "configmap",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: redisConfigMap,
					},
					Items: []corev1.KeyToPath{{Key: "sentinel-config-file", Path: "sentinel.conf"}},
				},
			},
		},
		{
			Name: "sentinelconfig",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	sentiLabel := label.New().Instance(instanceName).Sentinel()
	setName := controller.SentinelMemberName(rcName)
	storageClassName := rc.Spec.Redis.StorageClassName
	if storageClassName == "" {
		storageClassName = controller.DefaultStorageClassName
	}

	pvc, err := smm.volumeClaimTemplate(rc, &storageClassName)
	if err != nil {
		return nil, err
	}

	sentiSet := &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      setName,
			Labels:    sentiLabel.Labels(),
			OwnerReferences: []metav1.OwnerReference{
				controller.GetOwnerRef(rc),
			},
		},
		Spec: apps.StatefulSetSpec{
			Replicas: func() *int32 { r := rc.Spec.Sentinel.Replicas; return &r }(),
			Selector: sentiLabel.LabelSelector(),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      sentiLabel.Labels(),
					Annotations: controller.AnnProm(2379),
				},
				Spec: corev1.PodSpec{
					Affinity: util.AffinityForNodeSelector(ns,
						true, sentiLabel.Labels(),
						rc.Spec.Redis.NodeSelector),
					InitContainers: []corev1.Container{
						{
							Name:  "copy-config",
							Image: "busybox:latest",
							Command: []string{
								"sh", "-c", "cp /configmap/* /etc/redis",
							},
							VolumeMounts: volMounts,
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "redis-sentinel",
							Image:           rc.Spec.Redis.Image,
							ImagePullPolicy: rc.Spec.Redis.ImagePullPolicy,
							Command: []string{
								"bash", "-c", sentinelCmd,
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "sentinel",
									ContainerPort: int32(16379),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: volMounts,
							Resources: util.ResourceRequirement(v1alpha1.ContainerSpec{
								Requests: rc.Spec.Sentinel.Requests,
								Limits:   rc.Spec.Sentinel.Limits,
							}),
							Env: []corev1.EnvVar{
								{
									Name:  "CLUSTER_NAME",
									Value: rc.GetName(),
								},
							},
						},
					},
					Volumes:       vols,
					RestartPolicy: corev1.RestartPolicyAlways,
					Tolerations:   rc.Spec.Sentinel.Tolerations,
				},
			},
			ServiceName:         controller.SentinelPeerMemberName(rcName),
			PodManagementPolicy: apps.ParallelPodManagement,
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				pvc,
			},
		},
	}
	return sentiSet, nil
}

func (smm *sentinelMemberManager) volumeClaimTemplate(rc *v1alpha1.Redis, storageClassName *string) (corev1.PersistentVolumeClaim, error) {
	var pvc corev1.PersistentVolumeClaim

	size, _ := resource.ParseQuantity("10Mi")
	pvc = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: v1alpha1.SentinelMemberType.String()},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			StorageClassName: storageClassName,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
				Limits: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		},
	}

	return pvc, nil
}

// FakeSentinelMemberManager replica cluster member manager fake use test
type FakeSentinelMemberManager struct {
	err error
}

// NewFakeSentinelMemberManager new fake instance
func NewFakeSentinelMemberManager() *FakeSentinelMemberManager {
	return &FakeSentinelMemberManager{}
}

// SetSyncError sync err
func (fsmm *FakeSentinelMemberManager) SetSyncError(err error) {
	fsmm.err = err
}

// Sync sync info
func (fsmm *FakeSentinelMemberManager) Sync(_ *v1alpha1.Redis) error {
	if fsmm.err != nil {
		return fsmm.err
	}
	return nil
}
