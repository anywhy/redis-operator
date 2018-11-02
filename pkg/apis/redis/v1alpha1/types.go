package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterMode describes a redis cluster mode
type ClusterMode string

const (
	// MS means that redis cluster is master/slave
	MS ClusterMode = "MS"
	// Cluster means redis cluster is shard mode
	Cluster ClusterMode = "CLUSTER"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Redis is a redis cluster control's spec
type Redis struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines of the redis cluster
	Spec RedisClusterSpec `json:"spec"`

	// Most recently observed status of the redis cluster
	Status RedisClusterStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RedisList is RedisCluster list
type RedisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Redis `json:"items"`
}

// RedisClusterSpec redis cluster attributes
type RedisClusterSpec struct {
	ContainerSpec

	// Mode choose from /MS/CLUSTER
	Mode ClusterMode `json:"mode"`
	// The number of cluster members (masters)
	Members int32 `json:"members"`
	// The number of replicas for each master(redis cluster mode)
	ReplicationFactor *int32              `json:"replicationFactor,omitempty"`
	NodeSelector      map[string]string   `json:"nodeSelector,omitempty"`
	StorageClassName  string              `json:"storageClassName,omitempty"`
	Tolerations       []corev1.Toleration `json:"tolerations,omitempty"`

	Sentinels RedisSentinelSpec `json:"sentinels,omitempty"`
}

// RedisSentinelSpec redis sentinel attributes
type RedisSentinelSpec struct {
	ContainerSpec
	Replicas         int32               `json:"replicas"`
	Quorum           int32               `json:"quorum"`
	NodeSelector     map[string]string   `json:"nodeSelector,omitempty"`
	StorageClassName string              `json:"storageClassName,omitempty"`
	Tolerations      []corev1.Toleration `json:"tolerations,omitempty"`
}

// RedisClusterStatus represents the current status of a redis cluster.
type RedisClusterStatus struct {
}

// ContainerSpec is the container spec of a pod
type ContainerSpec struct {
	Image           string               `json:"image"`
	ImagePullPolicy corev1.PullPolicy    `json:"imagePullPolicy,omitempty"`
	Requests        *ResourceRequirement `json:"requests,omitempty"`
	Limits          *ResourceRequirement `json:"limits,omitempty"`
}

// ResourceRequirement is resource requirements for a pod
type ResourceRequirement struct {
	// CPU is how many cores a pod requires
	CPU string `json:"cpu,omitempty"`
	// Memory is how much memory a pod requires
	Memory string `json:"memory,omitempty"`
	// Storage is storage size a pod requires
	Storage string `json:"storage,omitempty"`
}
