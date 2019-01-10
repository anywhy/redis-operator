package e2e

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo" // revive:disable:dot-imports
	. "github.com/onsi/gomega" // revive:disable:dot-imports
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/anywhy/redis-operator/pkg/client/clientset/versioned"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)

	cfg, err := rest.InClusterConfig()
	if err != nil {
		panic(err)
	}
	cli, err = versioned.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	kubeCli, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	RunSpecs(t, "Redis Operator Smoke tests")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	By("Clearing old Redis Operator")
	Expect(clearOperator()).NotTo(HaveOccurred())

	By("Bootstrapping new Redis Operator")
	Expect(installOperator()).NotTo(HaveOccurred())

	return nil
}, func(data []byte) {})

var _ = Describe("Smoke", func() {
	for i := 0; i < len(fixtures); i++ {
		fixture := fixtures[i]
		It(fmt.Sprintf("Namespace: %s, clusterName: %s", fixture.ns, fixture.clusterName), func() {
			for _, testCase := range fixture.cases {
				testCase(fixture.ns, fixture.clusterName)
			}
		})
	}
})
