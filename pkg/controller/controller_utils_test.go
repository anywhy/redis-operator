package controller

import (
	"fmt"

	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
)

func newRedisCluster(name string) *v1alpha1.RedisCluster {
	return &v1alpha1.RedisCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
	}
}

func newService(rc *v1alpha1.RedisCluster, _ string) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetName(rc.Name, "master"),
			Namespace: metav1.NamespaceDefault,
			OwnerReferences: []metav1.OwnerReference{
				GetOwnerRef(rc),
			},
		},
	}
	return svc
}

func newDeployment(rc *v1alpha1.RedisCluster, name string) *extv1.Deployment {
	dep := &extv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetName(rc.Name, name),
			Namespace: metav1.NamespaceDefault,
			OwnerReferences: []metav1.OwnerReference{
				GetOwnerRef(rc),
			},
		},
	}
	return dep
}

func newStatefulSet(rc *v1alpha1.RedisCluster, name string) *apps.StatefulSet {
	set := &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetName(rc.Name, name),
			Namespace: metav1.NamespaceDefault,
			OwnerReferences: []metav1.OwnerReference{
				GetOwnerRef(rc),
			},
		},
	}
	return set
}

func collectEvents(source <-chan string) []string {
	done := false
	events := make([]string, 0)
	for !done {
		select {
		case event := <-source:
			events = append(events, event)
		default:
			done = true
		}
	}
	return events
}

// GetName concatenate redis cluster name and member name, used for controller managed resource name
func GetName(rcName string, name string) string {
	return fmt.Sprintf("%s-%s", rcName, name)
}
