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

package specexport

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/spectrum"
)

func TestPublishSetsPerBandGauges(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := New(reg)
	p.Publish([]spectrum.BandStats{
		{Band: spectrum.Band{Name: "ism-915-us"}, BinCount: 26, OccupancyPercent: 32, NoiseFloorDB: -69, MaxDB: -17, PeakFreqHz: 906.5e6},
	}, 1000)

	assert.Equal(t, 32.0, testutil.ToFloat64(p.occupancy.WithLabelValues("ism-915-us")))
	assert.Equal(t, -17.0, testutil.ToFloat64(p.peakDBM.WithLabelValues("ism-915-us")))
	assert.Equal(t, 906.5e6, testutil.ToFloat64(p.peakFreqHz.WithLabelValues("ism-915-us")))
	assert.Equal(t, -69.0, testutil.ToFloat64(p.noiseDBM.WithLabelValues("ism-915-us")))
	assert.Equal(t, 26.0, testutil.ToFloat64(p.bins.WithLabelValues("ism-915-us")))
	assert.Equal(t, 1.0, testutil.ToFloat64(p.sweeps))
	assert.Equal(t, 1000.0, testutil.ToFloat64(p.lastSweep))
}

func TestUncoveredBandDeletesValueGaugesButKeepsBinsZero(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := New(reg)
	// First sweep covers the band.
	p.Publish([]spectrum.BandStats{
		{Band: spectrum.Band{Name: "ism-915-us"}, BinCount: 26, OccupancyPercent: 32, MaxDB: -17, PeakFreqHz: 906.5e6, NoiseFloorDB: -69},
	}, 1000)
	// Next sweep does not cover it (BinCount 0): value gauges must be removed, not
	// left reading a stale 32 percent, and bins should read 0.
	p.Publish([]spectrum.BandStats{
		{Band: spectrum.Band{Name: "ism-915-us"}, BinCount: 0},
	}, 1001)

	assert.Equal(t, 0.0, testutil.ToFloat64(p.bins.WithLabelValues("ism-915-us")))
	// A deleted gauge has no series; count the metrics to prove it is gone.
	assert.Equal(t, 0, testutil.CollectAndCount(p.occupancy), "occupancy series removed for an uncovered band")
	assert.Equal(t, 0, testutil.CollectAndCount(p.peakDBM))
}

func TestPublishErrorIncrementsCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := New(reg)
	p.PublishError()
	p.PublishError()
	assert.Equal(t, 2.0, testutil.ToFloat64(p.sweepErr))
}

func TestMetricsAreExposedInTextFormat(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := New(reg)
	p.Publish([]spectrum.BandStats{
		{Band: spectrum.Band{Name: "ism-915-us"}, BinCount: 26, OccupancyPercent: 32, MaxDB: -17, PeakFreqHz: 906.5e6, NoiseFloorDB: -69},
	}, 1000)

	mfs, err := reg.Gather()
	require.NoError(t, err)
	var names []string
	for _, mf := range mfs {
		names = append(names, mf.GetName())
	}
	joined := strings.Join(names, ",")
	for _, want := range []string{
		"nephmesh_spectrum_occupancy_percent",
		"nephmesh_spectrum_peak_dbm",
		"nephmesh_spectrum_peak_frequency_hz",
		"nephmesh_spectrum_noise_floor_dbm",
		"nephmesh_spectrum_sweeps_total",
	} {
		assert.Contains(t, joined, want)
	}
}
