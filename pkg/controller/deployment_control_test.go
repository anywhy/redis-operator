package controller

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	extlisters "k8s.io/client-go/listers/extensions/v1beta1"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

func TestServiceControlCreateDeployment(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")
	dep := newDeployment(rc, "sentinel")
	fakeClient := &fake.Clientset{}
	control := NewDeploymentControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("create", "deployments", func(action core.Action) (bool, runtime.Object, error) {
		create := action.(core.CreateAction)
		return true, create.GetObject(), nil
	})

	err := control.CreateDeployment(rc, dep)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestServiceControlCreateDeploymentExits(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")
	dep := newDeployment(rc, "sentinel")
	fakeClient := &fake.Clientset{}
	control := NewDeploymentControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("create", "deployments", func(action core.Action) (bool, runtime.Object, error) {
		return true, dep, apierrors.NewAlreadyExists(action.GetResource().GroupResource(), dep.Name)
	})

	err := control.CreateDeployment(rc, dep)
	g.Expect(err).To(Succeed())
}

func TestServiceControlCreateDeploymentExitsNotConrollBySameCluster(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")
	dep := newDeployment(rc, "sentinel")
	dep1 := newDeployment(rc, "sentinel1")

	dep.UID = "123"
	dep1.UID = "abc"

	fakeClient := &fake.Clientset{}
	control := NewDeploymentControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("create", "deployments", func(action core.Action) (bool, runtime.Object, error) {
		return true, dep, apierrors.NewAlreadyExists(action.GetResource().GroupResource(), dep.Name)
	})

	err := control.CreateDeployment(rc, dep)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring("already exists and is not managed by RedisCluster"))
}

func TestServiceControlCreateDeploymentFailed(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")

	dep := newDeployment(rc, "sentinel")
	fakeClient := &fake.Clientset{}
	control := NewDeploymentControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("create", "deployments", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("apiserver is down"))
	})
	err := control.CreateDeployment(rc, dep)
	g.Expect(err).To(HaveOccurred())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeWarning))
}

func TestServiceControlUpdateDeployment(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")

	dep := newDeployment(rc, "sentinel")
	dep.Spec.Paused = true
	fakeClient := &fake.Clientset{}
	control := NewDeploymentControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("update", "deployments", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), nil
	})
	updateDep, err := control.UpdateDeployment(rc, dep)
	g.Expect(err).To(Succeed())
	g.Expect(updateDep.Spec.Paused).To(Equal(true))

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestServiceControlUpdateDeploymentConflictSuccess(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")
	dep := newDeployment(rc, "sentinel")
	dep.Spec.Paused = false

	fakeClient := &fake.Clientset{}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	depOld := newDeployment(rc, "sentinel")
	depOld.Spec.Paused = true

	err := indexer.Add(depOld)
	g.Expect(err).To(Succeed())

	depLister := extlisters.NewDeploymentLister(indexer)
	control := NewDeploymentControl(fakeClient, depLister, recorder)
	conflict := false
	fakeClient.AddReactor("update", "deployments", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		if !conflict {
			conflict = true
			return true, depOld, apierrors.NewConflict(action.GetResource().GroupResource(), dep.Name, errors.New("conflict"))
		}
		return true, update.GetObject(), nil
	})
	updateDep, err := control.UpdateDeployment(rc, dep)
	g.Expect(err).To(Succeed())
	g.Expect(updateDep.Spec.Paused).To(Equal(true))

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestServiceControlDeleteDeployment(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")

	dep := newDeployment(rc, "sentinel")
	fakeClient := &fake.Clientset{}
	control := NewDeploymentControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("delete", "deployments", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	err := control.DeleteDeployment(rc, dep)
	g.Expect(err).To(Succeed())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeNormal))
}

func TestServiceControlDeleteDeploymentFaild(t *testing.T) {
	g := NewGomegaWithT(t)
	recorder := record.NewFakeRecorder(10)

	rc := newRedisCluster("redis-demo")

	dep := newDeployment(rc, "sentinel")
	fakeClient := &fake.Clientset{}
	control := NewDeploymentControl(fakeClient, nil, recorder)
	fakeClient.AddReactor("delete", "deployments", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("apiserver is down"))
	})
	err := control.DeleteDeployment(rc, dep)
	g.Expect(err).To(HaveOccurred())

	events := collectEvents(recorder.Events)
	g.Expect(events).To(HaveLen(1))
	g.Expect(events[0]).To(ContainSubstring(corev1.EventTypeWarning))
}
