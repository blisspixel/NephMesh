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
		fprint(stderr, rootUsage)
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
	case "version", "-V", "--version":
		fprintln(stdout, versionString())
		return exitOK
	case "-h", "--help", "help":
		fprint(stdout, rootUsage)
		return exitOK
	default:
		return cliError(stdout, stderr, "", "", fmt.Sprintf("unknown command %q", args[0]), "Run 'nephmeshctl -h'.", nil)
	}
}

func prepare(fs *flag.FlagSet, formatFlag *string, args []string, stdin io.Reader, stdout, stderr io.Writer, cmd string) (format, path string, code int, ok bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "", "", exitOK, false
		}
		return "", "", exitUsage, false
	}
	format, err := chooseFormat(*formatFlag, isTerminal(stdout))
	if err != nil {
		return "", "", cliError(stdout, stderr, "", cmd, err.Error(), "Pass -o json or -o text.", err), false
	}
	path, err = resolveInputPath(fs, fs.Lookup("f").Value.String(), isTerminal(stdin))
	if err != nil {
		return "", "", cliError(stdout, stderr, format, cmd, err.Error(), inputNextStep(err), err), false
	}
	return format, path, exitOK, true
}

func runPlan(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	_ = fs.String("f", "-", "CommunicationIntent path")
	formatFlag := fs.String("o", "", "json or text")
	fs.Usage = func() {
		commandHelp(stderr, "plan [-f file] [-o json|text]",
			"Dry-run a CommunicationIntent. Nothing is applied.",
			[]string{
				"-f file       CommunicationIntent path (default: stdin)",
				"-o json|text  text on a terminal, json when piped",
			})
	}
	format, path, code, ok := prepare(fs, formatFlag, args, stdin, stdout, stderr, "plan")
	if !ok {
		return code
	}
	data, err := readInput(path, stdin)
	if err != nil {
		return failRead(stdout, stderr, format, "plan", err)
	}
	out, err := plan.Run(data)
	if err != nil {
		next := "Check the file is one CommunicationIntent."
		if strings.Contains(err.Error(), "expected kind") {
			next = "Pass a CommunicationIntent, for example examples/regional-intent.yaml."
		}
		if strings.Contains(err.Error(), "multiple YAML documents") {
			next = "Pass one CommunicationIntent document."
		}
		if errors.Is(err, plan.ErrEmptyInput) {
			next = "Pass -f with a CommunicationIntent, or pipe one document."
		}
		return cliError(stdout, stderr, format, "plan", err.Error(), next, err)
	}
	return writeOut(stdout, stderr, format, out, colorizeReport(out.Text(), allowColor(isTerminal(stdout))))
}

func runSpectrum(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("spectrum", flag.ContinueOnError)
	fs.SetOutput(stderr)
	_ = fs.String("f", "-", "sweep CSV path")
	formatFlag := fs.String("o", "", "json or text")
	margin := fs.Float64("margin-db", spectrum.DefaultOptions().ThresholdMarginDB, "dB above the noise floor")
	percentile := fs.Float64("noise-percentile", spectrum.DefaultOptions().NoiseFloorPercentile, "noise-floor percentile")
	fs.Usage = func() {
		commandHelp(stderr, "spectrum [-f file] [-o json|text]",
			"Per-band occupancy from a receive-only sweep CSV.",
			[]string{
				"-f file              sweep CSV path (default: stdin)",
				"-o json|text         text on a terminal, json when piped",
				"-margin-db N         dB above the noise floor counts as occupied",
				"-noise-percentile N  noise-floor percentile, 0 to 100",
			})
	}
	format, path, code, ok := prepare(fs, formatFlag, args, stdin, stdout, stderr, "spectrum")
	if !ok {
		return code
	}
	data, err := readInput(path, stdin)
	if err != nil {
		return failRead(stdout, stderr, format, "spectrum", err)
	}
	opts := spectrum.Options{ThresholdMarginDB: *margin, NoiseFloorPercentile: *percentile}
	bins, err := spectrum.ParseSweep(bytes.NewReader(data))
	if err != nil {
		return cliError(stdout, stderr, format, "spectrum", err.Error(), "Pass an rtl_power or hackrf_sweep CSV.", err)
	}
	stats := spectrum.Analyze(bins, spectrum.DefaultBands(), opts)
	style := spectrum.TextStyle{ASCII: !unicodeOK(), ANSI: allowColor(isTerminal(stdout))}
	return writeOut(stdout, stderr, format, stats, spectrum.RenderText(stats, bins, style))
}

