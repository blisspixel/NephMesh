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

func TestConvergesWithWriteOnlyPasswordAgainstFakeDevice(t *testing.T) {
	// The fake now models the real device: it does not echo the MQTT password back
	// in its export. So a node with a password must still converge, because the
	// write-only field is excluded from the drift comparison (config.ForComparison).
	// Before that exclusion this looped forever, and now the fake, not only the
	// sim, guards the regression.
	desired := map[string]any{
		"config": map[string]any{"lora": map[string]any{"region": "US"}},
		"module_config": map[string]any{"mqtt": map[string]any{
			"enabled": true, "address": "10.0.0.5", "password": "s3cret",
		}},
	}
	dev := device.NewFake(map[string]any{}, 1) // starts drifted, reboot window 1

	state := State{}
	var final Outcome
	for i := 0; i < 6; i++ {
		out, err := Converge(context.Background(), dev, desired, DesiredChannels{}, state)
		require.NoError(t, err)
		state = out.NextState()
		if out.Ready {
			final = out
			break
		}
	}
	require.True(t, final.Ready, "a node with a write-only MQTT password must converge, not reboot-loop")
	assert.True(t, final.ConfigInSync)

	// The device received the password (via Apply) but never echoes it back.
	live, err := dev.ExportConfig(context.Background())
	require.NoError(t, err)
	mqtt := live["module_config"].(map[string]any)["mqtt"].(map[string]any)
	_, hasPw := mqtt["password"]
	assert.False(t, hasPw, "the fake models the real device: the password is not echoed back")
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
	state := out.NextState()
	var final Outcome
	for i := 0; i < 5; i++ {
		out, err = Converge(context.Background(), dev, desiredUS(), chans, state)
		require.NoError(t, err)
		state = out.NextState()
		if out.Ready {
			final = out
			break
		}
	}
	require.True(t, final.Ready, "the device converges once channels are written and it reboots")
	assert.True(t, final.ConfigInSync)
	assert.Equal(t, 1, dev.ChannelApplies, "channels are written exactly once, not on every pass")
}

func TestInfoFromExport(t *testing.T) {
	info := infoFromExport(map[string]any{
		"nodeId":        "!abcd1234",
		"deviceMetrics": map[string]any{"airUtilTx": 2.5, "channelUtilization": 11.0},
	})
	assert.Equal(t, "!abcd1234", info.NodeID)
	require.NotNil(t, info.AirUtilTx)
	assert.Equal(t, 2.5, *info.AirUtilTx)
	require.NotNil(t, info.ChannelUtilization)
	assert.Equal(t, 11.0, *info.ChannelUtilization)

	// Absent identity yields an empty NodeID (so Converge falls back to --info),
	// and absent telemetry stays nil rather than a misleading 0.
	empty := infoFromExport(map[string]any{})
	assert.Empty(t, empty.NodeID)
	assert.Nil(t, empty.AirUtilTx)
	assert.Nil(t, empty.ChannelUtilization)
}

func TestConvergePrefersExportIdentityOverInfo(t *testing.T) {
	// When the export carries the local node id, Converge uses it and does not
	// fall back to --info, which on a real mesh could return a neighbor's values.
	live := desiredUS()
	live["nodeId"] = "!feed0001"
	dev := &countingInfoFake{Fake: device.NewFake(live, 0)}

	out, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{})
	require.NoError(t, err)
	assert.Equal(t, "!feed0001", out.Info.NodeID, "identity comes from the export")
	assert.Zero(t, dev.infoCalls, "no --info fallback when the export carries identity")
}

// countingInfoFake counts Info calls to prove Converge avoids the --info fallback
// when the export already carries the node id.
type countingInfoFake struct {
	*device.Fake
	infoCalls int
}

func (f *countingInfoFake) Info(ctx context.Context) (device.Info, error) {
	f.infoCalls++
	return f.Fake.Info(ctx)
}

func TestConvergeSurfacesTelemetryWhileDrifted(t *testing.T) {
	// The airtime prediction needs the radio's measured utilization at the moment
	// a preset change is still pending, which is when the config is NOT converged.
	// So telemetry must be surfaced on the drifted (apply-pending) path, not only
	// once converged.
	dev := device.NewFake(map[string]any{}, 0) // drifted from desired US
	util := 15.0
	dev.SetInfo(device.Info{NodeID: "!x", ChannelUtilization: &util})

	out, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{})
	require.NoError(t, err)
	assert.False(t, out.Ready, "the device is drifted and applying")
	require.NotNil(t, out.Info.ChannelUtilization, "telemetry is surfaced while drifted, so the airtime prediction has inputs")
	assert.Equal(t, 15.0, *out.Info.ChannelUtilization)
}

