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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CLIClient drives a device over TCP by shelling out to the Meshtastic CLI,
// the same tool the Phase 1 applier used. It is intended to run alongside the
// controller as a sidecar or in an image that carries the pinned CLI, so the
// controller stays pure Go and the flaky, reboot-prone link is isolated to this
// one component. Its parsing is unit tested; the exec paths are exercised by
// the hardware and testcontainers integration tests, not by unit tests.
type CLIClient struct {
	// Host is the device address passed to `meshtastic --host`.
	Host string
	// Bin is the CLI binary name; defaults to "meshtastic".
	Bin string
	// runFn executes the CLI and returns its combined output. It defaults to
	// the real os/exec path; tests inject a stub so the command orchestration
	// (argument shaping, parsing, error mapping) is unit tested without the
	// binary. The exec path itself is covered by integration tests.
	runFn func(ctx context.Context, args ...string) (string, error)
}

const exportMarker = "# start of Meshtastic configure yaml"

func (c *CLIClient) bin() string {
	if c.Bin == "" {
		return "meshtastic"
	}
	return c.Bin
}

// looksUnreachable reports whether CLI output indicates the device could not be
// reached, so the caller can requeue rather than treat it as a hard failure.
func looksUnreachable(output string) bool {
	o := strings.ToLower(output)
	return strings.Contains(o, "timed out") ||
		strings.Contains(o, "connection refused") ||
		strings.Contains(o, "error connecting") ||
		strings.Contains(o, "no route to host")
}

func (c *CLIClient) run(ctx context.Context, args ...string) (string, error) {
	if c.runFn != nil {
		return c.runFn(ctx, args...)
	}
	return c.execRun(ctx, args...)
}

func (c *CLIClient) execRun(ctx context.Context, args ...string) (string, error) {
	full := append([]string{"--host", c.Host}, args...)
	cmd := exec.CommandContext(ctx, c.bin(), full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if looksUnreachable(string(out)) {
			return string(out), ErrUnreachable
		}
		return string(out), fmt.Errorf("%s %s: %w: %s", c.bin(), strings.Join(full, " "), err, strings.TrimSpace(string(out)))
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

// ExportConfig runs `meshtastic --export-config` and parses the result.
func (c *CLIClient) ExportConfig(ctx context.Context) (map[string]any, error) {
	out, err := c.run(ctx, "--export-config")
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
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_, err = c.run(ctx, "--configure", filepath.Clean(f.Name()))
	return err
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
	for _, field := range strings.Fields(out) {
		token := strings.Trim(field, `"',:{}[]`)
		if strings.HasPrefix(token, "!") && len(token) == 9 {
			return Info{NodeID: token}
		}
	}
	return Info{}
}