func runAdvise(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("advise", flag.ContinueOnError)
	fs.SetOutput(stderr)
	_ = fs.String("f", "-", "sweep CSV path")
	bandName := fs.String("band", "ism-915-us", "band to advise on")
	ollamaURL := fs.String("ollama-url", "http://localhost:11434", "local Ollama server")
	model := fs.String("model", "llama3.2:3b", "Ollama model")
	preset := fs.String("preset", "LONG_FAST", "current modem preset")
	region := fs.String("region", "US", "region")
	approved := fs.String("approved", "LONG_FAST,MEDIUM_SLOW,LONG_MODERATE", "approved presets")
	numGPU := fs.Int("num-gpu", -1, "GPU layers")
	formatFlag := fs.String("o", "", "json or text")
	fs.Usage = func() {
		commandHelp(stderr, "advise [-f file] [-model M]",
			"Ask a local model for a report-only action.",
			[]string{
				"-f file         sweep CSV (default: stdin)",
				"-band name      band to advise on (default ism-915-us)",
				"-ollama-url URL local Ollama server",
				"-model name     Ollama model (default llama3.2:3b)",
				"-preset name    current modem preset",
				"-region code    regulatory region",
				"-approved list  comma-separated approved presets",
				"-num-gpu N      GPU layers (0 is CPU, -1 lets Ollama decide)",
				"-o json|text    text on a terminal, json when piped",
			})
	}
	format, path, code, ok := prepare(fs, formatFlag, args, stdin, stdout, stderr, "advise")
	if !ok {
		return code
	}
	data, err := readInput(path, stdin)
	if err != nil {
		return failRead(stdout, stderr, format, "advise", err)
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
		return cliError(stdout, stderr, format, "advise", fmt.Sprintf("unknown band %q", *bandName), "Pass -band with one of ism-433, ism-868-eu, ism-915-us.", nil)
	}
	stats := spectrum.Analyze(mustBins(bytes.NewReader(data)), []spectrum.Band{band}, spectrum.DefaultOptions())
	series, serr := spectrum.ParseSweepSeries(bytes.NewReader(data))
	if serr != nil {
		return cliError(stdout, stderr, format, "advise", serr.Error(), "Pass an rtl_power or hackrf_sweep CSV.", serr)
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
	rec, _, err := advisor.New(client).Advise(context.Background(), sit)
	if err != nil {
		if isUnreachable(err) {
			return cliError(stdout, stderr, format, "advise",
				fmt.Sprintf("no Ollama server at %s", *ollamaURL),
				"Start it with 'ollama serve', or pass -ollama-url.", err)
		}
		return cliError(stdout, stderr, format, "advise", "the model request failed", "Check -model and -ollama-url.", err)
	}

	var text strings.Builder
	fmt.Fprintf(&text, "recommendation: %s", rec.Action)
	if rec.TargetPreset != "" {
		fmt.Fprintf(&text, " -> %s", rec.TargetPreset)
	}
	fmt.Fprintf(&text, " (confidence %s)\n  rationale: %s\n", rec.Confidence, rec.Rationale)
	return writeOut(stdout, stderr, format, rec, colorizeReport(text.String(), allowColor(isTerminal(stdout))))
}

// runDecode reads decoded LoRa payloads as hex (one packet per line, as the SDR
// decoder emits them) and prints each packet's clear-text Meshtastic header: who
// sent it, to whom, on which channel. This is the out-of-band witness readout,
// the sender read straight off the air, independent of any node's self-report.
func runDecode(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("decode", flag.ContinueOnError)
	fs.SetOutput(stderr)
	_ = fs.String("f", "-", "hex payload file")
	formatFlag := fs.String("o", "", "json or text")
	fs.Usage = func() {
		commandHelp(stderr, "decode [-f file] [-o json|text]",
			"Meshtastic headers from decoded LoRa payloads.",
			[]string{
				"-f file       hex payloads, one per line (default: stdin)",
				"-o json|text  text on a terminal, json when piped",
			})
	}
	format, path, code, ok := prepare(fs, formatFlag, args, stdin, stdout, stderr, "decode")
	if !ok {
		return code
	}
	data, err := readInput(path, stdin)
	if err != nil {
		return failRead(stdout, stderr, format, "decode", err)
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
		if format == "json" {
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
		return cliError(stdout, stderr, format, "decode", "no valid packets decoded", "Pass hex payloads, one per line.", nil)
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

func runResilience(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("resilience", flag.ContinueOnError)
	fs.SetOutput(stderr)
	_ = fs.String("f", "-", "probe log path")
	at := fs.Float64("at", 0, "perturbation time")
	phasesCSV := fs.String("phases", "", "boundary times")
	labelsCSV := fs.String("labels", "baseline,degraded,adapted", "phase labels")
	tolerance := fs.Float64("tolerance", 0.1, "unchanged drop")
	receiversCSV := fs.String("receivers", "", "receiver nodes")
	formatFlag := fs.String("o", "", "json or text")
	fs.Usage = func() {
		commandHelp(stderr, "resilience (-at T | -phases T,T)",
			"Delivery before and after a perturbation.",
			[]string{
				"-f file        probe log (default: stdin)",
				"-at SECONDS    before/after split, Unix seconds",
				"-phases T,T    ascending boundary times",
				"-labels A,B,C  phase names (default baseline,degraded,adapted)",
				"-tolerance F   delivery drop still counted as unchanged",
				"-receivers A,B names (default: probe summary, else observed)",
				"-o json|text   text on a terminal, json when piped",
			})
	}
	format, path, code, ok := prepare(fs, formatFlag, args, stdin, stdout, stderr, "resilience")
	if !ok {
		return code
	}
	usePhases := *phasesCSV != ""
	if usePhases && *at > 0 {
		return cliError(stdout, stderr, format, "resilience", "both -at and -phases were set", "Use one of them.", nil)
	}
	if !usePhases && *at <= 0 {
		return cliError(stdout, stderr, format, "resilience", "no split time was set", "Pass -at or -phases.", nil)
	}

	data, err := readInput(path, stdin)
	if err != nil {
		return failRead(stdout, stderr, format, "resilience", err)
	}
	events, declared, hasDeclared, err := resilience.ParseProbe(bytes.NewReader(data))
	if err != nil {
		return cliError(stdout, stderr, format, "resilience", "cannot parse the probe log", "Pass the probe's event log.", err)
	}
	var receivers []string
	switch {
	case *receiversCSV != "":
		receivers = splitCSV(*receiversCSV)
	case hasDeclared:
		receivers = declared
	}

	var payload any
	var text string
	if usePhases {
		boundaries := make([]float64, 0)
		for _, s := range splitCSV(*phasesCSV) {
			v, perr := strconv.ParseFloat(s, 64)
			if perr != nil {
				return cliError(stdout, stderr, format, "resilience", fmt.Sprintf("invalid -phases value %q", s), "Pass ascending Unix times, comma-separated.", perr)
			}
			boundaries = append(boundaries, v)
		}
		labels := splitCSV(*labelsCSV)
		if len(labels) != len(boundaries)+1 {
			return cliError(stdout, stderr, format, "resilience",
				fmt.Sprintf("-labels has %d entries but -phases has %d boundaries", len(labels), len(boundaries)),
				fmt.Sprintf("Pass -labels with %d names, one per phase.", len(boundaries)+1), nil)
		}
		report := resilience.ReducePhases(events, boundaries, labels, *tolerance, receivers)
		payload, text = report, report.Text()
	} else {
		report := resilience.Reduce(events, *at, *tolerance, receivers)
		payload, text = report, report.Text()
	}

	return writeOut(stdout, stderr, format, payload, colorizeReport(text, allowColor(isTerminal(stdout))))
}

func writeOut(stdout, stderr io.Writer, format string, payload any, text string) int {
	if format == "text" {
		fprint(stdout, text)
		return exitOK
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return cliError(stdout, stderr, format, "", "cannot encode the result", "Retry, or pass -o text.", err)
	}
	return exitOK
}
