package member

import (
	"fmt"

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

type replicaMemberManager struct {
	setControl      controller.StatefulSetControlInterface
	svcControl      controller.ServiceControlInterface
	svcLister       corelisters.ServiceLister
	setLister       appslisters.StatefulSetLister
	podLister       corelisters.PodLister
	podControl      controller.PodControlInterface
	redisScaler     Scaler
	replicaUpgrader Upgrader
}

// ServiceConfig config to a K8s service
type ServiceConfig struct {
	Name       string
	Port       int32
	SvcLabel   func(label.Label) label.Label
	MemberName func(clusterName string) string
	Headless   bool
}

// NewReplicaMemberManager new redis instance manager
func NewReplicaMemberManager(
	setControl controller.StatefulSetControlInterface,
	svcControl controller.ServiceControlInterface,
	svcLister corelisters.ServiceLister,
	podLister corelisters.PodLister,
	podControl controller.PodControlInterface,
	setLister appslisters.StatefulSetLister,
	redisScaler Scaler,
	replicaUpgrader Upgrader) manager.Manager {
	rmm := &replicaMemberManager{
		setControl:      setControl,
		svcControl:      svcControl,
		svcLister:       svcLister,
		podLister:       podLister,
		podControl:      podControl,
		setLister:       setLister,
		redisScaler:     redisScaler,
		replicaUpgrader: replicaUpgrader,
	}
	return rmm
}

// Sync	implements redis logic for syncing Redis.
func (rmm *replicaMemberManager) Sync(rc *v1alpha1.RedisCluster) error {
	svcList := []ServiceConfig{{
		Name:     "master",
		Port:     6379,
		SvcLabel: func(l label.Label) label.Label { return l.Master() },
		MemberName: func(clusterName string) string {
			return controller.RedisMemberName(clusterName)
		},
		Headless: false,
	},
		{
			Name:     "master-peer",
			Port:     6379,
			SvcLabel: func(l label.Label) label.Label { return l.Master() },
			MemberName: func(clusterName string) string {
				return fmt.Sprintf("%s-master-peer", controller.RedisMemberName(clusterName))
			},
			Headless: true,
		},
	}

	if rc.Spec.Mode == v1alpha1.Replica && rc.Spec.Redis.Replicas > 1 {
		slaveSvcList := []ServiceConfig{{
			Name:     "salve",
			Port:     6379,
			SvcLabel: func(l label.Label) label.Label { return l.Slave() },
			MemberName: func(clusterName string) string {
				return fmt.Sprintf("%s-slave", controller.RedisMemberName(clusterName))
			},
			Headless: false,
		}}
		svcList = append(svcList, slaveSvcList...)
	}

	for _, svc := range svcList {
		if err := rmm.syncServiceForReplicaRedis(rc, svc); err != nil {
			return err
		}
	}

	return rmm.syncStatefulSetForReplicaRedis(rc)
}

// sync service
func (rmm *replicaMemberManager) syncServiceForReplicaRedis(rc *v1alpha1.RedisCluster, svc ServiceConfig) error {
	// create service
	ns, rcName := rc.GetNamespace(), rc.GetName()
	newSvc := rmm.getNewRedisServiceForRedis(rc, svc)
	oldSvcTmp, err := rmm.svcLister.Services(ns).Get(svc.MemberName(rcName))
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := setServiceLastAppliedConfigAnnotation(newSvc); err != nil {
				return err
			}

			return rmm.svcControl.CreateService(rc, newSvc)
		}
		return err
	}

	oldSvc := oldSvcTmp.DeepCopy()
	if ok, err := serviceEqual(newSvc, oldSvc); err != nil {
		return err
	} else if !ok {
		svc := *oldSvc
		svc.Spec = newSvc.Spec
		svc.Spec.ClusterIP = oldSvc.Spec.ClusterIP
		if err = setServiceLastAppliedConfigAnnotation(&svc); err != nil {
			return err
		}

		_, err := rmm.svcControl.UpdateService(rc, &svc)
		return err
	}

	return nil
}

