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

package resilience

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEventsTakesMarkedLinesOnly(t *testing.T) {
	in := strings.Join([]string{
		"lora-decode: starting",                                  // noise
		`EVENT {"ev": "sent", "seq": 0, "node": "sim1", "t": 1}`, // event
		`EVENT {"ev": "recv", "seq": 0, "node": "sim2", "t": 1.2}`,
		`EVENT {"ev": "bogus", "seq": 9, "node": "x", "t": 5}`,    // wrong ev, dropped
		`EVENT {not json}`,                                        // malformed, skipped
		`SUMMARY {"sent": 1, "delivered": 1}`,                    // summary ignored
	}, "\n")
	evs, err := ParseEvents(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, evs, 2)
	assert.Equal(t, "sent", evs[0].Ev)
	assert.Equal(t, "sim2", evs[1].Node)
}

// events builds a fully-delivered run: seqs 0..n-1 sent one second apart, each
// received by every receiver latencyMs after it was sent.
func events(n int, receivers []string, latencyMs float64) []Event {
	var out []Event
	for i := 0; i < n; i++ {
		ts := float64(i)
		out = append(out, Event{Ev: "sent", Seq: i, Node: "sim1", T: ts})
		for _, r := range receivers {
			out = append(out, Event{Ev: "recv", Seq: i, Node: r, T: ts + latencyMs/1000})
		}
	}
	return out
}

func TestReduceHeldUpWhenDeliveryUnchanged(t *testing.T) {
	recv := []string{"sim2", "sim3"}
	evs := events(6, recv, 200) // sent at t=0..5, perturbation at t=3
	r := Reduce(evs, 3, 0.1, recv)

	assert.Equal(t, 3, r.Before.Sent)
	assert.Equal(t, 3, r.After.Sent)
	assert.Equal(t, 6, r.Before.Expected) // 3 sent * 2 receivers
	assert.Equal(t, 6, r.Before.Delivered)
	assert.InDelta(t, 1.0, r.Before.DeliveryRatio, 1e-9)
	assert.InDelta(t, 1.0, r.After.DeliveryRatio, 1e-9)
	assert.InDelta(t, 200, r.After.LatencyMsP50, 1e-6)
	assert.True(t, r.HeldUp, "delivery unchanged across the perturbation")
}

func TestReduceNotHeldUpWhenDeliveryFalls(t *testing.T) {
	recv := []string{"sim2", "sim3"}
	// Full delivery before t=3; after t=3 the receipts are dropped entirely.
	var evs []Event
	for i := 0; i < 6; i++ {
		evs = append(evs, Event{Ev: "sent", Seq: i, Node: "sim1", T: float64(i)})
		if i < 3 {
			for _, r := range recv {
				evs = append(evs, Event{Ev: "recv", Seq: i, Node: r, T: float64(i) + 0.2})
			}
		}
	}
	r := Reduce(evs, 3, 0.1, recv)
	assert.InDelta(t, 1.0, r.Before.DeliveryRatio, 1e-9)
	assert.InDelta(t, 0.0, r.After.DeliveryRatio, 1e-9)
	assert.False(t, r.HeldUp, "a full drop after the perturbation is not held up")
}

func TestReduceInfersReceiversFromReceipts(t *testing.T) {
	recv := []string{"sim2", "sim3"}
	evs := events(4, recv, 150)
	r := Reduce(evs, 2, 0.1, nil) // no explicit receivers
	assert.Equal(t, []string{"sim2", "sim3"}, r.Receivers)
	assert.True(t, r.HeldUp)
}

func TestReduceCreditsANodeAtMostOncePerMessage(t *testing.T) {
	recv := []string{"sim2"}
	// sim2 reports seq 0 twice (a rebroadcast the node delivered again); it must
	// still count as one delivery, so delivered never exceeds expected.
	evs := []Event{
		{Ev: "sent", Seq: 0, Node: "sim1", T: 0},
		{Ev: "recv", Seq: 0, Node: "sim2", T: 0.2},
		{Ev: "recv", Seq: 0, Node: "sim2", T: 0.9},
	}
	r := Reduce(evs, 100, 0.1, recv) // everything is "before"
	assert.Equal(t, 1, r.Before.Expected)
	assert.Equal(t, 1, r.Before.Delivered)
	assert.InDelta(t, 200, r.Before.LatencyMsP50, 1e-6, "earliest receipt sets latency")
}

