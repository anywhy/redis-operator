package member

import (
	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// Sync	implements sentinel logic for syncing rediscluster.
func (smm *sentinelMemberManager) Sync(rc *v1alpha1.RedisCluster) error {
	// Sync sentinel service
	if err := smm.syncSentinelServiceForRedisCluster(rc); err != nil {
		return err
	}

	// Sync sentinel headless service
	if err := smm.syncSentinelHeadlessServiceForRedisCluster(rc); err != nil {
		return err
	}

	return smm.syncSentinelStatefulSetForRedisCluster(rc)
}

func (smm *sentinelMemberManager) syncSentinelStatefulSetForRedisCluster(rc *v1alpha1.RedisCluster) error {
	ns, rcName := rc.Namespace, rc.Name

	newSentiSet := smm.getNewSentinelStatefulSet(rc)
	oldSentiSet, err := smm.setLister.StatefulSets(ns).Get(controller.SentinelMemberName(rcName))
	if err != nil {
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
		}
		return err
	}

	err = smm.syncRedisClusterStatus(rc, oldSentiSet)
	if err != nil {
		return err
	}
	return nil
}

func (smm *sentinelMemberManager) syncSentinelServiceForRedisCluster(rc *v1alpha1.RedisCluster) error {
	return smm.syncSentinelService(rc, controller.SentinelMemberName(rc.Name))
}

func (smm *sentinelMemberManager) syncSentinelHeadlessServiceForRedisCluster(rc *v1alpha1.RedisCluster) error {
	return smm.syncSentinelService(rc, controller.SentinelPeerMemberName(rc.Name))
}

func (smm *sentinelMemberManager) syncSentinelService(rc *v1alpha1.RedisCluster, svcName string) error {
	svcNew := smm.getNewSentinelServiceForRedisCluster(rc)
	svcOld, err := smm.svcLister.Services(rc.Namespace).Get(svcName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err := setServiceLastAppliedConfigAnnotation(svcNew)
			if err != nil {
				return err
			}
			return smm.svcControl.CreateService(rc, svcNew)
		}
		return err
	}

	if ok, err := serviceEqual(svcNew, svcOld); err != nil {
		return err
	} else if !ok {
		svc := *svcOld
		svc.Spec = svcNew.Spec
		svc.Spec.ClusterIP = svcOld.Spec.ClusterIP
		if err = setServiceLastAppliedConfigAnnotation(&svc); err != nil {
			return err
		}

		_, err := smm.svcControl.UpdateService(rc, &svc)
		return err
	}

	return nil
}

func (smm *sentinelMemberManager) syncRedisClusterStatus(rc *v1alpha1.RedisCluster, set *apps.StatefulSet) error {
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

func (smm *sentinelMemberManager) getNewSentinelServiceForRedisCluster(rc *v1alpha1.RedisCluster) *corev1.Service {
	ns, rcName := rc.Namespace, rc.Name
	svcName := controller.SentinelMemberName(rcName)
	sentiLabel := label.New().Cluster(rcName).Sentinel().Labels()

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            svcName,
			Namespace:       ns,
			Labels:          sentiLabel,
			OwnerReferences: []metav1.OwnerReference{controller.GetOwnerRef(rc)},
		},
		Spec: corev1.ServiceSpec{
			Type: controller.GetServiceType(rc.Spec.Services, v1alpha1.SentinelMemberType.String()),
			Ports: []corev1.ServicePort{
				{
					Name:       "sentinel",
					Port:       16379,
					TargetPort: intstr.FromInt(16379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: sentiLabel,
		},
	}
}

func (smm *sentinelMemberManager) getNewSentinelHeadlessServiceForRedisCluster(rc *v1alpha1.RedisCluster) *corev1.Service {
	ns, rcName := rc.Namespace, rc.Name
	svcName := controller.SentinelPeerMemberName(rcName)
	sentiLabel := label.New().Cluster(rcName).Sentinel().Labels()

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            svcName,
			Namespace:       ns,
			Labels:          sentiLabel,
			OwnerReferences: []metav1.OwnerReference{controller.GetOwnerRef(rc)},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{
					Name:       "peer",
					Port:       16379,
					TargetPort: intstr.FromInt(16379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: sentiLabel,
		},
	}
}

func (smm *sentinelMemberManager) sentinelIsUpgrading(set *apps.StatefulSet, rc *v1alpha1.RedisCluster) (bool, error) {
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

func (smm *sentinelMemberManager) getNewSentinelStatefulSet(rc *v1alpha1.RedisCluster) *apps.StatefulSet {
	ns, rcName := rc.GetNamespace(), rc.GetName()
	sentiConfigMap := controller.SentinelMemberName(rcName)

	volMounts := []corev1.VolumeMount{
		{Name: "configfile", MountPath: "/etc/redis"},
		// {Name: "log-dir", MountPath: "/var/log/sentinel"},
	}
	vols := []corev1.Volume{
		{Name: "configfile",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: sentiConfigMap,
					},
					Items: []corev1.KeyToPath{{Key: "config-file", Path: "sentinel.conf"}},
				},
			},
		},
	}

	depLabel := label.New().Cluster(rcName).Sentinel()
	depName := controller.SentinelMemberName(rcName)
	set := &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      depName,
			Labels:    depLabel.Labels(),
			OwnerReferences: []metav1.OwnerReference{
				controller.GetOwnerRef(rc),
			},
		},
		Spec: apps.StatefulSetSpec{
			Replicas: func() *int32 { r := rc.Spec.Sentinels.Replicas; return &r }(),
			Selector: depLabel.LabelSelector(),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      depLabel.Labels(),
					Annotations: controller.AnnProm(2379),
				},
				Spec: corev1.PodSpec{
					Affinity: util.AffinityForNodeSelector(ns,
						true, depLabel.Labels(),
						rc.Spec.Sentinels.NodeSelector),
					Containers: []corev1.Container{
						{
							Name:            "redis-sentinel",
							Image:           rc.Spec.Sentinels.Image,
							ImagePullPolicy: rc.Spec.Sentinels.ImagePullPolicy,
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
							Resources:    util.ResourceRequirement(rc.Spec.Sentinels.ContainerSpec),
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
					Tolerations:   rc.Spec.Sentinels.Tolerations,
				},
			},
		},
	}
	return set
}
