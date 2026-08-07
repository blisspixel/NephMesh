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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/secret"
)

func TestFakeApplyChannelsStoresHashedAndReboots(t *testing.T) {
	ctx := context.Background()
	f := NewFake(map[string]any{}, 1)
	rawKey := "\x09\x0a\x0b\x0c"

	err := f.ApplyChannels(ctx, []ChannelWrite{{Index: 1, Name: "ops", Key: secret.New(rawKey), UplinkEnabled: true}})
	require.NoError(t, err)
	assert.Equal(t, 1, f.ChannelApplies)

	// The device is unreachable through the reboot window.
	_, err = f.ExportConfig(ctx)
	assert.ErrorIs(t, err, ErrUnreachable, "the device reboots after a channel apply")

	// After the window the export shows the channel with the key hashed, never raw.
	cfg, err := f.ExportConfig(ctx)
	require.NoError(t, err)
	chans, ok := cfg["channels"].([]any)
	require.True(t, ok)
	require.Len(t, chans, 1)
	ch := chans[0].(map[string]any)
	assert.Equal(t, "ops", ch["name"])
	assert.Equal(t, true, ch["uplinkEnabled"])
	sum := sha256.Sum256([]byte(rawKey))
	assert.Equal(t, hex.EncodeToString(sum[:]), ch["pskHash"], "the fake stores the key as a hash, matching the exporter")
}

func TestFakeApplyChannelsDefaultKeyHashesToShorthand(t *testing.T) {
	ctx := context.Background()
	f := NewFake(map[string]any{}, 0)
	require.NoError(t, f.ApplyChannels(ctx, []ChannelWrite{{Index: 0, Name: "primary"}})) // zero key -> default

	cfg, err := f.ExportConfig(ctx)
	require.NoError(t, err)
	ch := cfg["channels"].([]any)[0].(map[string]any)
	sum := sha256.Sum256([]byte{0x01})
	assert.Equal(t, hex.EncodeToString(sum[:]), ch["pskHash"], "a zero key models the device default (single 0x01 byte)")
}

func TestFakeApplyRebootsAndBecomesUnreachable(t *testing.T) {
	ctx := context.Background()
	f := NewFake(map[string]any{}, 2)

	require.NoError(t, f.Apply(ctx, map[string]any{"config": map[string]any{"lora": map[string]any{"region": "US"}}}))
	assert.Equal(t, 1, f.Applies)

	// The two calls in the reboot window are unreachable.
	_, err := f.ExportConfig(ctx)
	assert.ErrorIs(t, err, ErrUnreachable)
	_, err = f.ExportConfig(ctx)
	assert.ErrorIs(t, err, ErrUnreachable)

	// Then the applied config is observable.
	cfg, err := f.ExportConfig(ctx)
	require.NoError(t, err)
	lora := cfg["config"].(map[string]any)["lora"].(map[string]any)
	assert.Equal(t, "US", lora["region"], "apply merges into live config")
}

func TestFakeExportReturnsIndependentCopy(t *testing.T) {
	f := NewFake(map[string]any{"config": map[string]any{"lora": map[string]any{"region": "US"}}}, 0)
	cfg, err := f.ExportConfig(context.Background())
	require.NoError(t, err)
	cfg["config"].(map[string]any)["lora"].(map[string]any)["region"] = "EU_868"

	again, _ := f.ExportConfig(context.Background())
	assert.Equal(t, "US", again["config"].(map[string]any)["lora"].(map[string]any)["region"],
		"mutating an exported copy must not change device state")
}

func TestFakeRebootAndInfoRespectWindow(t *testing.T) {
	ctx := context.Background()
	f := NewFake(map[string]any{}, 1)
	require.NoError(t, f.Reboot(ctx))
	assert.Equal(t, 1, f.Reboots)

	_, err := f.Info(ctx)
	assert.ErrorIs(t, err, ErrUnreachable, "info is unreachable during the reboot window")

	info, err := f.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, "!6e000001", info.NodeID)

	// Rebooting while already mid-reboot is refused, mirroring the real device.
	f.unreachableFor = 1
	assert.True(t, errors.Is(f.Reboot(ctx), ErrUnreachable))
}
