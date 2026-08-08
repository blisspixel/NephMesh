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

package mcpserver

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decode(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func handle(t *testing.T, s *Server, msg string) (map[string]any, bool) {
	t.Helper()
	resp, ok := s.HandleMessage([]byte(msg))
	if !ok {
		return nil, false
	}
	return decode(t, resp), true
}

func TestInitializeNegotiatesSupportedVersion(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	m, ok := handle(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`)
	require.True(t, ok)
	assert.EqualValues(t, 1, m["id"])
	res := m["result"].(map[string]any)
	assert.Equal(t, "2025-03-26", res["protocolVersion"], "a supported requested version is echoed")
	caps := res["capabilities"].(map[string]any)
	assert.Contains(t, caps, "tools")
	info := res["serverInfo"].(map[string]any)
	assert.Equal(t, "nephmesh-mcp", info["name"])
}

func TestInitializeFallsBackToLatestForUnknownVersion(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	m, _ := handle(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	res := m["result"].(map[string]any)
	assert.Equal(t, latestProtocolVersion(), res["protocolVersion"])
}

func TestInitializedNotificationHasNoResponse(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	_, ok := s.HandleMessage([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	assert.False(t, ok, "a notification produces no response")
}

func TestPing(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	m, ok := handle(t, s, `{"jsonrpc":"2.0","id":"p","method":"ping"}`)
	require.True(t, ok)
	assert.Equal(t, "p", m["id"])
	assert.NotNil(t, m["result"])
}

func TestToolsListAdvertisesPlanIntent(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	m, _ := handle(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	res := m["result"].(map[string]any)
	tools := res["tools"].([]any)
	require.Len(t, tools, 1)
	tool := tools[0].(map[string]any)
	assert.Equal(t, "plan_intent", tool["name"])
	schema := tool["inputSchema"].(map[string]any)
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "intent")
}

func TestToolsCallPlanIntentString(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	// The intent is a JSON string containing YAML.
	yaml := "spec:\n  region: US\n  approvedModemPresets: [MEDIUM_SLOW]\n  expectedTraffic:\n    messagesPerMinutePerNode: 4\n  nodes:\n    - {name: relief-01, connection: {tcp: {host: 10.0.0.51}}}\n"
	args, err := json.Marshal(map[string]any{"name": "plan_intent", "arguments": map[string]any{"intent": yaml}})
	require.NoError(t, err)
	m, _ := handle(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":`+string(args)+`}`)

	res := m["result"].(map[string]any)
	assert.Equal(t, false, res["isError"])
	content := res["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	// The text payload is the stable plan JSON.
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	assert.Equal(t, true, out["feasible"])
	assert.Equal(t, "MEDIUM_SLOW", out["selectedPreset"])
}

func TestToolsCallPlanIntentInlineObject(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	// The intent is an inlined JSON object, not a string.
	params := `{"name":"plan_intent","arguments":{"intent":{"spec":{"region":"US","approvedModemPresets":["LONG_FAST"],"nodes":[{"name":"n1","connection":{"tcp":{"host":"10.0.0.1"}}}]}}}}`
	m, _ := handle(t, s, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":`+params+`}`)
	res := m["result"].(map[string]any)
	assert.Equal(t, false, res["isError"])
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	assert.Contains(t, text, "LONG_FAST")
}

func TestToolsCallBadIntentIsToolError(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	params := `{"name":"plan_intent","arguments":{"intent":"region: US\n\tbad: : :"}}`
	m, _ := handle(t, s, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":`+params+`}`)
	res := m["result"].(map[string]any)
	assert.Equal(t, true, res["isError"], "a bad intent is a tool error result, not a protocol error")
	assert.Nil(t, m["error"])
}

func TestToolsCallUnknownToolIsProtocolError(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	params := `{"name":"do_something_else","arguments":{}}`
	m, _ := handle(t, s, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":`+params+`}`)
	e := m["error"].(map[string]any)
	assert.EqualValues(t, codeInvalidParams, e["code"])
}

func TestUnknownMethodIsMethodNotFound(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	m, _ := handle(t, s, `{"jsonrpc":"2.0","id":7,"method":"resources/list"}`)
	e := m["error"].(map[string]any)
	assert.EqualValues(t, codeMethodNotFound, e["code"])
}

func TestParseErrorForMalformedJSON(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	m, ok := handle(t, s, `{not json`)
	require.True(t, ok)
	e := m["error"].(map[string]any)
	assert.EqualValues(t, codeParseError, e["code"])
}

func TestWrongJSONRPCVersionRejected(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	m, _ := handle(t, s, `{"jsonrpc":"1.0","id":8,"method":"ping"}`)
	e := m["error"].(map[string]any)
	assert.EqualValues(t, codeInvalidRequest, e["code"])
}

func TestServeLoopHandlesMultipleMessages(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	require.NoError(t, s.Serve(strings.NewReader(in), &out))

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	// Two responses: initialize and tools/list. The notification yields none.
	require.Len(t, lines, 2)
	first := decode(t, []byte(lines[0]))
	assert.EqualValues(t, 1, first["id"])
	second := decode(t, []byte(lines[1]))
	assert.EqualValues(t, 2, second["id"])
}

func TestEmptyLineIgnored(t *testing.T) {
	s := New("nephmesh-mcp", "test")
	_, ok := s.HandleMessage([]byte("   \n"))
	assert.False(t, ok)
}
