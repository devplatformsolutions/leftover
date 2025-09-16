/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// package v1alpha1

// import (
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// )

// // LeftoverNodePoolSpec defines the desired state of LeftoverNodePool
// // XValidation ensures exactly one of nodeClassName or nodeClassSelector is set.
// // +kubebuilder:validation:XValidation:rule="has(self.nodeClassName) || (has(self.nodeClassSelector) && size(self.nodeClassSelector) > 0)",message="one of nodeClassName or nodeClassSelector must be set"
// // +kubebuilder:validation:XValidation:rule="!(has(self.nodeClassName) && has(self.nodeClassSelector))",message="only one of nodeClassName or nodeClassSelector may be set"
// type LeftoverNodePoolSpec struct {
// 	Region   string   `json:"region"`
// 	Families []string `json:"families,omitempty"`
// 	// Exact EC2NodeClass name (exclusive with nodeClassSelector)
// 	NodeClassName string `json:"nodeClassName,omitempty"`
// 	// Label selector for EC2NodeClass (must match exactly one)
// 	NodeClassSelector map[string]string `json:"nodeClassSelector,omitempty"`

// 	// +kubebuilder:default=1
// 	// +kubebuilder:validation:Minimum=1
// 	MinGPUs int `json:"minGPUs,omitempty"`

// 	// +kubebuilder:default=2
// 	TargetCount int32 `json:"targetCount,omitempty"`

// 	// +kubebuilder:default=5
// 	MinSpotScore int32 `json:"minSpotScore,omitempty"`

// 	// +kubebuilder:default=5
// 	// +kubebuilder:validation:Minimum=1
// 	MaxInstanceTypes int `json:"maxInstanceTypes,omitempty"`

// 	// +kubebuilder:default=2
// 	// +kubebuilder:validation:Minimum=1
// 	MaxZones int `json:"maxZones,omitempty"`

// 	// +kubebuilder:default=spot
// 	// +kubebuilder:validation:Enum=spot;on-demand
// 	CapacityType string `json:"capacityType,omitempty"`

// 	SubnetSelectorTags        map[string]string `json:"subnetSelectorTags,omitempty"`
// 	SecurityGroupSelectorTags map[string]string `json:"securityGroupSelectorTags,omitempty"`

// 	// Karpenter knobs
// 	// +kubebuilder:default="10%"
// 	BudgetsNodes string `json:"budgetsNodes,omitempty"`
// 	// +kubebuilder:default="2m"
// 	ConsolidateAfter string            `json:"consolidateAfter,omitempty"`
// 	Labels           map[string]string `json:"labels,omitempty"`
// 	Taints           []string          `json:"taints,omitempty"`

// 	// +kubebuilder:default=7
// 	// +kubebuilder:validation:Minimum=1
// 	RequeueMinutes int `json:"requeueMinutes,omitempty"`

// 	// +kubebuilder:default=true
// 	OnDemandFallback bool `json:"onDemandFallback,omitempty"`
// }

// // LeftoverNodePoolStatus defines the observed state of LeftoverNodePool.
// type LeftoverNodePoolStatus struct {
// 	SelectedInstanceTypes []string           `json:"selectedInstanceTypes,omitempty"`
// 	SelectedZones         []string           `json:"selectedZones,omitempty"`
// 	LastSyncTime          metav1.Time        `json:"lastSyncTime,omitempty"`
// 	Conditions            []metav1.Condition `json:"conditions,omitempty"`
// }

// // +kubebuilder:object:root=true
// // +kubebuilder:subresource:status
// // +kubebuilder:resource:path=leftovernodepools,scope=Cluster,shortName=lonp,categories=gpu;leftover

// // LeftoverNodePool is the Schema for the leftovernodepools API
// type LeftoverNodePool struct {
// 	metav1.TypeMeta   `json:",inline"`
// 	metav1.ObjectMeta `json:"metadata,omitempty"`

// 	Spec   LeftoverNodePoolSpec   `json:"spec"`
// 	Status LeftoverNodePoolStatus `json:"status,omitempty"`
// }

// // +kubebuilder:object:root=true

// // LeftoverNodePoolList contains a list of LeftoverNodePool
// type LeftoverNodePoolList struct {
// 	metav1.TypeMeta `json:",inline"`
// 	metav1.ListMeta `json:"metadata,omitempty"`
// 	Items           []LeftoverNodePool `json:"items"`
// }

