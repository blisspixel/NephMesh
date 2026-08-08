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

// Package plan is the agent-facing core: it parses a CommunicationIntent and
// runs the report-only compiler, producing a stable, machine-readable verdict.
// It is the single shared implementation behind both the nephmeshctl CLI and the
// nephmesh-mcp MCP server, so an agent (Claude Code, Codex, a local LLM) gets the
// same answer whichever surface it calls, and that answer is exactly what the
// operator's controller would compute. It needs no cluster and no hardware: the
// compiler is pure, so this is a true offline dry-run.
package plan

import (
	"errors"
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"

	intentv1alpha1 "github.com/blisspixel/nephmesh/api/intent/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/intent"
)

// Output is the stable JSON contract an agent consumes. It is a dedicated type,
// not the internal compiler Result, so the wire shape is decoupled from the
// implementation and can be documented and versioned on its own terms.
type Output struct {
	// Feasible reports whether the intent can be compiled into node specs.
	Feasible bool `json:"feasible"`
	// Reason is a stable machine token for the feasibility verdict.
	Reason string `json:"reason"`
	// Message is a human-readable explanation of the verdict.
	Message string `json:"message"`
	// SelectedPreset is the modem preset chosen from the approved set, when feasible.
	SelectedPreset string `json:"selectedPreset,omitempty"`
	// NodeCount is how many nodes the intent renders to.
	NodeCount int `json:"nodeCount"`
	// Airtime is the fleet-wide airtime verdict (evaluated only when the intent
	// declares expectedTraffic).
	Airtime AirtimeOutput `json:"airtime"`
	// ProposedNodes is the rendered MeshtasticNode specs. Report-only: these are
	// what the operator would create, not created.
	ProposedNodes []intentv1alpha1.ProposedNode `json:"proposedNodes,omitempty"`
}

// AirtimeOutput is the fleet airtime verdict. When Evaluated is false the intent
// declared no expectedTraffic. When true, PredictedUtilizationPercent is a
// conservative floor (mesh rebroadcast ignored), so WithinBudget=false is
// authoritative and WithinBudget=true is advisory.
type AirtimeOutput struct {
	Evaluated                   bool   `json:"evaluated"`
	WithinBudget                bool   `json:"withinBudget"`
	PredictedUtilizationPercent int    `json:"predictedUtilizationPercent"`
	Reason                      string `json:"reason,omitempty"`
	Message                     string `json:"message,omitempty"`
}

// ErrEmptyInput is returned when there is nothing to parse.
var ErrEmptyInput = errors.New("no intent provided")

// ParseSpec reads a CommunicationIntent from YAML or JSON bytes and returns its
// spec. It accepts either a full CommunicationIntent object (the same document an
// operator would apply, with a spec field) or a bare spec, so an agent can pass
// whichever it has. Structural CRD validation (field patterns, list bounds) is
// enforced by the apiserver at admission; this offline path runs the semantic
// compiler, which is what determines feasibility.
func ParseSpec(data []byte) (intentv1alpha1.CommunicationIntentSpec, error) {
	var spec intentv1alpha1.CommunicationIntentSpec
	if len(trimSpace(data)) == 0 {
		return spec, ErrEmptyInput
	}

	// Detect a full object (has a top-level spec) versus a bare spec by probing
	// for the spec key. sigs.k8s.io/yaml converts YAML to JSON, so a JSON input
	// works through the same path.
	var probe map[string]any
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return spec, fmt.Errorf("parse intent: %w", err)
	}
	if _, hasSpec := probe["spec"]; hasSpec {
		var obj intentv1alpha1.CommunicationIntent
		if err := yaml.Unmarshal(data, &obj); err != nil {
			return spec, fmt.Errorf("parse CommunicationIntent: %w", err)
		}
		return obj.Spec, nil
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return spec, fmt.Errorf("parse CommunicationIntent spec: %w", err)
	}
	return spec, nil
}

// Run parses an intent and compiles it into the stable Output. It is the whole
// offline dry-run: parse, then the pure report-only compiler.
func Run(data []byte) (Output, error) {
	spec, err := ParseSpec(data)
	if err != nil {
		return Output{}, err
	}
	return FromResult(intent.Compile(spec)), nil
}

// FromResult projects the internal compiler Result onto the stable wire Output.
func FromResult(r intent.Result) Output {
	return Output{
		Feasible:       r.Feasible,
		Reason:         r.Reason,
		Message:        r.Message,
		SelectedPreset: r.SelectedPreset,
		NodeCount:      len(r.Proposed),
		Airtime: AirtimeOutput{
			Evaluated:                   r.Airtime.Evaluated,
			WithinBudget:                r.Airtime.WithinBudget,
			PredictedUtilizationPercent: r.Airtime.PredictedUtilizationPercent,
			Reason:                      r.Airtime.Reason,
			Message:                     r.Airtime.Message,
		},
		ProposedNodes: r.Proposed,
	}
}

// Text renders the verdict as a short human-readable summary for a terminal. The
// machine contract is the JSON Output; this is for a person reading the CLI.
func (o Output) Text() string {
	var b strings.Builder
	verdict := "INFEASIBLE"
	if o.Feasible {
		verdict = "FEASIBLE"
	}
	fmt.Fprintf(&b, "%s (%s): %s\n", verdict, o.Reason, o.Message)
	if o.Feasible {
		fmt.Fprintf(&b, "  preset:  %s\n", o.SelectedPreset)
		fmt.Fprintf(&b, "  nodes:   %d\n", o.NodeCount)
	}
	switch {
	case !o.Airtime.Evaluated:
		fmt.Fprintf(&b, "  airtime: not evaluated (%s)\n", o.Airtime.Reason)
	case o.Airtime.WithinBudget:
		fmt.Fprintf(&b, "  airtime: within budget, floor ~%d%% (%s)\n", o.Airtime.PredictedUtilizationPercent, o.Airtime.Reason)
	default:
		fmt.Fprintf(&b, "  airtime: OVER BUDGET, floor ~%d%% (%s)\n", o.Airtime.PredictedUtilizationPercent, o.Airtime.Reason)
	}
	for _, n := range o.ProposedNodes {
		fmt.Fprintf(&b, "  node %s -> region=%s preset=%s role=%s\n", n.Name, n.Spec.Region, n.Spec.ModemPreset, n.Spec.Role)
	}
	return b.String()
}

// trimSpace reports the input with leading and trailing ASCII whitespace removed,
// without importing strings for one call in a hot-path-free helper.
func trimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && isSpace(b[start]) {
		start++
	}
	end := len(b)
	for end > start && isSpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
