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

// minHealthyDelivery is the delivery ratio a window must reach before a "held
// up" or "survived" verdict is asserted. It stops the reducer from calling a run
// that never delivered a success: a zero (or near-zero) baseline has nothing to
// hold up and nothing to recover to.
const minHealthyDelivery = 0.5

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
		if !strings.HasPrefix(line, marker) {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line[len(marker):]), &e); err != nil {
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
	sentT, recvBySeq, resolved := index(events, receivers)

	var beforeLat, afterLat []float64
	var before, after Window
	for seq, ts := range sentT {
		w, lat := &before, &beforeLat
		if ts >= perturbationT {
			w, lat = &after, &afterLat
		}
		w.Sent++
		w.Expected += len(resolved)
		for _, tr := range recvBySeq[seq] {
			w.Delivered++
			*lat = append(*lat, (tr-ts)*1000)
		}
	}
	finalize(&before, beforeLat)
	finalize(&after, afterLat)

	drop := before.DeliveryRatio - after.DeliveryRatio
	return Report{
		Receivers:     resolved,
		PerturbationT: perturbationT,
		Before:        before,
		After:         after,
		Tolerance:     tolerance,
		// "Held up" requires a healthy baseline to hold up (a run that never
		// delivered has nothing to preserve) and delivery no worse than that
		// baseline beyond the tolerance. This mirrors the baseline-healthy check
		// ReducePhases applies to its first phase, so the two reducers agree.
		HeldUp: before.DeliveryRatio >= minHealthyDelivery && after.Sent > 0 && drop <= tolerance,
	}
}

// index builds the per-message send times and the per-seq, per-receiver earliest
// receipt map, resolving the receiver set (inferred from receipts when receivers
// is nil). Reduce and ReducePhases share it, so the crediting rules (one credit
// per node per message; receipts on out-of-scope nodes ignored) live in one place.
func index(events []Event, receivers []string) (sentT map[int]float64,
	recvBySeq map[int]map[string]float64, resolved []string) {
	resolved = receivers
	if len(resolved) == 0 {
		set := map[string]struct{}{}
		for _, e := range events {
			if e.Ev == "recv" {
				set[e.Node] = struct{}{}
			}
		}
		for n := range set {
			resolved = append(resolved, n)
		}
		sort.Strings(resolved)
	}
	inScope := make(map[string]struct{}, len(resolved))
	for _, n := range resolved {
		inScope[n] = struct{}{}
	}

	sentT = map[int]float64{}
	for _, e := range events {
		if e.Ev != "sent" {
			continue
		}
		if _, ok := sentT[e.Seq]; !ok {
			sentT[e.Seq] = e.T
		}
	}
	recvBySeq = map[int]map[string]float64{}
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
	return sentT, recvBySeq, resolved
}

// Phase is the delivery outcome for the messages ORIGINATED within one labeled,
// half-open span [StartT, EndT) of the timeline (for example baseline, degraded,
// adapted). A receipt is credited to the phase of its own send, never the phase
// it arrives in, so a late receipt never crosses a boundary.
type Phase struct {
	Label string `json:"label"`
	// StartT and EndT bound the phase's half-open span; nil means open (the first
	// phase starts at the beginning, the last ends at the end). Pointers, not an
	// infinity, so the report marshals to JSON (encoding/json rejects Inf/NaN).
	StartT *float64 `json:"startT"`
	EndT   *float64 `json:"endT"`
	Window Window   `json:"window"`
	// DropVsBaseline is phase[0].DeliveryRatio minus this phase's ratio (0 for the
	// baseline itself). Degraded is DropVsBaseline greater than Tolerance.
	DropVsBaseline float64 `json:"dropVsBaseline"`
	Degraded       bool    `json:"degraded"`
}

// PhaseReport is the N-phase generalization of Report: a labeled delivery
// timeline and a machine-checkable survival verdict measured against the first
// (baseline) phase.
type PhaseReport struct {
	Receivers  []string  `json:"receivers"`
	Boundaries []float64 `json:"boundaries"`
	Phases     []Phase   `json:"phases"`
	Tolerance  float64   `json:"tolerance"`
	// Valid is false when the inputs are ill-formed (labels count is not
	// boundaries+1, boundaries are not ascending, or a phase has no originated
	// traffic to judge); a pure report, never a panic.
	Valid bool `json:"valid"`
	// Survived is the verdict: some non-baseline phase Degraded AND the final
	// phase is not Degraded (delivery fell under load, then recovered to within
	// Tolerance of baseline). This is the "held up" for a multi-phase timeline.
	Survived bool `json:"survived"`
}

