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

func TestHelp(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"help"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, exitOK, code)
	assert.Contains(t, out.String(), "nephmeshctl")
}
