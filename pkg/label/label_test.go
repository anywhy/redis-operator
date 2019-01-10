package label

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/labels"
)

func TestLabelNew(t *testing.T) {
	g := NewGomegaWithT(t)

	l := New()
	g.Expect(l[NameLabelKey]).To(Equal("redis-cluster"))
	g.Expect(l[ManagedByLabelKey]).To(Equal("redis-operator"))
}

func TestLabelInstance(t *testing.T) {
	g := NewGomegaWithT(t)

	l := New()
	l.Instance("demo")
	g.Expect(l[InstanceLabelKey]).To(Equal("demo"))
}

func TestLabelMode(t *testing.T) {
	g := NewGomegaWithT(t)

	l := New()
	l.ClusterMode(ReplicaClusterLabelKey)
	g.Expect(l.ClusterModeType()).To(Equal("replica"))
}

func TestLabelComponent(t *testing.T) {
	g := NewGomegaWithT(t)

	l := New()
	l.Component("master")
	g.Expect(l.ComponentType()).To(Equal("master"))
}

func TestLabelReplica(t *testing.T) {
	g := NewGomegaWithT(t)

	l := New()
	l.Replica()
	g.Expect(l.ClusterModeType()).To(Equal(ReplicaClusterLabelKey))
}

func TestLabelRedisCluster(t *testing.T) {
	g := NewGomegaWithT(t)

	l := New()
	l.RedisCluster()
	g.Expect(l.ClusterModeType()).To(Equal(RedisClusterLabelKey))
}

func TestLabelMater(t *testing.T) {
	g := NewGomegaWithT(t)

	l := New()
	l.Master()

	g.Expect(l.IsMaster()).To(BeTrue())
}

func TestLabelSlave(t *testing.T) {
	g := NewGomegaWithT(t)

	l := New()
	l.Slave()

	g.Expect(l.IsSlave()).To(BeTrue())
}

func TestLabelSentinel(t *testing.T) {
	g := NewGomegaWithT(t)

	l := New()
	l.Sentinel()

	g.Expect(l.IsSentinel()).To(BeTrue())
}

func TestLabelSelector(t *testing.T) {
	g := NewGomegaWithT(t)

	l := New()
	l.Replica()
	l.Master()
	l.Instance("demo")
	l.Namespace("ns-1")
	s, err := l.Selector()
	st := labels.Set(map[string]string{
		NameLabelKey:        "redis-cluster",
		ManagedByLabelKey:   "redis-operator",
		ComponentLabelKey:   "master",
		InstanceLabelKey:    "demo",
		NamespaceLabelKey:   "ns-1",
		ClusterModeLabelKey: "replica",
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(s.Matches(st)).To(BeTrue())
}
