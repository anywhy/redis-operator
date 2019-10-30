package meta

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubeinformers "k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/controller"
	"github.com/anywhy/redis-operator/pkg/label"
)

func TestMetaManagerSync(t *testing.T) {
	g := NewGomegaWithT(t)

	type testcase struct {
		name             string
		podHasLabels     bool
		pvcHasLabels     bool
		pvcHasVolumeName bool
		podRefPvc        bool
		podUpdateErr     bool
		getClusterErr    bool
		pvcUpdateErr     bool
		pvUpdateErr      bool
		podChanged       bool
		pvcChanged       bool
		pvChanged        bool
	}

	testFn := func(test *testcase, t *testing.T) {
		t.Log(test.name)

		rc := newRedisClusterForMeta()
		pod1 := newPod(rc)

		ns := rc.GetNamespace()
		pv1 := newPV()
		pvc1 := newPVC(rc)

		if !test.podHasLabels {
			pod1.Labels = nil
		}
		if !test.pvcHasLabels {
			pvc1.Labels = nil
		}
		if !test.pvcHasVolumeName {
			pvc1.Spec.VolumeName = ""
		}

		if !test.podRefPvc {
			pod1.Spec = newPodSpec(v1alpha1.RedisMemberType.String(), pvc1.GetName())
		}
		mm, fakePodControl, fakePVCControl, fakePVControl, podIndexer, pvcIndexer, pvIndexer := newFakeMetaManager()
		err := podIndexer.Add(pod1)
		g.Expect(err).NotTo(HaveOccurred())
		err = pvIndexer.Add(pv1)
		g.Expect(err).NotTo(HaveOccurred())
		err = pvcIndexer.Add(pvc1)
		g.Expect(err).NotTo(HaveOccurred())

		if test.podUpdateErr {
			fakePodControl.SetUpdatePodError(errors.NewInternalError(fmt.Errorf("API server failed")), 0)
		}

		if test.getClusterErr {
			fakePodControl.SetGetClusterError(errors.NewInternalError(fmt.Errorf("Redis server failed")), 0)
		}

		if test.pvcUpdateErr {
			fakePVCControl.SetUpdatePVCError(errors.NewInternalError(fmt.Errorf("API server failed")), 0)
		}
		if test.pvUpdateErr {
			fakePVControl.SetUpdatePVError(errors.NewInternalError(fmt.Errorf("API server failed")), 0)
		}

		err = mm.Sync(rc)
		if test.podUpdateErr || test.getClusterErr {
			g.Expect(err).To(HaveOccurred())

			_, err := mm.podLister.Pods(ns).Get(pod1.Name)
			g.Expect(err).NotTo(HaveOccurred())

			pvc, err := mm.pvcLister.PersistentVolumeClaims(ns).Get(pvc1.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pvcMetaInfoMatchDesire(pvc)).To(Equal(false))

			pv, err := mm.pvLister.Get(pv1.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pvMetaInfoMatchDesire(ns, pv)).To(Equal(false))
		}

		if test.pvcUpdateErr {
			g.Expect(err).To(HaveOccurred())

			pvc, err := mm.pvcLister.PersistentVolumeClaims(ns).Get(pvc1.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pvcMetaInfoMatchDesire(pvc)).To(Equal(false))

			pv, err := mm.pvLister.Get(pv1.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pvMetaInfoMatchDesire(ns, pv)).To(Equal(false))
		}

		if test.pvUpdateErr {
			g.Expect(err).To(HaveOccurred())

			pv, err := mm.pvLister.Get(pv1.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pvMetaInfoMatchDesire(ns, pv)).To(Equal(false))
		}

		if test.podChanged {
			_, err := mm.podLister.Pods(ns).Get(pod1.Name)
			g.Expect(err).NotTo(HaveOccurred())
		} else {
			_, err := mm.podLister.Pods(ns).Get(pod1.Name)
			g.Expect(err).NotTo(HaveOccurred())
		}

		if test.pvcChanged {
			pvc, err := mm.pvcLister.PersistentVolumeClaims(ns).Get(pvc1.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pvcMetaInfoMatchDesire(pvc)).To(Equal(true))
		} else {
			pvc, err := mm.pvcLister.PersistentVolumeClaims(ns).Get(pvc1.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pvcMetaInfoMatchDesire(pvc)).To(Equal(false))
		}

		if test.pvChanged {
			g.Expect(err).NotTo(HaveOccurred())
			pv, err := mm.pvLister.Get(pv1.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pvMetaInfoMatchDesire(ns, pv)).To(Equal(true))
		} else {
			pv, err := mm.pvLister.Get(pv1.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pvMetaInfoMatchDesire(ns, pv)).To(Equal(false))
		}

	}

	tests := []testcase{
		{
			name:             "normal",
			podHasLabels:     true,
			pvcHasLabels:     true,
			pvcHasVolumeName: true,
			podRefPvc:        true,
			podUpdateErr:     false,
			getClusterErr:    false,
			pvcUpdateErr:     false,
			pvUpdateErr:      false,
			podChanged:       true,
			pvcChanged:       true,
			pvChanged:        true,
		},
		{
			name:             "pod don't have labels",
			podHasLabels:     false,
			pvcHasLabels:     true,
			pvcHasVolumeName: true,
			podRefPvc:        true,
			podUpdateErr:     false,
			getClusterErr:    false,
			pvcUpdateErr:     false,
			pvUpdateErr:      false,
			podChanged:       false,
			pvcChanged:       false,
			pvChanged:        false,
		},

		{
			name:             "pvc don't have labels",
			podHasLabels:     true,
			pvcHasLabels:     false,
			pvcHasVolumeName: true,
			podRefPvc:        true,
			podUpdateErr:     false,
			getClusterErr:    false,
			pvcUpdateErr:     false,
			pvUpdateErr:      false,
			podChanged:       true,
			pvcChanged:       true,
			pvChanged:        true,
		},
		{
			name:             "pvc don't have volume",
			podHasLabels:     true,
			pvcHasLabels:     true,
			pvcHasVolumeName: false,
			podRefPvc:        true,
			podUpdateErr:     false,
			getClusterErr:    false,
			pvcUpdateErr:     false,
			pvUpdateErr:      false,
			podChanged:       true,
			pvcChanged:       true,
			pvChanged:        false,
		},
		{
			name:             "update pod error",
			podHasLabels:     true,
			pvcHasLabels:     true,
			pvcHasVolumeName: true,
			podRefPvc:        true,
			podUpdateErr:     true,
			getClusterErr:    false,
			pvcUpdateErr:     false,
			pvUpdateErr:      false,
			podChanged:       false,
			pvcChanged:       false,
			pvChanged:        false,
		},
		{
			name:             "get cluster error",
			podHasLabels:     true,
			pvcHasLabels:     true,
			pvcHasVolumeName: true,
			podRefPvc:        true,
			podUpdateErr:     true,
			getClusterErr:    false,
			pvcUpdateErr:     false,
			pvUpdateErr:      false,
			podChanged:       false,
			pvcChanged:       false,
			pvChanged:        false,
		},
		{
			name:             "update pvc error",
			podHasLabels:     true,
			pvcHasLabels:     true,
			pvcHasVolumeName: true,
			podRefPvc:        true,
			podUpdateErr:     false,
			getClusterErr:    false,
			pvcUpdateErr:     true,
			pvUpdateErr:      false,
			podChanged:       false,
			pvcChanged:       false,
			pvChanged:        false,
		},
		{
			name:             "update pv error",
			podHasLabels:     true,
			pvcHasLabels:     true,
			pvcHasVolumeName: true,
			podRefPvc:        true,
			podUpdateErr:     false,
			getClusterErr:    false,
			pvcUpdateErr:     false,
			pvUpdateErr:      true,
			podChanged:       true,
			pvcChanged:       true,
			pvChanged:        false,
		},
	}

	for i := range tests {
		testFn(&tests[i], t)
	}
}

