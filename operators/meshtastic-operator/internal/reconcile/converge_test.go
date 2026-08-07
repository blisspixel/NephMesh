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

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/config"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/secret"
)

func desiredUS() map[string]any {
	return map[string]any{"config": map[string]any{"lora": map[string]any{"region": "US"}}}
}

func TestConvergeSurfacesLiveChannels(t *testing.T) {
	live := desiredUS()
	live["channels"] = []any{
		map[string]any{"index": 0, "name": "primary", "pskHash": "abc", "uplinkEnabled": true},
	}
	dev := device.NewFake(live, 0)

	out, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{})
	require.NoError(t, err)
	require.Len(t, out.LiveChannels, 1, "the device's channels are surfaced for the controller to diff")
	assert.Equal(t, int32(0), out.LiveChannels[0].Index)
	assert.Equal(t, "primary", out.LiveChannels[0].Name)
	assert.Equal(t, "abc", out.LiveChannels[0].PSKHash)
	assert.True(t, out.LiveChannels[0].UplinkEnabled)
}

func TestConvergesChannelsThroughApplyAndReboot(t *testing.T) {
	// The device starts with the scalar config already converged (region US) and
	// only the default primary channel; the desired state adds a secondary.
	dev := device.NewFake(desiredUS(), 1) // reboot window 1
	rawKey := "\x09\x0a\x0b\x0c"
	chans := DesiredChannels{
		Compare: []config.ChannelState{{Index: 1, Name: "ops", PSKHash: config.PSKHash([]byte(rawKey)), UplinkEnabled: true}},
		Write:   []device.ChannelWrite{{Index: 1, Name: "ops", Key: secret.New(rawKey), UplinkEnabled: true}},
	}

	// Step 1: scalar config already matches, so only channels apply, then reboot.
	out, err := Converge(context.Background(), dev, desiredUS(), chans, State{})
	require.NoError(t, err)
	assert.True(t, out.RebootPending)
	assert.False(t, out.Ready)
	assert.Equal(t, 1, dev.ChannelApplies, "channels applied once")
	assert.Equal(t, 0, dev.Applies, "the scalar config was already converged, so it is not reapplied")

	// Step 2..n: the device reboots (unreachable), then converges with channels in sync.
	state := State{RebootPending: out.RebootPending, ApplyAttempts: out.ApplyAttempts}
	var final Outcome
	for i := 0; i < 5; i++ {
		out, err = Converge(context.Background(), dev, desiredUS(), chans, state)
		require.NoError(t, err)
		state = State{RebootPending: out.RebootPending, ApplyAttempts: out.ApplyAttempts}
		if out.Ready {
			final = out
			break
		}
	}
	require.True(t, final.Ready, "the device converges once channels are written and it reboots")
	assert.True(t, final.ConfigInSync)
	assert.Equal(t, 1, dev.ChannelApplies, "channels are written exactly once, not on every pass")
}

func TestAlreadyConvergedIsReadyWithoutApply(t *testing.T) {
	dev := device.NewFake(desiredUS(), 0)
	out, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{})
	require.NoError(t, err)
	assert.True(t, out.Ready)
	assert.True(t, out.ConfigInSync)
	assert.False(t, out.RebootPending)
	assert.Equal(t, DriftCheckInterval, out.Requeue)
	assert.Equal(t, 0, dev.Applies, "a converged device must not be written")
}

func TestConvergedResetsApplyAttempts(t *testing.T) {
	dev := device.NewFake(desiredUS(), 0)
	out, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{ApplyAttempts: 3})
	require.NoError(t, err)
	assert.Equal(t, int32(0), out.ApplyAttempts, "convergence clears the attempt counter")
}

func TestDriftedDeviceAppliesOnceThenConvergesAcrossReboot(t *testing.T) {
	// Device starts empty (drifted from desired US) and stays unreachable for
	// two calls after a reboot.
	dev := device.NewFake(map[string]any{}, 2)

	// Step 1: drift detected, config applied, reboot pending.
	out, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{})
	require.NoError(t, err)
	assert.True(t, out.RebootPending)
	assert.False(t, out.Ready)
	assert.Equal(t, RebootWait, out.Requeue)
	assert.Equal(t, int32(1), out.ApplyAttempts)
	assert.Equal(t, 1, dev.Applies)

	// Steps 2..n: device is rebooting (unreachable). Reported as still
	// rebooting because reboot was pending, not as a fresh connect failure.
	state := State{RebootPending: out.RebootPending, ApplyAttempts: out.ApplyAttempts}
	sawUnreachable := false
	var final Outcome
	for i := 0; i < 6; i++ {
		out, err = Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, state)
		require.NoError(t, err)
		state = State{RebootPending: out.RebootPending, ApplyAttempts: out.ApplyAttempts}
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
	assert.Equal(t, int32(0), final.ApplyAttempts, "converged: counter reset")
	assert.Equal(t, 1, dev.Applies, "config is applied exactly once, not on every pass")
}

func TestApplyLoopIsBoundedThenDegraded(t *testing.T) {
	// A device that never converges (its export never contains the desired
	// value) must not reboot forever. After MaxApplyAttempts it is Degraded.
	dev := &neverConverges{}
	state := State{}
	var out Outcome
	var err error
	for i := 0; i < MaxApplyAttempts+2; i++ {
		out, err = Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, state)
		require.NoError(t, err)
		state = State{RebootPending: out.RebootPending, ApplyAttempts: out.ApplyAttempts}
	}
	assert.True(t, out.Degraded, "an unconvergeable device becomes Degraded, not an infinite loop")
	assert.False(t, out.RebootPending)
	assert.Equal(t, MaxApplyAttempts, dev.applies, "apply stops once the bound is reached")
}

func TestUnreachableFromStartRequeuesWithoutError(t *testing.T) {
	dev := device.NewFake(map[string]any{}, 3)
	_ = dev.Reboot(context.Background()) // now unreachable for 3 calls

	out, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{})
	require.NoError(t, err, "an unreachable device is a requeue, not an error")
	assert.False(t, out.Reachable)
	assert.Equal(t, ReconnectBackoff, out.Requeue)
	assert.Equal(t, 0, dev.Applies, "cannot apply to an unreachable device")
}

// neverConverges is a device whose export never reflects the desired config, so
// applies never converge. It counts applies so the bound can be asserted.
type neverConverges struct{ applies int }

func (n *neverConverges) ExportConfig(context.Context) (map[string]any, error) {
	return map[string]any{}, nil // always empty: never matches desired
}
func (n *neverConverges) Apply(context.Context, map[string]any) error { n.applies++; return nil }
func (n *neverConverges) Reboot(context.Context) error                { return nil }
func (n *neverConverges) Info(context.Context) (device.Info, error)   { return device.Info{}, nil }
func (n *neverConverges) ApplyChannels(context.Context, []device.ChannelWrite) error {
	return nil
}
