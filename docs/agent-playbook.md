# Agent playbook

Canonical commands and entry points for working on NephMesh, written to be tool-agnostic. They work the same whether you are Claude Code, Codex, GitHub Copilot, another coding agent, or a human at a terminal. Nothing here is specific to one assistant.

The layering, from most to least portable:

- **`AGENTS.md`** is the source of conventions. It is the emerging cross-tool standard that Codex, Cursor, Copilot, and others read, and `CLAUDE.md` simply points to it. Read it first.
- **This playbook** is the command surface: the exact, deterministic commands for common tasks, so any agent knows the real entry points instead of guessing.
- **Agent-facing tools for operating the mesh** exist as an offline first slice: `nephmeshctl plan` (a shell command) and `nephmesh-mcp` (a Model Context Protocol stdio server) both dry-run a `CommunicationIntent` through the report-only compiler and return a feasibility, preset, and fleet-airtime verdict, with no cluster and no hardware. See `docs/guides/agent-interface.md`. The richer MCP layer that exposes live mesh and spectrum state (neighbor counts, band occupancy) and applies intents to a cluster remains roadmapped (Phase 5); MCP is chosen over an assistant-specific skill format precisely because it is portable across agents.

## Environment notes

- Shell is POSIX (`sh`). On Windows use Git Bash or WSL2; scripts avoid Bash-only and OS-specific constructs.
- The working directory can reset between separate shell invocations, so `cd` inside each command rather than relying on a persisted directory.
- Go 1.25+ is expected for the Go modules; Docker for package render and the device integration test; a local cluster (kind, k3d, or k3s) only for the demo.

## One command to validate everything

Before committing, run the full gate aggregator from the repo root. It runs every check and skips a group with a clear message if its tooling is absent, so it is safe on any machine:

```sh
sh hack/check-all.sh
```

This runs the repo gates (license headers, writing style, manifest control-surface exposure, transmit interlock), then each Go module (build, vet, lint, test, coverage floor) if `go` is present, then the kpt package render if `docker` is present. Exit code 0 means everything that could run, passed.

## Common tasks

| Task | Command |
|---|---|
| Run every gate (pre-commit) | `sh hack/check-all.sh` |
| Fast repo gates only (no Docker/Go) | `make check` or the individual `sh hack/check-*.sh` |
| Build, vet, test a Go module | `cd api && go build ./... && go vet ./... && go test ./... -cover` (same shape for `operators/meshtastic-operator`) |
| Enforce the coverage floor | `cd <module> && sh <repo-root>/hack/check-coverage.sh 80` |
| Regenerate CRD and deepcopy | `cd api && make generate manifests` (pins controller-gen) |
| Render and validate kpt packages | `sh hack/check-packages.sh` |
| Run the Phase 1 virtual mesh demo | `sh demo/phase1/scripts/demo.sh` then `sh demo/phase1/scripts/teardown.sh` (needs a cluster) |
| Dry-run a CommunicationIntent (no cluster) | `cd operators/meshtastic-operator && go run ./cmd/nephmeshctl plan -f ../../examples/regional-intent.yaml` (see `docs/guides/agent-interface.md`) |
| Operator integration test vs sim device | see the header of `operators/meshtastic-operator/internal/reconcile/integration_test.go` |
| MeshToad plus handheld RF bench | set `SENSOR_SSH` and `COM_PORT`, tunnel `ssh -N -L 14403:127.0.0.1:4403 "$SENSOR_SSH"`, then `sh demo/meshtoad-gateway/run.sh` |
| Build the operator image locally (no push) | `sh hack/build-operator-image.sh` |

## Rules that are enforced, not just requested

Do not rely on remembering these; the gates fail the build if they are violated. But knowing them avoids wasted cycles:

- No emojis, em dashes, or AI attribution anywhere (commit messages, code, docs). `hack/check-style.sh`.
- Apache-2.0 header on every source file. `hack/check-headers.sh`.
- No manifest exposes the device API or broker outside the cluster. `hack/check-manifests.sh`.
- No unmarked radio-transmit entry point. `hack/check-transmit.sh`.
- Every kpt package renders cleanly. `hack/check-packages.sh`.
- Go code passes golangci-lint and holds the coverage floor. `.golangci.yml`, `hack/check-coverage.sh`.
- Commits are DCO signed (`git commit -s`).

## Where the code lives

`api/` (CRD types), `operators/meshtastic-operator/` (the operator), `packages/` (kpt blueprints), `demo/` (reproducible demos), `docs/` (everything else). The full target layout and its rationale are in `docs/plans/engineering-conventions.md`.
