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
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/advisor"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/airtime"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/meshframe"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/plan"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/resilience"
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
	case "decode":
		return runDecode(args[1:], stdin, stdout, stderr)
	case "resilience":
		return runResilience(args[1:], stdin, stdout, stderr)
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

// runDecode reads decoded LoRa payloads as hex (one packet per line, as the SDR
// decoder emits them) and prints each packet's clear-text Meshtastic header: who
// sent it, to whom, on which channel. This is the out-of-band witness readout,
// the sender read straight off the air, independent of any node's self-report.
func runDecode(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("decode", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("f", "-", "file of hex LoRa payloads, one per line; - is stdin")
	format := fs.String("o", "text", "output format: json or text")
	fs.Usage = func() {
		fprintln(stderr, "usage: nephmeshctl decode [-f file] [-o json|text]")
		fprintln(stderr, "  Parse Meshtastic packet headers from decoded LoRa payloads (hex, one per line).")
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
		fprintf(stderr, "read: %v\n", err)
		return exitUsage
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	seen := 0
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Remove all internal whitespace (spaces and tabs) and an optional 0x/0X
		// prefix, so grouped, tab-separated, or prefixed hex all parse.
		hexStr := strings.Join(strings.Fields(trimmed), "")
		hexStr = strings.TrimPrefix(hexStr, "0x")
		hexStr = strings.TrimPrefix(hexStr, "0X")
		if hexStr == "" {
			continue
		}
		raw, derr := hex.DecodeString(hexStr)
		if derr != nil {
			fprintf(stderr, "skip unparsable hex: %v\n", derr)
			continue
		}
		h, perr := meshframe.ParseHeader(raw)
		if perr != nil {
			fprintf(stderr, "skip: %v\n", perr)
			continue
		}
		seen++
		if *format == "json" {
			if err := enc.Encode(h); err != nil {
				fprintf(stderr, "encode: %v\n", err)
				return exitUsage
			}
			continue
		}
		fprintf(stdout, "from %s to %s  id 0x%08x  hop %d/%d  channel 0x%02x%s\n",
			h.FromID(), h.ToID(), h.ID, h.HopLimit, h.HopStart, h.ChannelHash, ackStr(h))
	}
	if seen == 0 {
		fprintln(stderr, "no valid packets decoded")
		return exitUsage
	}
	return exitOK
}

func ackStr(h meshframe.Header) string {
	if h.WantAck {
		return "  wantAck"
	}
	return ""
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

func runResilience(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("resilience", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("f", "-", "delivery-probe event log (JSONL from demo/resilience/probe.py); - is stdin")
	at := fs.Float64("at", 0, "perturbation time, Unix seconds: before/after mode (messages before it vs after)")
	phasesCSV := fs.String("phases", "", "multi-phase mode: comma-separated boundary times (Unix seconds), ascending; used with -labels")
	labelsCSV := fs.String("labels", "baseline,degraded,adapted", "phase labels for -phases; count must be len(phases)+1")
	tolerance := fs.Float64("tolerance", 0.1, "largest delivery-ratio drop from baseline still counted as unchanged")
	receiversCSV := fs.String("receivers", "", "comma-separated receiver nodes; inferred from the log when empty")
	format := fs.String("o", "text", "output format: json or text")
	fs.Usage = func() {
		fprintln(stderr, "usage: nephmeshctl resilience (-at UNIXTIME | -phases T1,T2 -labels a,b,c) [-f file] [-tolerance F] [-receivers list] [-o json|text]")
		fprintln(stderr, "  Reduce a delivery-probe log to a verdict around a perturbation: -at for a")
		fprintln(stderr, "  before/after split, -phases for a baseline/degraded/adapted timeline. See demo/resilience.")
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
	usePhases := *phasesCSV != ""
	if usePhases && *at > 0 {
		fprintln(stderr, "use -at (before/after) or -phases (multi-phase), not both")
		return exitUsage
	}
	if !usePhases && *at <= 0 {
		fprintln(stderr, "set -at (before/after) or -phases (multi-phase timeline)")
		return exitUsage
	}

	data, err := readInput(*file, stdin)
	if err != nil {
		fprintf(stderr, "read probe log: %v\n", err)
		return exitUsage
	}
	events, err := resilience.ParseEvents(bytes.NewReader(data))
	if err != nil {
		fprintf(stderr, "parse probe log: %v\n", err)
		return exitUsage
	}
	var receivers []string
	if *receiversCSV != "" {
		receivers = splitCSV(*receiversCSV)
	}

	var payload any
	var text string
	if usePhases {
		boundaries := make([]float64, 0)
		for _, s := range splitCSV(*phasesCSV) {
			v, perr := strconv.ParseFloat(s, 64)
			if perr != nil {
				fprintf(stderr, "invalid -phases value %q: %v\n", s, perr)
				return exitUsage
			}
			boundaries = append(boundaries, v)
		}
		report := resilience.ReducePhases(events, boundaries, splitCSV(*labelsCSV), *tolerance, receivers)
		payload, text = report, report.Text()
	} else {
		report := resilience.Reduce(events, *at, *tolerance, receivers)
		payload, text = report, report.Text()
	}

	if *format == "text" {
		fprint(stdout, text)
		return exitOK
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		fprintf(stderr, "encode: %v\n", err)
		return exitUsage
	}
	return exitOK
}

func usage(w io.Writer) {
	fprintln(w, "nephmeshctl: the NephMesh command-line interface")
	fprintln(w, "")
	fprintln(w, "usage:")
	fprintln(w, "  nephmeshctl plan [-f file] [-o json|text]       dry-run a CommunicationIntent")
	fprintln(w, "  nephmeshctl spectrum [-f file] [-o json|text]   reduce an SDR sweep to per-band occupancy")
	fprintln(w, "  nephmeshctl advise [-f file] [-model M]          ask a local LLM to propose a report-only action")
	fprintln(w, "  nephmeshctl decode [-f file]                     read Meshtastic packet headers off decoded LoRa payloads")
	fprintln(w, "  nephmeshctl resilience -at T [-f file]           split a delivery-probe log at a perturbation, before vs after")
	fprintln(w, "")
	fprintln(w, "plan reads a CommunicationIntent (the same document you would apply to a")
	fprintln(w, "cluster) and reports feasibility, the selected preset, fleet airtime, and the")
	fprintln(w, "proposed MeshtasticNode specs.")
	fprintln(w, "")
	fprintln(w, "spectrum reads a receive-only rtl_power/hackrf_sweep CSV and reports per-band")
	fprintln(w, "occupancy, noise floor, and peak power. Both need no cluster and no hardware.")
}
