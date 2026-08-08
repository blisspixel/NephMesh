//go:build integration

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

// Integration test: drive the real Converge loop, through the CLI-backed device
// client, against a live meshtasticd running in simulation mode. It is gated by
// a build tag and by MESH_TEST_HOST so it never runs in the normal unit suite;
// it needs Docker, the Meshtastic CLI on PATH (or MESH_BIN), and a reachable
// sim device. This is the "executed against real firmware" bar for the operator
// that unit tests with the fake cannot provide.
//
// Bring up the device (from a POSIX shell):
//
//	docker run -d --name simdev --restart=always -p 14403:4403 \
//	  meshtastic/meshtasticd:beta-debian \
//	  meshtasticd --sim --fsdir=/var/lib/meshtasticd --port=4403
//	MESH_TEST_HOST=127.0.0.1:14403 go test -tags integration ./internal/reconcile -run Integration -v
//
// --restart=always matters: applying config makes meshtasticd exit (the device
// reboot), and the container must bring it back for the loop to re-verify.
package reconcile

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/config"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/secret"
)

// helperArgv reads a helper command from env or falls back to the bundled path,
// so the test runs in CI (where python has the meshtastic library) and locally
// against a checkout's virtualenv by overriding MESH_EXPORTER / MESH_APPLIER.
func helperArgv(env, fallback string) []string {
	v := os.Getenv(env)
	if v == "" {
		v = fallback
	}
	return strings.Fields(v)
}

func TestIntegrationConvergeAgainstSimDevice(t *testing.T) {
	host := os.Getenv("MESH_TEST_HOST")
	if host == "" {
		t.Skip("set MESH_TEST_HOST=host:port of a running meshtasticd --sim to run this")
	}
	// Use the same helpers production uses: the exporter (which emits the channel
	// set the CLI's --export-config omits) and the file-fed channel applier.
	cli := &device.CLIClient{
		Host:     host,
		Bin:      os.Getenv("MESH_BIN"),
		Exporter: helperArgv("MESH_EXPORTER", "python ../../hack/mesh-export.py"),
		Applier:  helperArgv("MESH_APPLIER", "python ../../hack/mesh-apply.py"),
	}
	// Exercise every field whose export path the builder emits, so the test fails
	// if any does not round-trip (the never-converge risk). Owner is a top-level
	// scalar pair confirmed to round-trip through --configure.
	desired := map[string]any{
		"owner":       "NephMesh Sim 01",
		"owner_short": "NM01",
		"config": map[string]any{
			"lora":   map[string]any{"region": "US", "modemPreset": "MEDIUM_SLOW"},
			"device": map[string]any{"role": "ROUTER"},
		},
		// MQTT with a password. The password is a write-only field the device never
		// echoes back, so this converging against real firmware is the regression
		// guard for the drift-comparison exclusion (config.ForComparison): the fake
		// hid this bug by merging and echoing the password, but a node with an MQTT
		// password used to reboot-loop forever against a real device.
		"module_config": map[string]any{"mqtt": map[string]any{
			"enabled": true, "address": "127.0.0.1",
			"json_enabled": true, "encryption_enabled": false, "tls_enabled": false,
			"password": "sim-broker-secret",
		}},
	}
	// Reconcile a secondary channel with a Secret-backed key, so the channel
	// export path (via the helper) and the file-fed apply are exercised against
	// real firmware, not just the fake.
	rawKey := "\x09\x0a\x0b\x0c"
	wantHash := config.PSKHash([]byte(rawKey))
	chans := DesiredChannels{
		Compare: []config.ChannelState{{Index: 1, Name: "relief", PSKHash: wantHash}},
		Write:   []device.ChannelWrite{{Index: 1, Name: "relief", Key: secret.New(rawKey)}},
	}

	// Drive the state machine to Ready. Poll on a short fixed interval rather than
	// the production Requeue delays so the test finishes promptly, while still
	// exercising the real apply, device reboot, and re-verify sequence (twice: the
	// scalar config and then the channel each reboot the device).
	ctx := context.Background()
	state := State{}
	deadline := time.Now().Add(4 * time.Minute)
	var out Outcome
	for time.Now().Before(deadline) {
		var err error
		out, err = Converge(ctx, cli, desired, chans, state)
		require.NoError(t, err, "convergence step should not hard-error against the sim")
		state = State{RebootPending: out.RebootPending, ApplyAttempts: out.ApplyAttempts}
		if out.Ready {
			require.True(t, out.ConfigInSync, "a ready device reports its config in sync")
			require.NotEmpty(t, out.Info.NodeID, "a reachable device reports its node id")
			found := false
			for _, ch := range out.LiveChannels {
				if ch.Index == 1 && ch.Name == "relief" && ch.PSKHash == wantHash {
					found = true
				}
			}
			require.True(t, found, "the secondary channel converged with its key hash, live channels: %+v", out.LiveChannels)
			return
		}
		require.Less(t, out.ApplyAttempts, int32(MaxApplyAttempts),
			"the device should converge well before the apply bound; a bound hit means a field never echoes back")
		time.Sleep(4 * time.Second)
	}
	t.Fatalf("device did not converge within the deadline (last outcome: %+v)", out)
}
