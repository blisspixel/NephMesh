# NephMesh agent instructions

These instructions apply to any AI coding agent working in this repository.

## What this project is

An independent experimental project applying Nephio-style intent-driven automation (Kubernetes, kpt/Porch, Configuration as Data) to Meshtastic LoRa mesh gateways and SDR spectrum sensing. It is not affiliated with the official Nephio project or the Linux Foundation. We consume Nephio as a third party, the same pattern OpenAirInterface uses for its packages.

## Orientation

- Start with `docs/roadmap.md` (phased plan in dependency order; check phase status before proposing work) and `docs/architecture.md`.
- `docs/research/` holds the sourced research base (Nephio mechanics, Meshtastic automation surface, SDR and Kubernetes device access, prior art, lab setup, terminology and legality). Prefer citing and updating these over re-deriving facts.
- Planned layout: the full target tree (with the phase that creates each part) is in `docs/plans/engineering-conventions.md`: `packages/`, `api/`, `krm-functions/`, `operators/`, `exporters/`, `demo/`, plus `distros/` only if cloud-specific material ever exists. Do not create a directory until its roadmap phase starts.

## Writing and output style

- No emojis anywhere: code, docs, commit messages, comments.
- No em dashes in prose. Use commas, colons, parentheses, or separate sentences.
- No AI attribution of any kind: no "Generated with", no "Co-Authored-By" AI trailers, no tool names in commit messages or file headers.
- Plain, factual prose. Detailed and well researched, but humble: this is an experiment, so write "explores" and "would be", not product-launch language. Cite sources with links in research docs.
- The README stays short and links into docs/ rather than duplicating them. When a roadmap phase demo lands, capture it (screenshot or terminal capture) and add it to the README Status section and the release notes.

## Code quality bars

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
- Cross-platform development: contributors may be on Windows, macOS, or Linux (the maintainer's dev machine is Windows). Everything hardware-free must work from any of the three via Docker Desktop plus kind or k3d, with WSL2 as the recommended Windows path. Scripts assume a POSIX shell (Git Bash or WSL2 on Windows); never write Windows-only or Mac-only tooling. Hardware-attached steps (USB radios, the SDR) are documented for Linux hosts, because that is where devices plug in (a Linux box or the arm64 SBCs), and never block the hardware-free path.
- Upstream-compatible engineering: follow the conventions in `docs/research/nephio-codebase.md` (Go 1.25.x, kptdev fn SDK, condkptsdk pattern, golden tests with testify, Apache-2.0 file headers, DCO sign-off) so code could integrate into the Nephio ecosystem later with minimal rework. We are humbly experimenting with what an extension might look like; planning ahead keeps the door open without presuming anything gets upstreamed.

## External facts agents commonly get wrong

- Meshtastic node config (region, channels, MQTT) is NOT in meshtasticd's `config.yaml` (that file is host and radio config). Node config lives in protobuf prefs, set via the TCP 4403 client API or the Python CLI.
- MQTT JSON topics are lossy and unsupported on nRF52 devices. The protobuf `msh/REGION/2/e/...` ServiceEnvelope topics are canonical.
- HackRF One was discontinued in September 2025; HackRF Pro is the current product. RTL-SDR Blog V4 is end of line.
- The public `mqtt.meshtastic.org` broker is heavily restricted; demos use a private broker.
- Encryption on license-free ISM bands is legal in the US (FCC Part 15). The encryption prohibition is an amateur-radio (Part 97) rule.
