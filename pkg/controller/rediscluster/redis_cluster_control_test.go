package rediscluster

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/client/clientset/versioned/fake"
	informers "github.com/anywhy/redis-operator/pkg/client/informers/externalversions"
	"github.com/anywhy/redis-operator/pkg/controller"
	mm "github.com/anywhy/redis-operator/pkg/manager/member"
	"github.com/anywhy/redis-operator/pkg/manager/meta"
)

func TestRedisControlUpdateRedis(t *testing.T) {
	g := NewGomegaWithT(t)

	type testcase struct {
		name                             string
		update                           func(redi *v1alpha1.RedisCluster)
		syncReclaimPolicyErr             bool
		syncReplicaMemberManagerErr      bool
		syncRedisClusterMemberManagerErr bool
		syncMetaManagerErr               bool
		errExpectFn                      func(*GomegaWithT, error)
	}

	testFn := func(test *testcase, t *testing.T) {
		t.Logf("testcase: %s", test.name)

		rc := newRedisForRedisControl()
		if test.update != nil {
			test.update(rc)
		}

		control, reclaimPolicyManager, replicaMemberManager, redisClusterMemberManager, metaManager := newFakeRedisControl()

		if test.syncReclaimPolicyErr {
			reclaimPolicyManager.SetSyncError(fmt.Errorf("reclaim policy sync error"))
		}
		if test.syncReplicaMemberManagerErr {
			replicaMemberManager.SetSyncError(fmt.Errorf("replica cluster member manager sync error"))
		}
		if test.syncRedisClusterMemberManagerErr {
			redisClusterMemberManager.SetSyncError(fmt.Errorf("redis cluster member manager sync error"))
		}
		if test.syncMetaManagerErr {
			metaManager.SetSyncError(fmt.Errorf("meta manager sync error"))
		}

		err := control.UpdateRedisCluster(rc)
		if test.errExpectFn != nil {
			test.errExpectFn(g, err)
		}

	}

	tests := []testcase{
		{
			name:                             "reclaim policy sync error",
			update:                           nil,
			syncReclaimPolicyErr:             true,
			syncReplicaMemberManagerErr:      false,
			syncRedisClusterMemberManagerErr: false,
			syncMetaManagerErr:               false,
			errExpectFn: func(g *GomegaWithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(strings.Contains(err.Error(), "reclaim policy sync error")).To(Equal(true))
			},
		},
		// {
		// 	name:                             "redis cluster member manager sync error",
		// 	update:                           nil,
		// 	syncReclaimPolicyErr:             false,
		// 	syncReplicaMemberManagerErr:      false,
		// 	syncRedisClusterMemberManagerErr: true,
		// 	syncMetaManagerErr:               false,
		// 	errExpectFn: func(g *GomegaWithT, err error) {
		// 		g.Expect(err).To(HaveOccurred())
		// 		g.Expect(strings.Contains(err.Error(), "redis cluster member manager sync error")).To(Equal(true))
		// 	},
		// },
		{
			name:                             "replica cluster member manager sync error",
			update:                           nil,
			syncReclaimPolicyErr:             false,
			syncReplicaMemberManagerErr:      true,
			syncRedisClusterMemberManagerErr: false,
			syncMetaManagerErr:               false,
			errExpectFn: func(g *GomegaWithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(strings.Contains(err.Error(), "replica cluster member manager sync error")).To(Equal(true))
			},
		},
		{
			name:                             "meta manager sync error",
			update:                           nil,
			syncReclaimPolicyErr:             false,
			syncReplicaMemberManagerErr:      false,
			syncRedisClusterMemberManagerErr: false,
			syncMetaManagerErr:               true,
			errExpectFn: func(g *GomegaWithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(strings.Contains(err.Error(), "meta manager sync error")).To(Equal(true))
			},
		},
		{
			name: "normal",
			update: func(redi *v1alpha1.RedisCluster) {
				redi.Status.Redis.Masters[0] = v1alpha1.RedisMember{Name: "demo-redis-0", ID: "dd"}
				redi.Status.Redis.StatefulSet = &apps.StatefulSetStatus{ReadyReplicas: 2}
			},
			syncReclaimPolicyErr:             false,
			syncReplicaMemberManagerErr:      false,
			syncRedisClusterMemberManagerErr: false,
			syncMetaManagerErr:               false,
			errExpectFn: func(g *GomegaWithT, err error) {
				g.Expect(err).NotTo(HaveOccurred())
			},
		},
	}

	for i := range tests {
		testFn(&tests[i], t)
	}

}

func TestRedisStatusEquality(t *testing.T) {
	g := NewGomegaWithT(t)
	rcStatus := v1alpha1.RedisClusterStatus{}

	rcStatusCopy := rcStatus.DeepCopy()
	rcStatusCopy.Redis = v1alpha1.ServerStatus{}
	rcStatusCopy.Sentinel = v1alpha1.SentinelStatus{}
	g.Expect(apiequality.Semantic.DeepEqual(&rcStatus, rcStatusCopy)).To(Equal(true))

	rcStatusCopy = rcStatus.DeepCopy()
	rcStatusCopy.Redis.Phase = v1alpha1.NormalPhase
	rcStatusCopy.Sentinel.Phase = v1alpha1.NormalPhase
	g.Expect(apiequality.Semantic.DeepEqual(&rcStatus, rcStatusCopy)).To(Equal(false))
}

func newFakeRedisControl() (ControlInterface, *meta.FakeReclaimPolicyManager, *mm.FakeReplicaMemberManager, *mm.FakeRedisClusterMemberManager, *meta.FakeMetaManager) {
	cli := fake.NewSimpleClientset()
	rcInformer := informers.NewSharedInformerFactory(cli, 0).Anywhy().V1alpha1().RedisClusters()
	recorder := record.NewFakeRecorder(10)

	rcControl := controller.NewFakeFakeRedisControl(rcInformer)
	replicaMemberManager := mm.NewFakeReplicaMemberManager()
	redisClusterMemberManager := mm.NewFakeRedisClusterMemberManager()
	reclaimPolicyManager := meta.NewFakeReclaimPolicyManager()
	metaManager := meta.NewFakeMetaManager()

	control := NewDefaultRedisControl(rcControl, replicaMemberManager, redisClusterMemberManager, reclaimPolicyManager, metaManager, recorder)

	return control, reclaimPolicyManager, replicaMemberManager, redisClusterMemberManager, metaManager
}

func newRedisForRedisControl() *v1alpha1.RedisCluster {
	return &v1alpha1.RedisCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Redis",
			APIVersion: "anywhy.github/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-redis",
			Namespace: corev1.NamespaceDefault,
			UID:       types.UID("test"),
		},
		Spec: v1alpha1.RedisClusterSpec{
			Mode: "replica",
			Redis: v1alpha1.ServerSpec{
				Replicas: 2,
			},
		},
	}
}
