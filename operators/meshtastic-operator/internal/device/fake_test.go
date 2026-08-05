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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
