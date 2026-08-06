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
// against a live meshtasticd (sim or hardware) and prints each step's outcome.
// It is a smoke and demonstration tool, not part of the operator image: it lets
// you watch the export, diff, apply-only-drift, reboot, and re-verify sequence
// converge without standing up a cluster. Point it at a device with -host (or
// the MESH_TEST_HOST env var) and make sure the meshtastic CLI is on PATH.
//
//	go run ./cmd/reconcile-demo -host 127.0.0.1:14403
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/config"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/reconcile"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/secret"
)

func main() {
	host := flag.String("host", os.Getenv("MESH_TEST_HOST"), "device address passed to meshtastic --host")
	flag.Parse()
	if *host == "" {
		fmt.Fprintln(os.Stderr, "set -host or MESH_TEST_HOST to a reachable meshtasticd")
		os.Exit(2)
	}

	// A real MeshtasticNode intent, built by the same config builder the
	// controller uses (no broker password in this demo).
	// A non-default modem preset: Meshtastic's export omits fields left at the
	// device default (LONG_FAST), which would read as permanent drift, so this
	// demo declares a value the device actually reports back.
	spec := meshv1alpha1.MeshtasticNodeSpec{
		Region:      "US",
		ModemPreset: "MEDIUM_SLOW",
		Owner:       &meshv1alpha1.OwnerSpec{LongName: "NephMesh Field 01", ShortName: "NF01"},
	}
	desired := config.BuildDesired(spec, "", secret.Value{})

	dev := &device.CLIClient{Host: *host}
	ctx := context.Background()

	fmt.Printf("nephmesh reconcile-demo: driving meshtasticd at %s\n", *host)
	fmt.Printf("desired intent: region=%s modemPreset=%s owner=%q\n\n",
		spec.Region, spec.ModemPreset, spec.Owner.LongName)

	state := reconcile.State{}
	for step := 1; step <= 15; step++ {
		out, err := reconcile.Converge(ctx, dev, desired, state)
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

		state = reconcile.State{RebootPending: out.RebootPending, ApplyAttempts: out.ApplyAttempts}
		if out.Ready {
			fmt.Printf("\nconverged: node %s, config in sync, Ready=true\n", out.Info.NodeID)
			return
		}
		time.Sleep(3 * time.Second)
	}

	fmt.Println("\ndid not converge within the step budget")
	os.Exit(1)
}
