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

// Command nephmeshctl is the agent- and human-facing CLI for NephMesh. Its first
// subcommand, plan, dry-runs a CommunicationIntent through the report-only
// compiler and prints the verdict (feasibility, selected preset, fleet airtime,
// and the proposed MeshtasticNode specs) as JSON or text. It needs no cluster and
// no hardware: the compiler is pure, so an agent (Claude Code, Codex, a local
// LLM) or a person can iterate on a mesh design and get grounded, deterministic
// feedback before anything is applied. The JSON output is the same contract the
// nephmesh-mcp server exposes as an MCP tool; both wrap internal/plan.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/advisor"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/airtime"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/plan"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/spectrum"
)

// exit codes: 0 success, 2 usage or input error. A compiled verdict of
// infeasible or over-budget is still a successful evaluation (exit 0); the
// verdict is in the output, so scripts branch on the payload, not the exit code.
const (
	exitOK    = 0
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// Diagnostic writes go to injected writers (for testability), whose errors are
// unactionable in a CLI; these helpers make that explicit and keep errcheck
// satisfied without scattering blank assignments.
func fprintf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
func fprintln(w io.Writer, a ...any)               { _, _ = fmt.Fprintln(w, a...) }
func fprint(w io.Writer, a ...any)                 { _, _ = fmt.Fprint(w, a...) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "plan":
		return runPlan(args[1:], stdin, stdout, stderr)
	case "spectrum":
		return runSpectrum(args[1:], stdin, stdout, stderr)
	case "advise":
		return runAdvise(args[1:], stdin, stdout, stderr)
	case "-h", "--help", "help":
		usage(stdout)
		return exitOK
	default:
		fprintf(stderr, "unknown command %q\n", args[0])
		usage(stderr)
		return exitUsage
	}
}

