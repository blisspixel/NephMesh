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

package reconcile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
)

func desiredUS() map[string]any {
	return map[string]any{"config": map[string]any{"lora": map[string]any{"region": "US"}}}
}

func TestAlreadyConvergedIsReadyWithoutApply(t *testing.T) {
	dev := device.NewFake(desiredUS(), 0)
	out, err := Converge(context.Background(), dev, desiredUS(), false)
	require.NoError(t, err)
	assert.True(t, out.Ready)
	assert.True(t, out.ConfigInSync)
	assert.False(t, out.RebootPending)
	assert.Equal(t, DriftCheckInterval, out.Requeue)
	assert.Equal(t, 0, dev.Applies, "a converged device must not be written")
}

func TestDriftedDeviceAppliesOnceThenConvergesAcrossReboot(t *testing.T) {
	// Device starts empty (drifted from desired US) and stays unreachable for
	// two calls after a reboot.
	dev := device.NewFake(map[string]any{}, 2)

	// Step 1: drift detected, config applied, reboot pending.
	out, err := Converge(context.Background(), dev, desiredUS(), false)
	require.NoError(t, err)
	assert.True(t, out.RebootPending)
	assert.False(t, out.Ready)
	assert.Equal(t, RebootWait, out.Requeue)
	assert.Equal(t, 1, dev.Applies)

	// Steps 2..n: device is rebooting (unreachable). Reported as still
	// rebooting because reboot was pending, not as a fresh connect failure.
	rebootPending := out.RebootPending
	sawUnreachable := false
	var final Outcome
	for i := 0; i < 6; i++ {
		out, err = Converge(context.Background(), dev, desiredUS(), rebootPending)
		require.NoError(t, err)
		rebootPending = out.RebootPending
		if !out.Reachable {
			sawUnreachable = true
			assert.Equal(t, ReconnectBackoff, out.Requeue)
		}
		if out.Ready {
			final = out
			break
		}
	}
	assert.True(t, sawUnreachable, "the reboot window should be observed as unreachable")
	assert.True(t, final.Ready, "the device converges once it comes back")
	assert.True(t, final.ConfigInSync)
	assert.False(t, final.RebootPending)
	assert.Equal(t, 1, dev.Applies, "config is applied exactly once, not on every pass")
}

func TestUnreachableFromStartRequeuesWithoutError(t *testing.T) {
	dev := device.NewFake(map[string]any{}, 0)
	dev.Apply(context.Background(), map[string]any{}) // opens no window (rebootWindow 0)
	// Force an unreachable window directly by seeding a reboot.
	dev = device.NewFake(map[string]any{}, 3)
	dev.Reboot(context.Background()) // now unreachable for 3 calls

	out, err := Converge(context.Background(), dev, desiredUS(), false)
	require.NoError(t, err, "an unreachable device is a requeue, not an error")
	assert.False(t, out.Reachable)
	assert.Equal(t, ReconnectBackoff, out.Requeue)
	assert.Equal(t, 0, dev.Applies, "cannot apply to an unreachable device")
}
