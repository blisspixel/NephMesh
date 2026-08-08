/*
Copyright 2026 The NephMesh Authors.

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

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
)

// CommunicationIntentSpec declares desired mesh communications at an outcome
// level, above per-device configuration. A compiler renders it into the concrete
// MeshtasticNode specs that satisfy it. Today the compiler runs report-only: the
// rendering and a feasibility verdict are written to status and nothing is
// created or written to a device.
type CommunicationIntentSpec struct {
	// region is the regulatory region every rendered node must use. It is a hard
	// invariant (band, duty cycle, power); see docs/reference/regulatory-matrix.md.
	// +kubebuilder:validation:Pattern=`^[A-Z0-9_]+$`
	// +kubebuilder:validation:MaxLength=32
	Region string `json:"region"`

	// approvedModemPresets is the set of modem presets a rendered node may run.
	// The compiler selects from this set (the first entry is preferred); an empty
	// set is infeasible. This is the "approved operating set" from the doctrine:
	// the strategic layer owns the allowed set, a lower layer the selection.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	ApprovedModemPresets []string `json:"approvedModemPresets"`

	// role is the device role every rendered node runs.
	// +kubebuilder:validation:Pattern=`^[A-Z0-9_]+$`
	// +kubebuilder:validation:MaxLength=32
	// +optional
	Role string `json:"role,omitempty"`

	// channels are the channels every rendered node carries (PSKs referenced from
	// Secrets, never inlined), the same shape a MeshtasticNode declares. Keyed by
	// index so a duplicate channel index is rejected at admission. Meshtastic
	// supports up to 8 channels.
	// +optional
	// +listType=map
	// +listMapKey=index
	// +kubebuilder:validation:MaxItems=8
	Channels []meshv1alpha1.ChannelSpec `json:"channels,omitempty"`

	// nodes are the target devices this intent renders to. Keyed by name so two
	// targets cannot share a name (they would render two colliding MeshtasticNode
	// specs); the collision is rejected at admission rather than surfaced later.
	// The bound keeps one intent to a reviewable fleet and, because each target
	// carries a Connection with a CEL rule, keeps the CRD within the apiserver's
	// validation cost budget; address a larger region with several intents.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	// +listType=map
	// +listMapKey=name
	Nodes []NodeTarget `json:"nodes"`
}

// NodeTarget names one device the intent applies to and how to reach it.
type NodeTarget struct {
	// name is the MeshtasticNode name the rendering would carry.
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
	// connection is how to reach this device.
	Connection meshv1alpha1.ConnectionSpec `json:"connection"`
	// owner optionally sets this node's owner names.
	// +optional
	Owner *meshv1alpha1.OwnerSpec `json:"owner,omitempty"`
}

// CommunicationIntentStatus reports the compiler's output. In report-only mode it
// is the proposed rendering and a feasibility verdict, never an actuation.
type CommunicationIntentStatus struct {
	// conditions carry Feasible: True when the intent can be rendered, False with
	// a reason when it cannot (IntentInfeasible is a legitimate reported state, not
	// a controller error).
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// observedGeneration is the spec generation last compiled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// selectedModemPreset is the preset the compiler chose from the approved set.
	// +optional
	SelectedModemPreset string `json:"selectedModemPreset,omitempty"`
	// nodeCount is how many nodes the intent renders to.
	// +optional
	NodeCount int `json:"nodeCount,omitempty"`
	// proposedNodes is the rendered MeshtasticNode specs the intent compiles to.
	// Report-only: these are what the operator would create, not created. Bounded
	// to match spec.nodes (each rendered spec carries a Connection with a CEL
	// rule, so the bound also keeps the CRD within the validation cost budget).
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=64
	ProposedNodes []ProposedNode `json:"proposedNodes,omitempty"`
}

// ProposedNode is one rendered MeshtasticNode the compiler would create. The Spec
// is a MeshtasticNodeSpec directly: the compiled output IS the device-level
// resource, per ADR 0001.
type ProposedNode struct {
	Name string                          `json:"name"`
	Spec meshv1alpha1.MeshtasticNodeSpec `json:"spec"`
}

// Feasibility condition and reasons.
const (
	// ConditionFeasible is True when the intent can be rendered into node specs.
	ConditionFeasible = "Feasible"

	ReasonRenderable    = "Renderable"
	ReasonNoApprovedSet = "NoApprovedModemPresets"
	ReasonUnknownPreset = "UnknownModemPreset"
	ReasonNoTargetNodes = "NoTargetNodes"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.spec.region`
// +kubebuilder:printcolumn:name="Preset",type=string,JSONPath=`.status.selectedModemPreset`
// +kubebuilder:printcolumn:name="Nodes",type=integer,JSONPath=`.status.nodeCount`
// +kubebuilder:printcolumn:name="Feasible",type=string,JSONPath=`.status.conditions[?(@.type=="Feasible")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CommunicationIntent is outcome-level mesh communications intent that compiles to
// MeshtasticNode specs. It is report-only today: the operator writes the proposed
// rendering and a feasibility verdict to status and never actuates (ADR 0001).
type CommunicationIntent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CommunicationIntentSpec   `json:"spec,omitempty"`
	Status CommunicationIntentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CommunicationIntentList is a list of CommunicationIntent.
type CommunicationIntentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CommunicationIntent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CommunicationIntent{}, &CommunicationIntentList{})
}