func runPlan(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("f", "-", "CommunicationIntent file to read (YAML or JSON); - is stdin")
	format := fs.String("o", "json", "output format: json or text")
	fs.Usage = func() {
		fprintln(stderr, "usage: nephmeshctl plan [-f file] [-o json|text]")
		fprintln(stderr, "  Dry-run a CommunicationIntent through the report-only compiler.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if *format != "json" && *format != "text" {
		fprintf(stderr, "invalid -o %q: want json or text\n", *format)
		return exitUsage
	}

	data, err := readInput(*file, stdin)
	if err != nil {
		fprintf(stderr, "read intent: %v\n", err)
		return exitUsage
	}

	out, err := plan.Run(data)
	if err != nil {
		fprintf(stderr, "plan: %v\n", err)
		return exitUsage
	}

	if *format == "text" {
		fprint(stdout, out.Text())
		return exitOK
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fprintf(stderr, "encode: %v\n", err)
		return exitUsage
	}
	return exitOK
}

func runSpectrum(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("spectrum", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("f", "-", "rtl_power/hackrf_sweep CSV to read; - is stdin")
	format := fs.String("o", "json", "output format: json or text")
	margin := fs.Float64("margin-db", spectrum.DefaultOptions().ThresholdMarginDB, "dB above the noise floor a bin must be to count as occupied")
	percentile := fs.Float64("noise-percentile", spectrum.DefaultOptions().NoiseFloorPercentile, "per-band percentile taken as the noise floor (0..100)")
	fs.Usage = func() {
		fprintln(stderr, "usage: nephmeshctl spectrum [-f file] [-o json|text] [-margin-db N] [-noise-percentile N]")
		fprintln(stderr, "  Reduce a receive-only SDR sweep to per-band occupancy. See docs/guides/spectrum-validation.md.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if *format != "json" && *format != "text" {
		fprintf(stderr, "invalid -o %q: want json or text\n", *format)
		return exitUsage
	}

	data, err := readInput(*file, stdin)
	if err != nil {
		fprintf(stderr, "read sweep: %v\n", err)
		return exitUsage
	}
	opts := spectrum.Options{ThresholdMarginDB: *margin, NoiseFloorPercentile: *percentile}
	stats, err := spectrum.Sense(bytes.NewReader(data), spectrum.DefaultBands(), opts)
	if err != nil {
		fprintf(stderr, "spectrum: %v\n", err)
		return exitUsage
	}

	if *format == "text" {
		fprint(stdout, spectrum.RenderText(stats))
		return exitOK
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(stats); err != nil {
		fprintf(stderr, "encode: %v\n", err)
		return exitUsage
	}
	return exitOK
}

func runAdvise(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("advise", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("f", "-", "rtl_power/hackrf_sweep CSV to reason over; - is stdin")
	bandName := fs.String("band", "ism-915-us", "band to advise on")
	ollamaURL := fs.String("ollama-url", "http://localhost:11434", "local Ollama server")
	model := fs.String("model", "llama3.2:3b", "Ollama model")
	preset := fs.String("preset", "LONG_FAST", "current modem preset")
	region := fs.String("region", "US", "region")
	approved := fs.String("approved", "LONG_FAST,MEDIUM_SLOW,LONG_MODERATE", "comma-separated approved presets")
	numGPU := fs.Int("num-gpu", -1, "GPU layers for Ollama (0 forces CPU for a capable model on a memory-tight edge host; -1 lets Ollama decide)")
	format := fs.String("o", "text", "output format: json or text")
	fs.Usage = func() {
		fprintln(stderr, "usage: nephmeshctl advise [-f file] [-ollama-url URL] [-model M] [-preset P] [-approved list]")
		fprintln(stderr, "  Ask a local LLM to reason over sensed spectrum and propose a report-only action.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if *format != "json" && *format != "text" {
		fprintf(stderr, "invalid -o %q: want json or text\n", *format)
		return exitUsage
	}

	data, err := readInput(*file, stdin)
	if err != nil {
		fprintf(stderr, "read sweep: %v\n", err)
		return exitUsage
	}

	// Locate the target band, then build the sensed situation: occupancy and peak
	// from the aggregate reduction, and classified emissions from the time series.
	var band spectrum.Band
	found := false
	for _, b := range spectrum.DefaultBands() {
		if b.Name == *bandName {
			band, found = b, true
		}
	}
	if !found {
		fprintf(stderr, "unknown band %q\n", *bandName)
		return exitUsage
	}
	stats := spectrum.Analyze(mustBins(bytes.NewReader(data)), []spectrum.Band{band}, spectrum.DefaultOptions())
	series, serr := spectrum.ParseSweepSeries(bytes.NewReader(data))
	if serr != nil {
		fprintf(stderr, "advise: %v\n", serr)
		return exitUsage
	}
	emissions := spectrum.Classify(series, band, spectrum.DefaultClassifyOptions())

	sit := advisor.Situation{
		Band:                  stats[0],
		Emissions:             emissions,
		CurrentPreset:         *preset,
		Region:                *region,
		ApprovedPresets:       splitCSV(*approved),
		ChannelCeilingPercent: airtime.RecommendedChannelUtilizationPercent,
	}
	client := advisor.NewOllama(*ollamaURL, *model)
	client.NumGPU = *numGPU
	rec, raw, err := advisor.New(client).Advise(context.Background(), sit)
	if err != nil {
		fprintf(stderr, "advise: %v\n", err)
		return exitUsage
	}

	if *format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rec); err != nil {
			fprintf(stderr, "encode: %v\n", err)
			return exitUsage
		}
		return exitOK
	}
	fprintf(stdout, "recommendation: %s", rec.Action)
	if rec.TargetPreset != "" {
		fprintf(stdout, " -> %s", rec.TargetPreset)
	}
	fprintf(stdout, " (confidence %s)\n  rationale: %s\n", rec.Confidence, rec.Rationale)
	_ = raw
	return exitOK
}

// mustBins parses a sweep, returning nil bins on error (Analyze handles empty).
func mustBins(r io.Reader) []spectrum.Bin {
	bins, err := spectrum.ParseSweep(r)
	if err != nil {
		return nil
	}
	return bins
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// readInput reads the intent from a file, or from stdin when the path is "-".
func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path)
}

func usage(w io.Writer) {
	fprintln(w, "nephmeshctl: the NephMesh command-line interface")
	fprintln(w, "")
	fprintln(w, "usage:")
	fprintln(w, "  nephmeshctl plan [-f file] [-o json|text]       dry-run a CommunicationIntent")
	fprintln(w, "  nephmeshctl spectrum [-f file] [-o json|text]   reduce an SDR sweep to per-band occupancy")
	fprintln(w, "  nephmeshctl advise [-f file] [-model M]          ask a local LLM to propose a report-only action")
	fprintln(w, "")
	fprintln(w, "plan reads a CommunicationIntent (the same document you would apply to a")
	fprintln(w, "cluster) and reports feasibility, the selected preset, fleet airtime, and the")
	fprintln(w, "proposed MeshtasticNode specs.")
	fprintln(w, "")
	fprintln(w, "spectrum reads a receive-only rtl_power/hackrf_sweep CSV and reports per-band")
	fprintln(w, "occupancy, noise floor, and peak power. Both need no cluster and no hardware.")
}
