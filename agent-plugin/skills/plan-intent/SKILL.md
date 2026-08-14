---
name: plan-intent
description: Dry-run a CommunicationIntent through NephMesh's report-only compiler. Use when the user wants to know if a mesh design is feasible or within the airtime budget, with no cluster and no radio write.
license: Apache-2.0
---

# Plan a CommunicationIntent (report-only)

This skill never actuates a radio. It compiles an intent and prints a verdict.

## When to use

The user has or wants a `CommunicationIntent` (region, approved presets, channels, target nodes) and needs feasibility or fleet airtime, not a write.

## How

Prefer the CLI or the MCP tool. Both wrap the same `internal/plan` core.

```sh
cd operators/meshtastic-operator
go run ./cmd/nephmeshctl plan -f ../../examples/regional-intent.yaml
```

Or call the `plan_intent` tool on `nephmesh-mcp` (stdio). Input is the intent as YAML or JSON.

## How to read the result

- `feasible` is renderability, not "this will work on the air."
- `airtime.withinBudget` is a separate verdict. `false` is authoritative (the floor already oversubscribes). `true` is advisory.
- `proposedNodes` are what the operator would create. They are not created.

Do not `kubectl apply` those specs unless the user explicitly asked to actuate, and even then the operator's RBAC is the report-only gate. Autonomous writes stay behind ADR 0002.

See `docs/guides/agent-interface.md` and `examples/regional-intent.yaml`.
