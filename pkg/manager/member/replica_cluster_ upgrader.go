package member

import (
	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/label"
)

type replicaUpgrader struct {
	podControl controller.PodControlInterface
	podLister  corelisters.PodLister
}

// NewReplicaUpgrader returns a replica cluster Upgrader
func NewReplicaUpgrader(podControl controller.PodControlInterface,
	podLister corelisters.PodLister) Upgrader {
	return &replicaUpgrader{
		podControl: podControl,
		podLister:  podLister,
	}
}

func (ru *replicaUpgrader) Upgrade(rc *v1alpha1.Redis, oldSet *apps.StatefulSet, newSet *apps.StatefulSet) error {
	ns, rcName := rc.GetNamespace(), rc.GetName()

	if rc.Status.Phase == v1alpha1.UpgradePhase {
		_, podSpec, err := GetLastAppliedConfig(oldSet)
		if err != nil {
			return err
		}
		newSet.Spec.Template.Spec = *podSpec
		return nil
	}

	rc.Status.Phase = v1alpha1.UpgradePhase
	if !templateEqual(newSet.Spec.Template, oldSet.Spec.Template) {
		return nil
	}

	setUpgradePartition(newSet, *oldSet.Spec.UpdateStrategy.RollingUpdate.Partition)
	for i := rc.Status.Replica.StatefulSet.Replicas - 1; i >= 0; i-- {
		podName := replicaPodName(rcName, i)
		pod, err := ru.podLister.Pods(ns).Get(podName)
		if err != nil {
			return err
		}

		_, exist := pod.Labels[apps.ControllerRevisionHashLabelKey]
		if !exist {
			return controller.RequeueErrorf("replicacluster: [%s/%s]'s redis pod: [%s] has no label: %s",
				ns, rcName, podName, apps.ControllerRevisionHashLabelKey)
		}

		if shoud, err := ru.shoudUpgradePod(rc, pod); err != nil || !shoud {
			if controller.IsRequeueError(err) {
				return err
			}
			continue
		}

		return ru.upgradeReplicaPod(rc, i, newSet)
	}

	return nil
}

func (ru *replicaUpgrader) upgradeReplicaPod(rc *v1alpha1.Redis, ordinal int32, newSet *apps.StatefulSet) error {
	setUpgradePartition(newSet, ordinal)
	return nil
}

func (ru *replicaUpgrader) shoudUpgradePod(rc *v1alpha1.Redis, pod *corev1.Pod) (bool, error) {
	ns, rcName := rc.GetNamespace(), rc.GetName()
	instanceName := rc.GetLabels()[label.InstanceLabelKey]
	l, err := label.New().Instance(instanceName).Replica().Slave().Selector()
	if err != nil {
		return false, controller.RequeueErrorf(err.Error())
	}

	slavePods, err := ru.podLister.Pods(ns).List(l)
	if err != nil {
		return false, controller.RequeueErrorf(err.Error())
	}

	// check all slaves upgraded
	upgraded := 0
	for _, slave := range slavePods {
		revision, exist := slave.Labels[apps.ControllerRevisionHashLabelKey]
		if !exist {
			return false, controller.RequeueErrorf("replicacluster: [%s/%s]'s redis pod: [%s] has no label: %s",
				ns, rcName, slave.Name, apps.ControllerRevisionHashLabelKey)
		}
		if revision == rc.Status.Replica.StatefulSet.UpdateRevision {
			upgraded++
		}
	}

	revision, _ := pod.Labels[apps.ControllerRevisionHashLabelKey]
	role := pod.GetLabels()[label.ComponentLabelKey]
	if revision == rc.Status.Replica.StatefulSet.UpdateRevision ||
		(upgraded != int(rc.Spec.Redis.Members)-1 && role == label.MasterLabelKey) {
		return false, nil
	}

	return true, nil
}
