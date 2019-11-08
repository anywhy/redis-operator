package member

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
)

func TestReplicaMemberManagerSyncCreate(t *testing.T) {
	g := NewGomegaWithT(t)

	type testcase struct {
		name                            string
		prepare                         func(cluster *v1alpha1.RedisCluster)
		errWhenCreateStatefulSet        bool
		errWhenCreateReplicaPeerService bool
		err                             bool
		errExpectFn                     func(*GomegaWithT, error)
		relipaMasterSvcCreated          bool
		rplicaMasterPeerSvcCreated      bool
		relipaSlaveSvcCreated           bool
		relipaSlavePeerSvcCreated       bool
		setCreated                      bool
	}

	testFn := func(test *testcase, t *testing.T) {
		t.Log(test.name)

		rc := newRedisForReplica()
		ns, rcName := rc.Namespace, rc.Name
		oldSpec := rc.Spec
		if test.prepare != nil {
			test.prepare(rc)
		}

		rc.Status.Redis.StatefulSet = &apps.StatefulSetStatus{ReadyReplicas: 2}
		rmm, fakeSetControl, fakeSvcControl, _, _ := newFakeReplicaMemberManager()
		if test.errWhenCreateStatefulSet {
			fakeSetControl.SetCreateStatefulSetError(errors.NewInternalError(fmt.Errorf("API server failed")), 0)
		}

		if test.errWhenCreateReplicaPeerService {
			fakeSvcControl.SetCreateServiceError(errors.NewInternalError(fmt.Errorf("API server failed")), 0)
		}

		err := rmm.Sync(rc)
		test.errExpectFn(g, err)
		g.Expect(rc.Spec).To(Equal(oldSpec))

		svc, err := rmm.svcLister.Services(ns).Get(controller.RedisMemberName(rcName) + "-master")
		if test.relipaMasterSvcCreated {
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(svc).NotTo(Equal(nil))
		} else {
			expectErrIsNotFound(g, err)
		}

		svc, err = rmm.svcLister.Services(ns).Get(controller.RedisMemberName(rcName) + "-master-peer")
		if test.rplicaMasterPeerSvcCreated {
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(svc).NotTo(Equal(nil))
		} else {
			expectErrIsNotFound(g, err)
		}

		svc, err = rmm.svcLister.Services(ns).Get(controller.RedisMemberName(rcName) + "-slave")
		if test.relipaSlavePeerSvcCreated {
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(svc).NotTo(Equal(nil))
		} else {
			expectErrIsNotFound(g, err)
		}
		svc, err = rmm.svcLister.Services(ns).Get(controller.RedisMemberName(rcName) + "-slave-peer")
		if test.relipaSlavePeerSvcCreated {
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(svc).NotTo(Equal(nil))
		} else {
			expectErrIsNotFound(g, err)
		}

		set, err := rmm.setLister.StatefulSets(ns).Get(controller.RedisMemberName(rcName))
		if test.setCreated {
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(set).NotTo(Equal(nil))
		} else {
			expectErrIsNotFound(g, err)
		}
	}

	tests := []testcase{
		{
			name:                            "normal",
			prepare:                         nil,
			errWhenCreateStatefulSet:        false,
			err:                             false,
			errExpectFn:                     errExpectRequeue,
			errWhenCreateReplicaPeerService: false,
			relipaMasterSvcCreated:          true,
			rplicaMasterPeerSvcCreated:      true,
			relipaSlaveSvcCreated:           true,
			relipaSlavePeerSvcCreated:       true,
			setCreated:                      true,
		},
		{
			name:                     "error when create statefulset",
			prepare:                  nil,
			errWhenCreateStatefulSet: true,
			err:                      false,
			errExpectFn: func(g *GomegaWithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(strings.Contains(err.Error(), "API server failed")).To(BeTrue())
			},
			errWhenCreateReplicaPeerService: false,
			relipaMasterSvcCreated:          true,
			rplicaMasterPeerSvcCreated:      true,
			relipaSlaveSvcCreated:           true,
			relipaSlavePeerSvcCreated:       true,
			setCreated:                      false,
		},
		{
			name:                     "error when create master service",
			prepare:                  nil,
			errWhenCreateStatefulSet: false,
			err:                      false,
			errExpectFn: func(g *GomegaWithT, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(strings.Contains(err.Error(), "API server failed")).To(BeTrue())
			},
			errWhenCreateReplicaPeerService: true,
			relipaMasterSvcCreated:          false,
			rplicaMasterPeerSvcCreated:      false,
			relipaSlaveSvcCreated:           false,
			relipaSlavePeerSvcCreated:       false,
			setCreated:                      false,
		},

		{
			name: " storage format is wrong",
			prepare: func(tc *v1alpha1.RedisCluster) {
				tc.Spec.Redis.Requests.Storage = "10xxxxi"
			},
			errWhenCreateStatefulSet: false,
			err:                      false,
			errExpectFn: func(g *GomegaWithT, err error) {
				g.Expect(err).To(HaveOccurred())
				//fmt.Printf("%v", err.Error())
				g.Expect(strings.Contains(err.Error(), "cant' get storage request size: 10xxxxi for Redis: default/demo")).To(BeTrue())
			},
			errWhenCreateReplicaPeerService: false,
			relipaMasterSvcCreated:          true,
			rplicaMasterPeerSvcCreated:      true,
			relipaSlaveSvcCreated:           true,
			relipaSlavePeerSvcCreated:       true,
			setCreated:                      false,
		},
	}

	for _, test := range tests {
		testFn(&test, t)
	}
}

