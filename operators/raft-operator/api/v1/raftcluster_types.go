package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RaftClusterPhase string

const (
	RaftClusterPhasePending   RaftClusterPhase = "Pending"
	RaftClusterPhaseCreating  RaftClusterPhase = "Creating"
	RaftClusterPhaseRunning   RaftClusterPhase = "Running"
	RaftClusterPhaseDegraded  RaftClusterPhase = "Degraded"
	RaftClusterPhaseFailed    RaftClusterPhase = "Failed"
)

type RaftClusterSpec struct {
	Size              int32             `json:"size"`
	Image             string            `json:"image,omitempty"`
	ElectionTimeout   int32             `json:"electionTimeout,omitempty"`
	HeartbeatInterval int32             `json:"heartbeatInterval,omitempty"`
	Persistence       PersistenceConfig `json:"persistence,omitempty"`
}

type PersistenceConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Size    string `json:"size,omitempty"`
}

type RaftClusterStatus struct {
	Phase             RaftClusterPhase `json:"phase,omitempty"`
	ReadyReplicas     int32            `json:"readyReplicas,omitempty"`
	CurrentReplicas   int32            `json:"currentReplicas,omitempty"`
	Leader            string           `json:"leader,omitempty"`
	CurrentTerm       uint64           `json:"currentTerm,omitempty"`
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Size",type="integer",JSONPath=".spec.size"
//+kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas"
//+kubebuilder:printcolumn:name="Leader",type="string",JSONPath=".status.leader"
//+kubebuilder:printcolumn:name="Term",type="integer",JSONPath=".status.currentTerm"
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

type RaftCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RaftClusterSpec   `json:"spec,omitempty"`
	Status RaftClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type RaftClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RaftCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RaftCluster{}, &RaftClusterList{})
}

func (c *RaftCluster) GetOwnerReference() metav1.OwnerReference {
	return *metav1.NewControllerRef(c, GroupVersion.WithKind("RaftCluster"))
}

func (c *RaftCluster) IsReady() bool {
	return c.Status.Phase == RaftClusterPhaseRunning &&
		c.Status.ReadyReplicas == c.Spec.Size
}

func (c *RaftCluster) SetPhase(phase RaftClusterPhase) {
	c.Status.Phase = phase
}