func TestPasswordOnlyRotationApplies(t *testing.T) {
	// Device already matches the echoed fields. A new MQTT password must still
	// apply: the live export never contains it, so only the stored hash can
	// detect the rotation.
	desired := map[string]any{
		"config": map[string]any{"lora": map[string]any{"region": "US"}},
		"module_config": map[string]any{"mqtt": map[string]any{
			"enabled": true, "address": "10.0.0.5", "password": "rotated",
		}},
	}
	live := map[string]any{
		"config":        map[string]any{"lora": map[string]any{"region": "US"}},
		"module_config": map[string]any{"mqtt": map[string]any{"enabled": true, "address": "10.0.0.5"}},
	}
	dev := device.NewFake(live, 0)
	oldHash := config.PSKHash([]byte("previous"))
	out, err := Converge(context.Background(), dev, desired, DesiredChannels{}, State{MQTTPasswordHash: oldHash})
	require.NoError(t, err)
	assert.True(t, out.RebootPending, "a password-only rotation must apply")
	assert.Equal(t, 1, dev.Applies)
	assert.Equal(t, config.WriteOnlyPasswordHash(desired), out.MQTTPasswordHash)
	assert.Contains(t, out.Drift, "module_config.mqtt.password")

	// Same password as last applied: no write.
	dev2 := device.NewFake(live, 0)
	out2, err := Converge(context.Background(), dev2, desired, DesiredChannels{}, State{MQTTPasswordHash: config.WriteOnlyPasswordHash(desired)})
	require.NoError(t, err)
	assert.True(t, out2.Ready)
	assert.Equal(t, 0, dev2.Applies, "an unchanged password is not reapplied")
}

func TestMissingChannelSetDoesNotApply(t *testing.T) {
	// Stock --export-config has no discrete channels key. Declared channels
	// must not be treated as missing (that reboot-looped). Scalar config can
	// still apply.
	chans := DesiredChannels{
		Compare: []config.ChannelState{{Index: 1, Name: "ops", PSKHash: "abc"}},
		Write:   []device.ChannelWrite{{Index: 1, Name: "ops"}},
	}
	// Already scalar-converged, no channels key (stock --export-config shape).
	dev := stubClient{live: desiredUS()}
	out, err := Converge(context.Background(), dev, desiredUS(), chans, State{})
	require.NoError(t, err)
	assert.False(t, out.Ready)
	assert.True(t, out.ChannelsUnobserved)
	assert.Equal(t, ReconnectBackoff, out.Requeue)
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

func TestConvergedWithBlankIdentityIsNotReady(t *testing.T) {
	// Config is converged, but neither the export nor the --info fallback carried a
	// node id (a partial or rate-limited read). Converge must not report a Ready
	// state built on no identity; it requeues as if mid-reboot.
	dev := device.NewFake(desiredUS(), 0) // converged from desired US
	dev.SetInfo(device.Info{NodeID: ""})  // the --info fallback also carries no identity
	out, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{})
	require.NoError(t, err)
	assert.False(t, out.Ready, "no identity must not read as Ready")
	assert.False(t, out.Reachable)
	assert.Empty(t, out.Info.NodeID)
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
	state := out.NextState()
	sawUnreachable := false
	var final Outcome
	for i := 0; i < 6; i++ {
		out, err = Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, state)
		require.NoError(t, err)
		state = out.NextState()
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
		state = out.NextState()
	}
	assert.True(t, out.Degraded, "an unconvergeable device becomes Degraded, not an infinite loop")
	assert.False(t, out.RebootPending)
	assert.Equal(t, MaxApplyAttempts, dev.applies, "apply stops once the bound is reached")
}

func TestUnreachablePreservesDegraded(t *testing.T) {
	dev := device.NewFake(map[string]any{}, 3)
	_ = dev.Reboot(context.Background())
	out, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{ApplyAttempts: MaxApplyAttempts})
	require.NoError(t, err)
	assert.False(t, out.Reachable)
	assert.True(t, out.Degraded, "an already-bound node must stay Degraded while offline, not flicker recovered")
	assert.Equal(t, int32(MaxApplyAttempts), out.ApplyAttempts)
}

func TestUnsupportedTransportDoesNotHammer(t *testing.T) {
	out, err := Converge(context.Background(), &device.Unsupported{Transport: "serial"}, desiredUS(), DesiredChannels{}, State{})
	require.NoError(t, err)
	assert.False(t, out.Ready)
	assert.False(t, out.Reachable)
	assert.Equal(t, DriftCheckInterval, out.Requeue, "an unimplemented transport must not retry every ReconnectBackoff")
	assert.Equal(t, 0, int(out.ApplyAttempts))
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