func TestReduceIgnoresOutOfScopeReceivers(t *testing.T) {
	// A receipt on a node not in the requested receiver set is not counted.
	evs := []Event{
		{Ev: "sent", Seq: 0, Node: "sim1", T: 0},
		{Ev: "recv", Seq: 0, Node: "sim9", T: 0.2},
	}
	r := Reduce(evs, 100, 0.1, []string{"sim2"})
	assert.Equal(t, 0, r.Before.Delivered)
	assert.InDelta(t, 0.0, r.Before.DeliveryRatio, 1e-9)
}

func TestReduceNotHeldUpWithNoTrafficAfter(t *testing.T) {
	recv := []string{"sim2"}
	evs := events(3, recv, 100) // all sent before t=10
	r := Reduce(evs, 10, 0.1, recv)
	assert.Equal(t, 0, r.After.Sent)
	assert.False(t, r.HeldUp, "no post-perturbation traffic means nothing was shown")
}

func TestReduceToleranceAbsorbsSmallDrop(t *testing.T) {
	recv := []string{"sim2", "sim3"}
	// Before: 2/2 msgs fully delivered. After: 2 msgs, one receiver misses one
	// message -> 3/4 = 0.75, a 0.25 drop.
	evs := []Event{
		{Ev: "sent", Seq: 0, Node: "sim1", T: 0}, {Ev: "recv", Seq: 0, Node: "sim2", T: 0.1}, {Ev: "recv", Seq: 0, Node: "sim3", T: 0.1},
		{Ev: "sent", Seq: 1, Node: "sim1", T: 1}, {Ev: "recv", Seq: 1, Node: "sim2", T: 1.1}, {Ev: "recv", Seq: 1, Node: "sim3", T: 1.1},
		{Ev: "sent", Seq: 2, Node: "sim1", T: 5}, {Ev: "recv", Seq: 2, Node: "sim2", T: 5.1}, {Ev: "recv", Seq: 2, Node: "sim3", T: 5.1},
		{Ev: "sent", Seq: 3, Node: "sim1", T: 6}, {Ev: "recv", Seq: 3, Node: "sim2", T: 6.1},
	}
	tight := Reduce(evs, 5, 0.1, recv)
	assert.False(t, tight.HeldUp, "a 0.25 drop exceeds a 0.1 tolerance")
	loose := Reduce(evs, 5, 0.3, recv)
	assert.True(t, loose.HeldUp, "the same drop is within a 0.3 tolerance")
}

func TestReportText(t *testing.T) {
	recv := []string{"sim2", "sim3"}
	r := Reduce(events(4, recv, 200), 2, 0.1, recv)
	txt := r.Text()
	assert.Contains(t, txt, "before")
	assert.Contains(t, txt, "after")
	assert.Contains(t, txt, "kept delivering")
}

// phased builds a sent event at ts (from sim1) plus, when deliver, a receipt on
// each receiver latencyMs later.
func phased(recv []string, seq int, ts float64, deliver bool) []Event {
	out := []Event{{Ev: "sent", Seq: seq, Node: "sim1", T: ts}}
	if deliver {
		for _, r := range recv {
			out = append(out, Event{Ev: "recv", Seq: seq, Node: r, T: ts + 0.2})
		}
	}
	return out
}

func TestReducePhasesThreePhaseSurvival(t *testing.T) {
	recv := []string{"sim2", "sim3"}
	var evs []Event
	for _, e := range [][]Event{
		phased(recv, 0, 0, true), phased(recv, 1, 1, true), phased(recv, 2, 2, true), // baseline
		phased(recv, 3, 10, false), phased(recv, 4, 11, false), phased(recv, 5, 12, false), // degraded
		phased(recv, 6, 20, true), phased(recv, 7, 21, true), phased(recv, 8, 22, true), // adapted
	} {
		evs = append(evs, e...)
	}
	r := ReducePhases(evs, []float64{5, 15}, []string{"baseline", "degraded", "adapted"}, 0.1, recv)
	require.True(t, r.Valid)
	assert.InDelta(t, 1.0, r.Phases[0].Window.DeliveryRatio, 1e-9)
	assert.InDelta(t, 0.0, r.Phases[1].Window.DeliveryRatio, 1e-9)
	assert.True(t, r.Phases[1].Degraded)
	assert.InDelta(t, 1.0, r.Phases[2].Window.DeliveryRatio, 1e-9)
	assert.False(t, r.Phases[2].Degraded)
	assert.True(t, r.Survived)
	txt := r.Text()
	assert.Contains(t, txt, "recovered")
	assert.Contains(t, txt, "degraded")
}

