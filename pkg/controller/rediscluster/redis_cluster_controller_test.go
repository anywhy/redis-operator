package rediscluster

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubeinformers "k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/client/clientset/versioned/fake"
	informers "github.com/anywhy/redis-operator/pkg/client/informers/externalversions"
	"github.com/anywhy/redis-operator/pkg/controller"
	mm "github.com/anywhy/redis-operator/pkg/manager/member"
	"github.com/anywhy/redis-operator/pkg/manager/meta"
)

func TestRedisControllerEnqueueRedis(t *testing.T) {
	g := NewGomegaWithT(t)
	rc := newRedis()
	rcc, _, _ := newFakeRedisController()

	rcc.enqueueRedis(rc)
	g.Expect(rcc.queue.Len()).To(Equal(1))
}

func TestRedisControllerAddStatefulset(t *testing.T) {
	g := NewGomegaWithT(t)
	type testcase struct {
		name              string
		initSet           func(*v1alpha1.RedisCluster) *apps.StatefulSet
		addRedisToIndexer bool
		expectedLen       int
	}

	testFn := func(test *testcase, t *testing.T) {
		rc := newRedis()
		set := test.initSet(rc)

		rcc, rcIndexer, _ := newFakeRedisController()

		if test.addRedisToIndexer {
			err := rcIndexer.Add(rc)
			g.Expect(err).NotTo(HaveOccurred())
		}
		rcc.addStatefulSet(set)
		g.Expect(rcc.queue.Len()).To(Equal(test.expectedLen))

	}

	tests := []testcase{
		{
			name: "normal",
			initSet: func(rc *v1alpha1.RedisCluster) *apps.StatefulSet {
				return newStatefuSet(rc)
			},
			addRedisToIndexer: true,
			expectedLen:       1,
		},
		{
			name: "have deletionTimestamp",
			initSet: func(rc *v1alpha1.RedisCluster) *apps.StatefulSet {
				set := newStatefuSet(rc)
				set.DeletionTimestamp = &metav1.Time{
					Time: time.Now().Add(30 * time.Second),
				}
				return set
			},
			addRedisToIndexer: true,
			expectedLen:       1,
		},
		{
			name: "with out ownerRef",
			initSet: func(rc *v1alpha1.RedisCluster) *apps.StatefulSet {
				set := newStatefuSet(rc)
				set.OwnerReferences = nil
				return set
			},
			addRedisToIndexer: false,
			expectedLen:       0,
		},

		{
			name: "without redis",
			initSet: func(rc *v1alpha1.RedisCluster) *apps.StatefulSet {
				return newStatefuSet(rc)
			},
			addRedisToIndexer: false,
			expectedLen:       0,
		},
	}

	for _, test := range tests {
		testFn(&test, t)
	}
}

func TestRedisControllerUpdateStatefulset(t *testing.T) {
	g := NewGomegaWithT(t)
	type testcase struct {
		name              string
		initSet           func(*v1alpha1.RedisCluster) *apps.StatefulSet
		updateSet         func(*apps.StatefulSet) *apps.StatefulSet
		addRedisToIndexer bool
		expectedLen       int
	}

	testFn := func(test *testcase, t *testing.T) {
		rc := newRedis()
		set := test.initSet(rc)
		newSet := test.updateSet(set)

		rcc, rcIndexer, _ := newFakeRedisController()

		if test.addRedisToIndexer {
			err := rcIndexer.Add(rc)
			g.Expect(err).NotTo(HaveOccurred())
		}
		rcc.updateStatefuSet(set, newSet)
		g.Expect(rcc.queue.Len()).To(Equal(test.expectedLen))

	}

	tests := []testcase{
		{
			name: "normal",
			initSet: func(rc *v1alpha1.RedisCluster) *apps.StatefulSet {
				return newStatefuSet(rc)
			},
			updateSet: func(set *apps.StatefulSet) *apps.StatefulSet {
				newSet := *set
				newSet.ResourceVersion = "100"
				return &newSet
			},
			addRedisToIndexer: true,
			expectedLen:       1,
		},
		{
			name: "same resouceVersion",
			initSet: func(rc *v1alpha1.RedisCluster) *apps.StatefulSet {
				set := newStatefuSet(rc)
				return set
			},
			updateSet: func(set *apps.StatefulSet) *apps.StatefulSet {
				newSet := *set
				return &newSet
			},
			addRedisToIndexer: true,
			expectedLen:       0,
		},
		{
			name: "with out ownerRef",
			initSet: func(rc *v1alpha1.RedisCluster) *apps.StatefulSet {
				set := newStatefuSet(rc)
				set.OwnerReferences = nil
				return set
			},
			updateSet: func(set *apps.StatefulSet) *apps.StatefulSet {
				newSet := *set
				newSet.ResourceVersion = "100"
				return &newSet
			},
			addRedisToIndexer: false,
			expectedLen:       0,
		},

		{
			name: "without redis",
			initSet: func(rc *v1alpha1.RedisCluster) *apps.StatefulSet {
				return newStatefuSet(rc)
			},
			updateSet: func(set *apps.StatefulSet) *apps.StatefulSet {
				newSet := *set
				newSet.ResourceVersion = "100"
				return &newSet
			},
			addRedisToIndexer: false,
			expectedLen:       0,
		},
	}

	for _, test := range tests {
		testFn(&test, t)
	}
}

