package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo" // revive:disable:dot-imports
	. "github.com/onsi/gomega" // revive:disable:dot-imports
)

func testCreate(ns, clusterName string) {
	By(fmt.Sprintf("When create the Redis cluster: %s/%s", ns, clusterName))

	instanceName := getInstanceName(ns, clusterName)
	cmdStr := fmt.Sprintf("helm install /charts/redis-cluster -f /redis-cluster-values.yaml"+
		" -n %s --namespace=%s --set clusterName=%s",
		instanceName, ns, clusterName)
	_, err := execCmd(cmdStr)

	Expect(err).NotTo(HaveOccurred())
}
