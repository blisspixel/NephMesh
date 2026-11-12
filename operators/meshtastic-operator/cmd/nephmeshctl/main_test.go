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

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const intentYAML = `
spec:
  region: US
  approvedModemPresets: [MEDIUM_SLOW]
  expectedTraffic:
    messagesPerMinutePerNode: 4
  nodes:
    - {name: relief-01, connection: {tcp: {host: 10.0.0.51}}}
`

func TestPlanJSONFromStdin(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"plan"}, strings.NewReader(intentYAML), &out, &errBuf)
	require.Equal(t, exitOK, code, "stderr: %s", errBuf.String())

	var m map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &m))
	assert.Equal(t, true, m["feasible"])
	assert.Equal(t, "MEDIUM_SLOW", m["selectedPreset"])
}

func TestPlanTextFromStdin(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"plan", "-o", "text"}, strings.NewReader(intentYAML), &out, &errBuf)
	require.Equal(t, exitOK, code)
	assert.Contains(t, out.String(), "feasible:")
	assert.NotContains(t, out.String(), "Renderable")
	assert.NotContains(t, out.String(), "\x1b")
}

func TestPlanInvalidFormat(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"plan", "-o", "xml"}, strings.NewReader(intentYAML), &out, &errBuf)
	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errBuf.String(), "invalid -o")
}

func TestPlanMalformedInputIsUsageError(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"plan"}, strings.NewReader("region: US\n\tbad: : :"), &out, &errBuf)
	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errBuf.String(), "plan:")
}

func TestNoArgsIsUsage(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run(nil, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, exitUsage, code)
}

func TestUnknownCommand(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"frobnicate"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, exitUsage, code)
	assert.Equal(t, "nephmeshctl: unknown command \"frobnicate\". Run 'nephmeshctl -h'.\n", errBuf.String())
	assert.NotContains(t, errBuf.String(), "commands:")
}

const sweepCSV = "2026-08-08, 12:00:00, 911000000, 920000000, 1000000, 20, -95.1, -94.0, -95.4, -93.8, -55.2, -48.6, -94.3, -95.5, -96.0\n"

func TestSpectrumJSONFromStdin(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"spectrum"}, strings.NewReader(sweepCSV), &out, &errBuf)
	require.Equal(t, exitOK, code, "stderr: %s", errBuf.String())

	var stats []map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &stats))
	require.NotEmpty(t, stats)
	// The 915 US band should show some occupancy from the two strong bins.
	var found bool
	for _, s := range stats {
		band := s["band"].(map[string]any)
		if band["name"] == "ism-915-us" {
			found = true
			assert.Positive(t, s["occupancyPercent"].(float64))
		}
	}
	assert.True(t, found, "the default bands include ism-915-us")
}

func TestSpectrumTextFromStdin(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"spectrum", "-o", "text"}, strings.NewReader(sweepCSV), &out, &errBuf)
	require.Equal(t, exitOK, code)
	assert.Contains(t, out.String(), "ism-915-us")
	assert.Contains(t, out.String(), "occupancy")
}

func TestSpectrumMalformedInputIsUsageError(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"spectrum"}, strings.NewReader("not a sweep\nat all\n"), &out, &errBuf)
	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errBuf.String(), "spectrum:")
}

func TestPlanHelpExitsZero(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"plan", "-h"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, exitOK, code, "an explicit help request is not an error")
}

func TestSpectrumHelpExitsZero(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"spectrum", "--help"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, exitOK, code)
}

const packetHex = "ffffffff040302017856341263080000dead"

func TestDecodeParsesPacket(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"decode", "-o", "text"}, strings.NewReader(packetHex+"\n"), &out, &errBuf)
	require.Equal(t, exitOK, code, "stderr: %s", errBuf.String())
	assert.Contains(t, out.String(), "from !01020304")
	assert.Contains(t, out.String(), "to ^all")
}

func TestDecodeToleratesPrefixAndWhitespace(t *testing.T) {
	// Uppercase 0X prefix and tab/space separated bytes must still parse.
	in := "0Xffffffff\t04030201 78563412 63080000\n# a comment\n\n"
	var out, errBuf bytes.Buffer
	code := run([]string{"decode", "-o", "text"}, strings.NewReader(in), &out, &errBuf)
	require.Equal(t, exitOK, code, "stderr: %s", errBuf.String())
	assert.Contains(t, out.String(), "from !01020304")
}

