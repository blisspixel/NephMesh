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
	"math"

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

	// Airtime is the fleet-wide airtime estimate, populated only when the intent
	// declares expectedTraffic. It is separate from Feasible: renderability and
	// fitting the airtime commons are distinct questions.
	Airtime AirtimeResult
}

// AirtimeResult is the fleet airtime verdict. Evaluated is false when the intent
// declares no expectedTraffic. When evaluated, PredictedUtilizationPercent is a
// conservative floor (rebroadcast ignored), so WithinBudget=false is
// authoritative and WithinBudget=true is advisory.
type AirtimeResult struct {
	Evaluated                   bool
	WithinBudget                bool
	PredictedUtilizationPercent int
	Reason                      string
	Message                     string
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
	// The CRD enforces unique node names via a list-map key, but this pure path
	// also runs offline via the CLI where that admission check has not applied.
	// Reject duplicates here so two targets never render to one colliding
	// MeshtasticNode and never inflate the node count and the airtime floor.
	seen := make(map[string]struct{}, len(spec.Nodes))
	for _, n := range spec.Nodes {
		if _, dup := seen[n.Name]; dup {
			return Result{
				Reason:  intentv1alpha1.ReasonDuplicateNode,
				Message: fmt.Sprintf("duplicate node name %q: two targets cannot render to the same MeshtasticNode", n.Name),
			}
		}
		seen[n.Name] = struct{}{}
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
		Airtime:        evaluateAirtime(spec, selected, len(proposed)),
	}
}

// evaluateAirtime estimates whether the rendered fleet fits the channel's airtime
// budget. It runs only when the intent declares expectedTraffic; otherwise it
// reports NotEvaluated rather than guessing at the offered load. The estimate is
// the conservative floor from the airtime model (rebroadcast ignored), so an
// over-budget verdict is authoritative and a within-budget one is advisory.
func evaluateAirtime(spec intentv1alpha1.CommunicationIntentSpec, preset string, nodeCount int) AirtimeResult {
	if spec.ExpectedTraffic == nil {
		return AirtimeResult{
			Reason:  intentv1alpha1.ReasonAirtimeNotEvaluated,
			Message: "no expectedTraffic declared; fleet airtime not evaluated",
		}
	}
	// A non-positive message rate is not a meaningful traffic declaration (the CRD
	// requires >= 1, but this pure path also runs offline via the CLI where that
	// admission check has not applied). Report not-evaluated rather than a
	// reassuring but meaningless "within budget, 0%".
	if spec.ExpectedTraffic.MessagesPerMinutePerNode <= 0 {
		return AirtimeResult{
			Reason:  intentv1alpha1.ReasonAirtimeNotEvaluated,
			Message: "expectedTraffic.messagesPerMinutePerNode must be >= 1; fleet airtime not evaluated",
		}
	}

	payload := spec.ExpectedTraffic.PayloadBytes
	if payload <= 0 {
		payload = airtime.RepresentativeFramePayloadBytes
	}
	percent, ok := airtime.FleetChannelUtilizationPercent(preset, nodeCount, spec.ExpectedTraffic.MessagesPerMinutePerNode, payload)
	if !ok {
		// The preset was already validated as known above, so this is unreachable
		// in practice; report it rather than claim a spurious verdict.
		return AirtimeResult{
			Reason:  intentv1alpha1.ReasonAirtimeNotEvaluated,
			Message: "fleet airtime could not be estimated for the selected preset",
		}
	}

	rounded := int(math.Round(percent))
	within := airtime.WithinChannelBudget(percent)
	res := AirtimeResult{
		Evaluated:                   true,
		WithinBudget:                within,
		PredictedUtilizationPercent: rounded,
	}
	if within {
		res.Reason = intentv1alpha1.ReasonWithinBudget
		res.Message = fmt.Sprintf(
			"estimated channel utilization floor is %.1f%% (ceiling %.0f%%); this ignores mesh rebroadcast, so actual utilization is higher and the device's measured airtime remains the ground truth",
			percent, airtime.RecommendedChannelUtilizationPercent)
	} else {
		res.Reason = intentv1alpha1.ReasonOverBudget
		res.Message = fmt.Sprintf(
			"estimated channel utilization floor is %.1f%%, over the %.0f%% ceiling by the conservative floor alone (rebroadcast ignored), so the fleet is over budget for certain; reduce nodes or message rate, or choose a faster preset",
			percent, airtime.RecommendedChannelUtilizationPercent)
	}
	return res
}