func (rmm *replicaMemberManager) syncStatefulSetForReplicaRedis(rc *v1alpha1.RedisCluster) error {
	ns, rcName := rc.GetNamespace(), rc.Name

	newSet, err := rmm.getNewReplicaStatefulSet(rc)
	if err != nil {
		return err
	}

	oldSet, err := rmm.setLister.StatefulSets(ns).Get(controller.RedisMemberName(rcName))
	if apierrors.IsNotFound(err) {
		err = setStatefulSetLastAppliedConfigAnnotation(newSet)
		if err != nil {
			return err
		}

		if err := rmm.setControl.CreateStatefulSet(rc, newSet); err != nil {
			return err
		}
		rc.Status.Redis.StatefulSet = &apps.StatefulSetStatus{}
		return controller.RequeueErrorf("RedisCluster: [%s/%s], waiting for Replica cluster running", ns, rcName)
	}

	// sync status
	if err := rmm.syncReplicaStatefulSetStatus(rc, oldSet); err != nil {
		glog.Errorf("failed to sync Redis: [%s/%s]'s status, error: %v", ns, rcName, err)
		return err
	}

	// sync replica cluster node role
	if err := rmm.setRoleLabelsForCluster(rc, oldSet); err != nil {
		glog.Errorf("failed to sync Redis role: [%s/%s]'s status, error: %v", ns, rcName, err)
		return err
	}

	if !templateEqual(newSet.Spec.Template, oldSet.Spec.Template) || rc.Status.Redis.Phase == v1alpha1.UpgradePhase {
		if err := rmm.replicaUpgrader.Upgrade(rc, oldSet, newSet); err != nil {
			return err
		}
	}

	if *newSet.Spec.Replicas > *oldSet.Spec.Replicas {
		if err := rmm.redisScaler.ScaleOut(rc, newSet, oldSet); err != nil {
			return err
		}
	}

	if *newSet.Spec.Replicas < *oldSet.Spec.Replicas {
		if err := rmm.redisScaler.ScaleIn(rc, newSet, oldSet); err != nil {
			return err
		}
	}

	if !statefulSetEqual(*newSet, *oldSet) {
		set := *oldSet
		set.Spec.Template = newSet.Spec.Template
		*set.Spec.Replicas = *newSet.Spec.Replicas
		set.Spec.UpdateStrategy = newSet.Spec.UpdateStrategy
		if err := setStatefulSetLastAppliedConfigAnnotation(&set); err != nil {
			return err
		}

		_, err = rmm.setControl.UpdateStatefulSet(rc, &set)
		return err
	}

	return nil
}

// setRoleLabelsForCluster sync redis cluster node role
func (rmm *replicaMemberManager) setRoleLabelsForCluster(rc *v1alpha1.RedisCluster, set *apps.StatefulSet) error {
	ns, rcName := rc.GetNamespace(), rc.GetName()

	defaultMasterPod := ordinalPodName(v1alpha1.RedisMemberType, rcName, 0)
	oldMasters := rc.Status.Redis.Masters
	if len(oldMasters) > 0 && oldMasters[0] != "" {
		defaultMasterPod = rc.Status.Redis.Masters[0]
	}

	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	selector, err := label.New().Instance(instanceName).Redis().ReplicaMode().Selector()
	if err != nil {
		return err
	}

	pods, err := rmm.podLister.Pods(ns).List(selector)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	for _, pod := range pods {
		podCopy := pod.DeepCopy()
		nodeRoleLabelKey := podCopy.Labels[label.ClusterNodeRoleLabelKey]
		if len(nodeRoleLabelKey) == 0 && podCopy.GetName() == defaultMasterPod ||
			nodeRoleLabelKey == label.MasterNodeLabelKey {
			podCopy.Labels[label.ClusterNodeRoleLabelKey] = label.MasterNodeLabelKey
		} else {
			podCopy.Labels[label.ClusterNodeRoleLabelKey] = label.SlaveNodeLabelKey
		}

		// 更新节点
		if _, err := rmm.podControl.UpdatePod(rc, podCopy); err != nil {
			return err
		}
	}

	return nil
}

func (rmm *replicaMemberManager) getNewRedisServiceForRedis(rc *v1alpha1.RedisCluster, svcConfig ServiceConfig) *corev1.Service {
	ns, rcName := rc.Namespace, rc.Name

	svcName := svcConfig.MemberName(rcName)
	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	rediLabel := svcConfig.SvcLabel(label.New().Instance(instanceName).Redis()).ReplicaMode().Labels()
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
			Selector:                 rediLabel,
			PublishNotReadyAddresses: true,
		},
	}

	if svcConfig.Headless {
		svc.Spec.ClusterIP = "None"
	} else {
		svc.Spec.Type = controller.GetServiceType(rc.Spec.Services, svcConfig.Name)
	}

	return svc
}

