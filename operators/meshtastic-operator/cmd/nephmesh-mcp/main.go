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

// Command nephmesh-mcp is a Model Context Protocol server over stdio. It exposes
// NephMesh's report-only intent compiler as a single MCP tool, plan_intent, so an
// MCP-capable host (Claude Code, Codex) can dry-run a CommunicationIntent as a
// native tool call. It needs no cluster and no hardware; the compiler is pure.
//
// It communicates on stdin and stdout using the MCP stdio transport
// (newline-delimited JSON-RPC 2.0), so stdout carries only protocol messages;
// diagnostics go to stderr. Register it with an MCP host as a stdio server whose
// command is this binary.
package main

import (
	"fmt"
	"os"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/mcpserver"
)

// version is the advertised server version; overridden at release via -ldflags.
var version = "dev"

func main() {
	s := mcpserver.New("nephmesh-mcp", version)
	if err := s.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "nephmesh-mcp: %v\n", err)
		os.Exit(1)
	}
}
