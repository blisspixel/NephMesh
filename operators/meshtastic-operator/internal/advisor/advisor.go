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

// Package advisor asks a local language model to reason over sensed spectrum and
// mesh state and PROPOSE a mesh action. It is the edge-brain half of the intent
// loop: a model running on the sensor host itself (no cloud) reads the occupancy,
// the classified emissions, and the current radio config, and recommends whether
// to hold or reconfigure, with a rationale a human can weigh.
//
// It is advisory and report-only by construction. The model never actuates; it
// returns a recommendation that a human (or, later, the safety kernel of ADR
// 0002) approves. This keeps the "AI proposes, the reconcile loop enforces, a
// human approves" separation the project depends on: the model is not the control
// loop. The LLM is reached through a narrow interface so the prompt-building and
// the parsing are deterministic and unit-tested; only the model call is external.
package advisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/spectrum"
)

// LLMClient is the narrow seam to a language model. Complete sends a system and a
// user prompt and returns the model's raw text. The Ollama client implements it;
// tests use a stub.
type LLMClient interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// Action is the recommended move. The set is deliberately small and every option
// is safe to surface report-only.
type Action string

const (
	// ActionHold recommends no change (the default and the safe answer).
	ActionHold Action = "hold"
	// ActionChangePreset recommends moving to a different approved modem preset.
	ActionChangePreset Action = "change_preset"
	// ActionInvestigate recommends a human look, for example at suspected
	// interference the sensor cannot classify confidently.
	ActionInvestigate Action = "investigate"
)

// Confidence is categorical: models express calibrated categories better than
// spurious numeric precision.
type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

// Recommendation is the model's proposed action. It actuates nothing.
type Recommendation struct {
	Action       Action     `json:"action"`
	TargetPreset string     `json:"targetPreset,omitempty"`
	Rationale    string     `json:"rationale"`
	Confidence   Confidence `json:"confidence"`
}

// Situation is the sensed and declared context handed to the model.
type Situation struct {
	Band            spectrum.BandStats
	Emissions       []spectrum.Emission
	CurrentPreset   string
	Region          string
	ApprovedPresets []string
	// ChannelCeilingPercent is the recommended occupancy ceiling for the band.
	ChannelCeilingPercent float64
}

// Advisor turns a Situation into a Recommendation via the model.
type Advisor struct {
	LLM LLMClient
}

// New builds an Advisor over the given model client.
func New(llm LLMClient) *Advisor { return &Advisor{LLM: llm} }

const systemPrompt = `You are a cautious spectrum-management advisor for a LoRa mesh network used for resilient communications. You do not control anything: you only propose an action that a human will review. Prefer HOLD unless there is clear, sustained evidence that a change helps. A single noisy reading is not evidence. Respect the airtime commons: prefer changes that reduce airtime pressure, and never recommend a preset outside the approved set. Spectrum sensing is receive-only. Reply with a single JSON object and nothing else.`

// Advise builds the prompt, calls the model, and parses the recommendation. It
// returns the recommendation and the model's raw reply (useful for logging and
// for showing a human what the model actually said).
func (a *Advisor) Advise(ctx context.Context, s Situation) (Recommendation, string, error) {
	raw, err := a.LLM.Complete(ctx, systemPrompt, buildPrompt(s))
	if err != nil {
		return Recommendation{}, "", err
	}
	rec, err := parseRecommendation(raw, s.ApprovedPresets)
	return rec, raw, err
}

// buildPrompt renders the situation as a compact, model-legible brief.
func buildPrompt(s Situation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sensed spectrum for band %s (%.0f-%.0f MHz):\n", s.Band.Band.Name, s.Band.Band.LowHz/1e6, s.Band.Band.HighHz/1e6)
	if s.Band.BinCount == 0 {
		b.WriteString("  the sweep did not cover this band (no data).\n")
	} else {
		fmt.Fprintf(&b, "  occupancy %.1f%% (recommended ceiling %.0f%%), noise floor %.1f dB, peak %.1f dB at %.3f MHz.\n",
			s.Band.OccupancyPercent, s.ChannelCeilingPercent, s.Band.NoiseFloorDB, s.Band.MaxDB, s.Band.PeakFreqHz/1e6)
	}
	if len(s.Emissions) == 0 {
		b.WriteString("Classified emissions: none detected.\n")
	} else {
		b.WriteString("Classified emissions (packet = LoRa-like mesh traffic, continuous/wideband = possible interference):\n")
		for _, e := range s.Emissions {
			fmt.Fprintf(&b, "  - %s at %.3f MHz, %.0f kHz wide, duty %.0f%%, peak %.1f dB, confidence %.2f\n",
				e.Class, e.PeakFreqHz/1e6, e.BandwidthHz/1e3, e.DutyCyclePercent, e.PeakDB, e.Confidence)
		}
	}
	fmt.Fprintf(&b, "Current modem preset: %s. Region: %s. Approved presets: %s.\n",
		s.CurrentPreset, s.Region, strings.Join(s.ApprovedPresets, ", "))
	b.WriteString("\nRecommend one action. Reply with JSON only:\n")
	b.WriteString(`{"action":"hold|change_preset|investigate","targetPreset":"<an approved preset, only if action is change_preset>","rationale":"<one or two sentences>","confidence":"low|medium|high"}`)
	return b.String()
}

// parseRecommendation extracts the JSON object from the model reply (models
// sometimes wrap it in prose or code fences), validates the action and the target
// preset against the approved set, and normalizes confidence. An out-of-set
// preset or an unknown action is downgraded to a safe HOLD with a note rather
// than trusted, so a hallucinated field can never propose an illegal change.
func parseRecommendation(raw string, approved []string) (Recommendation, error) {
	obj := extractJSONObject(raw)
	if obj == "" {
		return Recommendation{}, fmt.Errorf("no JSON object in model reply: %q", truncate(raw, 200))
	}
	var rec Recommendation
	if err := json.Unmarshal([]byte(obj), &rec); err != nil {
		return Recommendation{}, fmt.Errorf("parse model reply: %w", err)
	}

	switch rec.Action {
	case ActionHold, ActionInvestigate:
		rec.TargetPreset = ""
	case ActionChangePreset:
		if !contains(approved, rec.TargetPreset) {
			// A change to a non-approved (or empty) preset is not actionable;
			// keep the rationale but make the proposal safe.
			rec.Action = ActionInvestigate
			rec.TargetPreset = ""
			rec.Rationale = "model proposed a preset outside the approved set; downgraded to investigate. " + rec.Rationale
		}
	default:
		rec.Action = ActionHold
		rec.TargetPreset = ""
		rec.Rationale = "model returned an unrecognized action; defaulting to hold. " + rec.Rationale
	}

	switch rec.Confidence {
	case ConfidenceLow, ConfidenceMedium, ConfidenceHigh:
	default:
		rec.Confidence = ConfidenceLow
	}
	return rec, nil
}

// extractJSONObject returns the first balanced {...} run in s, tolerating code
// fences and surrounding prose.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case c == '\\':
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