func (rmm *replicaMemberManager) syncReplicaStatefulSetStatus(rc *v1alpha1.RedisCluster, set *apps.StatefulSet) error {
	rc.Status.Redis.StatefulSet = &set.Status
	upgrading, err := rmm.replicaIsUpgrading(set, rc)
	if err != nil {
		rc.Status.Redis.Synced = false
		return err
	}
	if upgrading {
		rc.Status.Redis.Phase = v1alpha1.UpgradePhase
	} else {
		rc.Status.Redis.Phase = v1alpha1.NormalPhase
	}

	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	selector, err := label.New().Instance(instanceName).Redis().ReplicaMode().Selector()
	if err != nil {
		rc.Status.Redis.Synced = false
		return err
	}
	pods, err := rmm.podLister.Pods(rc.GetNamespace()).List(selector)
	if err != nil && !apierrors.IsNotFound(err) {
		rc.Status.Redis.Synced = false
		return err
	}

	oldMasters := rc.Status.Redis.Masters
	rediStatus, masters := map[string]v1alpha1.RedisMember{}, []string{}
	for _, pod := range pods {
		podCopy := pod.DeepCopy()
		nodeRoleLabelKey := podCopy.Labels[label.ClusterNodeRoleLabelKey]
		rediStatus[podCopy.GetName()] = v1alpha1.RedisMember{
			Name:               podCopy.GetName(),
			Role:               nodeRoleLabelKey,
			LastTransitionTime: metav1.Now(),
		}
		if nodeRoleLabelKey == label.MasterNodeLabelKey {
			masters = append(masters, podCopy.GetName())
		}
	}

	// if has not new master use old master
	if len(oldMasters) > 0 && len(masters) == 0 {
		masters = oldMasters
	}

	rc.Status.Redis.Members = rediStatus
	rc.Status.Redis.Masters = masters
	rc.Status.Redis.Synced = true

	return nil
}

func (rmm *replicaMemberManager) replicaIsUpgrading(set *apps.StatefulSet, rc *v1alpha1.RedisCluster) (bool, error) {
	if statefulSetIsUpgrading(set) {
		return true, nil
	}

	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	selector, err := label.New().Instance(instanceName).Redis().ReplicaMode().Selector()
	if err != nil {
		return false, err
	}
	setPods, err := rmm.podLister.Pods(rc.GetNamespace()).List(selector)
	if err != nil {
		return false, err
	}
	for _, pod := range setPods {
		revisionHash, exist := pod.Labels[apps.ControllerRevisionHashLabelKey]
		if !exist {
			return false, nil
		}
		if revisionHash != rc.Status.Redis.StatefulSet.UpdateRevision {
			return true, nil
		}
	}

	return false, nil
}

func (rmm *replicaMemberManager) getNewReplicaStatefulSet(rc *v1alpha1.RedisCluster) (*apps.StatefulSet, error) {
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
	rediLabel := label.New().Instance(instanceName).Redis().ReplicaMode()
	storageClassName := rc.Spec.Redis.StorageClassName
	if storageClassName == "" {
		storageClassName = controller.DefaultStorageClassName
	}

	pvc, err := rmm.volumeClaimTemplate(rc, &storageClassName)
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
								{
									Name:  "PEER_MASTER_SERVICE_NAME",
									Value: fmt.Sprintf("%s-master-peer", controller.RedisMemberName(rcName)),
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

func (rmm *replicaMemberManager) volumeClaimTemplate(rc *v1alpha1.RedisCluster, storageClassName *string) (corev1.PersistentVolumeClaim, error) {
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

// FakeReplicaMemberManager rediscluster member manager fake use test
type FakeReplicaMemberManager struct {
	err error
}

// NewFakeReplicaMemberManager new fake instance
func NewFakeReplicaMemberManager() *FakeReplicaMemberManager {
	return &FakeReplicaMemberManager{}
}

// SetSyncError sync err
func (frmm *FakeReplicaMemberManager) SetSyncError(err error) {
	frmm.err = err
}

// Sync sync info
func (frmm *FakeReplicaMemberManager) Sync(_ *v1alpha1.RedisCluster) error {
	if frmm.err != nil {
		return frmm.err
	}
	return nil
}
