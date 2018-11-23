package member

import (
	"fmt"

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

type redisMemeberManager struct {
	setControl  controller.StatefulSetControlInterface
	svcControl  controller.ServiceControlInterface
	svcLister   corelisters.ServiceLister
	setLister   appslisters.StatefulSetLister
	podLister   corelisters.PodLister
	podControl  controller.PodControlInterface
	redisScaler Scaler
}

// ServiceConfig config to a K8s service
type ServiceConfig struct {
	Name       string
	Port       int32
	SvcLabel   func(label.Label) label.Label
	MemberName func(clusterName string) string
	Headless   bool
}

// NewRedisMemberManager new redis instance manager
func NewRedisMemberManager(
	setControl controller.StatefulSetControlInterface,
	svcControl controller.ServiceControlInterface,
	svcLister corelisters.ServiceLister,
	podLister corelisters.PodLister,
	podControl controller.PodControlInterface,
	setLister appslisters.StatefulSetLister,
	redisScaler Scaler) manager.Manager {
	return &redisMemeberManager{
		setControl:  setControl,
		svcControl:  svcControl,
		svcLister:   svcLister,
		podLister:   podLister,
		setLister:   setLister,
		redisScaler: redisScaler,
	}
}

// Sync	implements redis logic for syncing rediscluster.
func (rmm *redisMemeberManager) Sync(rc *v1alpha1.RedisCluster) error {
	if err := rmm.syncRedisServiceForRedisCluster(rc); err != nil {
		return err
	}
	return rmm.syncRedisStatefulSetForRedisCluster(rc)
}

func (rmm *redisMemeberManager) syncRedisServiceForRedisCluster(rc *v1alpha1.RedisCluster) error {
	svcList := []ServiceConfig{
		{
			Name:     "redis-master",
			Port:     6379,
			SvcLabel: func(l label.Label) label.Label { return l.Master() },
			MemberName: func(clusterName string) string {
				return fmt.Sprintf("%s-master", controller.RedisMemberName(clusterName))
			},
			Headless: false,
		},
		{
			Name:     "redis-master-peer",
			Port:     6379,
			SvcLabel: func(l label.Label) label.Label { return l.Master() },
			MemberName: func(clusterName string) string {
				return fmt.Sprintf("%s-master-peer", controller.RedisMemberName(clusterName))
			},
			Headless: true,
		},
		{
			Name:     "redis-salve",
			Port:     6379,
			SvcLabel: func(l label.Label) label.Label { return l.Slave() },
			MemberName: func(clusterName string) string {
				return fmt.Sprintf("%s-slave", controller.RedisMemberName(clusterName))
			},
			Headless: false,
		},
		{
			Name:     "redis-salve-peer",
			Port:     6379,
			SvcLabel: func(l label.Label) label.Label { return l.Slave() },
			MemberName: func(clusterName string) string {
				return fmt.Sprintf("%s-slave-peer", controller.RedisMemberName(clusterName))
			},
			Headless: true,
		},
	}

	for _, svc := range svcList {
		if err := rmm.syncRedisService(rc, svc); err != nil {
			return err
		}
	}

	return nil
}

func (rmm *redisMemeberManager) syncRedisStatefulSetForRedisCluster(rc *v1alpha1.RedisCluster) error {
	ns, rcName := rc.GetNamespace(), rc.Name

	newSet, err := rmm.getNewRedisStatefulSet(rc)
	if err != nil {
		return err
	}

	oldSet, err := rmm.setLister.StatefulSets(ns).Get(controller.RedisMemberName(rcName))
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = setStatefulSetLastAppliedConfigAnnotation(newSet)
			if err != nil {
				return err
			}

			if err := rmm.setControl.CreateStatefulSet(rc, newSet); err != nil {
				return err
			}
			rc.Status.Redis.StatefulSet = &apps.StatefulSetStatus{}
			return nil
		}

		return err
	}

	// init cluster master
	if _, err := rmm.initRedisMaster(rc); err != nil {
		return err
	}

	// sync status
	if err := rmm.syncRedisStatefulSetStatus(rc, oldSet); err != nil {
		return err
	}

	// TODO update
	if !templateEqual(newSet.Spec.Template, oldSet.Spec.Template) || rc.Status.Redis.Phase == v1alpha1.UpgradePhase {

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

// init redis master
func (rmm *redisMemeberManager) initRedisMaster(rc *v1alpha1.RedisCluster) (*corev1.Pod, error) {
	ns, rcName := rc.GetNamespace(), rc.GetName()

	masterPodName := ordinalPodName(v1alpha1.RedisMemberType, rcName, 0)
	if rc.Status.Redis != nil && rc.Status.Redis.MasterName != "" {
		masterPodName = rc.Status.Redis.MasterName
	}

	masterPod, err := rmm.podLister.Pods(ns).Get(masterPodName)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	if masterPod != nil {
		masterPodCopy := masterPod.DeepCopy()
		if masterPodCopy.Labels[label.ComponentLabelKey] != label.MasterLabelKey {
			return masterPod, nil
		}
		masterPodCopy.Labels[label.ComponentLabelKey] = label.MasterLabelKey
		rc.Status.Redis.MasterName = masterPodCopy.Name
		return rmm.podControl.UpdatePod(rc, masterPodCopy)
	}
	return masterPod, nil
}

func (rmm *redisMemeberManager) syncRedisService(rc *v1alpha1.RedisCluster, svcConfig ServiceConfig) error {
	ns, rcName := rc.GetNamespace(), rc.GetName()

	newSvc := rmm.getNewRedisServiceForRedisCluster(rc, svcConfig)
	oldSvc, err := rmm.svcLister.Services(ns).Get(svcConfig.MemberName(rcName))
	if err != nil {
		if apierrors.IsNotFound(err) {
			err := setServiceLastAppliedConfigAnnotation(newSvc)
			if err != nil {
				return err
			}
			return rmm.svcControl.CreateService(rc, oldSvc)
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

		_, err := rmm.svcControl.UpdateService(rc, &svc)
		return err
	}

	return nil
}

func (rmm *redisMemeberManager) getNewRedisServiceForRedisCluster(rc *v1alpha1.RedisCluster, svcConfig ServiceConfig) *corev1.Service {
	ns, rcName := rc.Namespace, rc.Name

	svcName := svcConfig.MemberName(rcName)
	rediLabel := svcConfig.SvcLabel(label.New().Cluster(rcName)).Labels()
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
		svc.Spec.Type = controller.GetServiceType(rc.Spec.Redis.Services, v1alpha1.RedisMemberType.String())
	}

	return svc
}

func (rmm *redisMemeberManager) syncRedisStatefulSetStatus(rc *v1alpha1.RedisCluster, set *apps.StatefulSet) error {
	rc.Status.Redis.StatefulSet = &set.Status
	upgrading, err := rmm.redislIsUpgrading(set, rc)
	if err != nil {
		return err
	}
	if upgrading {
		rc.Status.Redis.Phase = v1alpha1.UpgradePhase
	} else {
		rc.Status.Redis.Phase = v1alpha1.NormalPhase
	}

	return nil
}

func (rmm *redisMemeberManager) redislIsUpgrading(set *apps.StatefulSet, rc *v1alpha1.RedisCluster) (bool, error) {
	if statefulSetIsUpgrading(set) {
		return true, nil
	}

	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	selector, err := label.New().Cluster(instanceName).Master().Slave().Selector()
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

// redis start commond
const serverCmd = `
set -uo pipefail
ROLE=""

elapseTime=0
period=1
threshold=30
while true; do
    sleep ${period}
    elapseTime=$(( elapseTime+period ))

    if [[ ${elapseTime} -ge ${threshold} ]]
    then
        echo "waiting for redis master role timeout" >&2
        exit 1
	fi

	if [[ -e /etc/podinfo/redisrole ]]; then
		ROLE=$(cat /etc/podinfo/redisrole)
	fi
	
done

ARGS=/etc/redis/redis.conf

if [[ X${ROLE} != Xmaster ]]
then
	ARGS="${ARGS} --slaveof ${PEER_MASTER_SERVICE_NAME}.${NAMESPACE}.svc 6379"
fi

echo "starting redis-server ..."
echo "redis-server ${ARGS}"
exec redis-server ${ARGS} 
`

func (rmm *redisMemeberManager) getNewRedisStatefulSet(rc *v1alpha1.RedisCluster) (*apps.StatefulSet, error) {
	ns, rcName := rc.GetNamespace(), rc.GetName()
	redisConfigMap := controller.RedisMemberName(rcName)

	podMount, podVolume := podinfoVolume()
	volMounts := []corev1.VolumeMount{
		podMount,
		{Name: "configfile", MountPath: "/etc/redis"},
		// {Name: "log-dir", MountPath: "/var/log/sentinel"},
	}
	vols := []corev1.Volume{
		podVolume,
		{Name: "configfile",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: redisConfigMap,
					},
					Items: []corev1.KeyToPath{{Key: "config-file", Path: "redis.conf"}},
				},
			},
		},
	}

	rediLabel := label.New().Cluster(rcName)
	setName := controller.RedisMemberName(rcName)
	storageClassName := rc.Spec.Redis.StorageClassName
	if storageClassName == "" {
		storageClassName = controller.DefaultStorageClassName
	}

	pvc, err := rmm.volumeClaimTemplate(rc, &storageClassName)
	if err != nil {
		return nil, err
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
			Replicas: func() *int32 { r := rc.Spec.Redis.Members; return &r }(),
			Selector: rediLabel.LabelSelector(),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      rediLabel.Labels(),
					Annotations: controller.AnnProm(2379),
				},
				Spec: corev1.PodSpec{
					Affinity: util.AffinityForNodeSelector(ns,
						true, rediLabel.Labels(),
						rc.Spec.Redis.NodeSelector),
					Containers: []corev1.Container{
						{
							Name:            "redis",
							Image:           rc.Spec.Redis.Image,
							ImagePullPolicy: rc.Spec.Redis.ImagePullPolicy,
							Command: []string{
								"bash", "-c", serverCmd,
							},
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
			PodManagementPolicy: apps.ParallelPodManagement,
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				pvc,
			},
		},
	}
	return rediSet, nil
}

func (rmm *redisMemeberManager) volumeClaimTemplate(rc *v1alpha1.RedisCluster, storageClassName *string) (corev1.PersistentVolumeClaim, error) {
	var q, limit resource.Quantity
	var err error

	ns, rcName := rc.GetNamespace(), rc.GetName()
	var pvc corev1.PersistentVolumeClaim

	if rc.Spec.Redis.Requests != nil && rc.Spec.Redis.Requests.Storage != "" {
		size := rc.Spec.Redis.Requests.Storage
		q, err = resource.ParseQuantity(size)
		if err != nil {
			return pvc, fmt.Errorf("cant' get storage request size: %s for RedisCluster: %s/%s, %v", size, ns, rcName, err)
		}
	}

	if rc.Spec.Redis.Limits != nil && rc.Spec.Redis.Limits.Storage != "" {
		size := rc.Spec.Redis.Limits.Storage
		limit, err = resource.ParseQuantity(size)
		if err != nil {
			return pvc, fmt.Errorf("cant' get storage limit size: %s for RedisCluster: %s/%s, %v", size, ns, rcName, err)
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