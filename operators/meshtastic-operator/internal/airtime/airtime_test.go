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

package airtime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeOnAirMatchesSemtechFormula(t *testing.T) {
	// A hand-computed reference point from the Semtech formula: SF7, BW125 kHz,
	// CR 4/5, 20-byte payload, 8-symbol preamble, explicit header, CRC on.
	// tSym = 2^7/125000 = 1.024 ms; preamble = 12.25*tSym; payload symbols = 43;
	// total = 56.576 ms.
	got := TimeOnAir(Params{SpreadingFactor: 7, BandwidthHz: 125000, CodingRate: 5}, 20, 8, true, true)
	assert.InDelta(t, 56.576, float64(got)/float64(time.Millisecond), 0.05,
		"time-on-air must match the Semtech reference calculation")
}

func TestLowDataRateOptimizationKicksInAtHighSF(t *testing.T) {
	// SF12/BW125 has a symbol time above 16 ms, so low-data-rate optimization
	// applies and a 40-byte frame is well over a second on air, the physical
	// reason the longest-range presets collapse a dense channel fastest.
	toa, ok := PresetTimeOnAir("LONG_SLOW", 40)
	assert.True(t, ok)
	assert.Greater(t, toa, time.Second, "SF12 narrow-band frames are seconds long")
}

func TestRangeTradesOffAgainstAirtime(t *testing.T) {
	// Longer-range presets cost strictly more airtime for the same payload,
	// which is the tension an airtime budget has to govern.
	shortFast, _ := PresetTimeOnAir("SHORT_FAST", 40)
	mediumSlow, _ := PresetTimeOnAir("MEDIUM_SLOW", 40)
	longFast, _ := PresetTimeOnAir("LONG_FAST", 40)
	assert.Less(t, shortFast, mediumSlow)
	assert.Less(t, mediumSlow, longFast)
}

func TestLargerPayloadCostsMoreAirtime(t *testing.T) {
	small, _ := PresetTimeOnAir("LONG_FAST", 16)
	large, _ := PresetTimeOnAir("LONG_FAST", 200)
	assert.Less(t, small, large)
}

func TestUnknownPresetReported(t *testing.T) {
	_, ok := PresetTimeOnAir("NOT_A_PRESET", 40)
	assert.False(t, ok)
}

func TestHealthyRespectsCeilings(t *testing.T) {
	assert.True(t, Healthy(10, 2), "well under both ceilings")
	assert.True(t, Healthy(RecommendedChannelUtilizationPercent, RecommendedAirUtilTxPercent), "at the ceilings is still healthy")
	assert.False(t, Healthy(40, 2), "channel utilization over ceiling")
	assert.False(t, Healthy(10, 15), "transmit airtime over ceiling")
}

func TestDutyCyclePercent(t *testing.T) {
	// A 1-second frame once every 10 seconds is 10% of the channel.
	assert.InDelta(t, 10.0, DutyCyclePercent(time.Second, 10*time.Second), 1e-9)
	assert.Equal(t, 0.0, DutyCyclePercent(time.Second, 0), "a zero period is guarded")
}

func TestPredictedChannelUtilizationScalesWithTimeOnAir(t *testing.T) {
	// Switching from a fast preset to a slower, longer-range one multiplies
	// per-frame time-on-air, so predicted utilization rises by the same ratio.
	slowToa, _ := PresetTimeOnAir("LONG_SLOW", RepresentativeFramePayloadBytes)
	fastToa, _ := PresetTimeOnAir("SHORT_FAST", RepresentativeFramePayloadBytes)
	ratio := float64(slowToa) / float64(fastToa)

	got, ok := PredictedChannelUtilizationPercent("SHORT_FAST", "LONG_SLOW", 5.0)
	assert.True(t, ok)
	assert.InDelta(t, 5.0*ratio, got, 1e-9, "predicted utilization scales by the time-on-air ratio")
	assert.Greater(t, got, 5.0, "a slower, longer-range preset raises utilization")

	// The reverse direction lowers it.
	down, ok := PredictedChannelUtilizationPercent("LONG_SLOW", "SHORT_FAST", 20.0)
	assert.True(t, ok)
	assert.Less(t, down, 20.0, "a faster preset lowers utilization")
}

