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

	// ClusterIDLabelKey is cluster id label key
	ClusterIDLabelKey string = "redis.anywhy.github/cluster-id"
	// AnnPodNameKey is pod name annotation key used in PV/PVC for synchronizing tidb cluster meta info
	AnnPodNameKey string = "redis.anywhy.github/pod-name"

	// MasterLabelKey redis master role label key
	MasterLabelKey string = "master"
	// SlaveLabelKey redis slave role label key
	SlaveLabelKey string = "slave"
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

// Cluster adds redis cluster kv pair to label
func (l Label) Cluster(name string) Label {
	l[InstanceLabelKey] = name
	return l
}

// Component adds redis component kv pair to label
func (l Label) Component(name string) Label {
	l[ComponentLabelKey] = name
	return l
}

// ComponentType returns component type
func (l Label) ComponentType() string {
	return l[ComponentLabelKey]
}

// Master label assigned redis master
func (l Label) Master() Label {
	l[ComponentLabelKey] = MasterLabelKey
	return l
}

// IsMaster label componet is master
func (l Label) IsMaster() bool {
	return l[ComponentLabelKey] == MasterLabelKey
}

// Slave label assigned redis slave
func (l Label) Slave() Label {
	l[ComponentLabelKey] = SlaveLabelKey
	return l
}

// IsSlave label componet is slave
func (l Label) IsSlave() bool {
	return l[ComponentLabelKey] == SlaveLabelKey
}

// ClusterListOptions returns a cluster ListOptions filter
func ClusterListOptions(clusterName string) metav1.ListOptions {
	return metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(
			New().Cluster(clusterName).Labels(),
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