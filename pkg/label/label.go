package label

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// The following labels are recommended by kubernetes https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/

	// NameLabelKey is Kubernetes recommended label key, it represents the name of the application
	// It should always be redis-cluster in our case.
	NameLabelKey string = "app.kubernetes.io/name"
	// InstanceLabelKey is Kubernetes recommended label key, it represents a unique name identifying the instance of an application
	// It's the cluster name in our case
	InstanceLabelKey string = "app.kubernetes.io/instance"
	// ComponentLabelKey is Kubernetes recommended label key, it represents the component within the architecture
	ComponentLabelKey string = "app.kubernetes.io/component"
	// ManagedByLabelKey is Kubernetes recommended label key, it represents the tool being used to manage the operation of an application
	// For resources managed by Redis Operator, its value is always redis-operator
	ManagedByLabelKey string = "app.kubernetes.io/managed-by"

	// NamespaceLabelKey is label key used in PV for easy querying
	NamespaceLabelKey string = "app.kubernetes.io/namespace"

	// AnnPodNameKey is pod name annotation key used in PV/PVC for synchronizing redis cluster meta info
	AnnPodNameKey string = "anywhy.github.io/pod-name"
	// ClusterGroupIDLabelKey is pod name annotation key used for redis cluster group synchronizing redis cluster meta info
	ClusterGroupIDLabelKey string = "anywhy.github.io/cluster-group-id"
	// ClusterModeLabelKey is redis cluster mode
	ClusterModeLabelKey string = "anywhy.github.io/cluster-mode"
	// ClusterNodeRoleLabelKey is redis node role
	ClusterNodeRoleLabelKey string = "anywhy.github.io/node-role"

	// MasterNodeLabelKey redis master role label key
	MasterNodeLabelKey string = "master"
	// SlaveNodeLabelKey redis slave role label key
	SlaveNodeLabelKey string = "slave"
	// RedisLabelKey redis component label key
	RedisLabelKey string = "redis"
	// SentinelLabelKey redis sentinel  component label key
	SentinelLabelKey string = "sentinel"

	// ReplicaClusterLabelKey means that redis cluster is master/slave
	ReplicaClusterLabelKey string = "replica"
	// RedisClusterLabelKey means redis cluster is shard mode
	RedisClusterLabelKey string = "cluster"
)

// Label is the label field in metadatas
type Label map[string]string

// New init a new Label for redis-opertator
func New() Label {
	return Label{
		NameLabelKey:      "redis-cluster",
		ManagedByLabelKey: "redis-operator",
	}
}

// Namespace adds namespace kv pair to label
func (l Label) Namespace(name string) Label {
	l[NamespaceLabelKey] = name
	return l
}

// Instance adds redis cluster kv pair to label
func (l Label) Instance(name string) Label {
	l[InstanceLabelKey] = name
	return l
}

// Component adds redis component kv pair to label
func (l Label) Component(name string) Label {
	l[ComponentLabelKey] = name
	return l
}

// ClusterMode add redis mode kv pair to label
func (l Label) ClusterMode(mode string) Label {
	l[ClusterModeLabelKey] = mode
	return l
}

// Replica replica cluster
func (l Label) Replica() Label {
	return l.ClusterMode(ReplicaClusterLabelKey)
}

// RedisCluster redis cluster
func (l Label) RedisCluster() Label {
	return l.ClusterMode(RedisClusterLabelKey)
}

// ClusterModeType return cluster mode
func (l Label) ClusterModeType() string {
	return l[ClusterModeLabelKey]
}

// ComponentType returns component type
func (l Label) ComponentType() string {
	return l[ComponentLabelKey]
}

// Group add redis cluster group id kv pair to label
func (l Label) Group(groupID string) Label {
	l[ClusterGroupIDLabelKey] = groupID
	return l
}

// GetGroup get redis cluster grop id
func (l Label) GetGroup() string {
	return l[ClusterGroupIDLabelKey]
}

// Master label assigned redis master
func (l Label) Master() Label {
	l[ClusterNodeRoleLabelKey] = MasterNodeLabelKey
	return l
}

// IsMaster label role is master
func (l Label) IsMaster() bool {
	return l[ClusterNodeRoleLabelKey] == MasterNodeLabelKey
}

// Redis label assigned redis sentinel
func (l Label) Redis() Label {
	l[ComponentLabelKey] = RedisLabelKey
	return l
}

// Sentinel label assigned redis sentinel
func (l Label) Sentinel() Label {
	l[ComponentLabelKey] = SentinelLabelKey
	return l
}

// IsSentinel label componet is sentinel
func (l Label) IsSentinel() bool {
	return l[ComponentLabelKey] == SentinelLabelKey
}

// Slave label assigned redis slave
func (l Label) Slave() Label {
	l[ClusterNodeRoleLabelKey] = SlaveNodeLabelKey
	return l
}

// IsSlave label role is slave
func (l Label) IsSlave() bool {
	return l[ClusterNodeRoleLabelKey] == ClusterNodeRoleLabelKey
}

// ClusterListOptions returns a cluster ListOptions filter
func ClusterListOptions(instanceName string) metav1.ListOptions {
	return metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(
			New().Instance(instanceName).Labels(),
		).String(),
	}
}

// Selector gets labels.Selector from label
func (l Label) Selector() (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(l.LabelSelector())
}

// LabelSelector gets LabelSelector from label
func (l Label) LabelSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: l}
}

// Labels converts label to map[string]string
func (l Label) Labels() map[string]string {
	return l
}
