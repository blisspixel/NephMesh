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

const noiseDB = -95.0

// bin builds a time series where dutyPct percent of n samples are at signalDB and
// the rest sit at the noise floor. dutyPct 0 is a pure-noise bin.
func bin(freqHz float64, n, dutyPct int, signalDB float64) BinSeries {
	powers := make([]float64, n)
	active := n * dutyPct / 100
	for i := 0; i < n; i++ {
		if i < active {
			powers[i] = signalDB
		} else {
			powers[i] = noiseDB
		}
	}
	return BinSeries{FreqHz: freqHz, Powers: powers}
}

var band915 = Band{Name: "ism-915-us", LowHz: 902e6, HighHz: 928e6}

func TestClassifyPacketShapedEmission(t *testing.T) {
	// A narrow, intermittent emitter (10 percent duty) surrounded by noise: the
	// shape of LoRa mesh traffic.
	series := []BinSeries{
		bin(904e6, 100, 0, 0),
		bin(905e6, 100, 0, 0),
		bin(906e6, 100, 10, -20), // the packet emitter
		bin(907e6, 100, 0, 0),
		bin(908e6, 100, 0, 0),
	}
	em := Classify(series, band915, DefaultClassifyOptions())
	require.Len(t, em, 1)
	assert.Equal(t, ClassPacket, em[0].Class)
	assert.InDelta(t, 906e6, em[0].PeakFreqHz, 1)
	assert.InDelta(t, 10, em[0].DutyCyclePercent, 0.5)
}

func TestClassifyContinuousNarrowbandCarrier(t *testing.T) {
	// A narrow emitter held on the whole time: a carrier, not packet traffic.
	series := []BinSeries{
		bin(904e6, 100, 0, 0),
		bin(910e6, 100, 100, -20), // always-on carrier
		bin(916e6, 100, 0, 0),
	}
	em := Classify(series, band915, DefaultClassifyOptions())
	require.Len(t, em, 1)
	assert.Equal(t, ClassContinuous, em[0].Class)
	assert.InDelta(t, 100, em[0].DutyCyclePercent, 0.5)
}

func TestClassifyWidebandEmission(t *testing.T) {
	// Several contiguous bins held on together: broadband energy, the jamming
	// or wideband-interference shape.
	series := []BinSeries{
		bin(904e6, 100, 0, 0),
		bin(915e6, 100, 90, -25),
		bin(916e6, 100, 90, -25),
		bin(917e6, 100, 90, -25),
		bin(918e6, 100, 90, -25),
		bin(924e6, 100, 0, 0),
	}
	em := Classify(series, band915, DefaultClassifyOptions())
	require.Len(t, em, 1)
	assert.Equal(t, ClassWideband, em[0].Class)
	assert.GreaterOrEqual(t, em[0].BandwidthHz, 1.5e6)
}

func TestClassifyGroupsWidebandOnACoarseGrid(t *testing.T) {
	// The same wideband emitter sampled on a 2 MHz grid must still group into one
	// emission. A fixed 1.6 MHz adjacency ceiling would split it into single bins;
	// the grid step is inferred from the sweep instead.
	series := []BinSeries{
		bin(904e6, 100, 0, 0),
		bin(910e6, 100, 90, -25),
		bin(912e6, 100, 90, -25),
		bin(914e6, 100, 90, -25),
		bin(916e6, 100, 90, -25),
		bin(922e6, 100, 0, 0),
	}
	em := Classify(series, band915, DefaultClassifyOptions())
	require.Len(t, em, 1, "the coarse-grid emitter is one emission, not four")
	assert.Equal(t, ClassWideband, em[0].Class)
	// Four 2 MHz-spaced centers: 6 MHz span plus one bin = 8 MHz.
	assert.InDelta(t, 8e6, em[0].BandwidthHz, 1)
}

func TestClassifyTwoBinInterfererIsWideband(t *testing.T) {
	// Default hackrf_sweep grid is 1 MHz. A 2 MHz interferer is two adjacent
	// bins; center-to-center span is 1 MHz and must not be labeled packet.
	series := []BinSeries{
		bin(904e6, 100, 0, 0),
		bin(910e6, 100, 90, -25),
		bin(911e6, 100, 90, -25),
		bin(920e6, 100, 0, 0),
	}
	em := Classify(series, band915, DefaultClassifyOptions())
	require.Len(t, em, 1)
	assert.Equal(t, ClassWideband, em[0].Class)
	assert.InDelta(t, 2e6, em[0].BandwidthHz, 1)
}

