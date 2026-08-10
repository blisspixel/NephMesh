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

package spectrum

import (
	"io"
	"math"
	"sort"
)

// Classification moves sensing from "the band is busy" to "busy with what". It
// reads a sweep's time structure (many passes over the same frequencies) rather
// than a single snapshot, so it can tell a packet-shaped emitter (a LoRa mesh:
// narrow and intermittent) from sustained or wideband interference (a carrier or
// a jammer: wide, or always on). It is deliberately a signature separation, not a
// jammer detector: it labels the time-frequency shape of energy with a confidence
// and an explicit unknown. Telling one mesh from another (mine versus a
// neighbor's) needs frame decode, which this does not do; that stays out of
// scope. The whole path is receive-only.

// EmissionClass is the time-frequency shape of a detected emission.
type EmissionClass string

const (
	// ClassPacket is narrow and intermittent: the shape of LoRa/Meshtastic
	// traffic (mine or a neighbor's; distinguishing them needs decode).
	ClassPacket EmissionClass = "packet"
	// ClassContinuous is narrow but sustained: a carrier or a narrowband
	// interferer that holds the frequency.
	ClassContinuous EmissionClass = "continuous"
	// ClassWideband spans many bins: broadband energy, the classic interference
	// or jamming shape when it is also sustained.
	ClassWideband EmissionClass = "wideband"
	// ClassUnknown is energy whose shape does not fall cleanly into the above.
	ClassUnknown EmissionClass = "unknown"
)

// BinSeries is one frequency bin's power over the whole capture: the time series
// classification reads to measure a bin's duty cycle.
type BinSeries struct {
	FreqHz float64
	Powers []float64
}

// Emission is a detected signal: a contiguous run of active frequency bins with
// its measured time-frequency features and a class.
type Emission struct {
	LowHz            float64       `json:"lowHz"`
	HighHz           float64       `json:"highHz"`
	BandwidthHz      float64       `json:"bandwidthHz"`
	PeakFreqHz       float64       `json:"peakFreqHz"`
	PeakDB           float64       `json:"peakDb"`
	DutyCyclePercent float64       `json:"dutyCyclePercent"`
	Class            EmissionClass `json:"class"`
	Confidence       float64       `json:"confidence"`
}

// ClassifyOptions tunes detection and the class boundaries. The defaults are
// heuristics to validate against real captures, not calibrated thresholds.
type ClassifyOptions struct {
	Options
	// PresentDutyPercent is the minimum fraction of samples above a bin's own
	// threshold for the bin to count as carrying an emitter (not just noise).
	PresentDutyPercent float64
	// SustainedDutyPercent is the duty above which a narrow emission is
	// continuous rather than packet-shaped.
	SustainedDutyPercent float64
	// WidebandHz is the bandwidth above which an emission is wideband regardless
	// of duty.
	WidebandHz float64
}

// DefaultClassifyOptions is a reasonable starting point: detection uses a
// band-wide noise floor plus a 10 dB margin (stricter than occupancy, so noise
// jitter does not read as a signal), a bin carries an emitter if it is above that
// threshold in at least 5% of samples, an emission wider than 1.5 MHz is
// wideband, and a narrow emission held above 60% of the time is continuous rather
// than packet-shaped.
func DefaultClassifyOptions() ClassifyOptions {
	return ClassifyOptions{
		Options:              Options{ThresholdMarginDB: 10, NoiseFloorPercentile: 25},
		PresentDutyPercent:   5,
		SustainedDutyPercent: 60,
		WidebandHz:           1_500_000,
	}
}

// ParseSweepSeries reads rtl_power/hackrf_sweep CSV into per-frequency time
// series. Each bin center collects one power sample per sweep pass that covered
// it, preserving the time structure that ParseSweep flattens away. Non-finite
// samples are dropped. Bins are returned sorted by frequency.
func ParseSweepSeries(r io.Reader) ([]BinSeries, error) {
	bins, err := ParseSweep(r)
	if err != nil {
		return nil, err
	}
	// Group by rounded center frequency; hackrf_sweep uses a stable bin grid
	// across passes, so the same center recurs each sweep.
	byFreq := make(map[int64]*BinSeries)
	for _, b := range bins {
		key := int64(math.Round(b.FreqHz))
		s, ok := byFreq[key]
		if !ok {
			s = &BinSeries{FreqHz: b.FreqHz}
			byFreq[key] = s
		}
		s.Powers = append(s.Powers, b.PowerDB)
	}
	out := make([]BinSeries, 0, len(byFreq))
	for _, s := range byFreq {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FreqHz < out[j].FreqHz })
	return out, nil
}

// binFeature is a per-bin intermediate: its duty cycle and peak.
type binFeature struct {
	freqHz  float64
	duty    float64 // percent of samples above the bin's own threshold
	peakDB  float64
	present bool
}

