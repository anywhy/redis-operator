package controller

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"github.com/anywhy/redis-operator/pkg/label"
)

func TestPVCControlUpdatePVCSuccess(t *testing.T) {
	g := NewGomegaWithT(t)
	fakeClient, pvcLister, _, recorder := newFakeClientAndRecorder()
	rc := newRedis("demo")
	pvc := newPVC(rc)
	control := NewRealPVCControl(fakeClient, recorder, pvcLister)
	fakeClient.AddReactor("update", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), nil
	})
	_, err := control.UpdatePVC(rc, pvc, nil)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestPVCControlUpdatePVCFaild(t *testing.T) {
	g := NewGomegaWithT(t)
	fakeClient, pvcLister, _, recorder := newFakeClientAndRecorder()
	rc := newRedis("demo")
	pvc := newPVC(rc)
	control := NewRealPVCControl(fakeClient, recorder, pvcLister)
	fakeClient.AddReactor("update", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("apiserver is down"))
	})
	_, err := control.UpdatePVC(rc, pvc, nil)
	g.Expect(err).To(HaveOccurred())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeWarning))
}

func TestPVCControlUpdatePVCWithPodSuccess(t *testing.T) {
	g := NewGomegaWithT(t)
	fakeClient, pvcLister, _, recorder := newFakeClientAndRecorder()
	rc := newRedis("demo")
	pvc := newPVC(rc)
	control := NewRealPVCControl(fakeClient, recorder, pvcLister)
	fakeClient.AddReactor("update", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), nil
	})

	pod := newPod(rc)
	updatePVC, err := control.UpdatePVC(rc, pvc, pod)
	g.Expect(err).To(Succeed())
	g.Expect(updatePVC.GetAnnotations()[label.AnnPodNameKey]).To(Equal(pod.Name))

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestPVCControlUpdatePVCConflictSuccess(t *testing.T) {
	g := NewGomegaWithT(t)
	fakeClient, pvcLister, _, recorder := newFakeClientAndRecorder()
	rc := newRedis("demo")
	pvc := newPVC(rc)
	control := NewRealPVCControl(fakeClient, recorder, pvcLister)
	conflict := false
	fakeClient.AddReactor("update", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		if !conflict {
			conflict = true
			return true, nil, apierrors.NewConflict(action.GetResource().GroupResource(), pvc.Name, errors.New("conflict"))
		}
		return true, update.GetObject(), nil
	})
	_, err := control.UpdatePVC(rc, pvc, nil)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestPVCControlDeletePVCSuccess(t *testing.T) {
	g := NewGomegaWithT(t)
	fakeClient, pvcLister, _, recorder := newFakeClientAndRecorder()
	rc := newRedis("demo")
	pvc := newPVC(rc)
	control := NewRealPVCControl(fakeClient, recorder, pvcLister)
	fakeClient.AddReactor("delete", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})

	err := control.DeletePVC(rc, pvc)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestPVCControlDeletePVCFaild(t *testing.T) {
	g := NewGomegaWithT(t)
	fakeClient, pvcLister, _, recorder := newFakeClientAndRecorder()
	rc := newRedis("demo")
	pvc := newPVC(rc)
	control := NewRealPVCControl(fakeClient, recorder, pvcLister)
	fakeClient.AddReactor("delete", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("apiserver is down"))
	})

	err := control.DeletePVC(rc, pvc)
	g.Expect(err).To(HaveOccurred())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeWarning))
}

func newFakeClientAndRecorder() (*fake.Clientset, corelisters.PersistentVolumeClaimLister, cache.Indexer, *record.FakeRecorder) {
	kubeCli := &fake.Clientset{}
	recorder := record.NewFakeRecorder(10)
	pvcInformer := kubeinformers.NewSharedInformerFactory(kubeCli, 0).Core().V1().PersistentVolumeClaims()
	return kubeCli, pvcInformer.Lister(), pvcInformer.Informer().GetIndexer(), recorder
}

func newPVC(rc *v1alpha1.RedisCluster) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-pvc-1",
			Namespace: corev1.NamespaceDefault,
			UID:       types.UID("pvc-demo"),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "demo-pvc-1",
		},
	}
}
