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

// Package config computes the minimal difference between desired and live
// Meshtastic configuration. It is a pure, dependency-light port of the
// convergence check validated in the Phase 1 demo applier: the device is
// converged when every field the desired state declares is already present,
// with an equal value, in the live exported config. Fields the device reports
// but the desired state does not mention are ignored, so a partial desired
// config does not fight the device's many defaults.
package config

import (
	"fmt"
	"sort"
	"strings"
)

// normKey compares configuration keys case- and underscore-insensitively,
// because the Meshtastic CLI accepts snake_case on write but exports camelCase
// (json_enabled in, jsonEnabled out). Without this, a converged node would
// look drifted forever.
func normKey(k string) string {
	return strings.ReplaceAll(strings.ToLower(k), "_", "")
}

// IsConverged reports whether live already satisfies desired: every desired
// key is present in live (compared with normKey) with an equal value,
// recursively for nested maps. Scalars compare by trimmed, lowercased string
// form, matching the demo applier's tolerant comparison of YAML-decoded values.
func IsConverged(desired, live map[string]any) bool {
	return len(Drift(desired, live)) == 0
}

// writeOnlyPaths are desired keys the device accepts on a write but never echoes
// back in its export (secrets it will not disclose, notably the MQTT password).
// They must still be applied, but including them in the comparison reports
// permanent drift, since the live export never contains them, which would make a
// node with an MQTT password reboot-loop forever and never converge.
var writeOnlyPaths = [][]string{
	{"module_config", "mqtt", "password"},
}

// ForComparison returns a copy of desired with the write-only keys removed, so
// the drift check does not treat an unverifiable field as permanent drift. The
// full desired, write-only keys included, is still what gets applied to the
// device; this affects only the comparison. Because the write-only field is
// dropped from the compare, it is applied whenever any other field drifts (which
// covers initial provisioning).
//
// Known gap: rotating ONLY a write-only field on an otherwise-converged node is
// not detected today. Channel PSKs escape this because the device echoes a channel
// hash, so a hash compare catches a rotation; the MQTT password is never echoed by
// the device, so a password-only rotation on an already-Ready node is currently a
// no-op. Closing it needs a stored last-applied-password hash compared each
// reconcile (mirroring the channel path), tracked as day-2-rotation work.
func ForComparison(desired map[string]any) map[string]any {
	out := copyNestedMaps(desired)
	for _, path := range writeOnlyPaths {
		removePath(out, path)
	}
	return out
}

func copyNestedMaps(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if sub, ok := v.(map[string]any); ok {
			out[k] = copyNestedMaps(sub)
		} else {
			out[k] = v
		}
	}
	return out
}

func removePath(m map[string]any, path []string) {
	switch len(path) {
	case 0:
		return
	case 1:
		delete(m, path[0])
	default:
		if sub, ok := m[path[0]].(map[string]any); ok {
			removePath(sub, path[1:])
		}
	}
}

// Drift returns the dotted paths of desired keys that are missing from live or
// whose values differ. An empty result means converged. Paths are sorted so
// callers and tests get deterministic output.
func Drift(desired, live map[string]any) []string {
	var paths []string
	collectDrift("", desired, live, &paths)
	sort.Strings(paths)
	return paths
}

func collectDrift(prefix string, desired, live map[string]any, out *[]string) {
	liveByNorm := make(map[string]any, len(live))
	for k, v := range live {
		liveByNorm[normKey(k)] = v
	}
	for k, dv := range desired {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		lv, ok := liveByNorm[normKey(k)]
		if !ok {
			*out = append(*out, path)
			continue
		}
		dMap, dIsMap := dv.(map[string]any)
		lMap, lIsMap := lv.(map[string]any)
		switch {
		case dIsMap && lIsMap:
			collectDrift(path, dMap, lMap, out)
		case dIsMap != lIsMap:
			*out = append(*out, path)
		default:
			if !scalarEqual(dv, lv) {
				*out = append(*out, path)
			}
		}
	}
}

// scalarEqual compares values by their string form, trimmed of surrounding
// whitespace so YAML representation quirks (a trailing newline, spacing) do not
// register as drift. It is deliberately case-sensitive: values like an MQTT
// root topic are case-significant, and folding case here would hide real drift
// and leave the device permanently misconfigured. Key comparison, not value
// comparison, is where case is ignored (see normKey).
func scalarEqual(a, b any) bool {
	return strings.TrimSpace(fmt.Sprint(a)) == strings.TrimSpace(fmt.Sprint(b))
}
