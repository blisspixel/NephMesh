# NephMesh agent instructions

These instructions apply to any AI coding agent working in this repository.

## What this project is

An independent experimental project applying Nephio-style intent-driven automation (Kubernetes, kpt/Porch, Configuration as Data) to radio systems generally: LoRa mesh, wider LoRa, and software defined radio. It is not affiliated with the official Nephio project or the Linux Foundation. We consume Nephio as a third party, the same pattern OpenAirInterface uses for its packages.

Scope is deliberately broader than any single radio. Meshtastic is the first concrete driver because it has the most complete programmable control surface (a documented API, scriptable config, a simulation mode), not because the project is only about Meshtastic. Software defined radio (the HackRF Pro and cheaper receivers) is a co-equal pillar, spectrum sensing today and more later, not a Meshtastic accessory. The design keeps a radio-agnostic seam so other LoRa ecosystems (LoRaWAN, other mesh firmwares) and richer SDR workloads can become additional drivers behind the same intent model. A new radio earns its way in by having a control surface worth reconciling; see `docs/faq.md`.

## Orientation

- `docs/agent-playbook.md` is the tool-agnostic command surface (works for any coding agent or a human): the exact commands for building, testing, rendering, and the single `sh hack/check-all.sh` that runs every gate before a commit. Skim it early.
- Start with `docs/roadmap.md` (phased plan in dependency order; check phase status before proposing work) and `docs/architecture.md`.
- `docs/research/` holds the sourced research base (Nephio mechanics, Meshtastic automation surface, SDR and Kubernetes device access, prior art, lab setup, terminology and legality). Prefer citing and updating these over re-deriving facts.
- Planned layout: the full target tree (with the phase that creates each part) is in `docs/plans/engineering-conventions.md`: `packages/`, `api/`, `krm-functions/`, `operators/`, `exporters/`, `demo/`, plus `distros/` only if cloud-specific material ever exists. Do not create a directory until its roadmap phase starts.

## Deepr expert council (optional host capacity)

When the host has the `deepr` MCP server connected (see project `.mcp.json`), use it for design questions against persistent domain experts. Expert store lives at `C:\Users\nicks\.deepr` on the operator machine.

Council (exact names):

- NephMesh Hybrid Resilient Comms
- Meshtastic LoRa Mesh Automation
- Commodity SDR and HackRF Spectrum Sensing
- Nephio Intent Network Automation

Rules for MCP use:

- Prefer read-only first: `deepr_list_experts`, `deepr_expert_handoff`.
- Consult with `synthesis_backend=local`, `budget=0`. Local synthesis can take several minutes; do not treat a short timeout as empty knowledge.
- Do not call metered research or mutate expert beliefs unless the operator asks.
- Experts advise; you write the code. Agent is never the live reconcile control loop.

CLI fallback if MCP tools are missing:

```text
deepr expert consult "question" -e "NephMesh Hybrid Resilient Comms" -e "Meshtastic LoRa Mesh Automation" -e "Commodity SDR and HackRF Spectrum Sensing" -e "Nephio Intent Network Automation" --local --budget 0 -y
```

Inventory note: `Documents: 0` is normal after `absorb --file`. Use `Claims` /
`claim_count` (or `deepr expert health-check`) to decide if knowledge exists.
Do not skip consult solely because documents and conversations are zero.

## Writing and output style

- No emojis anywhere: code, docs, commit messages, comments.
- No em dashes in prose. Use commas, colons, parentheses, or separate sentences.
- No AI attribution of any kind: no "Generated with", no "Co-Authored-By" AI trailers, no tool names in commit messages or file headers.
- Plain, factual prose. Detailed and well researched, but humble: this is an experiment, so write "explores" and "would be", not product-launch language. Cite sources with links in research docs.
- The README stays short and links into docs/ rather than duplicating them. When a roadmap phase demo lands, capture it (screenshot or terminal capture) and add it to the README Status section and the release notes.

## Code quality bars

The full standard, with the enforced-vs-gap breakdown and the assume-breach testing bar, is `docs/CODE-QUALITY-STANDARDS.md`. The essentials:

