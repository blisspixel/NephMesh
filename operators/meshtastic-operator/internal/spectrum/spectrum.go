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

// Package spectrum turns a receive-only SDR power sweep into per-band occupancy
// aggregates. It parses the rtl_power / hackrf_sweep CSV format (a wide, flat
// power-spectral-density dump) and reduces it to a small set of per-band numbers,
// occupancy percent, noise floor, and peak/mean power, rather than emitting a
// per-bin series (which would explode metric cardinality). The research sweep
// (docs/research/sdr-spectrum-sensing.md) found this reduction is the missing
// glue: mature exporters decode signals, none aggregate raw PSD into band
// occupancy.
//
// It is pure and hardware-free: it reads a CSV an SDR tool produced, it does not
// touch a radio. That keeps the analysis testable against synthetic sweeps now,
// with the same code validated against a real capture when an SDR is attached.
// This is spectrum SENSING (is the band busy, and how busy), not classification
// (busy with what); classification is deliberately later work
// (docs/research/spectrum-classification.md). The whole path is receive-only.
package spectrum

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Bin is one FFT bin of a sweep: a center frequency and its measured power. The
// rtl_power/hackrf_sweep power unit is dB (relative); it is treated as a relative
// power here, and occupancy is judged against the sweep's own noise floor rather
// than an absolute calibration this project does not have.
type Bin struct {
	FreqHz  float64 `json:"freqHz"`
	PowerDB float64 `json:"powerDb"`
}

// Band is a named, contiguous frequency range to summarize.
type Band struct {
	Name   string  `json:"name"`
	LowHz  float64 `json:"lowHz"`
	HighHz float64 `json:"highHz"`
}

// DefaultBands are the ISM ranges a Meshtastic-focused deployment cares about,
// including the band a US mesh transmits in (902-928 MHz), so an operator can
// watch their own mesh's channel and its neighbors. They are a convenience
// default, not a regulatory statement; see docs/reference/regulatory-matrix.md.
func DefaultBands() []Band {
	return []Band{
		{Name: "ism-433", LowHz: 433.05e6, HighHz: 434.79e6},
		{Name: "ism-868-eu", LowHz: 863e6, HighHz: 870e6},
		{Name: "ism-915-us", LowHz: 902e6, HighHz: 928e6},
	}
}

// Options tunes the energy-detection reduction. The defaults are deliberately
// simple and explained where they are used.
type Options struct {
	// ThresholdMarginDB is how far above the estimated noise floor a bin must be
	// to count as occupied. A margin absorbs noise-floor jitter so idle noise does
	// not read as traffic.
	ThresholdMarginDB float64
	// NoiseFloorPercentile is the per-band percentile taken as the noise floor
	// (0..100). A low percentile (say 25) tracks the quiet baseline even when part
	// of the band is busy, which a mean or median would be dragged up by.
	NoiseFloorPercentile float64
}

// DefaultOptions is a reasonable starting point: occupied means 6 dB above the
// 25th-percentile noise floor. These are heuristics to be validated against real
// captures, not calibrated thresholds.
func DefaultOptions() Options {
	return Options{ThresholdMarginDB: 6, NoiseFloorPercentile: 25}
}

// BandStats is the per-band reduction.
type BandStats struct {
	Band             Band    `json:"band"`
	BinCount         int     `json:"binCount"`
	OccupiedBins     int     `json:"occupiedBins"`
	OccupancyPercent float64 `json:"occupancyPercent"`
	NoiseFloorDB     float64 `json:"noiseFloorDb"`
	ThresholdDB      float64 `json:"thresholdDb"`
	MeanDB           float64 `json:"meanDb"`
	MaxDB            float64 `json:"maxDb"`
	// PeakFreqHz is the frequency of the strongest bin in the band, 0 if empty.
	PeakFreqHz float64 `json:"peakFreqHz"`
}