func TestPredictedChannelUtilizationUnknownPreset(t *testing.T) {
	_, ok := PredictedChannelUtilizationPercent("NOPE", "LONG_FAST", 5.0)
	assert.False(t, ok, "an unknown current preset yields no prediction")
	_, ok = PredictedChannelUtilizationPercent("LONG_FAST", "NOPE", 5.0)
	assert.False(t, ok, "an unknown desired preset yields no prediction")
}

func TestWithinChannelBudget(t *testing.T) {
	assert.True(t, WithinChannelBudget(RecommendedChannelUtilizationPercent))
	assert.True(t, WithinChannelBudget(10.0))
	assert.False(t, WithinChannelBudget(RecommendedChannelUtilizationPercent+0.1))
}

func TestFleetChannelUtilizationMatchesHandCalculation(t *testing.T) {
	// SHORT_FAST (SF7, BW250k, CR4/5), 40-byte payload, 16-symbol preamble has a
	// per-frame time-on-air of ~45.184 ms. Ten nodes at 6 messages/min each is 60
	// frames/min, so airtime/min = 60 * 45.184 = 2711.04 ms, which is
	// 2711.04/60000 = 4.5184% of the channel (rebroadcast ignored: this is a floor).
	got, ok := FleetChannelUtilizationPercent("SHORT_FAST", 10, 6, 40)
	assert.True(t, ok)
	assert.InDelta(t, 4.5184, got, 0.02, "fleet utilization must match the hand calculation")
}

func TestFleetChannelUtilizationScalesWithFleetAndRate(t *testing.T) {
	base, _ := FleetChannelUtilizationPercent("MEDIUM_SLOW", 10, 6, 40)
	moreNodes, _ := FleetChannelUtilizationPercent("MEDIUM_SLOW", 20, 6, 40)
	moreRate, _ := FleetChannelUtilizationPercent("MEDIUM_SLOW", 10, 12, 40)
	assert.InDelta(t, 2*base, moreNodes, 1e-9, "doubling the fleet doubles the floor")
	assert.InDelta(t, 2*base, moreRate, 1e-9, "doubling the message rate doubles the floor")
}

func TestFleetChannelUtilizationTripsBudgetForDenseFleet(t *testing.T) {
	// A large fleet on a long-range preset exceeds the recommended ceiling even by
	// the conservative floor, so infeasibility is certain.
	got, ok := FleetChannelUtilizationPercent("LONG_FAST", 100, 12, 40)
	assert.True(t, ok)
	assert.False(t, WithinChannelBudget(got), "a dense fleet on a slow preset is over budget by the floor alone")
}

func TestFleetChannelUtilizationGuards(t *testing.T) {
	_, ok := FleetChannelUtilizationPercent("NOT_A_PRESET", 10, 6, 40)
	assert.False(t, ok, "an unknown preset yields no estimate")
	got, ok := FleetChannelUtilizationPercent("LONG_FAST", 0, 6, 40)
	assert.True(t, ok, "a known preset with no nodes is a valid zero, not an error")
	assert.Equal(t, 0.0, got)
	got, ok = FleetChannelUtilizationPercent("LONG_FAST", 10, 0, 40)
	assert.True(t, ok)
	assert.Equal(t, 0.0, got, "no offered traffic is zero utilization")
}

func TestLongPresetsUseCodingRate48(t *testing.T) {
	// LONG_MODERATE and LONG_SLOW use 4/8 coding (per the Meshtastic firmware
	// preset table), which lengthens their time-on-air versus 4/5. Pin LONG_SLOW
	// (SF12, BW125k, CR4/8, 40-byte payload): ~3023 ms. A regression to 4/5 would
	// compute ~2236 ms, well below the assertion.
	toa, ok := PresetTimeOnAir("LONG_SLOW", 40)
	assert.True(t, ok)
	ms := float64(toa.Microseconds()) / 1000
	assert.InDelta(t, 3022.8, ms, 5.0, "LONG_SLOW time-on-air must reflect 4/8 coding")
	assert.Greater(t, ms, 2900.0, "a regression to 4/5 coding (~2236 ms) would fail this")
}
