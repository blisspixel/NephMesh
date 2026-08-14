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

// Command reconcile-demo runs the operator's real reconcile loop, the same
// Converge state machine and CLI-backed device client the controller uses,
// against a live meshtasticd or a real board (TCP via -host, or USB serial via
// -serial plus an -exporter) and prints each step's outcome. It lets you watch
// the export, diff, apply-only-drift, reboot, and re-verify sequence converge
// without a cluster.
//
//	go run ./cmd/reconcile-demo -host 127.0.0.1:14403
//	go run ./cmd/reconcile-demo -host 127.0.0.1:14403 -region US -role ROUTER
//	go run ./cmd/reconcile-demo -serial COM3 -exporter "python hack/mesh-export.py" -observe
//
// -observe reconciles an empty intent, so it reads the device but never
// modifies it: safe to point at a real, in-use board.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/airtime"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/config"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/reconcile"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/secret"
)

// keyNote describes a channel key for display without printing it.
func keyNote(key string) string {
	if key == "" {
		return "device default"
	}
	return "set (from -channel-key, not shown)"
}

// pct formats an optional percentage metric, or "n/a" when absent.
func pct(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", *v)
}

// dig walks a nested map[string]any, returning nil if any key is missing.
func dig(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func main() {
	host := flag.String("host", os.Getenv("MESH_TEST_HOST"), "device TCP address for meshtastic --host")
	serial := flag.String("serial", "", "USB serial port (e.g. COM3 or /dev/ttyACM0)")
	bin := flag.String("bin", os.Getenv("MESH_BIN"), "meshtastic CLI binary (default: meshtastic on PATH)")
	exporter := flag.String("exporter", "", `argv of the config exporter (e.g. "python hack/mesh-export.py"), required for -serial`)
	observe := flag.Bool("observe", false, "read-only: reconcile an empty intent so the device is never modified")
	region := flag.String("region", "", "desired region (apply mode)")
	preset := flag.String("preset", "", "desired modem preset (apply mode)")
	role := flag.String("role", "", "desired device role (apply mode), for example ROUTER or CLIENT")
	owner := flag.String("owner", "", "desired owner long name (apply mode)")
	ownerShort := flag.String("owner-short", "", "desired owner short name (apply mode)")
	applier := flag.String("applier", "", `argv of the channel-apply helper (e.g. "python hack/mesh-apply.py"), required to reconcile a channel`)
	channelName := flag.String("channel-name", "", "reconcile a secondary channel with this name (apply mode)")
	channelKey := flag.String("channel-key", "", "raw pre-shared key for -channel-name (empty uses the device default key)")
	channelIndex := flag.Int("channel-index", 1, "channel slot index for -channel-name")
	flag.Parse()

	if *host == "" && *serial == "" {
		fmt.Fprintln(os.Stderr, "set -host (TCP) or -serial (USB) to a reachable Meshtastic device")
		os.Exit(2)
	}

	dev := &device.CLIClient{Host: *host, Serial: *serial, Bin: *bin}
	if *exporter != "" {
		dev.Exporter = strings.Fields(*exporter)
	}
	if *serial != "" && len(dev.Exporter) == 0 {
		fmt.Fprintln(os.Stderr, "serial mode needs -exporter: the CLI's --export-config hangs over serial")
		os.Exit(2)
	}

	target := *host
	if *serial != "" {
		target = *serial
	}
	ctx := context.Background()

	var desired map[string]any
	var chans reconcile.DesiredChannels
	if *observe {
		// An empty intent converges immediately with no apply, so a real board
		// is never modified. Export first to show its current config.
		desired = map[string]any{}
		live, err := dev.ExportConfig(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("nephmesh reconcile-demo (observe, read-only): device at %s\n", target)
		fmt.Printf("live config read from device: region=%v role=%v owner=%v\n",
			dig(live, "config", "lora", "region"), dig(live, "config", "device", "role"), live["owner"])
		if preset, ok := dig(live, "config", "lora", "modemPreset").(string); ok {
			if toa, known := airtime.PresetTimeOnAir(preset, 40); known {
				fmt.Printf("airtime (predicted): %s is ~%.0f ms per 40-byte frame (~%.2f%% duty at 1 frame/min)\n",
					preset, float64(toa.Microseconds())/1000, airtime.DutyCyclePercent(toa, time.Minute))
			}
		}
		if info, ierr := dev.Info(ctx); ierr == nil && (info.AirUtilTx != nil || info.ChannelUtilization != nil) {
			fmt.Printf("airtime (measured by the radio): airUtilTx=%s channelUtilization=%s\n",
				pct(info.AirUtilTx), pct(info.ChannelUtilization))
		}
		fmt.Println()
	} else {
		spec := applySpec(*region, *preset, *role, *owner, *ownerShort)
		desired = config.BuildDesired(spec, "", secret.Value{})
		fmt.Printf("nephmesh reconcile-demo: driving Meshtastic device at %s\n", target)
		fmt.Printf("desired intent: %v\n", desired)

		if *channelName != "" {
			if *applier == "" {
				fmt.Fprintln(os.Stderr, "reconciling a channel needs -applier: keys are written through a file, not the command line")
				os.Exit(2)
			}
			dev.Applier = strings.Fields(*applier)
			var key secret.Value
			raw := config.DefaultPSKShorthand
			if *channelKey != "" {
				raw = []byte(*channelKey)
				if err := config.ValidChannelPSK(raw); err != nil {
					fmt.Fprintf(os.Stderr, "channel key: %v\n", err)
					os.Exit(2)
				}
				if config.IsDefaultPSK(raw) {
					raw = config.DefaultPSKShorthand
				} else {
					key = secret.New(*channelKey)
				}
			}
			chans = reconcile.DesiredChannels{
				Compare: []config.ChannelState{{Index: int32(*channelIndex), Name: *channelName, PSKHash: config.PSKHash(raw)}},
				Write:   []device.ChannelWrite{{Index: int32(*channelIndex), Name: *channelName, Key: key}},
			}
			fmt.Printf("desired channel: index=%d name=%q key=%s\n", *channelIndex, *channelName, keyNote(*channelKey))
		}
		fmt.Println()
	}

	state := reconcile.State{}
	for step := 1; step <= 15; step++ {
		out, err := reconcile.Converge(ctx, dev, desired, chans, state)
		if err != nil {
			fmt.Printf("  step %-2d error: %v\n", step, err)
			os.Exit(1)
		}

		note := ""
		switch {
		case out.RebootPending && !state.RebootPending:
			note = "  <- applied drift, device rebooting"
		case !out.Reachable:
			note = "  <- device rebooting, will re-verify"
		case out.Ready:
			note = "  <- converged"
		}
		fmt.Printf("  step %-2d reachable=%-5t inSync=%-5t rebootPending=%-5t ready=%-5t%s\n",
			step, out.Reachable, out.ConfigInSync, out.RebootPending, out.Ready, note)

		state = out.NextState()
		if out.Ready {
			fmt.Printf("\nconverged: node %s, config in sync, Ready=true\n", out.Info.NodeID)
			for _, ch := range chans.Compare {
				for _, live := range out.LiveChannels {
					if live.Index == ch.Index && live.Name == ch.Name && live.PSKHash == ch.PSKHash {
						fmt.Printf("secure channel provisioned: index=%d name=%q, key hash matches the device\n", ch.Index, ch.Name)
					}
				}
			}
			return
		}
		time.Sleep(3 * time.Second)
	}

	fmt.Println("\ndid not converge within the step budget")
	os.Exit(1)
}
