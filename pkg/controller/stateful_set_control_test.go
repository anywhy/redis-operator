package controller

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	appslisters "k8s.io/client-go/listers/apps/v1beta1"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

func TestServiceControlCreateStatefullSet(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")
	ss := newStatefulSet(rc, "group1")
	fakeClient := &fake.Clientset{}
	control := NewRealStatefuSetControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("create", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		create := action.(core.CreateAction)
		return true, create.GetObject(), nil
	})

	err := control.CreateStatefulSet(rc, ss)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestServiceControlCreateStatefullSetExits(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")
	ss := newStatefulSet(rc, "group1")
	fakeClient := &fake.Clientset{}
	control := NewRealStatefuSetControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("create", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		return true, ss, apierrors.NewAlreadyExists(action.GetResource().GroupResource(), ss.Name)
	})

	err := control.CreateStatefulSet(rc, ss)
	g.Expect(err).To(Succeed())
}

func TestServiceControlCreateStatefullSetExitsNotConrollBySameCluster(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")
	rc1 := newRedisCluster("redis-demo1")

	rc.UID = "123"
	rc1.UID = "abc"
	ss := newStatefulSet(rc1, "group1")
	fakeClient := &fake.Clientset{}
	control := NewRealStatefuSetControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("create", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		return true, ss, apierrors.NewAlreadyExists(action.GetResource().GroupResource(), ss.Name)
	})

	err := control.CreateStatefulSet(rc, ss)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring("already exists and is not managed by RedisCluster"))
}

func TestServiceControlCreateStatefullSetFailed(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")

	ss := newStatefulSet(rc, "group1")
	fakeClient := &fake.Clientset{}
	control := NewRealStatefuSetControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("create", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("apiserver is down"))
	})
	err := control.CreateStatefulSet(rc, ss)
	g.Expect(err).To(HaveOccurred())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeWarning))
}

func TestServiceControlUpdateStatefullSet(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")

	ss := newStatefulSet(rc, "group1")
	ss.Spec.ServiceName = "aa"
	fakeClient := &fake.Clientset{}
	control := NewRealStatefuSetControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("update", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), nil
	})
	updateDep, err := control.UpdateStatefulSet(rc, ss)
	g.Expect(err).To(Succeed())
	g.Expect(updateDep.Spec.ServiceName).To(Equal("aa"))

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestServiceControlUpdateStatefullSetConflictSuccess(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")
	ss := newStatefulSet(rc, "group1")
	ss.Spec.ServiceName = "aa"

	fakeClient := &fake.Clientset{}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	ssOld := newStatefulSet(rc, "group2")
	ssOld.Spec.ServiceName = "bb"

	err := indexer.Add(ssOld)
	g.Expect(err).To(Succeed())

	ssLister := appslisters.NewStatefulSetLister(indexer)
	control := NewRealStatefuSetControl(fakeClient, ssLister, recorder)
	conflict := false
	fakeClient.AddReactor("update", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		if !conflict {
			conflict = true
			return true, ssOld, apierrors.NewConflict(action.GetResource().GroupResource(), ss.Name, errors.New("conflict"))
		}
		return true, update.GetObject(), nil
	})
	updateDep, err := control.UpdateStatefulSet(rc, ss)
	g.Expect(err).To(Succeed())
	g.Expect(updateDep.Spec.ServiceName).To(Equal("aa"))

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestServiceControlDeleteStatefullSet(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")

	ss := newStatefulSet(rc, "group1")
	fakeClient := &fake.Clientset{}
	control := NewRealStatefuSetControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("delete", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	err := control.DeleteStatefulSet(rc, ss)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestServiceControlDeleteStatefullSetFaild(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")

	ss := newStatefulSet(rc, "group1")
	fakeClient := &fake.Clientset{}
	control := NewRealStatefuSetControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("delete", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("apiserver is down"))
	})
	err := control.DeleteStatefulSet(rc, ss)
	g.Expect(err).To(HaveOccurred())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeWarning))
}
