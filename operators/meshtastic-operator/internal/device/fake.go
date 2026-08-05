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

import "context"

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

	Applies  int
	Reboots  int
	Exports  int
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
	return deepCopyMap(f.config), nil
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
		if sub, ok := v.(map[string]any); ok {
			out[k] = deepCopyMap(sub)
		} else {
			out[k] = v
		}
	}
	return out
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
