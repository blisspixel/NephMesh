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

// Package mcpserver is a minimal, dependency-free Model Context Protocol server
// over stdio. It speaks the MCP stdio transport (newline-delimited JSON-RPC 2.0)
// and exposes NephMesh's report-only intent compiler as a single tool,
// plan_intent, so an MCP-capable host (Claude Code, Codex) can dry-run a
// CommunicationIntent as a native tool call. It implements the subset an MCP host
// needs to connect and call tools: initialize, the initialized notification,
// ping, tools/list, and tools/call. The tool logic is the same internal/plan core
// the nephmeshctl CLI uses, so the two surfaces never diverge.
//
// It is hand-rolled rather than built on an SDK to keep the module dependency
// footprint at zero for this and to keep the wire behavior under direct test. The
// JSON-RPC framing and every method are exercised by server_test.go; the one
// thing a unit test cannot prove is interoperability with a specific host, which
// is a follow-up validation, not a code path.
package mcpserver

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/plan"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/spectrum"
)

// The MCP protocol revisions this server understands. It negotiates by echoing
// the client's requested version when it is one of these, else it offers the
// latest it supports. Kept newest-first.
var supportedProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

func latestProtocolVersion() string { return supportedProtocolVersions[0] }

// JSON-RPC 2.0 error codes (subset used here).
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Server exposes the plan_intent tool over the MCP stdio transport.
type Server struct {
	name    string
	version string
}

// New builds a server that advertises the given implementation name and version.
func New(name, version string) *Server {
	return &Server{name: name, version: version}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string `json:"jsonrpc"`
	// ID is echoed verbatim from the request (as raw JSON) so a large-integer or
	// string id round-trips byte-for-byte; a nil ID marshals to null, which is
	// what JSON-RPC 2.0 requires for a parse-error response.
	ID json.RawMessage `json:"id"`
	// Result is raw JSON so a success response always carries a result member
	// (null when the handler has no payload); exactly one of Result/Error is set.
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Serve runs the transport loop: it reads newline-delimited JSON-RPC messages
// from r and writes each response to w, until r reaches EOF. Per-message failures
// become JSON-RPC error responses; only an unrecoverable write error stops the
// loop. Nothing but protocol messages is ever written to w.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	br := bufio.NewReader(r)
	bw := bufio.NewWriter(w)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if resp, ok := s.HandleMessage(line); ok {
				if _, werr := bw.Write(append(resp, '\n')); werr != nil {
					return werr
				}
				if ferr := bw.Flush(); ferr != nil {
					return ferr
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// HandleMessage processes one JSON-RPC message and returns its response bytes.
// The second return is false when the message is a notification (no id) or is
// empty, in which case nothing is written back. It is exported so the wire
// behavior can be driven directly by tests.
func (s *Server) HandleMessage(raw []byte) ([]byte, bool) {
	if len(trim(raw)) == 0 {
		return nil, false
	}
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		// Parse error: the id is unknown, so it is null per JSON-RPC 2.0.
		return marshalResponse(rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: codeParseError, Message: "parse error"},
		}), true
	}

	// A request carries an id (echoed verbatim, including a large integer or a
	// string); a notification has no id and never gets a response.
	isNotification := len(req.ID) == 0

	if req.JSONRPC != "2.0" {
		if isNotification {
			return nil, false
		}
		return marshalResponse(errorResponse(req.ID, &rpcError{Code: codeInvalidRequest, Message: "jsonrpc must be \"2.0\""})), true
	}

	result, rerr := s.dispatch(req.Method, req.Params)
	if isNotification {
		// Notifications never get a response, even on error.
		return nil, false
	}
	if rerr != nil {
		return marshalResponse(errorResponse(req.ID, rerr)), true
	}
	// A success response must always carry a result member (null when the handler
	// has no payload), never neither result nor error.
	return successResponse(req.ID, result), true
}

// dispatch routes a method to its handler, returning either a result or a
// JSON-RPC error. Unknown methods are MethodNotFound.
func (s *Server) dispatch(method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		return s.initialize(params), nil
	case "notifications/initialized":
		return nil, nil // notification; result is ignored
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return s.toolsList(), nil
	case "tools/call":
		return s.toolsCall(params)
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + method}
	}
}

func (s *Server) initialize(params json.RawMessage) any {
	// Negotiate the protocol version: echo the client's if we support it, else
	// offer our latest.
	requested := ""
	if len(params) > 0 {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(params, &p)
		requested = p.ProtocolVersion
	}
	version := latestProtocolVersion()
	for _, v := range supportedProtocolVersions {
		if v == requested {
			version = requested
			break
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    s.name,
			"version": s.version,
		},
		"instructions": "plan_intent dry-runs a NephMesh CommunicationIntent through the report-only compiler and returns feasibility, the selected modem preset, a fleet airtime verdict, and the proposed MeshtasticNode specs. It never actuates hardware or a cluster.",
	}
}

