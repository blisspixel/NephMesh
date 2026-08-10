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