// Classify detects emissions within a band from a time-resolved sweep and labels
// each by its time-frequency shape. It computes each bin's own noise floor and
// duty cycle, groups adjacent present bins into emissions, and classifies each by
// bandwidth and duty. A band with too little data yields no emissions rather than
// a guess.
func Classify(series []BinSeries, band Band, opts ClassifyOptions) []Emission {
	inBand := make([]BinSeries, 0, len(series))
	for _, s := range series {
		if s.FreqHz < band.LowHz || s.FreqHz >= band.HighHz || len(s.Powers) == 0 {
			continue
		}
		inBand = append(inBand, s)
	}
	if len(inBand) == 0 {
		return nil
	}

	// Band noise floor from per-bin quiet levels: each bin's own low percentile is
	// its quiet baseline, and a low percentile across those bins is the band's
	// noise floor. This is robust two ways at once. A zero-variance continuous
	// carrier is still detected, because it reads high against the quiet bins
	// (its own per-bin floor is high, but the floor is taken across bins). And a
	// wideband emitter covering most of the band no longer hides itself: pooling
	// every freq-by-time sample would let a signal that is the majority of the
	// pool raise the percentile up into the signal, but taking the quiet level
	// across bins keeps the reference at the still-quiet bins. The one case this
	// cannot see is a continuous emitter covering the entire band at once, which
	// leaves no in-band quiet reference at all; relative thresholding cannot
	// detect that, and it is a documented limit.
	perBinFloor := make([]float64, len(inBand))
	for i, s := range inBand {
		perBinFloor[i] = percentile(s.Powers, opts.NoiseFloorPercentile)
	}
	threshold := percentile(perBinFloor, opts.NoiseFloorPercentile) + opts.ThresholdMarginDB

	feats := make([]binFeature, 0, len(inBand))
	for _, s := range inBand {
		above := 0
		peak := s.Powers[0]
		for _, p := range s.Powers {
			if p > threshold {
				above++
			}
			if p > peak {
				peak = p
			}
		}
		duty := float64(above) / float64(len(s.Powers)) * 100
		feats = append(feats, binFeature{
			freqHz:  s.FreqHz,
			duty:    duty,
			peakDB:  peak,
			present: duty >= opts.PresentDutyPercent,
		})
	}
	step := gridStep(feats)
	// feats are already frequency-sorted (series is). Group contiguous present
	// bins into emissions.
	var emissions []Emission
	i := 0
	for i < len(feats) {
		if !feats[i].present {
			i++
			continue
		}
		j := i
		for j+1 < len(feats) && feats[j+1].present && feats[j+1].freqHz-feats[j].freqHz <= 1.5*step {
			j++
		}
		emissions = append(emissions, buildEmission(feats[i:j+1], opts))
		i = j + 1
	}
	return emissions
}

// gridStep infers the sweep's bin spacing from the smallest positive gap between
// consecutive in-band bins, so run-grouping works at any sweep resolution rather
// than assuming a fixed 1 MHz grid (a coarser grid would otherwise break every
// run into single bins). A lone bin has no spacing; default to 1 MHz. Runs are
// then extended across bins within 1.5 grid steps, which tolerates one missing
// bin but breaks on a real frequency gap.
func gridStep(feats []binFeature) float64 {
	step := math.MaxFloat64
	for i := 1; i < len(feats); i++ {
		if g := feats[i].freqHz - feats[i-1].freqHz; g > 0 && g < step {
			step = g
		}
	}
	if step == math.MaxFloat64 {
		return 1_000_000
	}
	return step
}

func buildEmission(run []binFeature, opts ClassifyOptions) Emission {
	low := run[0].freqHz
	high := run[len(run)-1].freqHz
	// Bandwidth spans the run plus one bin's worth on each edge is not known
	// precisely; use the center span, which is a floor on the true occupied width.
	bandwidth := high - low
	var maxDuty, peak, peakFreq float64
	peak = run[0].peakDB
	peakFreq = run[0].freqHz
	for _, f := range run {
		if f.duty > maxDuty {
			maxDuty = f.duty
		}
		if f.peakDB > peak {
			peak = f.peakDB
			peakFreq = f.freqHz
		}
	}
	class, conf := classify(bandwidth, maxDuty, len(run), opts)
	return Emission{
		LowHz:            low,
		HighHz:           high,
		BandwidthHz:      bandwidth,
		PeakFreqHz:       peakFreq,
		PeakDB:           peak,
		DutyCyclePercent: maxDuty,
		Class:            class,
		Confidence:       conf,
	}
}

// classify labels an emission from its bandwidth and peak duty cycle. Wide energy
// is wideband; narrow energy is continuous when it is held above the sustained
// threshold and packet-shaped when it is intermittent. Confidence falls near the
// decision boundaries and when a single bin makes the width ambiguous.
func classify(bandwidthHz, dutyPercent float64, binCount int, opts ClassifyOptions) (EmissionClass, float64) {
	if bandwidthHz >= opts.WidebandHz {
		// Confidence grows with how far past the wideband threshold it spans.
		conf := clamp(bandwidthHz/opts.WidebandHz-1, 0.5, 1)
		return ClassWideband, conf
	}
	// A single-bin emission has no measured width, so narrow-band shape is less
	// certain; cap its confidence.
	widthConf := 1.0
	if binCount <= 1 {
		widthConf = 0.6
	}
	if dutyPercent >= opts.SustainedDutyPercent {
		conf := clamp((dutyPercent-opts.SustainedDutyPercent)/(100-opts.SustainedDutyPercent), 0.5, 1) * widthConf
		return ClassContinuous, conf
	}
	// Packet-shaped: intermittent and narrow. Confidence is higher the further
	// below the sustained boundary the duty sits.
	conf := clamp((opts.SustainedDutyPercent-dutyPercent)/opts.SustainedDutyPercent, 0.5, 1) * widthConf
	return ClassPacket, conf
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
