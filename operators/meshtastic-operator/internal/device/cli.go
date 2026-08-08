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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/secret"
)

// CLIClient drives a device over TCP by shelling out to the Meshtastic CLI,
// the same tool the Phase 1 applier used. It is intended to run alongside the
// controller as a sidecar or in an image that carries the pinned CLI, so the
// controller stays pure Go and the flaky, reboot-prone link is isolated to this
// one component. Its parsing is unit tested; the exec paths are exercised by
// the hardware and testcontainers integration tests, not by unit tests.
type CLIClient struct {
	// Host is the device address passed to `meshtastic --host` (TCP transport).
	Host string
	// Serial, when set, selects the USB serial transport (`meshtastic --port`)
	// instead of TCP. Exactly one of Host or Serial is used.
	Serial string
	// Bin is the CLI binary name; defaults to "meshtastic".
	Bin string
	// Exporter, when set, is the argv of a config exporter run instead of the
	// CLI's `--export-config`, which re-requests config over admin messages and
	// hangs on some devices over serial. The connection flag (--serial/--host)
	// is appended. The bundled hack/mesh-export.py reads the config that streams
	// on connect. Empty means use the CLI's own --export-config (TCP default).
	Exporter []string
	// Applier, when set, is the argv of the channel-apply helper (the bundled
	// hack/mesh-apply.py), which writes channels through a file so keys never
	// reach the command line. The connection flag and `--channels-file <path>`
	// are appended. Empty means channel apply is unavailable, and ApplyChannels
	// reports that rather than leaking a key onto the command line.
	Applier []string
	// runFn executes the CLI and returns its combined output. It defaults to
	// the real os/exec path; tests inject a stub so the command orchestration
	// (argument shaping, parsing, error mapping) is unit tested without the
	// binary. The exec path itself is covered by integration tests.
	runFn func(ctx context.Context, args ...string) (string, error)
	// execFn executes an arbitrary binary (the exporter), injectable for tests.
	execFn func(ctx context.Context, name string, args ...string) (string, error)
}

const exportMarker = "# start of Meshtastic configure yaml"

func (c *CLIClient) bin() string {
	if c.Bin == "" {
		return "meshtastic"
	}
	return c.Bin
}

// looksUnreachable reports whether CLI output indicates the device could not be
// reached, so the caller can requeue rather than treat it as a hard failure. The
// serial parameter matters: the port-open phrases below are a transient reboot
// signal only on a serial transport; on TCP the same strings come from a fatal
// helper or path error (a missing file, a permission problem) that must NOT be
// swallowed as unreachable, or a misconfiguration would requeue forever and never
// surface or degrade.
func looksUnreachable(output string, serial bool) bool {
	o := strings.ToLower(output)
	// Transport-agnostic connection failures.
	if strings.Contains(o, "timed out") ||
		strings.Contains(o, "connection refused") ||
		strings.Contains(o, "error connecting") ||
		strings.Contains(o, "no route to host") {
		return true
	}
	if !serial {
		return false
	}
	// Serial only: while a device reboots after an apply it drops off the USB bus
	// and the port briefly cannot be opened. That is a transient unreachable
	// window (the loop should requeue), not a hard failure.
	return strings.Contains(o, "could not open port") ||
		strings.Contains(o, "permissionerror") ||
		strings.Contains(o, "access is denied") ||
		strings.Contains(o, "no such file or directory") ||
		strings.Contains(o, "device disconnected")
}

// connArgs is the CLI connection flag for the configured transport: --port for
// serial, --host for TCP.
func (c *CLIClient) connArgs() []string {
	if c.Serial != "" {
		return []string{"--port", c.Serial}
	}
	return []string{"--host", c.Host}
}

func (c *CLIClient) run(ctx context.Context, args ...string) (string, error) {
	if c.runFn != nil {
		return c.runFn(ctx, args...)
	}
	return c.execRun(ctx, args...)
}