func newFakeMetaManager() (
	*metaManager,
	*controller.FakePodControl,
	*controller.FakePVCControl,
	*controller.FakePVControl,
	cache.Indexer,
	cache.Indexer,
	cache.Indexer,
) {
	kubeCli := kubefake.NewSimpleClientset()

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeCli, 0)
	podInformer := kubeInformerFactory.Core().V1().Pods()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes()

	podControl := controller.NewFakePodControl(podInformer)
	pvcControl := controller.NewFakePVCControl(pvcInformer)
	pvControl := controller.NewFakePVControl(pvInformer, pvcInformer)

	return &metaManager{
			pvcLister:  pvcInformer.Lister(),
			pvcControl: pvcControl,
			pvLister:   pvInformer.Lister(),
			pvControl:  pvControl,
			podLister:  podInformer.Lister(),
			podControl: podControl,
		}, podControl, pvcControl, pvControl, podInformer.Informer().GetIndexer(),
		pvcInformer.Informer().GetIndexer(), pvInformer.Informer().GetIndexer()
}

func pvcMetaInfoMatchDesire(pvc *corev1.PersistentVolumeClaim) bool {
	return pvc.Annotations[label.AnnPodNameKey] == controller.TestPodName
}

func pvMetaInfoMatchDesire(ns string, pv *corev1.PersistentVolume) bool {
	return pv.Labels[label.NamespaceLabelKey] == ns &&
		pv.Annotations[label.AnnPodNameKey] == controller.TestPodName
}

func newPod(rc *v1alpha1.RedisCluster) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      controller.TestPodName,
			Namespace: corev1.NamespaceDefault,
			UID:       types.UID("test"),
			Labels: map[string]string{
				label.NameLabelKey:        controller.TestName,
				label.ComponentLabelKey:   controller.TestComponentName,
				label.ManagedByLabelKey:   controller.TestManagedByName,
				label.InstanceLabelKey:    rc.GetLabels()[label.InstanceLabelKey],
				label.ClusterModeLabelKey: label.ReplicaClusterLabelKey,
			},
		},
		Spec: newPodSpec(v1alpha1.RedisMemberType.String(), "pvc-1"),
	}
}

func newPodSpec(volumeName, pvcName string) corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "container",
				Image: "test",
				VolumeMounts: []corev1.VolumeMount{
					{Name: volumeName, MountPath: "/var/tmp/test"},
				},
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
					},
				},
			},
		},
	}
}
