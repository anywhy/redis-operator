package member

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
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
	"github.com/anywhy/redis-operator/pkg/util/redis"
)

type sentinelMemberManager struct {
	setControl controller.StatefulSetControlInterface
	svcControl controller.ServiceControlInterface
	svcLister  corelisters.ServiceLister
	setLister  appslisters.StatefulSetLister
	podLister  corelisters.PodLister
	podControl controller.PodControlInterface
}

// NewSentinelMemberManager new redis sentinel manager
func NewSentinelMemberManager(
	setControl controller.StatefulSetControlInterface,
	svcControl controller.ServiceControlInterface,
	svcLister corelisters.ServiceLister,
	podLister corelisters.PodLister,
	podControl controller.PodControlInterface,
	setLister appslisters.StatefulSetLister) manager.Manager {
	return &sentinelMemberManager{
		setControl: setControl,
		svcControl: svcControl,
		svcLister:  svcLister,
		podLister:  podLister,
		podControl: podControl,
		setLister:  setLister,
	}
}

// Sync	implements sentinel logic for syncing Redis.
func (smm *sentinelMemberManager) Sync(rc *v1alpha1.RedisCluster) error {
	if rc.Spec.Mode == v1alpha1.Replica && rc.ShoudEnableSentinel() {
		if err := smm.syncSentinelService(rc, ServiceConfig{
			Name:     "peer",
			Port:     26379,
			SvcLabel: func(l label.Label) label.Label { return l.Sentinel() },
			MemberName: func(clusterName string) string {
				return controller.SentinelPeerMemberName(clusterName)
			},
			Headless: true,
		}); err != nil {
			return err
		}

		return smm.syncSentinelStatefulSetForRedis(rc)
	}
	return nil
}

