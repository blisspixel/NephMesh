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
	"fmt"
	"math"
	"sort"
	"strings"
)

// TextStyle controls the human rendering. JSON never uses it. ASCII is the
// fallback when the locale is not UTF-8 or TERM=dumb. ANSI colours the peak
// cell and dims "not covered"; the caller turns it off for a pipe, NO_COLOR,
// or TERM=dumb.
type TextStyle struct {
	ASCII bool
	ANSI  bool
}

const (
	ansiYellow = "\x1b[33m"
	ansiDim    = "\x1b[2m"
	ansiReset  = "\x1b[0m"

	// spectrumStripMax is the widest strip that still leaves room for the
	// band name and the peak label inside 80 columns.
	spectrumStripMax = 48
	blockLevels      = "▁▂▃▄▅▆▇█"
	asciiLevels      = "_.-=#"
)

// RenderText renders per-band stats as an 80-column table plus one spectrum
// strip per covered band. The machine contract is the JSON form of []BandStats;
// this view adds nothing to that struct. bins may be nil, in which case the
// table is printed without strips.
func RenderText(stats []BandStats, bins []Bin, style TextStyle) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-12s %-17s %-14s %8s  %s\n",
		"band", "range MHz", "occupancy", "noise dB", "peak dB @ MHz")
	for _, s := range stats {
		writeBandRow(&b, s, style)
	}
	anyStrip := false
	var strips strings.Builder
	for _, s := range stats {
		if s.BinCount == 0 {
			continue
		}
		line := renderStrip(s, bins, style)
		if line == "" {
			continue
		}
		anyStrip = true
		strips.WriteString(line)
		strips.WriteByte('\n')
	}
	if anyStrip {
		b.WriteByte('\n')
		b.WriteString(strips.String())
	}
	return b.String()
}

func writeBandRow(b *strings.Builder, s BandStats, style TextStyle) {
	rng := fmt.Sprintf("%.3f-%.3f", s.Band.LowHz/1e6, s.Band.HighHz/1e6)
	if s.BinCount == 0 {
		covered := "not covered"
		if style.ANSI {
			covered = ansiDim + covered + ansiReset
		}
		fmt.Fprintf(b, "%-12s %-17s %s\n", s.Band.Name, rng, covered)
		return
	}
	occ := fmt.Sprintf("%.1f%% %d/%d", s.OccupancyPercent, s.OccupiedBins, s.BinCount)
	noise := fmt.Sprintf("%.1f", s.NoiseFloorDB)
	peak := fmt.Sprintf("%.1f @ %.3f", s.MaxDB, s.PeakFreqHz/1e6)
	fmt.Fprintf(b, "%-12s %-17s %-14s %8s  %s\n", s.Band.Name, rng, occ, noise, peak)
}

// renderStrip draws one row of the band, scaled from its noise floor to its
// peak. Cells are one bin, or a max-pooled group when the band has more bins
// than fit. The peak cell is the only coloured cell, and only when that peak
// clears the occupancy margin.
func renderStrip(s BandStats, bins []Bin, style TextStyle) string {
	powers, freqs := bandPowers(bins, s.Band)
	if len(powers) == 0 {
		return ""
	}
	levels := blockLevels
	if style.ASCII {
		levels = asciiLevels
	}
	low := s.Band.LowHz / 1e6
	high := s.Band.HighHz / 1e6
	peakMHz := s.PeakFreqHz / 1e6
	prefix := fmt.Sprintf("%s  %.0f ", s.Band.Name, low)
	suffix := fmt.Sprintf(" %.0f  peak %.3f", high, peakMHz)
	cells := len(powers)
	if cells > spectrumStripMax {
		cells = spectrumStripMax
	}
	room := 80 - len(prefix) - len(suffix)
	if room < 8 {
		room = 8
	}
	if cells > room {
		cells = room
	}
	grouped, groupFreq := poolBins(powers, freqs, cells)
	runes := make([]rune, cells)
	peakIdx := 0
	levelRunes := []rune(levels)
	nLevel := len(levelRunes)
	for i, p := range grouped {
		runes[i] = levelRunes[levelIndex(p, s.NoiseFloorDB, s.MaxDB, nLevel)]
		if i == 0 || p > grouped[peakIdx] {
			peakIdx = i
		}
	}
	// Prefer the group that actually contains the reported peak frequency when
	// several groups share the max power.
	for i, f := range groupFreq {
		if math.Abs(f-s.PeakFreqHz) < math.Abs(groupFreq[peakIdx]-s.PeakFreqHz) && grouped[i] == grouped[peakIdx] {
			peakIdx = i
		}
	}
	colorPeak := style.ANSI && s.MaxDB > s.ThresholdDB
	var b strings.Builder
	b.WriteString(prefix)
	for i, r := range runes {
		if colorPeak && i == peakIdx {
			b.WriteString(ansiYellow)
			b.WriteRune(r)
			b.WriteString(ansiReset)
			continue
		}
		b.WriteRune(r)
	}
	b.WriteString(suffix)
	return b.String()
}

func bandPowers(bins []Bin, band Band) (powers, freqs []float64) {
	type pair struct {
		f, p float64
	}
	var rows []pair
	for _, bin := range bins {
		if bin.FreqHz < band.LowHz || bin.FreqHz >= band.HighHz {
			continue
		}
		rows = append(rows, pair{bin.FreqHz, bin.PowerDB})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].f < rows[j].f })
	powers = make([]float64, len(rows))
	freqs = make([]float64, len(rows))
	for i, row := range rows {
		powers[i] = row.p
		freqs[i] = row.f
	}
	return powers, freqs
}

// poolBins reduces a band to n cells. Each cell is the max power in its
// frequency group, so a narrow peak is not averaged away.
func poolBins(powers, freqs []float64, n int) ([]float64, []float64) {
	out := make([]float64, n)
	mid := make([]float64, n)
	total := len(powers)
	for i := 0; i < n; i++ {
		start := i * total / n
		end := (i + 1) * total / n
		if end <= start {
			end = start + 1
		}
		if end > total {
			end = total
		}
		maxP := powers[start]
		for _, p := range powers[start:end] {
			if p > maxP {
				maxP = p
			}
		}
		out[i] = maxP
		mid[i] = freqs[(start+end-1)/2]
	}
	return out, mid
}

func levelIndex(power, noise, peak float64, n int) int {
	if n <= 1 {
		return 0
	}
	span := peak - noise
	if span <= 0 {
		return 0
	}
	t := (power - noise) / span
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	idx := int(math.Round(t * float64(n-1)))
	if idx < 0 {
		return 0
	}
	if idx >= n {
		return n - 1
	}
	return idx
}
