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
	assert.Contains(t, out.String(), "FEASIBLE")
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
	assert.Contains(t, errBuf.String(), "unknown command")
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
}