func newFakeRedisController() (*Controller, cache.Indexer, cache.Indexer) {

	cli := fake.NewSimpleClientset()
	kubeCli := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cli, 0)
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeCli, 0)

	recorder := record.NewFakeRecorder(10)

	rcInformer := informerFactory.Anywhy().V1alpha1().RedisClusters()
	setInformer := kubeInformerFactory.Apps().V1beta1().StatefulSets()
	svcInformer := kubeInformerFactory.Core().V1().Services()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes()
	podInformer := kubeInformerFactory.Core().V1().Pods()
	// nodeInformer := kubeInformerFactory.Core().V1().Nodes()

	rcControl := controller.NewRealRedisControl(cli, rcInformer.Lister(), recorder)
	setControl := controller.NewRealStatefuSetControl(kubeCli, setInformer.Lister(), recorder)
	svcControl := controller.NewRealServiceControl(kubeCli, svcInformer.Lister(), recorder)
	podControl := controller.NewRealPodControl(kubeCli, podInformer.Lister(), recorder)
	pvcControl := controller.NewRealPVCControl(kubeCli, recorder, pvcInformer.Lister())
	pvControl := controller.NewRealPVControl(kubeCli, pvcInformer.Lister(), pvInformer.Lister(), recorder)

	replicaScaler := mm.NewFakeReplicaScaler()
	replicaUpgrader := mm.NewFakeReplicaUpgraderr()

	rcc := NewController(
		kubeCli,
		cli,
		informerFactory,
		kubeInformerFactory,
		true,
	)

	rcc.control = NewDefaultRedisControl(
		rcControl,
		mm.NewReplicaMemberManager(
			setControl,
			svcControl,
			svcInformer.Lister(),
			podInformer.Lister(),
			podControl,
			setInformer.Lister(),
			replicaScaler,
			replicaUpgrader),
		mm.NewSentinelMemberManager(
			setControl,
			svcControl,
			svcInformer.Lister(),
			podInformer.Lister(),
			podControl,
			setInformer.Lister()),
		nil,
		meta.NewReclaimPolicyManager(
			pvcInformer.Lister(),
			pvInformer.Lister(),
			pvControl,
		),
		meta.NewMetaManager(
			pvcInformer.Lister(),
			pvcControl,
			pvInformer.Lister(),
			pvControl,
			podInformer.Lister(),
			podControl,
		),
		recorder,
	)

	rcc.rcListerSynced = alwaysReady
	rcc.setListerSynced = alwaysReady
	return rcc, rcInformer.Informer().GetIndexer(), setInformer.Informer().GetIndexer()
}

func newRedis() *v1alpha1.RedisCluster {
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
			Redis: v1alpha1.RedisSpec{
				ContainerSpec: v1alpha1.ContainerSpec{
					Image: "redis:latest",
				},
				Replicas: 2,
			},
		},
	}
}

func newStatefuSet(rc *v1alpha1.RedisCluster) *apps.StatefulSet {
	return &apps.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefuset",
			Namespace: corev1.NamespaceDefault,
			UID:       types.UID("test"),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(rc, controllerKind),
			},
			ResourceVersion: "1",
		},
		Spec: apps.StatefulSetSpec{
			Replicas: &rc.Spec.Redis.Replicas,
		},
	}
}

func alwaysReady() bool { return true }
