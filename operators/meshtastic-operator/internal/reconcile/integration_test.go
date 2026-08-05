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

//go:build integration

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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
)

func TestIntegrationConvergeAgainstSimDevice(t *testing.T) {
	host := os.Getenv("MESH_TEST_HOST")
	if host == "" {
		t.Skip("set MESH_TEST_HOST=host:port of a running meshtasticd --sim to run this")
	}
	cli := &device.CLIClient{Host: host, Bin: os.Getenv("MESH_BIN")}
	// Exercise every field whose export path the builder emits, so the test
	// fails if any of them does not round-trip (the never-converge risk).
	desired := map[string]any{"config": map[string]any{
		"lora":   map[string]any{"region": "US", "modemPreset": "MEDIUM_SLOW"},
		"device": map[string]any{"role": "ROUTER"},
	}}

	// Drive the state machine to Ready. Poll on a short fixed interval rather
	// than the production Requeue delays so the test finishes promptly, while
	// still exercising the real apply, device reboot, and re-verify sequence.
	ctx := context.Background()
	state := State{}
	deadline := time.Now().Add(3 * time.Minute)
	var out Outcome
	for time.Now().Before(deadline) {
		var err error
		out, err = Converge(ctx, cli, desired, state)
		require.NoError(t, err, "convergence step should not hard-error against the sim")
		state = State{RebootPending: out.RebootPending, ApplyAttempts: out.ApplyAttempts}
		if out.Ready {
			require.True(t, out.ConfigInSync, "a ready device reports its config in sync")
			require.NotEmpty(t, out.Info.NodeID, "a reachable device reports its node id")
			return
		}
		require.Less(t, out.ApplyAttempts, int32(MaxApplyAttempts),
			"the device should converge well before the apply bound; a bound hit means a field never echoes back")
		time.Sleep(4 * time.Second)
	}
	t.Fatalf("device did not converge within the deadline (last outcome: %+v)", out)
}
