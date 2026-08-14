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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
)

// stubClient lets a test inject specific errors on specific calls to exercise
// branches the deterministic Fake cannot reach naturally.
type stubClient struct {
	exportErr error
	live      map[string]any
	applyErr  error
	rebootErr error
	infoErr   error
}

func (s stubClient) ExportConfig(context.Context) (map[string]any, error) {
	if s.exportErr != nil {
		return nil, s.exportErr
	}
	return s.live, nil
}
func (s stubClient) Apply(context.Context, map[string]any) error { return s.applyErr }
func (s stubClient) Reboot(context.Context) error                { return s.rebootErr }
func (s stubClient) Info(context.Context) (device.Info, error)   { return device.Info{}, s.infoErr }
func (s stubClient) ApplyChannels(context.Context, []device.ChannelWrite) error {
	return s.applyErr
}

func TestExportUnexpectedErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	_, err := Converge(context.Background(), stubClient{exportErr: boom}, desiredUS(), DesiredChannels{}, State{})
	assert.ErrorIs(t, err, boom, "a non-unreachable export error is returned for rate-limited retry")
}

func TestInfoUnexpectedErrorPropagates(t *testing.T) {
	boom := errors.New("info parse failed")
	// Export succeeds (device reachable) but --info fails with a non-unreachable
	// error. It must be surfaced, not swallowed into a converged Ready with an
	// empty node id and no telemetry.
	dev := stubClient{live: desiredUS(), infoErr: boom}
	_, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{})
	assert.ErrorIs(t, err, boom, "an unexpected --info error is surfaced, not swallowed")
}

func TestApplyUnexpectedErrorPropagates(t *testing.T) {
	boom := errors.New("apply failed")
	dev := stubClient{live: map[string]any{}, applyErr: boom} // drifted, apply errors
	_, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{})
	assert.ErrorIs(t, err, boom)
}

func TestExportDeadlineExceededIsUnreachableNotError(t *testing.T) {
	// A hung CLI killed by the client timeout must requeue as unreachable, not
	// return a hard error the controller would retry immediately.
	out, err := Converge(context.Background(), stubClient{exportErr: context.DeadlineExceeded}, desiredUS(), DesiredChannels{}, State{})
	require.NoError(t, err)
	assert.False(t, out.Reachable)
	assert.Equal(t, ReconnectBackoff, out.Requeue)
}

func TestApplyDeadlineExceededCountsAsApply(t *testing.T) {
	// Apply wrote, then the session hung until the deadline. Count it toward
	// the apply bound and wait for reboot, the same as ErrUnreachable.
	dev := stubClient{live: map[string]any{}, applyErr: context.DeadlineExceeded}
	out, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{})
	require.NoError(t, err)
	assert.True(t, out.RebootPending)
	assert.Equal(t, int32(1), out.ApplyAttempts)
	assert.Equal(t, RebootWait, out.Requeue)
}

func TestCanceledContextStillPropagates(t *testing.T) {
	_, err := Converge(context.Background(), stubClient{exportErr: context.Canceled}, desiredUS(), DesiredChannels{}, State{})
	assert.ErrorIs(t, err, context.Canceled, "shutdown must not be rewritten as a requeue")
}

func TestApplyUnreachableDoesNotCommitPasswordHash(t *testing.T) {
	// Password-only drift: echoed MQTT fields already match. Apply drops
	// before the write is confirmed. Committing the hash would make the next
	// reachable export look Ready with the password never on the radio.
	desired := map[string]any{
		"config": map[string]any{"lora": map[string]any{"region": "US"}},
		"module_config": map[string]any{"mqtt": map[string]any{
			"enabled": true, "address": "10.0.0.5", "password": "never-landed",
		}},
	}
	live := map[string]any{
		"config":        map[string]any{"lora": map[string]any{"region": "US"}},
		"module_config": map[string]any{"mqtt": map[string]any{"enabled": true, "address": "10.0.0.5"}},
	}
	out, err := Converge(context.Background(), stubClient{live: live, applyErr: device.ErrUnreachable}, desired, DesiredChannels{}, State{})
	require.NoError(t, err)
	assert.True(t, out.RebootPending)
	assert.Empty(t, out.MQTTPasswordHash, "an unconfirmed write must not commit the password hash")

	// Reachable again with that prior hash: still drifted, must apply, not Ready.
	dev := device.NewFake(live, 0)
	out2, err := Converge(context.Background(), dev, desired, DesiredChannels{}, State{MQTTPasswordHash: out.MQTTPasswordHash})
	require.NoError(t, err)
	assert.False(t, out2.Ready)
	assert.Equal(t, 1, dev.Applies, "the password must still be written once a session holds")
}

func TestApplyUnreachableIsRequeueNotError(t *testing.T) {
	dev := stubClient{live: map[string]any{}, applyErr: device.ErrUnreachable} // drifted, apply hits reboot
	out, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{})
	require.NoError(t, err)
	assert.False(t, out.Reachable)
	assert.True(t, out.RebootPending, "apply-unreachable is treated as a write that rebooted the device")
	assert.Equal(t, int32(1), out.ApplyAttempts, "the apply bound must advance or a never-echoed field reboot-loops")
	assert.Equal(t, RebootWait, out.Requeue)
}

func TestRebootUnexpectedErrorIsRebootPending(t *testing.T) {
	// Apply already wrote the config. An unexpected reboot error (device already
	// dropping off after the apply) must record reboot-pending and increment
	// ApplyAttempts, not return an error the controller would discard without
	// writing status (which used to leave the apply bound stuck at 0).
	boom := errors.New("reboot failed")
	dev := stubClient{live: map[string]any{}, rebootErr: boom} // drifted, apply ok, reboot errors
	out, err := Converge(context.Background(), dev, desiredUS(), DesiredChannels{}, State{})
	require.NoError(t, err, "a post-apply reboot error is reboot-pending, not a hard failure")
	assert.True(t, out.RebootPending)
	assert.Equal(t, int32(1), out.ApplyAttempts)
	assert.Equal(t, RebootWait, out.Requeue)
}
