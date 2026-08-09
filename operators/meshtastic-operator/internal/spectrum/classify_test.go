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