- No god files: one responsibility per file; split before a file accumulates a second concern. Applies to YAML too (one workload per manifest file, as demo/phase1 already does).
- No placeholders in commits: zero TODO markers, stub bodies, or commented-out code. If work is unfinished, it is a roadmap or plan item, not a comment.
- No copy-paste domain logic. Tiny local duplication is acceptable only when an abstraction would cost more than it saves.
- Comments state constraints and non-obvious facts (validated findings, protocol quirks), never narrate what the next line does.
- Everything shipped is executed first: manifests are applied, scripts are run, packages are rendered. Nothing lands on the strength of looking correct.
- Tests-first for Go code when it arrives (testify plus golden tests per the engineering conventions); coverage must not regress.
- Prefer deterministic gates over instructions: if a rule matters, wire it into `hack/` checks and CI.

## Project conventions

- Consume Nephio, never fork or vendor it. Pin to a Nephio release (R6 as of August 2026).
- $0 first: every feature must work against simulated radios (`meshtasticd -s`, Meshtasticator, UERANSIM) before hardware variants, so CI and newcomers need no hardware.
- Hardware-agnostic SDR code: target SoapySDR; driver strings are configuration, not code.
- Receive-only by default. Transmit paths are explicit opt-in with the regulatory notes in `docs/research/sdr-spectrum-sensing.md` and `docs/research/terminology-and-legality.md`. Never add transmit features casually. The lab HackRF is not a certified Part 15 transmitter; over-the-air transmit tests happen only in authorized contexts.
- Minimal-diff reconciliation: Meshtastic nodes reboot on config apply, so operators must export, diff, and apply only drift.
- kpt packages are plain KRM YAML mutated by Kptfile pipelines. No Helm-style templating.
- Provider-neutral core: nothing in core packages, CRDs, or operators may import or assume a specific cloud provider (GCP, AWS, Azure). Local-first (kind, k3s, bare SBCs) is the default and the only environment CI depends on. Cloud-specific material, if it ever exists, lives in clearly separated distro directories, mirroring the Nephio catalog's `distros/` pattern.
- Control plane is not in the field: the Kubernetes layer provisions and manages the mesh from a powered site; deployed Meshtastic nodes run autonomously once configured and must keep working with the cluster gone. Never design the mesh to depend on the control plane at runtime.
- Air-gapped and offline is a first-class path, not an afterthought: prefer designs that can mirror every image, pre-provision nodes offline, and never require network-from-the-field or a default key. Do not add a hard dependency on an internet-only service.
- Cross-platform development: contributors may be on Windows, macOS, or Linux (the maintainer's dev machine is Windows). Everything hardware-free must work from any of the three via Docker Desktop plus kind or k3d, with WSL2 as the recommended Windows path. Scripts assume a POSIX shell (Git Bash or WSL2 on Windows); never write Windows-only or Mac-only tooling. Hardware-attached steps (USB radios, the SDR) are documented for Linux hosts, because that is where devices plug in (a Linux box or the arm64 SBCs), and never block the hardware-free path.
- Upstream-compatible engineering: follow the conventions in `docs/research/nephio-codebase.md` (Go 1.25.x, kptdev fn SDK, condkptsdk pattern, golden tests with testify, Apache-2.0 file headers, DCO sign-off) so code could integrate into the Nephio ecosystem later with minimal rework. We are humbly experimenting with what an extension might look like; planning ahead keeps the door open without presuming anything gets upstreamed.

## External facts agents commonly get wrong

- Meshtastic node config (region, channels, MQTT) is NOT in meshtasticd's `config.yaml` (that file is host and radio config). Node config lives in protobuf prefs, set via the TCP 4403 client API or the Python CLI.
- MQTT JSON topics are lossy and unsupported on nRF52 devices. The protobuf `msh/REGION/2/e/...` ServiceEnvelope topics are canonical.
- HackRF One was discontinued in September 2025; HackRF Pro is the current product. RTL-SDR Blog V4 is end of line.
- The public `mqtt.meshtastic.org` broker is heavily restricted; demos use a private broker.
- Our US-scoped research reads FCC Part 15 as permitting encryption on license-free ISM bands; the half-remembered prohibition is an amateur-radio (Part 97) rule. Never state legality as settled fact in docs or code: hedge it, scope it to the US, call it non-lawyer research, and link the DISCLAIMER. Contributions must not imply legal guidance.
