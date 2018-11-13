package controller

import (
	"fmt"
	"strings"

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

// SentinelMemberName returns sentinel name for redis
func SentinelMemberName(clusterName string) string {
	return fmt.Sprintf("%s-sentinel", clusterName)
}

// RedisMemberName return redis name for redis cluster
func RedisMemberName(clusterName string, group string) string {
	if strings.EqualFold(group, "") {
		return fmt.Sprintf("%s-redis", clusterName)
	}
	return fmt.Sprintf("%s-%s-redis", clusterName, group)
}
