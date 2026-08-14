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
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/secret"
)

func TestApplyChannelsKeepsKeysInFileNotArgv(t *testing.T) {
	rawKey := "\x09\x0a\x0b\x0c"
	keyB64 := base64.StdEncoding.EncodeToString([]byte(rawKey))

	var gotName string
	var gotArgs []string
	var fileContent []byte
	c := &CLIClient{
		Host:    "127.0.0.1:14403",
		Applier: []string{"mesh-apply.py"},
		execFn: func(_ context.Context, name string, args ...string) (string, error) {
			gotName, gotArgs = name, args
			for i, a := range args {
				if a == "--channels-file" && i+1 < len(args) {
					b, err := os.ReadFile(args[i+1]) // still present during the call
					require.NoError(t, err)
					fileContent = b
				}
			}
			return "applied 2 channel(s)\n", nil
		},
	}

	err := c.ApplyChannels(context.Background(), []ChannelWrite{
		{Index: 0, Name: "primary"}, // zero key: default
		{Index: 1, Name: "ops", Key: secret.New(rawKey), UplinkEnabled: true}, // explicit key
	})
	require.NoError(t, err)

	assert.Equal(t, "mesh-apply.py", gotName)
	joined := strings.Join(gotArgs, " ")
	assert.Contains(t, joined, "--host 127.0.0.1:14403", "the TCP connection flag is passed")
	assert.Contains(t, joined, "--channels-file", "the file path is passed, not the keys")
	assert.NotContains(t, joined, keyB64, "the key never appears on the command line")
	assert.NotContains(t, joined, rawKey, "the raw key never appears on the command line")

	var docs []map[string]any
	require.NoError(t, json.Unmarshal(fileContent, &docs))
	require.Len(t, docs, 2)
	assert.Equal(t, "default", docs[0]["psk"], "a zero key is the device default")
	assert.Equal(t, "base64:"+keyB64, docs[1]["psk"], "an explicit key is base64, in the file only")
	assert.Equal(t, true, docs[1]["uplinkEnabled"])
}

func TestApplyChannelsNoopAndMissingApplier(t *testing.T) {
	c := &CLIClient{Host: "h"}
	require.NoError(t, c.ApplyChannels(context.Background(), nil), "no channels is a no-op")
	err := c.ApplyChannels(context.Background(), []ChannelWrite{{Index: 1}})
	require.Error(t, err, "channels requested with no applier configured is an error, not a silent skip")
	assert.NotContains(t, err.Error(), "psk")
}

func TestApplyChannelsMapsUnreachableToRequeue(t *testing.T) {
	c := &CLIClient{
		Serial:  "COM3",
		Applier: []string{"mesh-apply.py"},
		execFn: func(context.Context, string, ...string) (string, error) {
			return "could not open port 'COM3': PermissionError", assert.AnError
		},
	}
	err := c.ApplyChannels(context.Background(), []ChannelWrite{{Index: 1, Name: "ops"}})
	assert.ErrorIs(t, err, ErrUnreachable, "a mid-reboot serial port maps to requeue, not a hard failure")
}

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

func TestRedactCLISecretsStripsPasswordLines(t *testing.T) {
	in := "Set lora.region to US\nSet mqtt.password to s3cret-never-log\nConnected\n"
	out := redactCLISecrets(in)
	assert.NotContains(t, out, "s3cret-never-log")
	assert.Contains(t, out, "Set lora.region to US")
	assert.Contains(t, out, "[REDACTED]")
}

func TestLooksUnreachableMatchesKnownCLIErrors(t *testing.T) {
	// Transport-agnostic connection failures count on either transport.
	assert.True(t, looksUnreachable("Error connecting to meshnode-sim:[Errno 110] Connection timed out", false))
	assert.True(t, looksUnreachable("Connection refused", false))
	assert.False(t, looksUnreachable("Set lora.region to US", false))

	// Serial reboot window: while a real board reboots after an apply it drops
	// off the USB bus, so the port cannot be opened. That is transient
	// unreachable (requeue), not a hard failure. Verified against a T-Deck.
	assert.True(t, looksUnreachable("could not open port 'COM3': PermissionError(13, 'Access is denied.', None, 5)", true))
	assert.True(t, looksUnreachable("[Errno 2] could not open port /dev/ttyACM0: No such file or directory", true))

	// The SAME strings on a TCP transport are a fatal helper or path error (a
	// missing helper, a permission problem), not a reboot, and must not be
	// swallowed as unreachable, or a misconfiguration would requeue forever.
	assert.False(t, looksUnreachable("FileNotFoundError: [Errno 2] No such file or directory: 'mesh-apply.py'", false),
		"a missing-helper error on TCP is fatal, not a transient reboot")
	assert.False(t, looksUnreachable("PermissionError: access is denied", false))

	// Post-apply TCP drop: the device reboots and RSTs the session. These used
	// to miss the matcher, so Apply returned a hard error and Converge re-applied.
	assert.True(t, looksUnreachable("ConnectionResetError: [Errno 104] Connection reset by peer", false))
	assert.True(t, looksUnreachable("BrokenPipeError: [Errno 32] Broken pipe", false))
	assert.True(t, looksUnreachable("OSError: [Errno 107] Transport endpoint is not connected", false))
}

