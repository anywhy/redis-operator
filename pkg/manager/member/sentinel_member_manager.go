package member

import (
	"github.com/anywhy/redis-operator/pkg/label"
	extv1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	extlisters "k8s.io/client-go/listers/extensions/v1beta1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/manager"
)

type sentinelMemberManager struct {
	deployControl controller.DeploymentControlInterface
	svcControl    controller.ServiceControlInterface
	svcLister     corelisters.ServiceLister
	deployLister  extlisters.DeploymentLister
}

// NewSentinelMemberManager new redis sentinel manager
func NewSentinelMemberManager(
	deployControl controller.DeploymentControlInterface,
	svcControl controller.ServiceControlInterface,
	svcLister corelisters.ServiceLister,
	deployLister extlisters.DeploymentLister) manager.Manager {
	return &sentinelMemberManager{
		deployControl: deployControl,
		svcControl:    svcControl,
		svcLister:     svcLister,
		deployLister:  deployLister,
	}
}

// Sync	implements sentinel logic for syncing rediscluster.
func (smm *sentinelMemberManager) Sync(*v1alpha1.RedisCluster) error {

	return nil
}

func (smm *sentinelMemberManager) getNewSentinelDeployment(rc *v1alpha1.RedisCluster) *extv1.Deployment {
	ns, rcName := rc.GetNamespace(), rc.GetName()

	depLabel := label.New().Cluster(rcName).Sentinel()
	depName := controller.SentinelMemberName(rcName)
	dep := &extv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      depName,
			Labels:    depLabel.Labels(),
			OwnerReferences: []metav1.OwnerReference{
				controller.GetOwnerRef(rc),
			},
		},
	}
	return dep
}