func TestReducePhasesNoRecovery(t *testing.T) {
	recv := []string{"sim2"}
	var evs []Event
	for _, e := range [][]Event{
		phased(recv, 0, 0, true), phased(recv, 1, 1, true), // baseline 100%
		phased(recv, 2, 10, false), phased(recv, 3, 11, false), // degraded
		phased(recv, 4, 20, false), phased(recv, 5, 21, false), // still degraded
	} {
		evs = append(evs, e...)
	}
	r := ReducePhases(evs, []float64{5, 15}, []string{"baseline", "degraded", "adapted"}, 0.1, recv)
	require.True(t, r.Valid)
	assert.True(t, r.Phases[1].Degraded)
	assert.True(t, r.Phases[2].Degraded)
	assert.False(t, r.Survived)
	assert.Contains(t, r.Text(), "did not recover")
}

func TestReducePhasesAllHealthy(t *testing.T) {
	recv := []string{"sim2"}
	var evs []Event
	for i := 0; i < 6; i++ {
		evs = append(evs, phased(recv, i, float64(i*4), true)...) // spaced, all deliver
	}
	r := ReducePhases(evs, []float64{6, 14}, []string{"baseline", "degraded", "adapted"}, 0.1, recv)
	require.True(t, r.Valid)
	assert.False(t, r.Survived, "nothing degraded means nothing to survive")
	assert.Contains(t, r.Text(), "held across all phases")
}

func TestReducePhasesCustodyAcrossBoundary(t *testing.T) {
	recv := []string{"sim2"}
	// Sent at 4.9 (phase 0), receipts land at 5.1 (after the boundary at 5): the
	// message is still credited to phase 0, and delivered there.
	evs := []Event{
		{Ev: "sent", Seq: 0, Node: "sim1", T: 4.9},
		{Ev: "recv", Seq: 0, Node: "sim2", T: 5.1},
		{Ev: "sent", Seq: 1, Node: "sim1", T: 6},
		{Ev: "recv", Seq: 1, Node: "sim2", T: 6.2},
	}
	r := ReducePhases(evs, []float64{5}, []string{"before", "after"}, 0.1, recv)
	require.True(t, r.Valid)
	assert.Equal(t, 1, r.Phases[0].Window.Sent, "the 4.9 send belongs to phase 0")
	assert.Equal(t, 1, r.Phases[0].Window.Delivered, "its late receipt still counts in phase 0")
	assert.Equal(t, 1, r.Phases[1].Window.Sent)
}

func TestReducePhasesBoundaryInclusiveStart(t *testing.T) {
	recv := []string{"sim2"}
	// A message sent exactly at boundary 5 belongs to the later phase [5, +inf).
	evs := []Event{
		{Ev: "sent", Seq: 0, Node: "sim1", T: 1},
		{Ev: "recv", Seq: 0, Node: "sim2", T: 1.2},
		{Ev: "sent", Seq: 1, Node: "sim1", T: 5},
		{Ev: "recv", Seq: 1, Node: "sim2", T: 5.2},
	}
	r := ReducePhases(evs, []float64{5}, []string{"a", "b"}, 0.1, recv)
	require.True(t, r.Valid)
	assert.Equal(t, 1, r.Phases[0].Window.Sent)
	assert.Equal(t, 1, r.Phases[1].Window.Sent, "T==boundary lands in the later phase")
}

func TestReducePhasesInfersReceivers(t *testing.T) {
	recv := []string{"sim2", "sim3"}
	var evs []Event
	for _, e := range [][]Event{
		phased(recv, 0, 0, true), phased(recv, 1, 1, true),
		phased(recv, 2, 10, false), phased(recv, 3, 11, false),
		phased(recv, 4, 20, true), phased(recv, 5, 21, true),
	} {
		evs = append(evs, e...)
	}
	r := ReducePhases(evs, []float64{5, 15}, []string{"baseline", "degraded", "adapted"}, 0.1, nil)
	assert.Equal(t, []string{"sim2", "sim3"}, r.Receivers)
	assert.True(t, r.Survived)
}

