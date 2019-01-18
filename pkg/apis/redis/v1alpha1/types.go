package v1alpha1

import (
	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterMode describes a redis cluster mode
type ClusterMode string

// MemberType represents member type
type MemberType string

const (
	// ReplicaCluster means that redis cluster is master/slave
	ReplicaCluster ClusterMode = "replica"
	// RedisCluster means redis cluster is shard mode
	RedisCluster ClusterMode = "cluster"

	// SentinelMemberType is sentinel container type
	SentinelMemberType MemberType = "sentinel"
	// RedisMemberType is sentinel container type
	RedisMemberType MemberType = "redis"
	// UnknownMemberType is unknown container type
	UnknownMemberType MemberType = "unknown"
)

// MemberPhase is the current state of member
type MemberPhase string

const (
	// NormalPhase represents normal state of Redis cluster.
	NormalPhase MemberPhase = "Normal"
	// UpgradePhase represents the Upgrading state of Redis cluster.
	UpgradePhase MemberPhase = "Upgrading"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Redis is a redis cluster control's spec
type Redis struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines of the redis cluster
	Spec RedisSpec `json:"spec"`

	// Most recently observed status of the redis cluster
	Status RedisStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RedisList is Redis list
type RedisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Redis `json:"items"`
}

// RedisSpec redis cluster attributes
type RedisSpec struct {
	// Mode choose from /MS/CLUSTER
	Mode ClusterMode `json:"mode"`

	Redis RedisInstanceSpec `json:"redis"`

	Sentinel RedisSentinelSpec `json:"sentinel,omitempty"`

	// Services list non-headless services type used in Redis
	Services        []Service                            `json:"services,omitempty"`
	PVReclaimPolicy corev1.PersistentVolumeReclaimPolicy `json:"pvReclaimPolicy,omitempty"`
}

// RedisInstanceSpec redis instance attributes
type RedisInstanceSpec struct {
	ContainerSpec

	// The number of cluster members (masters)
	Members int32 `json:"members"`
	// The number of replicas for each master(redis cluster mode)
	ReplicationFactor    *int32              `json:"replicationFactor,omitempty"`
	NodeSelector         map[string]string   `json:"nodeSelector,omitempty"`
	NodeSelectorRequired bool                `json:"nodeSelectorRequired,omitempty"`
	StorageClassName     string              `json:"storageClassName,omitempty"`
	Tolerations          []corev1.Toleration `json:"tolerations,omitempty"`
	SchedulerName        string              `json:"schedulerName,omitempty"`
}

// RedisSentinelSpec redis sentinel attributes
type RedisSentinelSpec struct {
	Enable     bool   `json:"enable,omitempty"`
	Replicas   int32  `json:"replicas,omitempty"`
	MasterName string `json:"masterName,omitempty"`
	Password   string `json:"password,omitempty"`

	Requests             *ResourceRequirement `json:"requests,omitempty"`
	Limits               *ResourceRequirement `json:"limits,omitempty"`
	StorageClassName     string               `json:"storageClassName,omitempty"`
	Tolerations          []corev1.Toleration  `json:"tolerations,omitempty"`
	SchedulerName        string               `json:"schedulerName,omitempty"`
	NodeSelector         map[string]string    `json:"nodeSelector,omitempty"`
	NodeSelectorRequired bool                 `json:"nodeSelectorRequired,omitempty"`
}

// RedisStatus represents the current status of a redis cluster.
type RedisStatus struct {
	Replica  ReplicaClusterStatus `json:"replica,omitempty"`
	Sentinel SentinelStatus       `json:"sentinel,omitempty"`
}

// ContainerSpec is the container spec of a pod
type ContainerSpec struct {
	Image           string               `json:"image,omitempty"`
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

// Service represent service type used in Redis
type Service struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// SentinelStatus is redis sentinel status
type SentinelStatus struct {
	Phase       MemberPhase             `json:"phase,omitempty"`
	StatefulSet *apps.StatefulSetStatus `json:"statefulset,omitempty"`
}

// ReplicaClusterStatus is ms cluster status
type ReplicaClusterStatus struct {
	Phase       MemberPhase             `json:"phase,omitempty"`
	StatefulSet *apps.StatefulSetStatus `json:"statefulset,omitempty"`
	MasterName  string                  `json:"masterName,omitempty"`
}
