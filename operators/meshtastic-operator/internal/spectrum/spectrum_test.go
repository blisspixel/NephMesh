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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSweepReadsRtlPowerFormat(t *testing.T) {
	// Two segments in the 915 MHz band, 1 MHz bins. Header: date, time, hz_low,
	// hz_high, hz_bin_width, num_samples, then dB values.
	csv := "2026-08-08, 12:00:00, 902000000, 905000000, 1000000, 20, -90.0, -89.5, -88.0\n" +
		"2026-08-08, 12:00:00, 905000000, 908000000, 1000000, 20, -40.0, -91.0, -90.5\n"
	bins, err := ParseSweep(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, bins, 6)
	// First bin center is low + width/2.
	assert.InDelta(t, 902.5e6, bins[0].FreqHz, 1)
	assert.InDelta(t, -90.0, bins[0].PowerDB, 1e-9)
	// The strong bin at the start of the second segment.
	assert.InDelta(t, 905.5e6, bins[3].FreqHz, 1)
	assert.InDelta(t, -40.0, bins[3].PowerDB, 1e-9)
}

func TestParseSweepSkipsCommentsAndShortRows(t *testing.T) {
	csv := "# a comment line\n" +
		"\n" +
		"2026-08-08, 12:00:00, 902000000, 904000000, 1000000, 20, -90.0, -89.0\n" +
		"too, short, row\n"
	bins, err := ParseSweep(strings.NewReader(csv))
	require.NoError(t, err)
	assert.Len(t, bins, 2, "only the one valid sweep row contributes bins")
}

func TestParseSweepRejectsNonFinite(t *testing.T) {
	// strconv.ParseFloat accepts "nan"/"inf"; a single one would poison the whole
	// band's noise floor, mean, and occupancy. They must be skipped like any other
	// unparseable bin.
	csv := "2026-08-08, 12:00:00, 902000000, 905000000, 1000000, 20, -90.0, nan, inf, -88.0\n"
	bins, err := ParseSweep(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, bins, 2, "the nan and inf bins are dropped, the two real bins kept")
	for _, b := range bins {
		assert.False(t, b.PowerDB != b.PowerDB, "no NaN survives") // NaN != NaN
	}
}

func TestParseSweepToleratesStrayQuoteWithoutLosingBins(t *testing.T) {
	// A stray quote must not abort the capture and discard already-parsed bins.
	csv := "2026-08-08, 12:00:00, 902000000, 905000000, 1000000, 20, -90.0, -89.0, -88.0\n" +
		"2026-08-08, 12:00:00, 905000000, 908000000, 1000000, 20, -40.0\", -91.0, -90.5\n"
	bins, err := ParseSweep(strings.NewReader(csv))
	require.NoError(t, err, "a stray quote must not fail the whole parse")
	assert.GreaterOrEqual(t, len(bins), 3, "the first row's valid bins survive")
}

func TestParseSweepErrorsWhenNotASweep(t *testing.T) {
	_, err := ParseSweep(strings.NewReader("hello\nworld\n"))
	assert.Error(t, err, "non-sweep text yields no bins and is an error")
}

func TestAnalyzeComputesOccupancyAgainstNoiseFloor(t *testing.T) {
	// A 915 band mostly at a -90 dB noise floor with three strong bins well above
	// it. With a 6 dB margin over the 25th-percentile floor, only the strong bins
	// count as occupied.
	var bins []Bin
	for i := 0; i < 20; i++ {
		bins = append(bins, Bin{FreqHz: 903e6 + float64(i)*1e6, PowerDB: -90})
	}
	bins[5].PowerDB = -50
	bins[6].PowerDB = -48
	bins[7].PowerDB = -55

	stats := Analyze(bins, []Band{{Name: "ism-915-us", LowHz: 902e6, HighHz: 928e6}}, DefaultOptions())
	require.Len(t, stats, 1)
	s := stats[0]
	assert.Equal(t, 20, s.BinCount)
	assert.Equal(t, 3, s.OccupiedBins)
	assert.InDelta(t, 15.0, s.OccupancyPercent, 1e-9, "3 of 20 bins occupied")
	assert.InDelta(t, -90, s.NoiseFloorDB, 1e-9, "the quiet baseline is the noise floor")
	assert.InDelta(t, -48, s.MaxDB, 1e-9)
	assert.InDelta(t, 909e6, s.PeakFreqHz, 1, "peak is the strongest bin")
}

func TestAnalyzeIdleBandIsZeroOccupancy(t *testing.T) {
	var bins []Bin
	for i := 0; i < 30; i++ {
		bins = append(bins, Bin{FreqHz: 903e6 + float64(i)*1e5, PowerDB: -95 + float64(i%3)}) // small jitter
	}
	stats := Analyze(bins, []Band{{Name: "ism-915-us", LowHz: 902e6, HighHz: 928e6}}, DefaultOptions())
	assert.Zero(t, stats[0].OccupiedBins, "noise-only band reads as unoccupied within the margin")
	assert.Zero(t, stats[0].OccupancyPercent)
}

func TestAnalyzeUncoveredBandReportedNotIdle(t *testing.T) {
	// Bins only in the 915 band; the 433 band is not covered by the sweep.
	bins := []Bin{{FreqHz: 915e6, PowerDB: -80}}
	stats := Analyze(bins, DefaultBands(), DefaultOptions())
	var band433 BandStats
	for _, s := range stats {
		if s.Band.Name == "ism-433" {
			band433 = s
		}
	}
	assert.Equal(t, 0, band433.BinCount, "an uncovered band has no bins, distinct from idle")
}

func TestSenseParsesAndAnalyzes(t *testing.T) {
	csv := "2026-08-08, 12:00:00, 902000000, 928000000, 1000000, 20" +
		strings.Repeat(", -90.0", 20) + ", -45.0" + strings.Repeat(", -90.0", 5) + "\n"
	stats, err := Sense(strings.NewReader(csv), DefaultBands(), DefaultOptions())
	require.NoError(t, err)
	var us BandStats
	for _, s := range stats {
		if s.Band.Name == "ism-915-us" {
			us = s
		}
	}
	assert.Positive(t, us.BinCount)
	assert.Equal(t, 1, us.OccupiedBins, "the single strong bin is detected")
}

func TestRenderTextSummarizesBands(t *testing.T) {
	stats := []BandStats{
		{Band: Band{Name: "ism-915-us", LowHz: 902e6, HighHz: 928e6}, BinCount: 20, OccupiedBins: 3, OccupancyPercent: 15, NoiseFloorDB: -90, MaxDB: -48, PeakFreqHz: 909e6},
		{Band: Band{Name: "ism-433", LowHz: 433.05e6, HighHz: 434.79e6}, BinCount: 0},
	}
	txt := RenderText(stats)
	assert.Contains(t, txt, "ism-915-us")
	assert.Contains(t, txt, "occupancy 15.0%")
	assert.Contains(t, txt, "not covered")
}

func TestPercentile(t *testing.T) {
	v := []float64{-100, -90, -80, -70, -60}
	assert.InDelta(t, -100, percentile(v, 0), 1e-9)
	assert.InDelta(t, -60, percentile(v, 100), 1e-9)
	assert.InDelta(t, -90, percentile(v, 25), 1e-9)
	assert.Zero(t, percentile(nil, 50), "empty input is guarded")
}
