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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

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
//
// The converging apply mechanism is validated against meshtasticd --sim, and it
// is simpler than the channel_url encoding first feared: channels do not need the
// single base64 channel_url round-tripped byte-for-byte. Each channel applies
// independently with the CLI's per-channel setters, in one invocation and so one
// reboot, and reads back exactly:
//
//	meshtastic --ch-index 1 --ch-set name ops --ch-set psk base64:AQIDBA==
//	-> Index 1: SECONDARY psk=secret { "psk": "AQIDBA==", "name": "ops" }
//
// The key representation the device stores, and therefore the representation the
// declared key must normalize to before hashing, is: the default key is the
// single byte 0x01 (exported psk "AQ=="; set with `--ch-set psk default`), no key
// is the empty byte string (set with `--ch-set psk none`), and an explicit key is
// its raw bytes (set with `--ch-set psk base64:<raw-key-base64>`). Because the
// apply is per-channel and the compare is by hash, the whole surface avoids the
// attempt-and-record fragility of a canonical URL encoding.

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

// DefaultPSKShorthand is the single 0x01 byte the device stores for the public
// default channel key (exported psk "AQ==").
var DefaultPSKShorthand = []byte{0x01}

// defaultPSKExpanded is the well-known 16-byte Meshtastic default that 0x01
// expands to at use time. A Secret holding these bytes is the public default
// written out; the device may store either form.
var defaultPSKExpanded = []byte{
	0xd4, 0xf1, 0xbb, 0x3a, 0x20, 0x29, 0x07, 0x59,
	0xf0, 0xbc, 0xff, 0xab, 0xcd, 0x4e, 0x69, 0x01,
}

// IsDefaultPSK reports whether raw is the public default, in either the 0x01
// shorthand or the expanded 16-byte form.
func IsDefaultPSK(raw []byte) bool {
	return bytes.Equal(raw, DefaultPSKShorthand) || bytes.Equal(raw, defaultPSKExpanded)
}

// NormalizePSK maps the expanded public default onto the 0x01 shorthand the
// device stores, so a Secret holding the well-known 16-byte key does not
// never-converge against a device that stored the shorthand.
func NormalizePSK(raw []byte) []byte {
	if IsDefaultPSK(raw) {
		return DefaultPSKShorthand
	}
	return raw
}

// ValidChannelPSK reports whether raw is a key Meshtastic will store as
// declared: the public default (0x01 or its 16-byte expansion), or a 16- or
// 32-byte explicit key. Other lengths are accepted by some CLI paths then
// silently truncated or rejected, which looks like permanent channel drift.
func ValidChannelPSK(raw []byte) error {
	if IsDefaultPSK(raw) {
		return nil
	}
	switch len(raw) {
	case 16, 32:
		return nil
	default:
		return fmt.Errorf("channel PSK is %d bytes; Meshtastic keys are 16 or 32 bytes", len(raw))
	}
}

// PSKHash returns the hex SHA-256 of a raw pre-shared key. An empty key yields
// the empty string, so "no declared key" and "no key on the device" compare
// equal rather than reading as drift. The public default is hashed as the 0x01
// shorthand regardless of which representation was supplied.
func PSKHash(raw []byte) string {
	return rawPSKHash(NormalizePSK(raw))
}

func rawPSKHash(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// SecretsFingerprint is a stable hash of the resolved Secret-backed desired
// state (MQTT password hash plus declared channels). Secret edits do not bump
// metadata.generation; the controller compares this to status to know the
// apply bound must be reset. It hashes hashes, never raw keys or passwords.
func SecretsFingerprint(passwordHash string, chans []ChannelState) string {
	h := sha256.New()
	_, _ = h.Write([]byte(passwordHash))
	_, _ = h.Write([]byte{0})
	sorted := append([]ChannelState(nil), chans...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Index < sorted[j].Index })
	for _, ch := range sorted {
		_, _ = fmt.Fprintf(h, "%d\n%s\n%s\n%t\n%t\n", ch.Index, ch.Name, ch.PSKHash, ch.UplinkEnabled, ch.DownlinkEnabled)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ChannelSetPresent reports whether the export carried a channels list (even
// an empty one). Stock --export-config omits this key (it emits channel_url
// instead), which is distinct from "the device has no channels".
func ChannelSetPresent(export map[string]any) bool {
	_, ok := export["channels"].([]any)
	return ok
}

// DesiredChannels projects the spec's declared channels into comparable state.
// The keys are not in the spec (they live in Secrets), so the caller resolves
// each channel's key and passes its hash by index; a channel with no entry in
// pskHashByIndex is treated as having no declared key. The result is sorted by
// index for deterministic comparison and output.
func DesiredChannels(spec meshv1alpha1.MeshtasticNodeSpec, pskHashByIndex map[int32]string) []ChannelState {
	out := make([]ChannelState, 0, len(spec.Channels))
	for _, ch := range spec.Channels {
		hash, ok := pskHashByIndex[ch.Index]
		if !ok {
			// No resolved key means the public default, the same 0x01
			// shorthand the controller uses when pskSecretRef is omitted.
			hash = PSKHash(DefaultPSKShorthand)
		}
		out = append(out, ChannelState{
			Index:           ch.Index,
			Name:            ch.Name,
			PSKHash:         hash,
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
		idx, ok := toInt32(m["index"])
		if !ok {
			// An unparseable index would otherwise default to slot 0 and either
			// mask a real channel-0 drift or fabricate one; skip it instead.
			continue
		}
		out = append(out, ChannelState{
			Index:           idx,
			Name:            toString(m["name"]),
			PSKHash:         normalizeLivePSKHash(toString(m["pskHash"])),
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
	switch b := v.(type) {
	case bool:
		return b
	case string:
		switch strings.TrimSpace(strings.ToLower(b)) {
		case "true", "yes", "on", "1":
			return true
		default:
			return false
		}
	case int:
		return b != 0
	case int32:
		return b != 0
	case int64:
		return b != 0
	case float64:
		return b != 0
	default:
		return false
	}
}

// toInt32 converts a decoded index value (YAML gives int/int64, JSON gives
// float64, a stray source might give a string) to int32. ok is false for an
// unparseable value, so the caller can skip the entry rather than defaulting it
// to slot 0. A non-integral float (JSON 1.9) is refused rather than truncated.
func toInt32(v any) (int32, bool) {
	switch n := v.(type) {
	case int:
		return int32(n), true
	case int32:
		return n, true
	case int64:
		return int32(n), true
	case float64:
		i := int32(n)
		if float64(i) != n {
			return 0, false
		}
		return i, true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		return int32(i), true
	default:
		return 0, false
	}
}

// normalizeLivePSKHash maps a hash of the expanded public default onto the
// shorthand hash, so a device that stored the 16-byte form still compares
// equal to a declared default channel.
func normalizeLivePSKHash(h string) string {
	if h == rawPSKHash(defaultPSKExpanded) {
		return rawPSKHash(DefaultPSKShorthand)
	}
	return h
}