func (c *CLIClient) execRun(ctx context.Context, args ...string) (string, error) {
	full := append(c.connArgs(), args...)
	cmd := exec.CommandContext(ctx, c.bin(), full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if looksUnreachable(string(out), c.Serial != "") {
			return string(out), ErrUnreachable
		}
		return string(out), fmt.Errorf("%s %s: %w: %s", c.bin(), strings.Join(full, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// runExporter runs the configured exporter with the transport connection flag,
// used in place of the CLI's --export-config where that command hangs.
func (c *CLIClient) runExporter(ctx context.Context) (string, error) {
	conn := "--host"
	value := c.Host
	if c.Serial != "" {
		conn, value = "--serial", c.Serial
	}
	args := append(append([]string(nil), c.Exporter[1:]...), conn, value)
	if c.execFn != nil {
		return c.execFn(ctx, c.Exporter[0], args...)
	}
	cmd := exec.CommandContext(ctx, c.Exporter[0], args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if looksUnreachable(string(out), c.Serial != "") {
			return string(out), ErrUnreachable
		}
		return string(out), fmt.Errorf("%s: %w: %s", c.Exporter[0], err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// parseExportConfig extracts the YAML configuration document from CLI output,
// which prefaces it with a marker line and may print connection chatter before
// it.
func parseExportConfig(out string) (map[string]any, error) {
	if i := strings.Index(out, exportMarker); i >= 0 {
		out = out[i:]
	}
	var cfg map[string]any
	if err := yaml.Unmarshal([]byte(out), &cfg); err != nil {
		return nil, fmt.Errorf("parsing exported config: %w", err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg, nil
}

// ExportConfig returns the device's live config as a map. It uses the
// configured exporter when set (required for serial, where the CLI's
// --export-config hangs), otherwise the CLI's own --export-config over TCP.
func (c *CLIClient) ExportConfig(ctx context.Context) (map[string]any, error) {
	var (
		out string
		err error
	)
	if len(c.Exporter) > 0 {
		out, err = c.runExporter(ctx)
	} else {
		out, err = c.run(ctx, "--export-config")
	}
	if err != nil {
		return nil, err
	}
	return parseExportConfig(out)
}

// Apply writes the desired config to a temporary file and runs
// `meshtastic --configure`. The device reboots as a result.
func (c *CLIClient) Apply(ctx context.Context, desired map[string]any) error {
	data, err := yaml.Marshal(desired)
	if err != nil {
		return fmt.Errorf("marshaling desired config: %w", err)
	}
	f, err := os.CreateTemp("", "nephmesh-configure-*.yaml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_, err = c.run(ctx, "--configure", filepath.Clean(f.Name()))
	return err
}

// ApplyChannels writes the channels to the device through the apply helper. The
// keys are marshaled into a temporary 0600 file and the helper is given the
// file path, so a key never appears in a process argument or a log line (the
// same posture the scalar Apply gets by writing `--configure` to a file). The
// device reboots as a result.
func (c *CLIClient) ApplyChannels(ctx context.Context, channels []ChannelWrite) error {
	if len(channels) == 0 {
		return nil
	}
	if len(c.Applier) == 0 {
		return fmt.Errorf("channel apply requested but no applier is configured")
	}

	type chanDoc struct {
		Index           int32  `json:"index"`
		Name            string `json:"name"`
		PSK             string `json:"psk"`
		UplinkEnabled   bool   `json:"uplinkEnabled"`
		DownlinkEnabled bool   `json:"downlinkEnabled"`
	}
	docs := make([]chanDoc, 0, len(channels))
	for _, ch := range channels {
		// The key is revealed only here, into the file, never onto argv or into
		// an error. A zero key is the device default.
		docs = append(docs, chanDoc{
			Index: ch.Index, Name: ch.Name, PSK: pskDirective(ch.Key),
			UplinkEnabled: ch.UplinkEnabled, DownlinkEnabled: ch.DownlinkEnabled,
		})
	}
	data, err := json.Marshal(docs)
	if err != nil {
		return fmt.Errorf("marshaling channels: %w", err)
	}

	f, err := os.CreateTemp("", "nephmesh-channels-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err := os.Chmod(f.Name(), 0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return c.runApplier(ctx, f.Name())
}

// pskDirective renders a channel key as the directive the apply helper reads: a
// zero key is the public default, an explicit key is its raw bytes base64-encoded.
// The raw key is revealed only here.
func pskDirective(key secret.Value) string {
	if key.IsZero() {
		return "default"
	}
	return "base64:" + base64.StdEncoding.EncodeToString([]byte(key.Reveal()))
}

// runApplier runs the channel-apply helper with the connection flag and the
// channels file path. Any error reports the helper and its output, which by
// contract never contains key material, and the file path, never its contents.
func (c *CLIClient) runApplier(ctx context.Context, channelsFile string) error {
	conn := "--host"
	value := c.Host
	if c.Serial != "" {
		conn, value = "--serial", c.Serial
	}
	args := append(append([]string(nil), c.Applier[1:]...), conn, value, "--channels-file", channelsFile)
	var (
		out string
		err error
	)
	if c.execFn != nil {
		out, err = c.execFn(ctx, c.Applier[0], args...)
	} else {
		cmd := exec.CommandContext(ctx, c.Applier[0], args...)
		b, e := cmd.CombinedOutput()
		out, err = string(b), e
	}
	if err != nil {
		if looksUnreachable(out, c.Serial != "") {
			return ErrUnreachable
		}
		return fmt.Errorf("%s: applying channels: %w: %s", c.Applier[0], err, strings.TrimSpace(out))
	}
	return nil
}

// Reboot runs `meshtastic --reboot`.
func (c *CLIClient) Reboot(ctx context.Context) error {
	_, err := c.run(ctx, "--reboot")
	return err
}

// Info returns best-effort device identity. Identity is not load-bearing in the
// convergence loop, so a parse miss yields an empty Info rather than an error.
func (c *CLIClient) Info(ctx context.Context) (Info, error) {
	out, err := c.run(ctx, "--info")
	if err != nil {
		return Info{}, err
	}
	return parseInfo(out), nil
}

// parseInfo pulls the node id token (for example "!6e000001") from --info
// output. It is intentionally forgiving.
func parseInfo(out string) Info {
	var info Info
	for _, field := range strings.Fields(out) {
		token := strings.Trim(field, `"',:{}[]`)
		if strings.HasPrefix(token, "!") && len(token) == 9 {
			info.NodeID = token
			break
		}
	}
	// Airtime telemetry from deviceMetrics (best effort: the first reported
	// value, which is the local node on a bench device).
	info.AirUtilTx = parseFloatMetric(out, "airUtilTx")
	info.ChannelUtilization = parseFloatMetric(out, "channelUtilization")
	return info
}

// parseFloatMetric pulls a numeric JSON field value out of --info output.
func parseFloatMetric(out, key string) *float64 {
	m := regexp.MustCompile(`"` + key + `"\s*:\s*(-?[0-9]+(?:\.[0-9]+)?)`).FindStringSubmatch(out)
	if m == nil {
		return nil
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	return &v
}
