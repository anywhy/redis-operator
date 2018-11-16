package member

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appslisters "k8s.io/client-go/listers/apps/v1beta1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/label"
	"github.com/anywhy/redis-operator/pkg/manager"
)

type redisMemeberManager struct {
	setControl controller.StatefulSetControlInterface
	svcControl controller.ServiceControlInterface
	svcLister  corelisters.ServiceLister
	setLister  appslisters.StatefulSetLister
	podLister  corelisters.PodLister
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
	setLister appslisters.StatefulSetLister) manager.Manager {
	return &redisMemeberManager{
		setControl: setControl,
		svcControl: svcControl,
		svcLister:  svcLister,
		podLister:  podLister,
		setLister:  setLister,
	}
}

// Sync	implements redis logic for syncing rediscluster.
func (rmm *redisMemeberManager) Sync(rc *v1alpha1.RedisCluster) error {

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
		svc.Spec.Type = controller.GetServiceType(rc.Spec.Services, v1alpha1.RedisMemberType.String())
	}

	return svc
}