// ReducePhases buckets events into len(labels) phases split by the ascending
// boundary times, computes each phase's Window and its delivery drop versus the
// baseline (first) phase, and derives the Survived verdict. len(labels) must be
// len(boundaries)+1 and boundaries must be ascending; a malformed call returns
// Valid=false rather than panicking. receivers nil is inferred from receipts.
func ReducePhases(events []Event, boundaries []float64, labels []string,
	tolerance float64, receivers []string) PhaseReport {
	rep := PhaseReport{Boundaries: boundaries, Tolerance: tolerance}
	if len(labels) != len(boundaries)+1 {
		return rep
	}
	for i := 1; i < len(boundaries); i++ {
		if boundaries[i] <= boundaries[i-1] {
			return rep // boundaries must be strictly ascending
		}
	}

	sentT, recvBySeq, resolved := index(events, receivers)
	rep.Receivers = resolved
	n := len(labels)
	windows := make([]Window, n)
	lats := make([][]float64, n)
	for seq, ts := range sentT {
		// Phase index = number of boundaries at or below ts (inclusive phase
		// start): a message sent exactly at boundary Tk belongs to the later phase.
		i := sort.Search(len(boundaries), func(k int) bool { return boundaries[k] > ts })
		windows[i].Sent++
		windows[i].Expected += len(resolved)
		for _, tr := range recvBySeq[seq] {
			windows[i].Delivered++
			lats[i] = append(lats[i], (tr-ts)*1000)
		}
	}

	rep.Valid = true
	rep.Phases = make([]Phase, n)
	for i := range windows {
		finalize(&windows[i], lats[i])
		var start, end *float64
		if i > 0 {
			start = &boundaries[i-1]
		}
		if i < len(boundaries) {
			end = &boundaries[i]
		}
		p := Phase{Label: labels[i], StartT: start, EndT: end, Window: windows[i]}
		if i > 0 {
			p.DropVsBaseline = windows[0].DeliveryRatio - windows[i].DeliveryRatio
			p.Degraded = p.DropVsBaseline > tolerance
		}
		rep.Phases[i] = p
		if windows[i].Sent == 0 {
			rep.Valid = false // a phase with no traffic cannot be judged
		}
	}

	anyDegraded := false
	for i := 1; i < n; i++ {
		if rep.Phases[i].Degraded {
			anyDegraded = true
		}
	}
	// Survival requires a healthy baseline to fall from and recover to: a run that
	// never delivered has nothing to survive.
	rep.Survived = rep.Valid && anyDegraded && !rep.Phases[n-1].Degraded &&
		rep.Phases[0].Window.DeliveryRatio >= minHealthyDelivery
	return rep
}

// Text renders the labeled delivery timeline and the survival verdict.
func (r PhaseReport) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "airtime-commons survival report (receivers: %s)\n", strings.Join(r.Receivers, ", "))
	if !r.Valid {
		fmt.Fprintln(&b, "  invalid: phase labels/boundaries mismatch, or a phase had no traffic")
		return b.String()
	}
	n := len(r.Phases)
	anyDegraded := false
	for i := 1; i < n; i++ {
		if r.Phases[i].Degraded {
			anyDegraded = true
		}
	}
	for i, p := range r.Phases {
		note := ""
		switch {
		case p.Degraded:
			note = fmt.Sprintf("  (down %.1f pts vs baseline)", p.DropVsBaseline*100)
		case i == n-1 && anyDegraded:
			note = "  (recovered)"
		}
		fmt.Fprintf(&b, "  %-9s delivery %5.1f%% (%d/%d), latency p50 %.0f ms%s\n",
			p.Label, p.Window.DeliveryRatio*100, p.Window.Delivered, p.Window.Expected, p.Window.LatencyMsP50, note)
	}
	var verdict string
	switch {
	case r.Survived:
		verdict = "delivery fell under load and recovered after the adaptation"
	case r.Phases[0].Window.DeliveryRatio < minHealthyDelivery:
		verdict = "baseline delivery was not healthy; nothing to judge"
	case anyDegraded:
		verdict = "delivery fell under load and did not recover"
	default:
		verdict = "delivery held across all phases (no degradation to recover from)"
	}
	fmt.Fprintf(&b, "  verdict: %s\n", verdict)
	return b.String()
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
	var verdict string
	switch {
	case r.HeldUp:
		verdict = "mesh kept delivering across the perturbation (management plane gone, traffic unaffected)"
	case r.Before.DeliveryRatio < minHealthyDelivery:
		verdict = "baseline delivery was not healthy; nothing to judge"
	default:
		verdict = "delivery fell after the perturbation"
	}
	fmt.Fprintf(&b, "  verdict: %s\n", verdict)
	return b.String()
}
