package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo" // revive:disable:dot-imports
	. "github.com/onsi/gomega" // revive:disable:dot-imports
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
)

func testCreate(ns, clusterName string) {
	By(fmt.Sprintf("When create the Redis cluster: %s/%s", ns, clusterName))

	instanceName := getInstanceName(ns, clusterName)
	cmdStr := fmt.Sprintf("helm install /charts/redis-cluster -f /redis-cluster-values.yaml"+
		" -n %s --namespace=%s --set clusterName=%s",
		instanceName, ns, clusterName)
	_, err := execCmd(cmdStr)

	Expect(err).NotTo(HaveOccurred())

	By("Then all members should running")
	err = wait.Poll(5*time.Second, 5*time.Minute, func() (bool, error) {
		return allMembersRunning(ns, clusterName)
	})
	Expect(err).NotTo(HaveOccurred())
}

func allMembersRunning(ns, clusterName string) (bool, error) {
	rc, err := cli.RedisV1alpha1().Redises(ns).Get(clusterName, metav1.GetOptions{})
	if err != nil {
		return false, nil
	}

	if rc.Spec.Mode == v1alpha1.ReplicaCluster {
		running, err := replicaMemberRuning(rc)
		if err != nil || !running {
			return false, nil
		}
	}

	return true, nil
}

func replicaMemberRuning(rc *v1alpha1.Redis) (bool, error) {
	ns, rcName := rc.Namespace, rc.Name
	setName := controller.RedisMemberName(rcName)
	replicaSet, err := kubeCli.AppsV1beta1().StatefulSets(ns).Get(setName, metav1.GetOptions{})
	if err != nil {
		logf(err.Error())
		return false, nil
	}

	logf("replicaSet.Status: %+v", replicaSet.Status)

	if rc.Status.Replica.StatefulSet == nil {
		logf("rc.Status.Replica.StatefulSet is nil")
		return false, nil
	}

	if *replicaSet.Spec.Replicas != rc.Spec.Redis.Members {
		logf("replicaSet.Spec.Replicas(%d) != rc.Spec.Redis.Members(%d)",
			*replicaSet.Spec.Replicas, rc.Spec.Redis.Members)
		return false, nil
	}

	if replicaSet.Status.ReadyReplicas != rc.Spec.Redis.Members {
		logf("replicaSet.Status.ReadyReplicas(%d) != %d",
			replicaSet.Status.ReadyReplicas, rc.Spec.Redis.Members)
		return false, nil
	}

	if replicaSet.Status.ReadyReplicas != replicaSet.Status.Replicas {
		logf("replicaSet.Status.ReadyReplicas(%d) != replicaSet.Status.Replicas(%d)",
			replicaSet.Status.ReadyReplicas, replicaSet.Status.Replicas)
		return false, nil
	}

	_, err = kubeCli.CoreV1().Services(ns).Get(controller.RedisMemberName(rcName), metav1.GetOptions{})
	if err != nil {
		logf(err.Error())
		return false, nil
	}
	_, err = kubeCli.CoreV1().Services(ns).Get(controller.RedisMemberName(rcName), metav1.GetOptions{})
	if err != nil {
		logf(err.Error())
		return false, nil
	}

	return true, nil
}