func newFakeReplicaMemberManager() (*replicaMemberManager, *controller.FakeStatefulSetControl, *controller.FakeServiceControl, cache.Indexer, cache.Indexer) {
	cli := fake.NewSimpleClientset()
	kubeCli := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cli, 0)
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeCli, 0)

	recorder := record.NewFakeRecorder(10)

	rcInformer := informerFactory.Anywhy().V1alpha1().RedisClusters()
	setInformer := kubeInformerFactory.Apps().V1beta1().StatefulSets()
	svcInformer := kubeInformerFactory.Core().V1().Services()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	podInformer := kubeInformerFactory.Core().V1().Pods()
	// nodeInformer := kubeInformerFactory.Core().V1().Nodes()

	setControl := controller.NewFakeStatefulSetControl(setInformer, rcInformer)
	svcControl := controller.NewFakeServiceControl(svcInformer, rcInformer)
	podControl := controller.NewRealPodControl(kubeCli, podInformer.Lister(), recorder)

	replicaScaler := NewFakeReplicaScaler()
	replicaUpgrader := NewFakeReplicaUpgraderr()
	replicaFailover := NewFakeReplicaFailover()

	return &replicaMemberManager{
		setControl,
		svcControl,
		svcInformer.Lister(),
		setInformer.Lister(),
		podInformer.Lister(),
		podControl,
		replicaScaler,
		replicaUpgrader,
		replicaFailover,
	}, setControl, svcControl, podInformer.Informer().GetIndexer(), pvcInformer.Informer().GetIndexer()

}

func newRedisForReplica() *v1alpha1.RedisCluster {
	return &v1alpha1.RedisCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Redis",
			APIVersion: "anywhy.github.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: corev1.NamespaceDefault,
			UID:       types.UID("test"),
		},
		Spec: v1alpha1.RedisClusterSpec{
			Mode: "replica",
			Redis: v1alpha1.ServerSpec{
				ContainerSpec: v1alpha1.ContainerSpec{
					Image: "test-image",
					Requests: &v1alpha1.ResourceRequirement{
						CPU:     "1",
						Memory:  "1Gi",
						Storage: "1Gi",
					},
				},
				Replicas:         2,
				StorageClassName: "my-storage-class",
			},
		},
	}
}

func expectErrIsNotFound(g *GomegaWithT, err error) {
	g.Expect(err).NotTo(Equal(nil))
	g.Expect(errors.IsNotFound(err)).To(Equal(true))
}

func errExpectRequeue(g *GomegaWithT, err error) {
	g.Expect(controller.IsRequeueError(err)).To(Equal(true))
}