func TestDecodeNoValidPacketsIsUsageError(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"decode"}, strings.NewReader("not hex\nzz\n"), &out, &errBuf)
	assert.Equal(t, exitUsage, code)
}

func TestHelp(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"help"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, exitOK, code)
	assert.Contains(t, out.String(), "nephmeshctl")
	for _, line := range strings.Split(out.String(), "\n") {
		assert.LessOrEqual(t, len(line), 80, line)
	}
	assert.Contains(t, out.String(), "plan")
	assert.Contains(t, out.String(), "spectrum")
	assert.Contains(t, out.String(), "advise")
	assert.Contains(t, out.String(), "decode")
	assert.Contains(t, out.String(), "resilience")
	assert.Contains(t, out.String(), "version")
}

func TestVersion(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"--version"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, exitOK, code)
	assert.Contains(t, out.String(), "nephmeshctl 0.2.4")
	assert.NotContains(t, out.String(), "(devel)")
	assert.Empty(t, errBuf.String())
}

func TestPlanRefusesCRDAndDeviceViolations(t *testing.T) {
	base := "spec:\n  region: US\n  role: CLIENT\n  approvedModemPresets: [LONG_FAST]\n  nodes:\n    - name: a\n      connection: {tcp: {host: h}}\n      owner: {shortName: AB}\n"
	cases := []struct {
		name   string
		doc    string
		reason string
	}{
		{"region MARS", strings.Replace(base, "region: US", "region: MARS", 1), "InvalidRegion"},
		{"role OVERLORD", strings.Replace(base, "role: CLIENT", "role: OVERLORD", 1), "InvalidRole"},
		{"short name", strings.Replace(base, "shortName: AB", "shortName: TOOLONGNAME", 1), "ShortNameTooLong"},
		{"channel 99", "spec:\n  region: US\n  approvedModemPresets: [LONG_FAST]\n  channels:\n    - {index: 99, name: x}\n  nodes:\n    - {name: a, connection: {tcp: {host: h}}}\n", "ChannelIndexOutOfRange"},
		{"negative traffic", "spec:\n  region: US\n  approvedModemPresets: [LONG_FAST]\n  expectedTraffic: {messagesPerMinutePerNode: -5}\n  nodes:\n    - {name: a, connection: {tcp: {host: h}}}\n", "MessagesPerMinuteOutOfRange"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := run([]string{"plan", "-o", "json"}, strings.NewReader(tc.doc), &out, &errBuf)
			require.Equal(t, exitOK, code, errBuf.String())
			var m map[string]any
			require.NoError(t, json.Unmarshal(out.Bytes(), &m))
			assert.Equal(t, false, m["feasible"])
			assert.Equal(t, tc.reason, m["reason"])
		})
	}
}

func TestPlanWithinBudgetNullWhenNotEvaluated(t *testing.T) {
	doc := "spec:\n  region: US\n  approvedModemPresets: [LONG_FAST]\n  nodes:\n    - {name: a, connection: {tcp: {host: h}}}\n"
	var out, errBuf bytes.Buffer
	code := run([]string{"plan", "-o", "json"}, strings.NewReader(doc), &out, &errBuf)
	require.Equal(t, exitOK, code, errBuf.String())
	var m map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &m))
	assert.Equal(t, true, m["feasible"])
	air := m["airtime"].(map[string]any)
	assert.Equal(t, false, air["evaluated"])
	assert.Nil(t, air["withinBudget"])
	assert.Equal(t, "NoExpectedTraffic", air["reason"])

	var text bytes.Buffer
	code = run([]string{"plan", "-o", "text"}, strings.NewReader(doc), &text, &errBuf)
	require.Equal(t, exitOK, code)
	assert.Contains(t, text.String(), "airtime: not evaluated")
	assert.NotContains(t, text.String(), "over budget")
}

func TestPlanWarnsOnUnknownField(t *testing.T) {
	doc := "spec:\n  region: US\n  frobnicate: 1\n  approvedModemPresets: [LONG_FAST]\n  nodes:\n    - {name: a, connection: {tcp: {host: h}}}\n"
	var out, errBuf bytes.Buffer
	code := run([]string{"plan", "-o", "json"}, strings.NewReader(doc), &out, &errBuf)
	require.Equal(t, exitOK, code, errBuf.String())
	var m map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &m))
	assert.Equal(t, true, m["feasible"])
	warns := m["warnings"].([]any)
	assert.Contains(t, warns, "unknown field spec.frobnicate")
}

func TestPlanPositionalFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/intent.yaml"
	require.NoError(t, os.WriteFile(path, []byte(intentYAML), 0o600))
	var out, errBuf bytes.Buffer
	code := run([]string{"plan", path}, strings.NewReader(""), &out, &errBuf)
	require.Equal(t, exitOK, code, errBuf.String())
	assert.Contains(t, out.String(), `"feasible": true`)
}

func TestPlanRejectsWrongKind(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"plan", "-o", "text"}, strings.NewReader("kind: MeshtasticNode\nspec:\n  region: US\n"), &out, &errBuf)
	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errBuf.String(), "expected kind CommunicationIntent, got MeshtasticNode")
	assert.Contains(t, errBuf.String(), "examples/regional-intent.yaml")
}

func TestPlanMissingFile(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"plan", "-f", "nope.yaml"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errBuf.String(), "cannot read nope.yaml (file not found)")
	assert.Contains(t, errBuf.String(), "Pass -f with a path that exists.")
	assert.NotContains(t, errBuf.String(), "no such file")
}

func TestSpectrumTextFitsAndJSONIsPlain(t *testing.T) {
	var text, textErr bytes.Buffer
	code := run([]string{"spectrum", "-o", "text"}, strings.NewReader(sweepCSV), &text, &textErr)
	require.Equal(t, exitOK, code, textErr.String())
	assert.Contains(t, text.String(), "ism-915-us")
	assert.NotContains(t, text.String(), "\x1b")
	for _, line := range strings.Split(text.String(), "\n") {
		assert.LessOrEqual(t, len([]rune(line)), 80, line)
	}

	var js, jsErr bytes.Buffer
	code = run([]string{"spectrum", "-o", "json"}, strings.NewReader(sweepCSV), &js, &jsErr)
	require.Equal(t, exitOK, code, jsErr.String())
	assert.NotContains(t, js.String(), "▁")
	assert.NotContains(t, js.String(), "\x1b")
}

func TestAdviseNamesAMissingServer(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"advise", "-o", "text", "-ollama-url", "http://127.0.0.1:1"}, strings.NewReader(sweepCSV), &out, &errBuf)
	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errBuf.String(), "no Ollama server at http://127.0.0.1:1")
	assert.Contains(t, errBuf.String(), "ollama serve")
	assert.NotContains(t, errBuf.String(), "dial tcp")
}

func TestResilienceSilentReceiverOnTheCLI(t *testing.T) {
	log := strings.Join([]string{
		`EVENT {"ev":"sent","seq":0,"node":"sim1","t":0}`,
		`EVENT {"ev":"recv","seq":0,"node":"sim2","t":0.1}`,
		`EVENT {"ev":"sent","seq":1,"node":"sim1","t":1}`,
		`EVENT {"ev":"recv","seq":1,"node":"sim2","t":1.1}`,
		`EVENT {"ev":"sent","seq":2,"node":"sim1","t":2}`,
		`EVENT {"ev":"recv","seq":2,"node":"sim2","t":2.1}`,
		`EVENT {"ev":"sent","seq":3,"node":"sim1","t":3}`,
		`EVENT {"ev":"recv","seq":3,"node":"sim2","t":3.1}`,
		`SUMMARY {"receivers":["sim2","sim3"],"sent":4,"expected":8,"delivered":4}`,
	}, "\n")
	var out, errBuf bytes.Buffer
	code := run([]string{"resilience", "-o", "text", "-at", "2"}, strings.NewReader(log), &out, &errBuf)
	require.Equal(t, exitOK, code, errBuf.String())
	assert.Contains(t, out.String(), "sim3 0/4")
	assert.Contains(t, out.String(), "overall 4/8")
	assert.NotContains(t, out.String(), "observed receivers only")
}

func TestAllowColorHonorsEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm")
	assert.False(t, allowColor(true))
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	assert.False(t, allowColor(true))
	assert.False(t, unicodeOK())
	// unicodeOK reads LC_ALL, then LC_CTYPE, then LANG. Clear the first two so
	// the LANG cases below are what the assertion measures.
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("TERM", "xterm")
	t.Setenv("LANG", "C")
	assert.False(t, unicodeOK())
	t.Setenv("LANG", "C.UTF-8")
	assert.True(t, unicodeOK())
}

func TestSubcommandHelpFits(t *testing.T) {
	for _, cmd := range []string{"plan", "spectrum", "advise", "decode", "resilience"} {
		var out, errBuf bytes.Buffer
		code := run([]string{cmd, "-h"}, strings.NewReader(""), &out, &errBuf)
		assert.Equal(t, exitOK, code, cmd)
		for _, line := range strings.Split(errBuf.String(), "\n") {
			assert.LessOrEqual(t, len(line), 80, "%s: %s", cmd, line)
		}
	}
}

