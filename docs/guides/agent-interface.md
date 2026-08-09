# Driving NephMesh from an agent

NephMesh is intent-driven, which makes it naturally usable by an AI agent (Claude
Code, Codex, a local LLM with tool use) or any script. You declare an outcome and
read back a verdict; you do not drive a radio directly. Two entry points expose
the report-only intent compiler so an agent can plan a mesh without a cluster and
without hardware:

- `nephmeshctl plan` is a shell command: a CommunicationIntent in, a JSON or text
  verdict out. Any agent that can run a command can use it.
- `nephmesh-mcp` is a Model Context Protocol server over stdio: it exposes the
  same compiler as the `plan_intent` tool, so an MCP-capable host can call it
  natively.

Both wrap the same pure `internal/plan` core, so the answer is identical whichever
you call, and it is exactly what the operator's controller would compute. Neither
touches a cluster or a device: the compiler is pure, so this is a true offline
dry-run. Actuation stays gated behind the signed-autonomy work (ADR 0002).

## The contract

Input is a `CommunicationIntent` (the same document you would apply to a cluster;
see `examples/regional-intent.yaml`). Output is a stable JSON object:

| Field | Meaning |
|---|---|
| `feasible` | Whether the intent compiles into node specs. |
| `reason`, `message` | A stable token and a human explanation of the verdict. |
| `selectedPreset` | The modem preset chosen from the approved set (first known). |
| `nodeCount` | How many nodes the intent renders to. |
| `airtime` | Fleet airtime verdict: `evaluated`, `withinBudget`, `predictedUtilizationPercent`, and a `reason`/`message`. Evaluated only when the intent declares `expectedTraffic`. |
| `proposedNodes` | The rendered per-device `MeshtasticNode` specs. Report-only: what the operator would create, not created. |

`feasible` and `airtime.withinBudget` are distinct verdicts: an intent can be
renderable but over the airtime budget. The airtime number is a conservative
floor (mesh rebroadcast ignored), so `withinBudget: false` is authoritative while
`true` is advisory.

## nephmeshctl plan

Build it (no dependencies beyond the Go toolchain):

```sh
cd operators/meshtastic-operator
go build -o nephmeshctl ./cmd/nephmeshctl
```

Use it, from a file or stdin, as JSON (default, for machines) or text (for people):

```sh
./nephmeshctl plan -f ../../examples/regional-intent.yaml            # JSON
./nephmeshctl plan -f ../../examples/regional-intent.yaml -o text    # summary
cat intent.yaml | ./nephmeshctl plan                                 # stdin
```

Exit code is 0 for any successful evaluation (the verdict, feasible or not, is in
the output) and 2 for a usage or parse error, so a script branches on the JSON
payload rather than the exit code.

`nephmeshctl` also has a `spectrum` subcommand: it reduces a receive-only SDR
sweep (rtl_power/hackrf_sweep CSV) to per-band occupancy, noise floor, and peak
power, as JSON or text. It is pure like `plan` (it reads a CSV a radio produced,
it touches no hardware), so an agent can analyze a capture offline. See the
[spectrum validation runbook](spectrum-validation.md).

```sh
./nephmeshctl spectrum -f sweep.csv -o text
```

And `nephmeshctl advise` closes the loop with a *local* model: it reduces and
classifies a sweep, hands the situation to a local Ollama server, and prints a
report-only recommendation (hold, change preset, or investigate) with a
rationale. The model proposes; it never actuates, and the output is validated
against the approved preset set so a hallucination cannot escape the envelope.
See the [edge advisor demo](../../demo/edge-advisor/).

```sh
./nephmeshctl advise -f sweep.csv -model qwen2.5:14b -ollama-url http://localhost:11434
```

## nephmesh-mcp (Model Context Protocol)

Build it:

```sh
cd operators/meshtastic-operator
go build -o nephmesh-mcp ./cmd/nephmesh-mcp
```

It speaks the MCP stdio transport (newline-delimited JSON-RPC 2.0) on stdin and
stdout; diagnostics go to stderr, so stdout carries only protocol messages.
Register it with an MCP host as a stdio server whose command is the binary. For
Claude Code:

```sh
claude mcp add nephmesh -- /absolute/path/to/nephmesh-mcp
```

Or in an MCP client configuration:

```json
{
  "mcpServers": {
    "nephmesh": {
      "command": "/absolute/path/to/nephmesh-mcp"
    }
  }
}
```

It advertises two tools:

- `plan_intent`: input `{ "intent": "<CommunicationIntent as YAML or JSON>" }`
  (a full object or a bare spec; a string or an inlined object both work). The
  result is the plan JSON above, returned as tool text.
- `sense_spectrum`: input `{ "sweep": "<rtl_power/hackrf_sweep CSV>" }`, with
  optional `marginDb` and `noisePercentile`. The result is the per-band
  occupancy JSON (the same reduction as `nephmeshctl spectrum`), returned as
  tool text.

An over-budget or infeasible intent, or an idle band, is a normal result, not a
tool error; a tool error (`isError: true`) means the input could not be parsed.

## What this is not, yet

This surface is offline planning only. Reading live mesh and spectrum state
(neighbor counts, band occupancy) and applying an intent to a cluster are
roadmapped as a later, richer MCP layer (see `docs/roadmap.md`); they involve a
live cluster and, for actuation, the signed-autonomy safety work. Keeping the
first slice a pure dry-run is deliberate: it is the safe, hardware-free thing an
agent can lean on today.
