package member

import (
	"net"
	"strings"

	"github.com/golang/glog"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/label"
)

type replicaFailover struct {
	haControl  controller.HAControlInterface
	podLister  corelisters.PodLister
	podControl controller.PodControlInterface
}

// NewReplicaFailover return a replica cluster Failover
func NewReplicaFailover(haControl controller.HAControlInterface,
	podLister corelisters.PodLister,
	podControl controller.PodControlInterface) Failover {
	return &replicaFailover{haControl, podLister, podControl}
}

func (rf *replicaFailover) Failover(rc *v1alpha1.Redis) {
	if rc.Status.Replica.Phase == v1alpha1.UpgradePhase {
		return
	}
	rf.haControl.Watch(rc, func(master string) error {
		instanceName := rc.GetLabels()[label.InstanceLabelKey]
		selector, err := label.New().Instance(instanceName).Replica().Selector()
		if err != nil {
			glog.Errorf("Replica cluster HA switch master error: %#v", err)
			return err
		}

		pods, err := rf.podLister.Pods(rc.Namespace).List(selector)
		if err != nil {
			glog.Errorf("Replica cluster HA switch master error: %#v", err)
			return err
		}

		for _, pod := range pods {
			addr := net.JoinHostPort(pod.Status.PodIP, "6379")
			if strings.EqualFold(master, addr) && pod.Labels[label.ComponentLabelKey] != label.MasterLabelKey {
				podCopy := pod.DeepCopy()
				podCopy.Labels[label.ComponentLabelKey] = label.MasterLabelKey
				if _, err := rf.podControl.UpdatePod(rc, podCopy); err != nil {
					glog.Errorf("Replica cluster HA switch master error: %#v", err)
					return err
				}
				glog.Infof("Replica cluster HA switch: %v to master", podCopy.Name)
				rc.Status.Replica.MasterName = pod.GetName()
			} else if !strings.EqualFold(master, addr) && pod.Labels[label.ComponentLabelKey] == label.MasterLabelKey {
				podCopy := pod.DeepCopy()
				podCopy.Labels[label.ComponentLabelKey] = label.SlaveLabelKey
				if _, err := rf.podControl.UpdatePod(rc, podCopy); err != nil {
					glog.Errorf("Replica cluster HA switch master error: %#v", err)
					return err
				}
				glog.Infof("Replica cluster HA switch: %v to slave", podCopy.Name)
			}
		}

		return nil
	})
}

func (rf *replicaFailover) Recover(*v1alpha1.Redis) {
	// Do nothing now
}

type fakeReplicaFailover struct{}

// NewFakeReplicaFailover returns a fake Failover
func NewFakeReplicaFailover() Failover {
	return &fakeReplicaFailover{}
}

func (frf *fakeReplicaFailover) Failover(_ *v1alpha1.Redis) {
	return
}

func (frf *fakeReplicaFailover) Recover(_ *v1alpha1.Redis) {
	return
}
