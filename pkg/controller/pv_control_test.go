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
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
)

func TestPVControlPatchPVReclaimPolicySuccess(t *testing.T) {
	g := NewGomegaWithT(t)
	fakeClient, pvcInformer, pvInformer, recorder := newFakeRecorderAndPVCInformer()
	rc := newRedis("demo")
	pv := newPV()
	control := NewRealPVControl(fakeClient, pvcInformer.Lister(), pvInformer.Lister(), recorder)
	fakeClient.AddReactor("patch", "persistentvolumes", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	err := control.PatchPVReclaimPolicy(rc, pv, rc.Spec.Redis.PVReclaimPolicy)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestPVControlPatchPVReclaimPolicyFaild(t *testing.T) {
	g := NewGomegaWithT(t)
	fakeClient, pvcInformer, pvInformer, recorder := newFakeRecorderAndPVCInformer()
	rc := newRedis("demo")
	pv := newPV()
	control := NewRealPVControl(fakeClient, pvcInformer.Lister(), pvInformer.Lister(), recorder)
	fakeClient.AddReactor("patch", "persistentvolumes", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("apiserver is down"))
	})

	err := control.PatchPVReclaimPolicy(rc, pv, rc.Spec.Redis.PVReclaimPolicy)
	g.Expect(err).To(HaveOccurred())
	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeWarning))
}

func TestPVControlPatchPVReclaimPolicyConflictSuccess(t *testing.T) {
	g := NewGomegaWithT(t)
	fakeClient, pvcInformer, pvInformer, recorder := newFakeRecorderAndPVCInformer()
	rc := newRedis("demo")
	pv := newPV()
	control := NewRealPVControl(fakeClient, pvcInformer.Lister(), pvInformer.Lister(), recorder)

	conflict := false
	fakeClient.AddReactor("patch", "persistentvolumes", func(action core.Action) (bool, runtime.Object, error) {
		if !conflict {
			conflict = true
			return true, nil, apierrors.NewConflict(action.GetResource().GroupResource(), pv.Name, errors.New("conflict"))
		}
		return true, nil, nil
	})
	err := control.PatchPVReclaimPolicy(rc, pv, rc.Spec.Redis.PVReclaimPolicy)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestPVControlUpdatePVSuccess(t *testing.T) {
	g := NewGomegaWithT(t)
	fakeClient, pvcInformer, pvInformer, recorder := newFakeRecorderAndPVCInformer()
	rc := newRedis("demo")
	pv := newPV()
	pv.Annotations = map[string]string{"a": "b"}
	pvc := newPVC(rc)
	pvcInformer.Informer().GetIndexer().Add(pvc)
	control := NewRealPVControl(fakeClient, pvcInformer.Lister(), pvInformer.Lister(), recorder)
	fakeClient.AddReactor("get", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	fakeClient.AddReactor("update", "persistentvolumes", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), nil
	})

	updatePV, err := control.UpdatePV(rc, pv)
	g.Expect(err).To(Succeed())
	g.Expect(updatePV.Annotations["a"]).To(Equal("b"))

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestPVControlUpdatePVConflictSuccess(t *testing.T) {
	g := NewGomegaWithT(t)
	fakeClient, pvcInformer, pvInformer, recorder := newFakeRecorderAndPVCInformer()
	rc := newRedis("demo")
	pv := newPV()
	pv.Annotations = map[string]string{"a": "b"}
	pvc := newPVC(rc)

	oldPV := newPV()
	pvcInformer.Informer().GetIndexer().Add(pvc)
	pvInformer.Informer().GetIndexer().Add(oldPV)
	control := NewRealPVControl(fakeClient, pvcInformer.Lister(), pvInformer.Lister(), recorder)
	fakeClient.AddReactor("get", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	conflict := false
	fakeClient.AddReactor("update", "persistentvolumes", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		if !conflict {
			conflict = true
			return true, oldPV, apierrors.NewConflict(action.GetResource().GroupResource(), pv.Name, errors.New("conflict"))
		}
		return true, update.GetObject(), nil
	})

	updatePV, err := control.UpdatePV(rc, pv)
	g.Expect(err).To(Succeed())
	g.Expect(updatePV.Annotations["a"]).To(Equal("b"))

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestPVControlUpdatePVFaild(t *testing.T) {
	g := NewGomegaWithT(t)
	fakeClient, pvcInformer, pvInformer, recorder := newFakeRecorderAndPVCInformer()
	rc := newRedis("demo")
	pv := newPV()
	pv.Annotations = map[string]string{"a": "b"}
	pvc := newPVC(rc)
	pvcInformer.Informer().GetIndexer().Add(pvc)
	control := NewRealPVControl(fakeClient, pvcInformer.Lister(), pvInformer.Lister(), recorder)
	fakeClient.AddReactor("get", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	fakeClient.AddReactor("update", "persistentvolumes", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), apierrors.NewInternalError(errors.New("apiserver is down"))
	})

	_, err := control.UpdatePV(rc, pv)
	g.Expect(err).To(HaveOccurred())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeWarning))
}

func newFakeRecorderAndPVCInformer() (*fake.Clientset, coreinformers.PersistentVolumeClaimInformer, coreinformers.PersistentVolumeInformer, *record.FakeRecorder) {
	fakeClient := &fake.Clientset{}
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(fakeClient, 0)
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes()
	recorder := record.NewFakeRecorder(10)
	return fakeClient, pvcInformer, pvInformer, recorder
}

func newPV() *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-pv-1",
			Namespace: "",
			UID:       types.UID("pv-demo"),
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			ClaimRef: &corev1.ObjectReference{
				APIVersion: "v1",
				Kind:       "PersistentVolumeClaim",
				Name:       "demo-pvc-1",
				Namespace:  corev1.NamespaceDefault,
				UID:        types.UID("pvc-demo"),
			},
		},
	}
}