func TestExecErrorUnreachableMapsDeadlineAndReset(t *testing.T) {
	dead, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	assert.True(t, execErrorUnreachable(dead, "", context.DeadlineExceeded, false),
		"a hung CLI killed by our deadline is a transient drop, not a hard failure")

	canceled, cancel2 := context.WithCancel(context.Background())
	cancel2()
	assert.False(t, execErrorUnreachable(canceled, "", context.Canceled, false),
		"a canceled parent (shutdown) must not be swallowed as unreachable")

	assert.True(t, execErrorUnreachable(context.Background(), "", errors.New("read tcp 127.0.0.1:4403: connection reset by peer"), false),
		"a RST with empty CLI output still maps: the phrase is often only in err.Error()")
	assert.False(t, execErrorUnreachable(context.Background(), "", errors.New("executable file not found in $PATH"), false),
		"a missing binary is a hard failure")
	assert.False(t, execErrorUnreachable(context.Background(), "", errors.New("fork/exec python3: no such file or directory"), false),
		"serial-only path phrases in err.Error() must not map on either transport")
}

func TestExporterMapsConnectionResetToUnreachable(t *testing.T) {
	c := &CLIClient{
		Host:     "h",
		Exporter: []string{"mesh-export.py"},
		execFn: func(context.Context, string, ...string) (string, error) {
			return "", errors.New("ConnectionResetError: [Errno 104] Connection reset by peer")
		},
	}
	_, err := c.ExportConfig(context.Background())
	assert.ErrorIs(t, err, ErrUnreachable)
}

func TestParseInfoFindsNodeID(t *testing.T) {
	out := `Owner: NephMesh (NM)
Nodes in mesh: { "!6e000001": { "num": 1845493761 } }`
	assert.Equal(t, "!6e000001", parseInfo(out).NodeID)
	assert.Empty(t, parseInfo("no node id here").NodeID)
}

func TestParseInfoPrefersMyInfoOverFirstNeighbor(t *testing.T) {
	out := `Owner: NephMesh (NM)
My info: { "user": { "id": "!aabbccdd" }, "deviceMetrics": { "channelUtilization": 4.0, "airUtilTx": 1.0 } }
Nodes in mesh: { "!01020304": { "deviceMetrics": { "channelUtilization": 40.0, "airUtilTx": 20.0 } }, "!aabbccdd": { "deviceMetrics": { "channelUtilization": 4.0, "airUtilTx": 1.0 } } }`
	info := parseInfo(out)
	assert.Equal(t, "!aabbccdd", info.NodeID, "identity comes from My info, not the first neighbor")
	require.NotNil(t, info.ChannelUtilization)
	assert.InDelta(t, 4.0, *info.ChannelUtilization, 1e-9, "metrics come from the local node, not the neighbor")
}

func TestParseInfoMetricsFromLocalNodesEntry(t *testing.T) {
	// My info has the id but no metrics; a neighbor is listed first in Nodes.
	out := `My info: { "user": { "id": "!aabbccdd" } }
Nodes in mesh: { "!01020304": { "deviceMetrics": { "channelUtilization": 40.0, "airUtilTx": 20.0 } }, "!aabbccdd": { "deviceMetrics": { "channelUtilization": 4.0, "airUtilTx": 1.0 } } }`
	info := parseInfo(out)
	assert.Equal(t, "!aabbccdd", info.NodeID)
	require.NotNil(t, info.ChannelUtilization)
	assert.InDelta(t, 4.0, *info.ChannelUtilization, 1e-9)
}

func TestParseInfoExtractsAirtimeMetrics(t *testing.T) {
	// Shape from a real --info deviceMetrics block. Synthetic node id.
	out := `Nodes in mesh: { "!01020304": { "deviceMetrics": { "channelUtilization": 12.5, "airUtilTx": 3.25 } } }`
	info := parseInfo(out)
	require.NotNil(t, info.AirUtilTx)
	assert.InDelta(t, 3.25, *info.AirUtilTx, 1e-9)
	require.NotNil(t, info.ChannelUtilization)
	assert.InDelta(t, 12.5, *info.ChannelUtilization, 1e-9)

	// Absent metrics stay nil, distinct from a real 0.0 on an idle node.
	assert.Nil(t, parseInfo("no metrics here").AirUtilTx)
}

func TestCLIClientAppliesDefaultTimeout(t *testing.T) {
	var sawDeadline bool
	c := &CLIClient{Host: "h", Timeout: 10 * time.Millisecond, runFn: func(ctx context.Context, _ ...string) (string, error) {
		_, sawDeadline = ctx.Deadline()
		return "", nil
	}}
	_, err := c.Info(context.Background())
	require.NoError(t, err)
	assert.True(t, sawDeadline, "a CLI call with no parent deadline must get one")
}

func TestCLIClientKeepsParentDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	var childDeadline time.Time
	c := &CLIClient{Host: "h", Timeout: 10 * time.Millisecond, runFn: func(ctx context.Context, _ ...string) (string, error) {
		d, ok := ctx.Deadline()
		require.True(t, ok)
		childDeadline = d
		return "", nil
	}}
	_, err := c.Info(parent)
	require.NoError(t, err)
	want, _ := parent.Deadline()
	assert.Equal(t, want, childDeadline, "a parent deadline is not replaced by a shorter default")
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