func (s *Server) toolsList() any {
	return map[string]any{
		"tools": []any{
			map[string]any{
				"name":        "plan_intent",
				"description": "Dry-run a NephMesh CommunicationIntent. Reports whether it is feasible, the modem preset selected from the approved set, a fleet airtime budget verdict (when expectedTraffic is given), and the proposed per-device MeshtasticNode specs. Report-only: it creates nothing and touches no radio.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"intent": map[string]any{
							"type":        "string",
							"description": "A CommunicationIntent as YAML or JSON. Either a full object (with a spec field) or a bare spec.",
						},
					},
					"required": []any{"intent"},
				},
			},
			map[string]any{
				"name":        "sense_spectrum",
				"description": "Reduce a receive-only SDR power sweep (rtl_power/hackrf_sweep CSV) to per-band occupancy: occupancy percent against each band's own noise floor, plus noise floor, peak, and mean power, for the 433/868/915 MHz ISM ranges. Analysis only; it reads a capture and touches no radio.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"sweep": map[string]any{
							"type":        "string",
							"description": "A power sweep in rtl_power/hackrf_sweep CSV format.",
						},
						"marginDb": map[string]any{
							"type":        "number",
							"description": "Optional: dB above the noise floor a bin must be to count as occupied (default 6).",
						},
						"noisePercentile": map[string]any{
							"type":        "number",
							"description": "Optional: per-band percentile taken as the noise floor, 0..100 (default 25).",
						},
					},
					"required": []any{"sweep"},
				},
			},
		},
	}
}

func (s *Server) toolsCall(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "invalid tools/call params: " + err.Error()}
	}
	switch p.Name {
	case "plan_intent":
		return s.callPlanIntent(p.Arguments)
	case "sense_spectrum":
		return s.callSenseSpectrum(p.Arguments)
	default:
		return nil, &rpcError{Code: codeInvalidParams, Message: "unknown tool: " + p.Name}
	}
}

func (s *Server) callPlanIntent(args json.RawMessage) (any, *rpcError) {
	var a struct {
		Intent json.RawMessage `json:"intent"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	intentBytes, err := intentArgument(a.Intent)
	if err != nil {
		return toolError(err.Error()), nil
	}
	out, err := plan.Run(intentBytes)
	if err != nil {
		// A bad intent is a tool-execution error the model should see, not a
		// protocol error, so it is returned as an error result.
		return toolError("plan: " + err.Error()), nil
	}
	return toolText(out), nil
}

func (s *Server) callSenseSpectrum(args json.RawMessage) (any, *rpcError) {
	var a struct {
		Sweep           string   `json:"sweep"`
		MarginDB        *float64 `json:"marginDb"`
		NoisePercentile *float64 `json:"noisePercentile"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if a.Sweep == "" {
		return toolError("sweep is required (rtl_power/hackrf_sweep CSV)"), nil
	}
	opts := spectrum.DefaultOptions()
	if a.MarginDB != nil {
		opts.ThresholdMarginDB = *a.MarginDB
	}
	if a.NoisePercentile != nil {
		opts.NoiseFloorPercentile = *a.NoisePercentile
	}
	stats, err := spectrum.Sense(strings.NewReader(a.Sweep), spectrum.DefaultBands(), opts)
	if err != nil {
		return toolError("spectrum: " + err.Error()), nil
	}
	return toolText(stats), nil
}

// toolText renders a value as pretty JSON in a successful tool result.
func toolText(v any) any {
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return toolError("internal: could not encode result")
	}
	return map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": string(pretty)},
		},
		"isError": false,
	}
}

// intentArgument accepts the intent either as a JSON string (containing YAML or
// JSON) or as an inlined JSON object, and returns the bytes to parse.
func intentArgument(raw json.RawMessage) ([]byte, error) {
	if len(trim(raw)) == 0 {
		return nil, plan.ErrEmptyInput
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		return []byte(s), nil
	}
	// An object or array: it is already JSON, which is valid YAML for the parser.
	return raw, nil
}

func toolError(msg string) any {
	return map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": msg},
		},
		"isError": true,
	}
}

func errorResponse(id json.RawMessage, e *rpcError) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: e}
}

// successResponse builds a success response whose result member is always present
// (null when the handler returned no payload), so a reply never has neither
// result nor error. The id is echoed verbatim.
func successResponse(id json.RawMessage, result any) []byte {
	raw, err := json.Marshal(result)
	if err != nil {
		return marshalResponse(errorResponse(id, &rpcError{Code: -32603, Message: "internal error"}))
	}
	return marshalResponse(rpcResponse{JSONRPC: "2.0", ID: id, Result: raw})
}

func marshalResponse(resp rpcResponse) []byte {
	b, err := json.Marshal(resp)
	if err != nil {
		// Marshalling our own response should never fail; fall back to a static
		// internal error rather than panic in the transport loop.
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal error"}}`)
	}
	return b
}

func trim(b []byte) []byte {
	start := 0
	for start < len(b) && isSpace(b[start]) {
		start++
	}
	end := len(b)
	for end > start && isSpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