// func init() {
// 	SchemeBuilder.Register(&LeftoverNodePool{}, &LeftoverNodePoolList{})
// }

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types
const (
	ConditionReady = "Ready"
)

// LeftoverNodePoolSpec defines the desired state of LeftoverNodePool
// Exactly one of nodeClassName or nodeClassSelector must be set.
// +kubebuilder:validation:XValidation:rule="has(self.nodeClassName) || (has(self.nodeClassSelector) && size(self.nodeClassSelector) > 0)",message="one of nodeClassName or nodeClassSelector must be set"
// +kubebuilder:validation:XValidation:rule="!(has(self.nodeClassName) && has(self.nodeClassSelector))",message="only one of nodeClassName or nodeClassSelector may be set"
type LeftoverNodePoolSpec struct {
	// AWS region (e.g. us-east-1)
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`

	// GPU instance families filter (e.g. g4dn, g5, p4). Empty = implementation defined discovery.
	Families []string `json:"families,omitempty"`

	// Exact EC2NodeClass name (exclusive with nodeClassSelector)
	NodeClassName string `json:"nodeClassName,omitempty"`
	// Label selector for EC2NodeClass (must match exactly one)
	NodeClassSelector map[string]string `json:"nodeClassSelector,omitempty"`

	// Minimum GPUs per instance type considered.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	MinGPUs int `json:"minGPUs,omitempty"`

	// Target pod count used in scoring heuristics.
	// +kubebuilder:default=2
	TargetCount int32 `json:"targetCount,omitempty"`

	// Minimum acceptable spot score (0..10). If none meet, fallback logic applies.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	MinSpotScore int32 `json:"minSpotScore,omitempty"`

	// Max distinct instance types to include in NodePool requirements.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	MaxInstanceTypes int `json:"maxInstanceTypes,omitempty"`

	// Max distinct zones to include.
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=1
	MaxZones int `json:"maxZones,omitempty"`

	// Capacity type preference: spot (default) or on-demand.
	// +kubebuilder:default=spot
	// +kubebuilder:validation:Enum=spot;on-demand
	CapacityType string `json:"capacityType,omitempty"`

	// Optional subnet selector tags (passed through to EC2NodeClass or used for extra logic)
	SubnetSelectorTags map[string]string `json:"subnetSelectorTags,omitempty"`
	// Optional SG selector tags
	SecurityGroupSelectorTags map[string]string `json:"securityGroupSelectorTags,omitempty"`

	// Karpenter disruption budgets (nodes percent/absolute; stored as single budget entry)
	// +kubebuilder:default="10%"
	BudgetsNodes string `json:"budgetsNodes,omitempty"`
	// ConsolidateAfter duration (e.g. "2m", "5m")
	// +kubebuilder:default="2m"
	ConsolidateAfter string `json:"consolidateAfter,omitempty"`

	// Additional node labels to set on provisioned nodes
	Labels map[string]string `json:"labels,omitempty"`
	// Taints list (string form: key[=value]:Effect) Effect in {NoSchedule,PreferNoSchedule,NoExecute}
	Taints []string `json:"taints,omitempty"`

	// Requeue interval in minutes.
	// +kubebuilder:default=7
	// +kubebuilder:validation:Minimum=1
	RequeueMinutes int `json:"requeueMinutes,omitempty"`

	// If true and no spot choice meets MinSpotScore, fallback to on-demand.
	// +kubebuilder:default=true
	OnDemandFallback bool `json:"onDemandFallback,omitempty"`
}

// LeftoverNodePoolStatus defines the observed state of LeftoverNodePool.
type LeftoverNodePoolStatus struct {
	Conditions            []metav1.Condition `json:"conditions,omitempty"`
	SelectedInstanceTypes []string           `json:"selectedInstanceTypes,omitempty"`
	SelectedZones         []string           `json:"selectedZones,omitempty"`
	LastPriceUSD          string             `json:"lastPriceUSD,omitempty"`
	LastScore             int                `json:"lastScore,omitempty"`
	LastSyncTime          metav1.Time        `json:"lastSyncTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=leftovernodepools,scope=Cluster,shortName=lonp,categories=gpu;leftover

// LeftoverNodePool is the Schema for the leftovernodepools API
type LeftoverNodePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LeftoverNodePoolSpec   `json:"spec"`
	Status LeftoverNodePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LeftoverNodePoolList contains a list of LeftoverNodePool
type LeftoverNodePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LeftoverNodePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LeftoverNodePool{}, &LeftoverNodePoolList{})
}
