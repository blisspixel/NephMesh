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

// Package intent lowers an outcome-level CommunicationIntent into the concrete
// MeshtasticNode specs that satisfy it. This is the compiler at the heart of the
// intent layer (ADR 0001): MeshtasticNode is the compiled output of a higher-level
// intent, not the source of truth. The compiler is pure and report-only, it
// renders and reports feasibility, it never creates a resource or writes to a
// device. Actuation is a later, deliberately gated step (ADR 0002).
package intent

import (
	"fmt"

	intentv1alpha1 "github.com/blisspixel/nephmesh/api/intent/v1alpha1"
	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/airtime"
)

// Result is the report-only output of compiling a CommunicationIntent: whether it
// can be rendered, the reason and message, the preset selected from the approved
// set, and the proposed MeshtasticNode specs. It actuates nothing.
type Result struct {
	Feasible       bool
	Reason         string
	Message        string
	SelectedPreset string
	Proposed       []intentv1alpha1.ProposedNode
}

// Compile lowers a CommunicationIntent to proposed MeshtasticNode specs, or
// reports why it cannot. It is pure and side-effect-free. Following the doctrine's
// lexicographic order, it first rejects an intent that cannot be satisfied (no
// target nodes, no approved preset, or an approved set with no preset the airtime
// model recognizes), then renders. The selected preset is the first approved one
// that is a known Meshtastic modem preset, so the strategic layer owns the
// approved SET and the compiler owns the selection. Infeasibility is a reported
// verdict, not an error.
func Compile(spec intentv1alpha1.CommunicationIntentSpec) Result {
	if len(spec.Nodes) == 0 {
		return Result{Reason: intentv1alpha1.ReasonNoTargetNodes, Message: "no target nodes to render"}
	}
	if len(spec.ApprovedModemPresets) == 0 {
		return Result{Reason: intentv1alpha1.ReasonNoApprovedSet, Message: "approvedModemPresets is empty; no preset to render"}
	}

	// Select the first approved preset the airtime model recognizes, so a typo'd
	// or unknown preset is caught here instead of failing to converge on a device.
	selected := ""
	for _, p := range spec.ApprovedModemPresets {
		if _, ok := airtime.PresetTimeOnAir(p, 1); ok {
			selected = p
			break
		}
	}
	if selected == "" {
		return Result{
			Reason:  intentv1alpha1.ReasonUnknownPreset,
			Message: fmt.Sprintf("none of the approved presets %v is a known Meshtastic modem preset", spec.ApprovedModemPresets),
		}
	}

	proposed := make([]intentv1alpha1.ProposedNode, 0, len(spec.Nodes))
	for _, n := range spec.Nodes {
		proposed = append(proposed, intentv1alpha1.ProposedNode{
			Name: n.Name,
			Spec: meshv1alpha1.MeshtasticNodeSpec{
				Region:      spec.Region,
				ModemPreset: selected,
				Role:        spec.Role,
				Channels:    spec.Channels,
				Connection:  n.Connection,
				Owner:       n.Owner,
			},
		})
	}
	return Result{
		Feasible:       true,
		Reason:         intentv1alpha1.ReasonRenderable,
		SelectedPreset: selected,
		Message:        fmt.Sprintf("renders %d node(s) at preset %s in region %s", len(proposed), selected, spec.Region),
		Proposed:       proposed,
	}
}