// ParseSweep reads rtl_power / hackrf_sweep CSV and returns the bins it contains.
// Each row is: date, time, hz_low, hz_high, hz_bin_width, num_samples, then one
// dB value per bin from hz_low upward. A bin's frequency is taken at the center
// of its step. Rows that are blank, commented, or too short to be a sweep row are
// skipped; a row whose numeric header fields do not parse is skipped rather than
// failing the whole capture. It errors only if a non-empty input yields no bins,
// which means the input was not a sweep at all.
func ParseSweep(r io.Reader) ([]Bin, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	cr.FieldsPerRecord = -1 // rows have variable bin counts
	cr.Comment = '#'
	// rtl_power/hackrf_sweep output is unquoted; tolerate a stray quote as literal
	// data so one odd line cannot abort the whole capture and discard valid bins.
	cr.LazyQuotes = true

	var bins []Bin
	rows := 0
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// A malformed CSV row (bad quoting) stops parsing; report it.
			return nil, fmt.Errorf("read sweep: %w", err)
		}
		rows++
		// 6 header fields plus at least one dB value.
		if len(rec) < 7 {
			continue
		}
		low, err1 := parseFloat(rec[2])
		bw, err3 := parseFloat(rec[4])
		if err1 != nil || err3 != nil || !isFinite(low) || !isFinite(bw) {
			continue
		}
		for j := 6; j < len(rec); j++ {
			p, err := parseFloat(rec[j])
			// strconv.ParseFloat accepts "nan"/"inf"; a single non-finite token
			// would poison the whole band's noise floor, mean, and occupancy, so
			// skip it like any other unparseable bin.
			if err != nil || !isFinite(p) {
				continue
			}
			center := low + bw*(float64(j-6)+0.5)
			bins = append(bins, Bin{FreqHz: center, PowerDB: p})
		}
	}
	if rows > 0 && len(bins) == 0 {
		return nil, fmt.Errorf("no power bins parsed from %d row(s); is this rtl_power/hackrf_sweep CSV?", rows)
	}
	return bins, nil
}

// Analyze reduces bins to per-band occupancy. Each band's noise floor is the
// requested percentile of its own bins' power, the threshold is that plus the
// margin, and occupancy is the fraction of bins above the threshold. A band with
// no bins in the sweep is returned with a zero BinCount so a caller can see it
// was not covered rather than mistaking it for idle.
func Analyze(bins []Bin, bands []Band, opts Options) []BandStats {
	out := make([]BandStats, 0, len(bands))
	for _, band := range bands {
		var powers []float64
		var maxDB float64
		var peakFreq float64
		var sum float64
		for _, b := range bins {
			if b.FreqHz < band.LowHz || b.FreqHz >= band.HighHz {
				continue
			}
			if len(powers) == 0 || b.PowerDB > maxDB {
				maxDB = b.PowerDB
				peakFreq = b.FreqHz
			}
			powers = append(powers, b.PowerDB)
			sum += b.PowerDB
		}
		stats := BandStats{Band: band, BinCount: len(powers)}
		if len(powers) == 0 {
			out = append(out, stats)
			continue
		}
		noise := percentile(powers, opts.NoiseFloorPercentile)
		threshold := noise + opts.ThresholdMarginDB
		occupied := 0
		for _, p := range powers {
			if p > threshold {
				occupied++
			}
		}
		stats.NoiseFloorDB = noise
		stats.ThresholdDB = threshold
		stats.OccupiedBins = occupied
		stats.OccupancyPercent = float64(occupied) / float64(len(powers)) * 100
		stats.MeanDB = sum / float64(len(powers))
		stats.MaxDB = maxDB
		stats.PeakFreqHz = peakFreq
		out = append(out, stats)
	}
	return out
}

// Sense is the whole receive-only reduction: parse a sweep CSV and analyze it
// into per-band occupancy. It is the one call the CLI and any exporter share.
func Sense(r io.Reader, bands []Band, opts Options) ([]BandStats, error) {
	bins, err := ParseSweep(r)
	if err != nil {
		return nil, err
	}
	return Analyze(bins, bands, opts), nil
}

// RenderText renders per-band stats as a short human-readable table. The machine
// contract is the JSON form of []BandStats; this is for a person at a terminal.
func RenderText(stats []BandStats) string {
	var b strings.Builder
	for _, s := range stats {
		if s.BinCount == 0 {
			fmt.Fprintf(&b, "%-12s %.3f-%.3f MHz: not covered by this sweep\n",
				s.Band.Name, s.Band.LowHz/1e6, s.Band.HighHz/1e6)
			continue
		}
		fmt.Fprintf(&b, "%-12s %.3f-%.3f MHz: occupancy %.1f%% (%d/%d bins), noise %.1f dB, peak %.1f dB @ %.3f MHz\n",
			s.Band.Name, s.Band.LowHz/1e6, s.Band.HighHz/1e6,
			s.OccupancyPercent, s.OccupiedBins, s.BinCount,
			s.NoiseFloorDB, s.MaxDB, s.PeakFreqHz/1e6)
	}
	return b.String()
}

// percentile returns the p-th percentile (0..100) of the values using nearest
// rank, guarding empty input and out-of-range p.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	rank := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}