func TestResolveInputPath(t *testing.T) {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	file := fs.String("f", "-", "file")
	require.NoError(t, fs.Parse([]string{"examples/regional-intent.yaml"}))
	path, err := resolveInputPath(fs, *file, true)
	require.NoError(t, err)
	assert.Equal(t, "examples/regional-intent.yaml", path)

	fs = flag.NewFlagSet("plan", flag.ContinueOnError)
	file = fs.String("f", "-", "file")
	require.NoError(t, fs.Parse([]string{"-f", "a.yaml", "b.yaml"}))
	_, err = resolveInputPath(fs, *file, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not both")

	fs = flag.NewFlagSet("plan", flag.ContinueOnError)
	file = fs.String("f", "-", "file")
	require.NoError(t, fs.Parse([]string{"a.yaml", "b.yaml"}))
	_, err = resolveInputPath(fs, *file, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected")

	fs = flag.NewFlagSet("plan", flag.ContinueOnError)
	file = fs.String("f", "-", "file")
	require.NoError(t, fs.Parse(nil))
	_, err = resolveInputPath(fs, *file, true)
	require.EqualError(t, err, "stdin is a terminal, so there is nothing to read")
}

func TestDecorateErrorPaintsProblemAndCommand(t *testing.T) {
	got := decorateError(
		"no Ollama server at http://localhost:11434",
		"Start it with 'ollama serve', or pass -ollama-url.",
	)
	assert.Contains(t, got, ansiRed+"no Ollama server at http://localhost:11434"+ansiReset)
	assert.Contains(t, got, ansiCyan+"ollama serve"+ansiReset)
	assert.NotContains(t, got, ansiCyan+"-ollama-url")
	plain := strings.ReplaceAll(got, ansiRed, "")
	plain = strings.ReplaceAll(plain, ansiCyan, "")
	plain = strings.ReplaceAll(plain, ansiReset, "")
	assert.Equal(t, "no Ollama server at http://localhost:11434. Start it with 'ollama serve', or pass -ollama-url.", plain)
}

func TestColorizeReportPaintsVerdictWordsOnly(t *testing.T) {
	plan := colorizeReport("feasible: 1 node at MEDIUM_SLOW in US\nairtime: over budget, about 40% of the 25% budget\ninfeasible: no target nodes\n", true)
	assert.Contains(t, plan, ansiGreen+"feasible"+ansiReset)
	assert.Contains(t, plan, ansiYellow+"over"+ansiReset)
	assert.Contains(t, plan, ansiRed+"infeasible"+ansiReset)
	assert.Contains(t, plan, ansiDim+": no target nodes"+ansiReset)
	assert.NotContains(t, colorizeReport("feasible: 1 node at MEDIUM_SLOW in US\n", false), "\x1b")

	held := colorizeReport("  verdict: mesh kept delivering across the perturbation\n", true)
	assert.Contains(t, held, ansiGreen+"kept delivering"+ansiReset)
	assert.NotContains(t, held, ansiGreen+"verdict")

	fell := colorizeReport("  verdict: delivery fell under load and recovered after the adaptation\n", true)
	assert.Contains(t, fell, ansiGreen+"recovered"+ansiReset)
	assert.NotContains(t, fell, ansiRed+"fell")

	plain := colorizeReport("feasible: 1 node at MEDIUM_SLOW in US\n", false)
	assert.NotContains(t, plain, "\x1b")
}

func TestPlanJSONErrorKeepsTheGoChain(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"plan", "-o", "json", "-f", "nope.yaml"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, exitUsage, code)
	assert.NotContains(t, errBuf.String(), "\x1b")
	var body map[string]string
	require.NoError(t, json.Unmarshal(out.Bytes(), &body))
	assert.NotEmpty(t, body["error"])
	assert.NotContains(t, errBuf.String(), body["error"])
}

func TestPlanRefusesBothFileForms(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"plan", "-f", "a.yaml", "b.yaml"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errBuf.String(), "not both")
	assert.Contains(t, errBuf.String(), "Drop one of them.")
}

func TestIsUnreachable(t *testing.T) {
	assert.True(t, isUnreachable(errors.New("dial tcp 127.0.0.1:1: connectex: connection refused")))
	assert.False(t, isUnreachable(errors.New("model returned an empty response")))
}
