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

package advisor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/spectrum"
)

// stubLLM returns a canned reply and records the prompts it was given.
type stubLLM struct {
	reply        string
	err          error
	system, user string
}

func (s *stubLLM) Complete(_ context.Context, system, user string) (string, error) {
	s.system, s.user = system, user
	return s.reply, s.err
}

func sampleSituation() Situation {
	return Situation{
		Band:                  spectrum.BandStats{Band: spectrum.Band{Name: "ism-915-us", LowHz: 902e6, HighHz: 928e6}, BinCount: 26, OccupancyPercent: 34, NoiseFloorDB: -69, MaxDB: -17, PeakFreqHz: 906.5e6},
		Emissions:             []spectrum.Emission{{Class: spectrum.ClassPacket, PeakFreqHz: 906.5e6, BandwidthHz: 250e3, DutyCyclePercent: 12, PeakDB: -17, Confidence: 0.8}},
		CurrentPreset:         "LONG_FAST",
		Region:                "US",
		ApprovedPresets:       []string{"LONG_FAST", "MEDIUM_SLOW"},
		ChannelCeilingPercent: 25,
	}
}

func TestBuildPromptIncludesTheSituation(t *testing.T) {
	p := buildPrompt(sampleSituation())
	for _, want := range []string{"ism-915-us", "occupancy 34", "packet", "906.5", "LONG_FAST", "MEDIUM_SLOW", "JSON"} {
		assert.Contains(t, p, want)
	}
}

func TestAdviseParsesAValidRecommendation(t *testing.T) {
	llm := &stubLLM{reply: `{"action":"hold","rationale":"packet traffic within budget","confidence":"high"}`}
	rec, raw, err := New(llm).Advise(context.Background(), sampleSituation())
	require.NoError(t, err)
	assert.Equal(t, ActionHold, rec.Action)
	assert.Equal(t, ConfidenceHigh, rec.Confidence)
	assert.NotEmpty(t, raw)
	// The model was handed the report-only framing.
	assert.Contains(t, llm.system, "only propose")
}

func TestParseHandlesFencedAndProseWrappedJSON(t *testing.T) {
	fenced := "Here is my answer:\n```json\n{\"action\":\"hold\",\"rationale\":\"quiet\",\"confidence\":\"medium\"}\n```\nthanks"
	rec, err := parseRecommendation(fenced, []string{"LONG_FAST"})
	require.NoError(t, err)
	assert.Equal(t, ActionHold, rec.Action)
	assert.Equal(t, ConfidenceMedium, rec.Confidence)
}

func TestChangePresetToApprovedIsKept(t *testing.T) {
	reply := `{"action":"change_preset","targetPreset":"MEDIUM_SLOW","rationale":"reduce airtime","confidence":"medium"}`
	rec, err := parseRecommendation(reply, []string{"LONG_FAST", "MEDIUM_SLOW"})
	require.NoError(t, err)
	assert.Equal(t, ActionChangePreset, rec.Action)
	assert.Equal(t, "MEDIUM_SLOW", rec.TargetPreset)
}

func TestChangePresetToUnapprovedIsDowngraded(t *testing.T) {
	// A hallucinated preset outside the approved set must never survive as an
	// actionable change.
	reply := `{"action":"change_preset","targetPreset":"SHORT_TURBO","rationale":"faster","confidence":"high"}`
	rec, err := parseRecommendation(reply, []string{"LONG_FAST", "MEDIUM_SLOW"})
	require.NoError(t, err)
	assert.Equal(t, ActionInvestigate, rec.Action, "an out-of-set preset is downgraded, not obeyed")
	assert.Empty(t, rec.TargetPreset)
	assert.Contains(t, rec.Rationale, "outside the approved set")
}

func TestUnknownActionDefaultsToHold(t *testing.T) {
	rec, err := parseRecommendation(`{"action":"launch_missiles","rationale":"x","confidence":"high"}`, []string{"LONG_FAST"})
	require.NoError(t, err)
	assert.Equal(t, ActionHold, rec.Action)
}

func TestInvalidConfidenceDefaultsToLow(t *testing.T) {
	rec, err := parseRecommendation(`{"action":"hold","rationale":"x","confidence":"absolutely"}`, nil)
	require.NoError(t, err)
	assert.Equal(t, ConfidenceLow, rec.Confidence)
}

func TestParseNoJSONIsError(t *testing.T) {
	_, err := parseRecommendation("I cannot help with that.", nil)
	assert.Error(t, err)
}

func TestExtractJSONObjectHandlesNestedAndStrings(t *testing.T) {
	// Braces inside a string must not end the object early.
	in := `prefix {"a":"has } brace","b":{"c":1}} suffix`
	assert.Equal(t, `{"a":"has } brace","b":{"c":1}}`, extractJSONObject(in))
}