func (smm *sentinelMemberManager) syncSentinelStatefulSetForRedis(rc *v1alpha1.RedisCluster) error {
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

	// sync status
	if err = smm.syncSentinelClusterStatus(rc, oldSentiSet); err != nil {
		glog.Errorf("failed to sync Redis: [%s/%s]'s status, error: %v", ns, rcName, err)
	}

	// sync subscribe switch master
	if err := smm.syncSubscribeSwitchMaster(rc); err != nil {
		glog.Errorf("failed to sync Sentinel subscribe switch master: [%s/%s]'s status, error: %v", ns, rcName, err)
		return err
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

func (smm *sentinelMemberManager) syncSentinelService(rc *v1alpha1.RedisCluster, svcConfig ServiceConfig) error {
	ns, rcName := rc.GetNamespace(), rc.GetName()

	newSvc := smm.getNewServiceForSentinel(rc, svcConfig)
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

func (smm *sentinelMemberManager) syncSentinelClusterStatus(rc *v1alpha1.RedisCluster, set *apps.StatefulSet) error {
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

func (smm *sentinelMemberManager) sentinelIsUpgrading(set *apps.StatefulSet, rc *v1alpha1.RedisCluster) (bool, error) {
	if statefulSetIsUpgrading(set) {
		return true, nil
	}

	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	selector, err := label.New().Instance(instanceName).ReplicaMode().Sentinel().Selector()
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

func (smm *sentinelMemberManager) syncSubscribeSwitchMaster(rc *v1alpha1.RedisCluster) error {
	subcribed := rc.Status.Sentinel.Subscribed
	if !subcribed && rc.AllSentinelPodsStarted() {
		password := rc.Spec.Sentinel.Password

		sentinel := redis.NewSentinel(rc.GetClusterName(), password)
		sentinels, err := smm.getSentinelPodsDomain(rc)
		if err != nil {
			return err
		}

		onMajoritySubscribed := func() {
			instanceName := rc.GetLabels()[label.InstanceLabelKey]
			selector, err := label.New().Instance(instanceName).Redis().ReplicaMode().Selector()
			if err != nil {
				glog.Errorf("Replica cluster HA switch master error: %#v", err)
			}

			// Update the pod role label of the replica cluster to achieve the goal of high availability of the cluster.
			newMaster, err := sentinel.Master(sentinels, time.Second*30)
			if err != nil {
				glog.Warningf("Get Replica cluster Master error: %#v", err)
			}
			if newMaster != "" {
				pods, err := smm.podLister.Pods(rc.Namespace).List(selector)
				if err != nil {
					glog.Errorf("Replica cluster HA switch master error: %#v", err)
				}
				for _, pod := range pods {
					podCopy := pod.DeepCopy()
					if strings.EqualFold(newMaster, podCopy.Status.PodIP) &&
						podCopy.Labels[label.ClusterNodeRoleLabelKey] != label.MasterNodeLabelKey {
						podCopy.Labels[label.ClusterNodeRoleLabelKey] = label.MasterNodeLabelKey
						if _, err := smm.podControl.UpdatePod(rc, podCopy); err != nil {
							glog.Errorf("Replica cluster HA switch master error: %#v", err)
						}

						glog.Infof("Replica cluster HA switch: %v to master", podCopy.Name)
						rc.Status.Redis.Masters[0] = pod.GetName()
					} else if pod.Labels[label.ClusterNodeRoleLabelKey] != label.SlaveNodeLabelKey {
						podCopy.Labels[label.ClusterNodeRoleLabelKey] = label.SlaveNodeLabelKey
						if _, err := smm.podControl.UpdatePod(rc, podCopy); err != nil {
							glog.Errorf("Replica cluster HA switch slave error: %#v", err)
						}
						glog.Infof("Replica cluster HA switch: %v to slave", podCopy.Name)
					}
				}
			}
		}

		if sentinel.Subscribe(sentinels, time.Second*30, onMajoritySubscribed) {
			subcribed = true
		}
	}

	rc.Status.Sentinel.Subscribed = subcribed
	return nil
}

func (smm *sentinelMemberManager) getNewServiceForSentinel(rc *v1alpha1.RedisCluster, svcConfig ServiceConfig) *corev1.Service {
	ns, rcName := rc.Namespace, rc.Name

	svcName := svcConfig.MemberName(rcName)
	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	rediLabel := svcConfig.SvcLabel(label.New().Instance(instanceName)).Sentinel().Labels()
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

// start cmd
const sentinelCmd = `cp -r /sentinel.conf /etc/redis/&&redis-server /etc/redis/sentinel.conf --sentinel`

func (smm *sentinelMemberManager) getNewSentinelStatefulSet(rc *v1alpha1.RedisCluster) (*apps.StatefulSet, error) {
	ns, rcName := rc.GetNamespace(), rc.GetName()
	redisConfigMap := controller.RedisMemberName(rcName)

	podMount, podVolume := podinfoVolume()
	volMounts := []corev1.VolumeMount{
		podMount,
		{Name: v1alpha1.SentinelMemberType.String(), ReadOnly: true, MountPath: "/data"},
		{Name: "config", MountPath: "/etc/redis"},
	}
	vols := []corev1.Volume{
		podVolume,
		{Name: v1alpha1.SentinelMemberType.String(),
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
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	sentiLabel := label.New().Instance(instanceName).Sentinel().ReplicaMode()
	setName := controller.SentinelMemberName(rcName)

	dnsPolicy := corev1.DNSClusterFirst // same as k8s defaults
	if rc.Spec.Redis.HostNetwork {
		dnsPolicy = corev1.DNSClusterFirstWithHostNet
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
					SchedulerName: rc.Spec.SchedulerName,
					Affinity:      rc.Spec.Redis.Affinity,
					NodeSelector:  rc.Spec.Redis.NodeSelector,
					HostNetwork:   rc.Spec.Redis.HostNetwork,
					DNSPolicy:     dnsPolicy,
					Containers: []corev1.Container{
						{
							Name:            "sentinel",
							Image:           rc.Spec.Redis.Image,
							ImagePullPolicy: rc.Spec.Redis.ImagePullPolicy,
							Command:         []string{"bash", "-c", sentinelCmd},
							Ports: []corev1.ContainerPort{
								{
									Name:          "sentinel",
									ContainerPort: int32(26379),
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
					Tolerations:   rc.Spec.Redis.Tolerations,
				},
			},
			ServiceName:         controller.SentinelPeerMemberName(rcName),
			PodManagementPolicy: apps.ParallelPodManagement,
		},
	}
	return sentiSet, nil
}

func (smm *sentinelMemberManager) getSentinelPodsDomain(rc *v1alpha1.RedisCluster) ([]string, error) {
	sentinels := []string{}
	setName := controller.SentinelMemberName(rc.GetName())
	for i := 0; i < int(rc.Spec.Sentinel.Replicas); i++ {
		sentinels = append(sentinels, fmt.Sprintf("%s-%s.%s.svc:26379", setName,
			strconv.Itoa(i), rc.GetNamespace()))
	}
	return sentinels, nil
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
func (fsmm *FakeSentinelMemberManager) Sync(_ *v1alpha1.RedisCluster) error {
	if fsmm.err != nil {
		return fsmm.err
	}
	return nil
}