func TestClassifyWidebandSurvivesSubGridGap(t *testing.T) {
	// A single sub-grid gap (two mis-tiled bins 0.5 MHz apart, from overlapping
	// hackrf_sweep segments) must not collapse run-grouping. With a minimum-gap
	// step it would shrink to 0.5 MHz and split the emitter into single bins; the
	// median grid step keeps the wideband emitter as one emission.
	series := []BinSeries{
		bin(904.0e6, 100, 0, 0), // two quiet bins 0.5 MHz apart: the sub-grid gap
		bin(904.5e6, 100, 0, 0),
		bin(910e6, 100, 90, -25), // a genuine wideband emitter on a 1 MHz grid
		bin(911e6, 100, 90, -25),
		bin(912e6, 100, 90, -25),
		bin(913e6, 100, 90, -25),
		bin(920e6, 100, 0, 0),
	}
	em := Classify(series, band915, DefaultClassifyOptions())
	require.Len(t, em, 1, "the sub-grid gap must not split the wideband emitter")
	assert.Equal(t, ClassWideband, em[0].Class)
}

func TestClassifyDetectsWidebandAgainstQuietBins(t *testing.T) {
	// A wideband emitter over part of the band, with quiet bins remaining as a
	// reference, is detected. (A signal covering the whole band leaves no
	// reference and is a documented limit, not tested here.)
	var series []BinSeries
	for f := 903e6; f < 916e6; f += 1e6 { // ~13 contiguous bins, always on
		series = append(series, bin(f, 100, 100, -25))
	}
	for f := 918e6; f < 928e6; f += 1e6 { // quiet reference bins
		series = append(series, bin(f, 100, 0, 0))
	}
	em := Classify(series, band915, DefaultClassifyOptions())
	require.NotEmpty(t, em)
	assert.Equal(t, ClassWideband, em[0].Class)
}

func TestClassifyIdleBandHasNoEmissions(t *testing.T) {
	series := []BinSeries{
		bin(904e6, 100, 0, 0),
		bin(910e6, 100, 0, 0),
		bin(916e6, 100, 0, 0),
	}
	assert.Empty(t, Classify(series, band915, DefaultClassifyOptions()))
}

func TestClassifySeparatesTwoEmissions(t *testing.T) {
	// A packet emitter and a continuous carrier, separated by noise, must be two
	// distinct emissions with different classes.
	series := []BinSeries{
		bin(906e6, 100, 8, -20),   // packet
		bin(907e6, 100, 0, 0),     // gap
		bin(908e6, 100, 0, 0),     // gap
		bin(920e6, 100, 100, -20), // continuous
	}
	em := Classify(series, band915, DefaultClassifyOptions())
	require.Len(t, em, 2)
	assert.Equal(t, ClassPacket, em[0].Class)
	assert.Equal(t, ClassContinuous, em[1].Class)
}

func TestParseSweepSeriesPreservesTimeStructure(t *testing.T) {
	// Two sweep passes over the same two bins: each bin should collect two
	// samples in time order.
	csv := "2026-08-09, 12:00:00, 902000000, 904000000, 1000000, 20, -90.0, -50.0\n" +
		"2026-08-09, 12:00:01, 902000000, 904000000, 1000000, 20, -91.0, -49.0\n"
	series, err := ParseSweepSeries(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, series, 2, "two distinct bin centers")
	for _, s := range series {
		assert.Len(t, s.Powers, 2, "two passes contribute two samples per bin")
	}
	// The second bin (903.5 MHz) carries the strong samples.
	assert.InDelta(t, 902.5e6, series[0].FreqHz, 1)
	assert.InDelta(t, 903.5e6, series[1].FreqHz, 1)
	assert.Contains(t, series[1].Powers, -50.0)
}

func TestClassifyEmpty(t *testing.T) {
	assert.Empty(t, Classify(nil, band915, DefaultClassifyOptions()))
}
