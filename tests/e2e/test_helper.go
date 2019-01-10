package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo" // revive:disable:dot-imports
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/anywhy/redis-operator/pkg/client/clientset/versioned"
	"github.com/anywhy/redis-operator/pkg/label"
)

const (
	operatorNs       = "redis-operator-e2e"
	operatorHelmName = "redis-operator-e2e"
)

var (
	cli     versioned.Interface
	kubeCli kubernetes.Interface
)

type testCase func(ns, name string)

type clusterFixture struct {
	ns          string
	clusterName string
	cases       []testCase
}

var fixtures = []clusterFixture{
	{
		ns:          "ns-1",
		clusterName: "cluster-name-1",
		cases: []testCase{
			testCreate,
		},
	},
}

func installOperator() error {
	_, err := execCmd(fmt.Sprintf(
		"helm install /charts/redis-operator -f /redis-operator-values.yaml -n %s --namespace=%s",
		operatorHelmName,
		operatorNs))
	if err != nil {
		return err
	}

	monitorRestartCount()
	return nil
}

func monitorRestartCount() {
	maxRestartCount := int32(3)

	go func() {
		defer GinkgoRecover()
		for {
			select {
			case <-time.After(5 * time.Second):
				podList, err := kubeCli.CoreV1().Pods(metav1.NamespaceAll).List(
					metav1.ListOptions{
						LabelSelector: labels.SelectorFromSet(
							label.New().Labels(),
						).String(),
					},
				)
				if err != nil {
					continue
				}

				for _, pod := range podList.Items {
					for _, cs := range pod.Status.ContainerStatuses {
						if cs.RestartCount > maxRestartCount {
							Fail(fmt.Sprintf("POD: %s/%s's container: %s's restartCount is greater than: %d",
								pod.GetNamespace(), pod.GetName(), cs.Name, maxRestartCount))
							return
						}
					}
				}
			}
		}
	}()
}

func clearOperator() error {
	for _, fixture := range fixtures {
		_, err := execCmd(fmt.Sprintf("helm del --purge %s", fmt.Sprintf("%s-%s", fixture.ns, fixture.clusterName)))
		if err != nil && isNotFound(err) {
			return err
		}

		_, err = execCmd(fmt.Sprintf("kubectl delete pvc -n %s --all", fixture.ns))
		if err != nil {
			return err
		}
	}

	_, err := execCmd(fmt.Sprintf("helm del --purge %s", operatorHelmName))
	if err != nil && isNotFound(err) {
		return err
	}

	for _, fixture := range fixtures {
		err = wait.Poll(5*time.Second, 5*time.Minute, func() (bool, error) {
			result, err := execCmd(fmt.Sprintf("kubectl get po --output=name -n %s", fixture.ns))
			if err != nil || result != "" {
				return false, nil
			}
			_, err = execCmd(fmt.Sprintf(`kubectl get pv -l %s=%s,%s=%s --output=name | xargs -I {} \
		kubectl patch {} -p '{"spec":{"persistentVolumeReclaimPolicy":"Delete"}}'`,
				label.NamespaceLabelKey, fixture.ns, label.InstanceLabelKey, getInstanceName(fixture.ns, fixture.clusterName)))
			if err != nil {
				logf(err.Error())
			}
			result, _ = execCmd(fmt.Sprintf("kubectl get pv -l %s=%s,%s=%s 2>/dev/null|grep Released",
				label.NamespaceLabelKey, fixture.ns, label.InstanceLabelKey, getInstanceName(fixture.ns, fixture.clusterName)))
			if result != "" {
				return false, nil
			}

			return true, nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func execCmd(cmdStr string) (string, error) {
	logf(fmt.Sprintf("$ %s\n", cmdStr))
	result, err := exec.Command("/bin/sh", "-c", cmdStr).CombinedOutput()
	resultStr := string(result)
	logf(resultStr)
	if err != nil {
		logf(err.Error())
		return resultStr, err
	}

	return resultStr, nil
}

func getInstanceName(ns, clusterName string) string {
	return fmt.Sprintf("%s-%s", ns, clusterName)
}

func isNotFound(err error) bool {
	return strings.Contains(err.Error(), "not found")
}

// logf log a message in INFO format
func logf(format string, args ...interface{}) {
	log("INFO", format, args...)
}

func log(level string, format string, args ...interface{}) {
	fmt.Fprintf(GinkgoWriter, nowStamp()+": "+level+": "+format+"\n", args...)
}

func nowStamp() string {
	return time.Now().Format(time.StampMilli)
}
