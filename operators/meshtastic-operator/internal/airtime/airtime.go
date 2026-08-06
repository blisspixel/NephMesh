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

// Package airtime models LoRa time-on-air, the physical cost of every frame a
// mesh sends. Airtime, not node count, is what a LoRa mesh actually runs out
// of: the founding scaling study (Bor et al., MSWiM 2016) and the Meshtastic
// preset link-budget work both show a channel collapses from airtime and
// collisions, and the longest-range presets have the longest airtime and
// collapse a dense channel fastest. Governing airtime as declared, enforced
// budget is the one guarantee a declarative intent system can offer over
// hand-tuned per-device configuration; this package is that model's foundation.
//
// The calculation is the standard Semtech LoRa time-on-air formula (SX1276
// datasheet section 4.1.1.7): symbol time from the spreading factor and
// bandwidth, a preamble term, and a payload symbol count that accounts for the
// coding rate, an optional CRC, the header mode, and low-data-rate optimization.
package airtime

import (
	"math"
	"time"
)

// Params are the LoRa radio parameters that determine time-on-air.
type Params struct {
	SpreadingFactor int // 7..12
	BandwidthHz     int // e.g. 250000
	CodingRate      int // the x in 4/x, so 5 means 4/5
}

// MeshtasticPreamble is the preamble length (in symbols) Meshtastic uses.
const MeshtasticPreamble = 16

// meshtasticPresets maps each Meshtastic modem preset to its radio parameters.
// All presets use coding rate 4/5. Values track the Meshtastic firmware preset
// table; the default is LONG_FAST.
var meshtasticPresets = map[string]Params{
	"SHORT_TURBO":   {SpreadingFactor: 7, BandwidthHz: 500000, CodingRate: 5},
	"SHORT_FAST":    {SpreadingFactor: 7, BandwidthHz: 250000, CodingRate: 5},
	"SHORT_SLOW":    {SpreadingFactor: 8, BandwidthHz: 250000, CodingRate: 5},
	"MEDIUM_FAST":   {SpreadingFactor: 9, BandwidthHz: 250000, CodingRate: 5},
	"MEDIUM_SLOW":   {SpreadingFactor: 10, BandwidthHz: 250000, CodingRate: 5},
	"LONG_FAST":     {SpreadingFactor: 11, BandwidthHz: 250000, CodingRate: 5},
	"LONG_MODERATE": {SpreadingFactor: 11, BandwidthHz: 125000, CodingRate: 5},
	"LONG_SLOW":     {SpreadingFactor: 12, BandwidthHz: 125000, CodingRate: 5},
}

// TimeOnAir returns the LoRa time-on-air of one frame with the given radio
// parameters and payload, per the Semtech formula. explicitHeader and crc match
// Meshtastic's use (both true).
func TimeOnAir(p Params, payloadBytes, preambleSymbols int, explicitHeader, crc bool) time.Duration {
	sf := float64(p.SpreadingFactor)
	// Symbol time in seconds.
	tSym := math.Exp2(sf) / float64(p.BandwidthHz)

	// Low-data-rate optimization is mandated when a symbol lasts longer than
	// 16 ms, which happens at the highest spreading factors and narrow bands.
	de := 0.0
	if tSym > 0.016 {
		de = 1.0
	}
	ih := 0.0
	if !explicitHeader {
		ih = 1.0
	}
	crcBits := 0.0
	if crc {
		crcBits = 1.0
	}

	tPreamble := (float64(preambleSymbols) + 4.25) * tSym

	numerator := 8*float64(payloadBytes) - 4*sf + 28 + 16*crcBits - 20*ih
	denominator := 4 * (sf - 2*de)
	// The (CR+4) multiplier in the datasheet equals the coding-rate denominator
	// x (4/x), which is CodingRate here.
	payloadSymbols := 8 + math.Max(math.Ceil(numerator/denominator)*float64(p.CodingRate), 0)
	tPayload := payloadSymbols * tSym

	return time.Duration((tPreamble + tPayload) * float64(time.Second))
}

// PresetTimeOnAir returns the time-on-air of one frame for a Meshtastic modem
// preset and payload size, using Meshtastic's defaults (16-symbol preamble,
// explicit header, CRC on). ok is false for an unknown preset.
func PresetTimeOnAir(preset string, payloadBytes int) (toa time.Duration, ok bool) {
	p, ok := meshtasticPresets[preset]
	if !ok {
		return 0, false
	}
	return TimeOnAir(p, payloadBytes, MeshtasticPreamble, true, true), true
}

// DutyCyclePercent returns the fraction of the channel one frame of the given
// time-on-air consumes when sent once per period, as a percentage. It is the
// unit an airtime budget is expressed in (for example the EU 10% ISM cap).
func DutyCyclePercent(toa, period time.Duration) float64 {
	if period <= 0 {
		return 0
	}
	return float64(toa) / float64(period) * 100
}

// Recommended airtime ceilings, as percentages. These are heuristics, not hard
// limits: Meshtastic mesh delivery degrades as channel utilization climbs
// (community guidance treats sustained use above ~25% as saturating), and a
// node's own transmit airtime should stay under the tightest regional duty
// cycle (the EU 10% ISM cap is the conservative default). The airtime-budget
// plan (docs/plans/airtime-budget.md) makes these region-aware and enforceable.
const (
	RecommendedChannelUtilizationPercent = 25.0
	RecommendedAirUtilTxPercent          = 10.0
)

// Healthy reports whether measured airtime utilization (the radio's own
// channelUtilization and airUtilTx telemetry) is within the recommended
// ceilings. It is the ground-truth check the operator surfaces as a condition.
func Healthy(channelUtilizationPercent, airUtilTxPercent float64) bool {
	return channelUtilizationPercent <= RecommendedChannelUtilizationPercent &&
		airUtilTxPercent <= RecommendedAirUtilTxPercent
}
