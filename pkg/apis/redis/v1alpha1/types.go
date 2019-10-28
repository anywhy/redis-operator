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
	// Replica means that redis cluster is master/slave
	Replica ClusterMode = "replica"
	// Cluster means redis cluster is shard mode
	Cluster ClusterMode = "cluster"

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

// RedisCluster is a redis cluster control's spec
type RedisCluster struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines of the redis cluster
	Spec RedisClusterSpec `json:"spec"`

	// Most recently observed status of the redis cluster
	Status RedisClusterStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RedisClusterList is Redis cluster list
type RedisClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RedisCluster `json:"items"`
}

// RedisClusterSpec redis cluster attributes
type RedisClusterSpec struct {
	// Mode choose from /replica/cluster
	Mode          ClusterMode `json:"mode"`
	SchedulerName string      `json:"schedulerName,omitempty"`

	Redis    ServerSpec   `json:"redis"`
	Sentinel SentinelSpec `json:"sentinel,omitempty"`

	// Services list non-headless services type used in Redis
	Services        []Service                            `json:"services,omitempty"`
	PVReclaimPolicy corev1.PersistentVolumeReclaimPolicy `json:"pvReclaimPolicy,omitempty"`
}

// ServerSpec redis instance attributes
type ServerSpec struct {
	ContainerSpec

	PodAttributesSpec

	// The number of cluster members
	Replicas int32 `json:"members"`

	StorageClassName string `json:"storageClassName,omitempty"`

	// The number of replicas for each master(redis cluster mode)
	ReplicationFactor *int32 `json:"replicationFactor,omitempty"`
}

// SentinelSpec redis sentinel attributes
type SentinelSpec struct {
	PodAttributesSpec

	Enable     bool   `json:"enable,omitempty"`
	Replicas   int32  `json:"replicas,omitempty"`
	MasterName string `json:"masterName,omitempty"`
	Password   string `json:"password,omitempty"`

	StorageClassName string `json:"storageClassName,omitempty"`

	Requests *ResourceRequirement `json:"requests,omitempty"`
	Limits   *ResourceRequirement `json:"limits,omitempty"`
}

// RedisClusterStatus represents the current status of a redis cluster.
type RedisClusterStatus struct {
	Redis    ServerStatus   `json:"replica,omitempty"`
	Sentinel SentinelStatus `json:"sentinel,omitempty"`
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

// RedisMember redis server
type RedisMember struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	Role   string `json:"role"`
	Health bool   `json:"health"`
	// Last time the health transitioned from one to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// PodAttributesSpec is a spec of some general attributes of Server, Sentinel Pods
type PodAttributesSpec struct {
	Affinity           *corev1.Affinity           `json:"affinity,omitempty"`
	NodeSelector       map[string]string          `json:"nodeSelector,omitempty"`
	Tolerations        []corev1.Toleration        `json:"tolerations,omitempty"`
	Annotations        map[string]string          `json:"annotations,omitempty"`
	HostNetwork        bool                       `json:"hostNetwork,omitempty"`
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`
	PriorityClassName  string                     `json:"priorityClassName,omitempty"`
}

// ServerStatus is cluster status
type ServerStatus struct {
	Phase          MemberPhase                   `json:"phase,omitempty"`
	StatefulSet    *apps.StatefulSetStatus       `json:"statefulset,omitempty"`
	Members        map[string]RedisMember        `json:"members,omitempty"`
	FailureMembers map[string]RedisFailureMember `json:"failureMembers,omitempty"`
	Masters        []RedisMember                 `json:"masters,omitempty"`
}

// RedisFailureMember is the redis cluster failure member information
type RedisFailureMember struct {
	PodName   string      `json:"podName,omitempty"`
	CreatedAt metav1.Time `json:"createdAt,omitempty"`
}

// SentinelStatus is redis sentinel status
type SentinelStatus struct {
	Phase       MemberPhase             `json:"phase,omitempty"`
	StatefulSet *apps.StatefulSetStatus `json:"statefulset,omitempty"`
}
