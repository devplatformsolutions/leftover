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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LeftoverNodePoolSpec defines the desired state of LeftoverNodePool
type LeftoverNodePoolSpec struct {
	Region   string   `json:"region"`
	Families []string `json:"families,omitempty"` // e.g. ["g5","g6","p5"]

	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	MinGPUs int `json:"minGPUs,omitempty"`

	// +kubebuilder:default=2
	TargetCount int32 `json:"targetCount,omitempty"`

	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	MaxInstanceTypes int `json:"maxInstanceTypes,omitempty"`

	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=1
	MaxZones int `json:"maxZones,omitempty"`

	// +kubebuilder:default=spot
	// +kubebuilder:validation:Enum=spot;on-demand
	CapacityType string `json:"capacityType,omitempty"`

	SubnetSelectorTags        map[string]string `json:"subnetSelectorTags,omitempty"`
	SecurityGroupSelectorTags map[string]string `json:"securityGroupSelectorTags,omitempty"`

	// Karpenter knobs
	// +kubebuilder:default="10%"
	BudgetsNodes string `json:"budgetsNodes,omitempty"`
	// +kubebuilder:default="2m"
	ConsolidateAfter string            `json:"consolidateAfter,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	Taints           []string          `json:"taints,omitempty"`

	// +kubebuilder:default=7
	// +kubebuilder:validation:Minimum=1
	RequeueMinutes int `json:"requeueMinutes,omitempty"`

	// +kubebuilder:default=true
	OnDemandFallback bool `json:"onDemandFallback,omitempty"`
}

// LeftoverNodePoolStatus defines the observed state of LeftoverNodePool.
type LeftoverNodePoolStatus struct {
	SelectedInstanceTypes []string           `json:"selectedInstanceTypes,omitempty"`
	SelectedZones         []string           `json:"selectedZones,omitempty"`
	LastSyncTime          metav1.Time        `json:"lastSyncTime,omitempty"`
	Conditions            []metav1.Condition `json:"conditions,omitempty"`
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
