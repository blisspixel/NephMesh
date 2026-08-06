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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseExportConfigStripsChatterBeforeMarker(t *testing.T) {
	out := `Connected to radio
` + exportMarker + `
config:
  lora:
    region: US
module_config:
  mqtt:
    enabled: true
`
	cfg, err := parseExportConfig(out)
	require.NoError(t, err)
	lora := cfg["config"].(map[string]any)["lora"].(map[string]any)
	assert.Equal(t, "US", lora["region"])
	mqtt := cfg["module_config"].(map[string]any)["mqtt"].(map[string]any)
	assert.Equal(t, true, mqtt["enabled"])
}

func TestParseExportConfigEmptyYieldsEmptyMap(t *testing.T) {
	cfg, err := parseExportConfig("")
	require.NoError(t, err)
	assert.Empty(t, cfg)
}

func TestLooksUnreachableMatchesKnownCLIErrors(t *testing.T) {
	assert.True(t, looksUnreachable("Error connecting to meshnode-sim:[Errno 110] Connection timed out"))
	assert.True(t, looksUnreachable("Connection refused"))
	assert.False(t, looksUnreachable("Set lora.region to US"))

	// Serial reboot window: while a real board reboots after an apply it drops
	// off the USB bus, so the port cannot be opened. That is transient
	// unreachable (requeue), not a hard failure. Verified against a T-Deck.
	assert.True(t, looksUnreachable("could not open port 'COM3': PermissionError(13, 'Access is denied.', None, 5)"))
	assert.True(t, looksUnreachable("[Errno 2] could not open port /dev/ttyACM0: No such file or directory"))
}

func TestParseInfoFindsNodeID(t *testing.T) {
	out := `Owner: NephMesh (NM)
Nodes in mesh: { "!6e000001": { "num": 1845493761 } }`
	assert.Equal(t, "!6e000001", parseInfo(out).NodeID)
	assert.Empty(t, parseInfo("no node id here").NodeID)
}

func TestCLIClientBinDefault(t *testing.T) {
	assert.Equal(t, "meshtastic", (&CLIClient{}).bin())
	assert.Equal(t, "custom", (&CLIClient{Bin: "custom"}).bin())
}

// withRunner returns a CLIClient whose exec is replaced by a stub that records
// the last args and returns canned output, so the command orchestration is
// tested without the CLI binary.
func withRunner(out string, err error, gotArgs *[]string) *CLIClient {
	return &CLIClient{Host: "h", runFn: func(_ context.Context, args ...string) (string, error) {
		if gotArgs != nil {
			*gotArgs = args
		}
		return out, err
	}}
}

func TestExportConfigParsesRunnerOutput(t *testing.T) {
	c := withRunner(exportMarker+"\nconfig:\n  lora:\n    region: US\n", nil, nil)
	cfg, err := c.ExportConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "US", cfg["config"].(map[string]any)["lora"].(map[string]any)["region"])
}

func TestExportConfigPropagatesUnreachable(t *testing.T) {
	c := withRunner("", ErrUnreachable, nil)
	_, err := c.ExportConfig(context.Background())
	assert.ErrorIs(t, err, ErrUnreachable)
}

func TestApplyMarshalsAndInvokesConfigure(t *testing.T) {
	var args []string
	c := withRunner("", nil, &args)
	err := c.Apply(context.Background(), map[string]any{"config": map[string]any{"lora": map[string]any{"region": "US"}}})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(args), 2)
	assert.Equal(t, "--configure", args[0], "apply runs meshtastic --configure <file>")
	assert.True(t, strings.HasSuffix(args[1], ".yaml"), "apply passes a yaml file path")
}

func TestConnArgsSelectsTransport(t *testing.T) {
	assert.Equal(t, []string{"--host", "h"}, (&CLIClient{Host: "h"}).connArgs())
	assert.Equal(t, []string{"--port", "COM3"}, (&CLIClient{Serial: "COM3"}).connArgs(),
		"serial transport uses meshtastic --port")
}

func TestExportConfigUsesExporterWithSerialFlag(t *testing.T) {
	// Verified against a real T-Deck: the CLI's --export-config hangs over
	// serial, so the client runs the exporter with --serial instead.
	var gotName string
	var gotArgs []string
	c := &CLIClient{
		Serial:   "COM3",
		Exporter: []string{"python", "mesh-export.py"},
		execFn: func(_ context.Context, name string, args ...string) (string, error) {
			gotName, gotArgs = name, args
			return exportMarker + "\nconfig:\n  lora:\n    region: US\n", nil
		},
	}
	cfg, err := c.ExportConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "US", cfg["config"].(map[string]any)["lora"].(map[string]any)["region"])
	assert.Equal(t, "python", gotName)
	assert.Equal(t, []string{"mesh-export.py", "--serial", "COM3"}, gotArgs)
}

func TestExporterUsesHostFlagOverTCP(t *testing.T) {
	var gotArgs []string
	c := &CLIClient{
		Host:     "1.2.3.4",
		Exporter: []string{"python", "mesh-export.py"},
		execFn: func(_ context.Context, _ string, args ...string) (string, error) {
			gotArgs = args
			return exportMarker + "\n{}\n", nil
		},
	}
	_, err := c.ExportConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"mesh-export.py", "--host", "1.2.3.4"}, gotArgs)
}

func TestRebootInvokesReboot(t *testing.T) {
	var args []string
	c := withRunner("", nil, &args)
	require.NoError(t, c.Reboot(context.Background()))
	assert.Equal(t, []string{"--reboot"}, args)
}

func TestInfoParsesNodeID(t *testing.T) {
	c := withRunner(`Nodes: { "!6e000001": {} }`, nil, nil)
	info, err := c.Info(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "!6e000001", info.NodeID)
}

func TestInfoPropagatesError(t *testing.T) {
	c := withRunner("", ErrUnreachable, nil)
	_, err := c.Info(context.Background())
	assert.ErrorIs(t, err, ErrUnreachable)
}
