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

// Command spectrum-exporter is a receive-only SDR spectrum sensor that publishes
// per-band occupancy, peak power, peak frequency, and noise floor as Prometheus
// metrics. It runs hackrf_sweep (or any rtl_power-format sweep tool) on a loop,
// reduces each sweep with internal/spectrum, and serves /metrics, putting sensed
// spectrum on the same observability plane as the operator's node health. It
// never transmits: hackrf_sweep only receives.
//
// Run it on the host the SDR is attached to (the Linux USB host):
//
//	spectrum-exporter -bind :9808 -freq-min 902 -freq-max 928 -interval 15s
//	curl -s localhost:9808/metrics | grep nephmesh_spectrum
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/specexport"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/spectrum"
)

func main() {
	var (
		bind      = flag.String("bind", ":9808", "address to serve /metrics on")
		sweepCmd  = flag.String("sweep-cmd", "hackrf_sweep", "sweep binary (rtl_power/hackrf_sweep CSV on stdout)")
		freqMin   = flag.Int("freq-min", 902, "sweep start frequency, MHz")
		freqMax   = flag.Int("freq-max", 928, "sweep end frequency, MHz")
		binWidth  = flag.Int("bin-width", 1000000, "sweep bin width, Hz")
		interval  = flag.Duration("interval", 15*time.Second, "time between sweeps")
		sweepSecs = flag.Int("sweep-seconds", 3, "integrate each sweep for this many seconds (0 = single pass)")
		marginDB  = flag.Float64("margin-db", spectrum.DefaultOptions().ThresholdMarginDB, "dB above the noise floor a bin must be to count as occupied")
		noisePct  = flag.Float64("noise-percentile", spectrum.DefaultOptions().NoiseFloorPercentile, "per-band noise-floor percentile (0..100)")
		replay    = flag.String("replay", "", "read this sweep CSV file each interval instead of running the sweep tool (simulation-first: publish real metrics with no SDR attached)")
		once      = flag.Bool("once", false, "run a single sweep, print /metrics to stdout, and exit")
	)
	flag.Parse()

	opts := spectrum.Options{ThresholdMarginDB: *marginDB, NoiseFloorPercentile: *noisePct}
	bands := spectrum.DefaultBands()

	reg := prometheus.NewRegistry()
	pub := specexport.New(reg)

	// capture returns one sweep's CSV, either replayed from a recorded file
	// (simulation-first: a real metrics source with no radio, mirroring the mesh
	// gateway's --sim node) or read live from the sweep tool on a sensor host.
	capture := func() ([]byte, error) {
		if *replay != "" {
			return os.ReadFile(*replay)
		}
		return runSweep(*sweepCmd, *freqMin, *freqMax, *binWidth, *sweepSecs)
	}

	sweep := func() {
		data, err := capture()
		if err != nil || len(data) == 0 {
			log.Printf("sweep failed: %v (%d bytes)", err, len(data))
			pub.PublishError()
			return
		}
		stats, err := spectrum.Sense(bytes.NewReader(data), bands, opts)
		if err != nil {
			log.Printf("parse failed: %v", err)
			pub.PublishError()
			return
		}
		pub.Publish(stats, time.Now().Unix())
	}

	if *once {
		sweep()
		mfs, _ := reg.Gather()
		for _, mf := range mfs {
			fmt.Println(mf.GetName())
		}
		return
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	srv := &http.Server{Addr: *bind, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	source := fmt.Sprintf("sweeping %d-%d MHz", *freqMin, *freqMax)
	if *replay != "" {
		source = "replaying " + *replay
	}
	go func() {
		log.Printf("spectrum-exporter serving on %s, %s every %s (receive-only)", *bind, source, *interval)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	// Sweep immediately, then on the interval, forever.
	sweep()
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for range ticker.C {
		sweep()
	}
}

// runSweep executes the sweep tool and returns its CSV output. A non-zero
// integration window runs the tool for that long (via SIGINT, which hackrf_sweep
// handles cleanly) and keeps whatever it emitted; a zero window does a single
// pass. The exit error from the interrupt is expected and ignored: the captured
// stdout is the result.
func runSweep(cmdName string, freqMin, freqMax, binWidth, seconds int) ([]byte, error) {
	args := []string{"-f", fmt.Sprintf("%d:%d", freqMin, freqMax), "-w", strconv.Itoa(binWidth)}
	var buf bytes.Buffer

	if seconds <= 0 {
		cmd := exec.Command(cmdName, append(args, "-1")...) //nolint:gosec // args are numeric flags, not user strings
		cmd.Stdout = &buf
		cmd.Stderr = nil
		err := cmd.Run()
		return buf.Bytes(), err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cmdName, args...) //nolint:gosec // args are numeric flags, not user strings
	cmd.Stdout = &buf
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 3 * time.Second
	_ = cmd.Run() // the interrupt at the window's end returns an error; the data is what matters
	if buf.Len() == 0 {
		return nil, fmt.Errorf("no output from %s", cmdName)
	}
	return buf.Bytes(), nil
}
