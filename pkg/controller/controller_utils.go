package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
)

const (
	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "%s %s/%s  already exists and is not managed by RedisCluster"
)

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = v1alpha1.SchemeGroupVersion.WithKind("RedisCluster")
	// DefaultStorageClassName is the default storageClassName
	DefaultStorageClassName string

	// ClusterScoped controls whether operator should manage kubernetes cluster wide Redis clusters
	ClusterScoped bool
)

// GetOwnerRef returns RedisCluster's OwnerReference
func GetOwnerRef(rc *v1alpha1.RedisCluster) metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return metav1.OwnerReference{
		APIVersion:         controllerKind.GroupVersion().String(),
		Kind:               controllerKind.Kind,
		Name:               rc.GetName(),
		UID:                rc.GetUID(),
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}
}

// AnnProm adds annotations for prometheus scraping metrics
func AnnProm(port int32) map[string]string {
	return map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/path":   "/metrics",
		"prometheus.io/port":   fmt.Sprintf("%d", port),
	}
}

// setIfNotEmpty set the value into map when value in not empty
func setIfNotEmpty(container map[string]string, key, value string) {
	if value != "" {
		container[key] = value
	}
}

// GetServiceType returns member's service type
func GetServiceType(services []v1alpha1.Service, serviceName string) corev1.ServiceType {
	for _, svc := range services {
		if svc.Name == serviceName {
			switch svc.Type {
			case "NodePort":
				return corev1.ServiceTypeNodePort
			case "LoadBalancer":
				return corev1.ServiceTypeLoadBalancer
			default:
				return corev1.ServiceTypeClusterIP
			}
		}
	}
	return corev1.ServiceTypeClusterIP
}

// SentinelMemberName returns sentinel name for redis
func SentinelMemberName(clusterName string) string {
	return fmt.Sprintf("%s-sentinel", clusterName)
}

// SentinelPeerMemberName returns sentinel peer service name
func SentinelPeerMemberName(clusterName string) string {
	return fmt.Sprintf("%s-sentinel-peer", clusterName)
}

// RedisMemberName return redis name for redis cluster
func RedisMemberName(clusterName string) string {
	return fmt.Sprintf("%s-redis", clusterName)
}
