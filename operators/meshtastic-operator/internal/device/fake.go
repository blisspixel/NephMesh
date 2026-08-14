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

package device

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/secret"
)

// Fake is an in-memory Client for tests. It models the real device's two
// awkward behaviors deterministically: a config Apply reboots the device,
// which makes it unreachable for a fixed number of subsequent calls (no wall
// clock, so tests are not flaky), and only after that window does the applied
// config become observable. Reachability is call-counted rather than
// time-based on purpose.
type Fake struct {
	config map[string]any
	// unreachableFor is the number of upcoming calls that return
	// ErrUnreachable, simulating the post-reboot window.
	unreachableFor int
	// rebootWindow is how many calls the device stays unreachable after a
	// reboot. Set to 0 for a device that never drops (useful for the
	// already-converged path).
	rebootWindow int
	info         Info

	Applies        int
	Reboots        int
	Exports        int
	ChannelApplies int
}

// NewFake returns a Fake seeded with the given live config and reboot window.
func NewFake(initial map[string]any, rebootWindow int) *Fake {
	if initial == nil {
		initial = map[string]any{}
	}
	return &Fake{
		config:       initial,
		rebootWindow: rebootWindow,
		info:         Info{NodeID: "!6e000001"},
	}
}

// SetInfo overrides the identity and telemetry the fake reports, for tests that
// need the device to report airtime (channel utilization, transmit airtime).
func (f *Fake) SetInfo(i Info) { f.info = i }

func (f *Fake) tick() bool {
	if f.unreachableFor > 0 {
		f.unreachableFor--
		return false
	}
	return true
}

// ExportConfig returns the live config, or ErrUnreachable during a reboot
// window.
func (f *Fake) ExportConfig(_ context.Context) (map[string]any, error) {
	f.Exports++
	if !f.tick() {
		return nil, ErrUnreachable
	}
	// Return a copy so callers cannot mutate the device's state.
	out := deepCopyMap(f.config)
	// Model the real device faithfully: a secret it accepts on write is never
	// echoed back in its export. Without this the fake would be MORE forgiving
	// than reality (it merges and re-emits everything applied), which is exactly
	// how a write-only-field bug, an MQTT password read as permanent drift, hid in
	// unit tests while breaking against a real device.
	stripWriteOnly(out)
	// The bundled exporter always emits a channels list (possibly empty). Stock
	// --export-config omits the key; that shape is tested with a stub, not this
	// fake, so a declared channel is drift rather than "unobserved".
	if _, ok := out["channels"]; !ok {
		out["channels"] = []any{}
	}
	return out, nil
}

// Applied returns the fake's raw stored config, including the write-only fields
// ExportConfig strips, so a test can verify what was actually written to the
// device (distinct from what the device would echo back).
func (f *Fake) Applied() map[string]any { return deepCopyMap(f.config) }

// stripWriteOnly removes the fields the real device accepts but never returns
// (the MQTT password), so the fake's export matches a real export.
func stripWriteOnly(m map[string]any) {
	mc, ok := m["module_config"].(map[string]any)
	if !ok {
		return
	}
	if mqtt, ok := mc["mqtt"].(map[string]any); ok {
		delete(mqtt, "password")
	}
}

// Apply merges desired into the live config and reboots, so the device is
// unreachable for the reboot window afterward. Returns ErrUnreachable if the
// device is already mid-reboot.
func (f *Fake) Apply(_ context.Context, desired map[string]any) error {
	if !f.tick() {
		return ErrUnreachable
	}
	mergeMap(f.config, desired)
	f.Applies++
	f.unreachableFor = f.rebootWindow
	return nil
}

// ApplyChannels models the device storing the given channels: it merges them
// into the exported config under "channels" the way the real exporter emits them
// (keyed by index, the key hashed, never stored raw), then reboots. This lets the
// convergence loop be tested end to end: apply channels, reboot, then an export
// shows them converged.
func (f *Fake) ApplyChannels(_ context.Context, channels []ChannelWrite) error {
	if !f.tick() {
		return ErrUnreachable
	}
	if len(channels) == 0 {
		return nil
	}
	byIndex := map[int32]map[string]any{}
	if raw, ok := f.config["channels"].([]any); ok {
		for _, item := range raw {
			if m, ok := item.(map[string]any); ok {
				byIndex[fakeInt32(m["index"])] = m
			}
		}
	}
	for _, ch := range channels {
		byIndex[ch.Index] = map[string]any{
			"index":           int(ch.Index),
			"name":            ch.Name,
			"pskHash":         fakeChannelPSKHash(ch.Key),
			"uplinkEnabled":   ch.UplinkEnabled,
			"downlinkEnabled": ch.DownlinkEnabled,
		}
	}
	indexes := make([]int32, 0, len(byIndex))
	for i := range byIndex {
		indexes = append(indexes, i)
	}
	sort.Slice(indexes, func(a, b int) bool { return indexes[a] < indexes[b] })
	list := make([]any, 0, len(indexes))
	for _, i := range indexes {
		list = append(list, byIndex[i])
	}
	f.config["channels"] = list
	f.ChannelApplies++
	f.unreachableFor = f.rebootWindow
	return nil
}

// fakeChannelPSKHash mirrors the device: a zero key is the default (the single
// byte 0x01), an explicit key is its raw bytes, hashed the same way the exporter
// and internal/config do, so the modeled export compares equal to a matching
// declared channel.
func fakeChannelPSKHash(key secret.Value) string {
	var raw []byte
	if key.IsZero() {
		raw = []byte{0x01}
	} else {
		raw = []byte(key.Reveal())
	}
	if len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func fakeInt32(v any) int32 {
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

// Reboot restarts the device, opening a fresh unreachable window.
func (f *Fake) Reboot(_ context.Context) error {
	if f.unreachableFor > 0 {
		return ErrUnreachable
	}
	f.Reboots++
	f.unreachableFor = f.rebootWindow
	return nil
}

// Info returns identity, or ErrUnreachable during a reboot window.
func (f *Fake) Info(_ context.Context) (Info, error) {
	if !f.tick() {
		return Info{}, ErrUnreachable
	}
	return f.info, nil
}

func deepCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}
	return out
}

// deepCopyValue copies nested maps AND slices, so the config returned by
// ExportConfig is fully independent of the Fake's internal state (a shallow copy
// would share the "channels" slice by reference, letting a test corrupt device
// state and mask a real bug).
func deepCopyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return deepCopyMap(t)
	case []any:
		cp := make([]any, len(t))
		for i, e := range t {
			cp[i] = deepCopyValue(e)
		}
		return cp
	default:
		return v
	}
}

func mergeMap(dst, src map[string]any) {
	for k, v := range src {
		if sv, ok := v.(map[string]any); ok {
			dv, ok := dst[k].(map[string]any)
			if !ok {
				dv = map[string]any{}
				dst[k] = dv
			}
			mergeMap(dv, sv)
			continue
		}
		dst[k] = v
	}
}
