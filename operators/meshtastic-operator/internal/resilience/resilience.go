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

// Package resilience reduces the delivery-probe event log to a before/after
// verdict around a perturbation, the measurement half of the resilience harness
// (demo/resilience). It answers one question with a number: did the mesh keep
// delivering across an event (a killed management plane, a killed node, a
// congested channel), or did delivery fall?
//
// It is pure and hardware-free: it reads a JSONL event log the probe produced
// and computes per-window delivery ratio and latency, so the reduction is
// unit-tested against synthetic logs and used unchanged against a real run.
package resilience

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Event is one probe event: a broadcast originated by the sender ("sent") or a
// receipt of that broadcast on a receiver node ("recv"). T is Unix seconds.
type Event struct {
	Ev   string  `json:"ev"`
	Seq  int     `json:"seq"`
	Node string  `json:"node"`
	T    float64 `json:"t"`
}

// ParseEvents reads the probe's output and returns its events. Each event is the
// JSON object following an "EVENT " marker; every other line (the SUMMARY line,
// stray logs) is ignored, and a single malformed event is skipped rather than
// failing the whole reduction.
func ParseEvents(r io.Reader) ([]Event, error) {
	const marker = "EVENT "
	var out []Event
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		i := strings.Index(line, marker)
		if i < 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line[i+len(marker):]), &e); err != nil {
			continue
		}
		if e.Ev == "sent" || e.Ev == "recv" {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// Window is the delivery outcome for the messages originated in one time span.
type Window struct {
	Sent          int     `json:"sent"`
	Expected      int     `json:"expected"`
	Delivered     int     `json:"delivered"`
	DeliveryRatio float64 `json:"deliveryRatio"`
	LatencyMsP50  float64 `json:"latencyMsP50"`
	LatencyMsMax  float64 `json:"latencyMsMax"`
}

// Report compares delivery before and after a perturbation.
type Report struct {
	Receivers     []string `json:"receivers"`
	PerturbationT float64  `json:"perturbationT"`
	Before        Window   `json:"before"`
	After         Window   `json:"after"`
	// Tolerance is the largest before-minus-after delivery-ratio drop still
	// treated as unchanged (measurement noise, not a real regression).
	Tolerance float64 `json:"tolerance"`
	// HeldUp is true when messages were originated after the perturbation and
	// their delivery ratio did not fall by more than Tolerance: the mesh kept
	// delivering across the event.
	HeldUp bool `json:"heldUp"`
}

// Reduce splits the events at perturbationT and computes the before and after
// windows. receivers, when nil, is inferred from the receipt events. tolerance
// is the delivery-ratio drop (before minus after) still counted as unchanged.
//
// A message is attributed to a window by when it was originated, so a receipt is
// always compared against its own send, never split across the boundary. A node
// is credited at most once per message, so delivered never exceeds expected.
func Reduce(events []Event, perturbationT, tolerance float64, receivers []string) Report {
	if len(receivers) == 0 {
		set := map[string]struct{}{}
		for _, e := range events {
			if e.Ev == "recv" {
				set[e.Node] = struct{}{}
			}
		}
		for n := range set {
			receivers = append(receivers, n)
		}
		sort.Strings(receivers)
	}
	inScope := make(map[string]struct{}, len(receivers))
	for _, n := range receivers {
		inScope[n] = struct{}{}
	}

	sentT := map[int]float64{}
	for _, e := range events {
		if e.Ev != "sent" {
			continue
		}
		if _, ok := sentT[e.Seq]; !ok {
			sentT[e.Seq] = e.T
		}
	}
	// seq -> node -> earliest receipt time, restricted to in-scope receivers.
	recvBySeq := map[int]map[string]float64{}
	for _, e := range events {
		if e.Ev != "recv" {
			continue
		}
		if _, ok := inScope[e.Node]; !ok {
			continue
		}
		m := recvBySeq[e.Seq]
		if m == nil {
			m = map[string]float64{}
			recvBySeq[e.Seq] = m
		}
		if _, ok := m[e.Node]; !ok {
			m[e.Node] = e.T
		}
	}

	var beforeLat, afterLat []float64
	var before, after Window
	for seq, ts := range sentT {
		w, lat := &before, &beforeLat
		if ts >= perturbationT {
			w, lat = &after, &afterLat
		}
		w.Sent++
		w.Expected += len(receivers)
		for _, tr := range recvBySeq[seq] {
			w.Delivered++
			*lat = append(*lat, (tr-ts)*1000)
		}
	}
	finalize(&before, beforeLat)
	finalize(&after, afterLat)

	drop := before.DeliveryRatio - after.DeliveryRatio
	return Report{
		Receivers:     receivers,
		PerturbationT: perturbationT,
		Before:        before,
		After:         after,
		Tolerance:     tolerance,
		HeldUp:        after.Sent > 0 && drop <= tolerance,
	}
}

// finalize computes the derived fields of a window from its raw latency samples.
func finalize(w *Window, lat []float64) {
	if w.Expected > 0 {
		w.DeliveryRatio = float64(w.Delivered) / float64(w.Expected)
	}
	if len(lat) > 0 {
		sort.Float64s(lat)
		w.LatencyMsP50 = lat[len(lat)/2]
		w.LatencyMsMax = lat[len(lat)-1]
	}
}

// Text renders the report for a person.
func (r Report) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "control-plane-independence report (receivers: %s)\n", strings.Join(r.Receivers, ", "))
	line := func(name string, w Window) {
		fmt.Fprintf(&b, "  %-6s delivery %5.1f%% (%d/%d), latency p50 %.0f ms, max %.0f ms\n",
			name, w.DeliveryRatio*100, w.Delivered, w.Expected, w.LatencyMsP50, w.LatencyMsMax)
	}
	line("before", r.Before)
	line("after", r.After)
	verdict := "mesh kept delivering across the perturbation (management plane gone, traffic unaffected)"
	if !r.HeldUp {
		verdict = "delivery fell after the perturbation"
	}
	fmt.Fprintf(&b, "  verdict: %s\n", verdict)
	return b.String()
}