func TestReducePhasesInvalidInputs(t *testing.T) {
	recv := []string{"sim2"}
	evs := phased(recv, 0, 0, true)
	// labels count must be boundaries+1.
	bad := ReducePhases(evs, []float64{5, 15}, []string{"a", "b"}, 0.1, recv)
	assert.False(t, bad.Valid)
	assert.Contains(t, bad.Text(), "invalid")
	// non-ascending boundaries.
	desc := ReducePhases(evs, []float64{15, 5}, []string{"a", "b", "c"}, 0.1, recv)
	assert.False(t, desc.Valid)
	// duplicate boundaries are not strictly ascending.
	dup := ReducePhases(evs, []float64{5, 5}, []string{"a", "b", "c"}, 0.1, recv)
	assert.False(t, dup.Valid, "duplicate boundary values are rejected")
	// a phase with no traffic cannot be judged.
	empty := ReducePhases(phased(recv, 0, 0, true), []float64{5}, []string{"a", "b"}, 0.1, recv)
	assert.False(t, empty.Valid, "phase b has no sends")
}

// A PhaseReport must marshal to JSON: open phase bounds are null, not the Inf
// value that encoding/json rejects (regression for the -o json phases path).
func TestPhaseReportJSONEncodable(t *testing.T) {
	recv := []string{"sim2", "sim3"}
	r := ReducePhases(events(6, recv, 200), []float64{2, 4}, []string{"baseline", "degraded", "adapted"}, 0.1, recv)
	out, err := json.Marshal(r)
	require.NoError(t, err, "PhaseReport must not contain Inf/NaN")
	assert.Contains(t, string(out), `"startT":null`, "the first phase opens at null")
	assert.Contains(t, string(out), `"endT":null`, "the last phase ends at null")
}

// A run that never delivered is not "held up": a zero baseline is not success.
func TestReduceZeroBaselineNotHeldUp(t *testing.T) {
	recv := []string{"sim2"}
	// Sends only, no receipts, on both sides of the split.
	evs := []Event{
		{Ev: "sent", Seq: 0, Node: "sim1", T: 0},
		{Ev: "sent", Seq: 1, Node: "sim1", T: 10},
	}
	r := Reduce(evs, 5, 0.1, recv)
	assert.InDelta(t, 0.0, r.After.DeliveryRatio, 1e-9)
	assert.False(t, r.HeldUp, "zero delivery must not read as held up")
	assert.Contains(t, r.Text(), "not healthy")
}

func TestReducePhasesZeroBaselineNotSurvived(t *testing.T) {
	recv := []string{"sim2"}
	// Baseline never delivers; a later phase does. Without a baseline to fall
	// from, this is not survival.
	var evs []Event
	for _, e := range [][]Event{
		phased(recv, 0, 0, false), phased(recv, 1, 1, false), // baseline 0%
		phased(recv, 2, 10, false), phased(recv, 3, 11, false), // degraded 0%
		phased(recv, 4, 20, true), phased(recv, 5, 21, true), // "adapted" 100%
	} {
		evs = append(evs, e...)
	}
	r := ReducePhases(evs, []float64{5, 15}, []string{"baseline", "degraded", "adapted"}, 0.1, recv)
	assert.False(t, r.Survived, "no healthy baseline means nothing survived")
	assert.Contains(t, r.Text(), "baseline delivery was not healthy")
}

// ParseEvents takes the marker only as a line prefix, so a log line that merely
// contains "EVENT " mid-string is not ingested as an event.
func TestParseEventsPrefixOnly(t *testing.T) {
	in := strings.Join([]string{
		`log: the EVENT {"ev": "sent", "seq": 7, "node": "x", "t": 1} was noted`, // mid-line, ignored
		`EVENT {"ev": "sent", "seq": 0, "node": "sim1", "t": 1}`,                  // real
	}, "\n")
	evs, err := ParseEvents(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, evs, 1)
	assert.Equal(t, 0, evs[0].Seq)
}
