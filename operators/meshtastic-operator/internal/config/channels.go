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

package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
)

// Channels are handled apart from the scalar field diff (BuildDesired/Drift)
// because the Meshtastic export encodes the whole channel set as a single
// channel_url (a base64 protobuf), not discrete fields, so they need their own
// projection and comparison. The device-facing apply path is deliberately
// separate and validated against a live device before it is wired into the
// converging loop (channels are attempt-and-record per the roadmap); this file
// is the pure, hardware-free half: project the declared channels and the live
// channels into a comparable form and report drift.
//
// The pre-shared key is compared by hash, never by value. Drift detection needs
// to know whether the key matches, not what it is, and hashing keeps the key out
// of the diff, the logs, and the status, matching the redacting-secret posture
// the rest of the operator holds.

// ChannelState is one channel's comparable configuration, keyed by the slot
// index the device uses. PSKHash is the hex SHA-256 of the raw key, or "" when
// there is no key.
type ChannelState struct {
	Index           int32
	Name            string
	PSKHash         string
	UplinkEnabled   bool
	DownlinkEnabled bool
}

// PSKHash returns the hex SHA-256 of a raw pre-shared key. An empty key yields
// the empty string, so "no declared key" and "no key on the device" compare
// equal rather than reading as drift.
func PSKHash(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// DesiredChannels projects the spec's declared channels into comparable state.
// The keys are not in the spec (they live in Secrets), so the caller resolves
// each channel's key and passes its hash by index; a channel with no entry in
// pskHashByIndex is treated as having no declared key. The result is sorted by
// index for deterministic comparison and output.
func DesiredChannels(spec meshv1alpha1.MeshtasticNodeSpec, pskHashByIndex map[int32]string) []ChannelState {
	out := make([]ChannelState, 0, len(spec.Channels))
	for _, ch := range spec.Channels {
		out = append(out, ChannelState{
			Index:           ch.Index,
			Name:            ch.Name,
			PSKHash:         pskHashByIndex[ch.Index],
			UplinkEnabled:   ch.UplinkEnabled,
			DownlinkEnabled: ch.DownlinkEnabled,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

// LiveChannels reads the channels the exporter emitted under the top-level
// "channels" key of a live export. The exporter emits each key as a hash
// (pskHash), never the raw key, so the operator compares without the key ever
// leaving the device. Malformed entries are skipped rather than failing the
// whole export, matching the tolerant parsing the field diff already uses.
func LiveChannels(export map[string]any) []ChannelState {
	raw, ok := export["channels"].([]any)
	if !ok {
		return nil
	}
	out := make([]ChannelState, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, ChannelState{
			Index:           toInt32(m["index"]),
			Name:            toString(m["name"]),
			PSKHash:         toString(m["pskHash"]),
			UplinkEnabled:   toBool(m["uplinkEnabled"]),
			DownlinkEnabled: toBool(m["downlinkEnabled"]),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

// ChannelDrift returns the dotted paths of declared channel fields that are
// missing or differ on the device (for example "channel[1].name",
// "channel[1].psk"). An empty result means every declared channel matches.
// Live channels the spec does not declare are ignored, mirroring the field diff:
// a partial desired state does not fight channels the operator does not manage.
func ChannelDrift(desired, live []ChannelState) []string {
	liveByIndex := make(map[int32]ChannelState, len(live))
	for _, ch := range live {
		liveByIndex[ch.Index] = ch
	}
	var paths []string
	for _, d := range desired {
		l, ok := liveByIndex[d.Index]
		if !ok {
			paths = append(paths, fmt.Sprintf("channel[%d]", d.Index))
			continue
		}
		if d.Name != l.Name {
			paths = append(paths, fmt.Sprintf("channel[%d].name", d.Index))
		}
		if d.PSKHash != l.PSKHash {
			paths = append(paths, fmt.Sprintf("channel[%d].psk", d.Index))
		}
		if d.UplinkEnabled != l.UplinkEnabled {
			paths = append(paths, fmt.Sprintf("channel[%d].uplinkEnabled", d.Index))
		}
		if d.DownlinkEnabled != l.DownlinkEnabled {
			paths = append(paths, fmt.Sprintf("channel[%d].downlinkEnabled", d.Index))
		}
	}
	sort.Strings(paths)
	return paths
}

// ChannelsConverged reports whether every declared channel already matches the
// device.
func ChannelsConverged(desired, live []ChannelState) bool {
	return len(ChannelDrift(desired, live)) == 0
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func toBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func toInt32(v any) int32 {
	switch n := v.(type) {
	case int:
		return int32(n)
	case int32:
		return n
	case int64:
		return int32(n)
	case float64:
		return int32(n)
	default:
		return 0
	}
}
