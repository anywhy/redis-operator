package controller

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

func TestServiceControlCreatesServices(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedis("redis-demo")
	svc := newService(rc, "master")
	fakeClient := &fake.Clientset{}
	control := NewRealServiceControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("create", "services", func(action core.Action) (bool, runtime.Object, error) {
		create := action.(core.CreateAction)
		return true, create.GetObject(), nil
	})

	err := control.CreateService(rc, svc)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestServiceControlCreatesServicesExits(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedis("redis-demo")
	svc := newService(rc, "master")
	fakeClient := &fake.Clientset{}
	control := NewRealServiceControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("create", "services", func(action core.Action) (bool, runtime.Object, error) {
		return true, svc, apierrors.NewAlreadyExists(action.GetResource().GroupResource(), svc.Name)
	})

	err := control.CreateService(rc, svc)
	g.Expect(err).To(Succeed())

	// events := collectEvents(recorder.Events)
	// g.Expect(events).To(HaveLen(1))
	// g.Expect(events[0]).To(ContainSubstring("already exists and is not managed by Redis"))
}

func TestServiceControlCreatesServicesExitsNotConrollBySameCluster(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedis("redis-demo")
	rc1 := newRedis("redis-demo1")

	rc.UID = "123"
	rc1.UID = "abc"
	svc := newService(rc1, "master")
	fakeClient := &fake.Clientset{}
	control := NewRealServiceControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("create", "services", func(action core.Action) (bool, runtime.Object, error) {
		return true, svc, apierrors.NewAlreadyExists(action.GetResource().GroupResource(), svc.Name)
	})

	err := control.CreateService(rc, svc)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring("already exists and is not managed by Redis"))
}

func TestServiceControlCreatesServiceFailed(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedis("redis-demo")

	svc := newService(rc, "master")
	fakeClient := &fake.Clientset{}
	control := NewRealServiceControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("create", "services", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("apiserver is down"))
	})
	err := control.CreateService(rc, svc)
	g.Expect(err).To(HaveOccurred())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeWarning))
}

func TestServiceControlUpdateService(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedis("redis-demo")

	svc := newService(rc, "master")
	svc.Spec.ClusterIP = "127.0.0.1"
	svc.Spec.LoadBalancerIP = "9.9.9.9"
	fakeClient := &fake.Clientset{}
	control := NewRealServiceControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("update", "services", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), nil
	})
	updateSvc, err := control.UpdateService(rc, svc)
	g.Expect(err).To(Succeed())
	g.Expect(updateSvc.Spec.ClusterIP).To(Equal("127.0.0.1"))
	g.Expect(updateSvc.Spec.LoadBalancerIP).To(Equal("9.9.9.9"))

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestServiceControlUpdateServiceConflictSuccess(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedis("redis-demo")
	svc := newService(rc, "master")
	svc.Spec.ClusterIP = "1.1.1.1"

	fakeClient := &fake.Clientset{}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	oldSvc := newService(rc, "master")
	oldSvc.Spec.ClusterIP = "2.2.2.2"

	err := indexer.Add(oldSvc)
	g.Expect(err).To(Succeed())

	svcLister := corelisters.NewServiceLister(indexer)
	control := NewRealServiceControl(fakeClient, svcLister, recorder)
	conflict := false
	fakeClient.AddReactor("update", "services", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		if !conflict {
			conflict = true
			return true, oldSvc, apierrors.NewConflict(action.GetResource().GroupResource(), svc.Name, errors.New("conflict"))
		}
		return true, update.GetObject(), nil
	})
	updateSvc, err := control.UpdateService(rc, svc)
	g.Expect(err).To(Succeed())
	g.Expect(updateSvc.Spec.ClusterIP).To(Equal("1.1.1.1"))

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestServiceControlDeleteService(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedis("redis-demo")

	svc := newService(rc, "master")
	fakeClient := &fake.Clientset{}
	control := NewRealServiceControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("delete", "services", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	err := control.DeleteService(rc, svc)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestServiceControlDeleteServiceFaild(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedis("redis-demo")

	svc := newService(rc, "master")
	fakeClient := &fake.Clientset{}
	control := NewRealServiceControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("delete", "services", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("apiserver is down"))
	})
	err := control.DeleteService(rc, svc)
	g.Expect(err).To(HaveOccurred())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeWarning))
}
