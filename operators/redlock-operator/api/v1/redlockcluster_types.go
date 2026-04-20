package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type ClusterPhase string

const (
	ClusterPhasePending   ClusterPhase = "Pending"
	ClusterPhaseCreating  ClusterPhase = "Creating"
	ClusterPhaseRunning   ClusterPhase = "Running"
	ClusterPhaseDegraded  ClusterPhase = "Degraded"
	ClusterPhaseFailed    ClusterPhase = "Failed"
)

type PersistenceConfig struct {
	Enabled bool `json:"enabled,omitempty"`
	Size    string `json:"size,omitempty"`
}

type RedlockClusterSpec struct {
	Size          int32             `json:"size"`
	Image         string            `json:"image,omitempty"`
	EnableFencing bool              `json:"enableFencing,omitempty"`
	LockTTL       int32             `json:"lockTTL,omitempty"`
	Persistence   PersistenceConfig `json:"persistence,omitempty"`
}

type RedlockClusterStatus struct {
	Phase           ClusterPhase      `json:"phase,omitempty"`
	ReadyReplicas   int32             `json:"readyReplicas,omitempty"`
	CurrentReplicas int32             `json:"currentReplicas,omitempty"`
	Leader          string            `json:"leader,omitempty"`
	Conditions      []metav1.Condition `json:"conditions,omitempty"`
}

type RedlockCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedlockClusterSpec   `json:"spec,omitempty"`
	Status RedlockClusterStatus `json:"status,omitempty"`
}

type RedlockClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedlockCluster `json:"items"`
}

func (in *RedlockCluster) DeepCopyObject() runtime.Object {
	out := &RedlockCluster{}
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = in.ObjectMeta
	out.Spec = in.Spec
	out.Status = in.Status
	return out
}

func (in *RedlockClusterList) DeepCopyObject() runtime.Object {
	out := &RedlockClusterList{}
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	out.Items = make([]RedlockCluster, len(in.Items))
	copy(out.Items, in.Items)
	return out
}

func (c *RedlockCluster) GetOwnerReference() metav1.OwnerReference {
	return *metav1.NewControllerRef(c, SchemeGroupVersion.WithKind("RedlockCluster"))
}

func (c *RedlockCluster) IsReady() bool {
	return c.Status.Phase == ClusterPhaseRunning &&
		c.Status.ReadyReplicas == c.Spec.Size
}

func (c *RedlockCluster) SetPhase(phase ClusterPhase) {
	c.Status.Phase = phase
}

func (c *RedlockCluster) UpdateReadyReplicas(count int32) {
	c.Status.ReadyReplicas = count
}
