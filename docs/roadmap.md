# NephMesh Roadmap

Phases are ordered by dependency, not by calendar. Each phase produces a working, demoable artifact and depends only on the phases before it. No dates or effort estimates: a phase is done when its demo works.

Status: [x] done, [ ] not started. Phase status is noted in each heading.

## Current lab inventory

The maintainer's lab already covers every phase through Phase 6 at $0 incremental cost:

- Dev PC (Windows): runs kind/k3d via Docker Desktop or WSL2 for the management cluster and Porch. All hardware-free work happens here; scripts use a POSIX shell (WSL2 or Git Bash).
- A separate Linux box: hosts the HackRF Pro (USB device work is Linux-hosted by policy; see AGENTS.md cross-platform rules).
- Raspberry Pi and Orange Pi 5: arm64 container hosts. The Orange Pi 5 is a good single-node k3s edge cluster; there is no need to pay for Kubernetes anywhere. The full Nephio sandbox (16 vCPU / 32 GB) is deferred; the slim path (kind + Porch + Config Sync) is the default.
- Meshtastic devices (a couple of boards, currently unplugged): the real-RF mesh for Phase 2 onward. They stay in the drawer until Phase 2.
- HackRF Pro: the spectrum sensor. Receive-only by default; it is not a certified Part 15 transmitter, so over-the-air transmit only in authorized contexts (see Phase 6 note).

Nothing needs to be purchased. Optional later additions: an SPI LoRa HAT or CH341 USB radio if a Pi should itself become a radio gateway (meshtasticd with real RF), and a cheap RTL-SDR if a site needs a dedicated sensor while the HackRF is busy elsewhere.

### How far the roadmap goes with zero devices plugged in

Most of it. Hardware-free on the Windows PC alone: all of Phase 0, all of Phase 1 (simulated radios), all of Phase 3 (packaging and Porch), most of Phase 4 (the operator develops and tests against `meshtasticd -s`; only the serial transport and reboot-timing behavior need a real board), the Phase 7 cellular leg (UERANSIM is a simulator), and all CI forever (a project rule). Devices are required only for: the Phase 2 gate itself, the real-hardware validation slice of Phase 4, Phase 5's second physical site (and even that can be rehearsed with two kind clusters on one PC), and Phase 6's real-RF closed loop. In other words, the boards and the HackRF can stay unplugged until the 0.2 gate is the actual next milestone.

## Ordering rationale

Prove the workload runs in containers first (Phase 1), then replace one-shot config jobs with true reconciliation (Phase 4), scale to multiple sites (Phase 5), close the loop from sensed spectrum back to intent (Phase 6). Two phases branch off Phase 1 and depend only on it, not on each other: real hardware and spectrum sensing (Phase 2) and Nephio-native packaging (Phase 3). Because they are independent, they can be built in either order. In practice packaging (Phase 3) came first because it is hardware-free, while the hardware phase waits for devices to be plugged in. The cellular leg (Phase 7) is independent of 4 through 6 in principle, but it is last because it needs the most compute and adds the least learning until the mesh side works.

The phase numbers are stable identifiers (they name plan files and appear throughout the docs); they are not a strict linear sequence you must follow integer by integer. The dependency graph, not the numbering, is the build order.

## Version path

Each phase's gate earns a 0.x release. Versions are earned by working demos, not by calendar, and because Phase 2 and Phase 3 are independent, their releases can land in either order (0.3's packaging is already done and its Porch gate is next; 0.2 waits on hardware).

| Version | Gate |
|---|---|
| 0.1 | Phase 1 demo: a virtual mesh node deployed, configured, and torn down declaratively (passed 2026-08-04) |
| 0.2 | Phase 2 demo: intent drives real radios; the mesh is visible in sensed spectrum (waits on hardware) |
| 0.3 | Phase 3 demo: packages consumable by a stock Porch install (this repo registered as a catalog). Packaging and specialization resources done and render-validated; the end-to-end Porch run is the remaining gate |
| 0.4 | Phase 4 demo: the MeshtasticNode operator reconciling drift on real hardware |
| 0.5 | Phase 5 demo: two sites managed from one Git repo with per-site specialization |
| 0.6 | Phase 6 demo: closed loop from sensed occupancy to reconciled channel change |
| 0.7 | Phase 7 demo: cellular outage fails over to mesh and back |
| 1.0 | See below |

1.0 means someone who is not the maintainer can succeed with it:

- A stranger can reproduce the Phase 1 through 5 demos from the repo alone on a clean machine (no hardware required through Phase 1 and 3; documented hardware list for the rest)
- The `MeshtasticNode` CRD API is stable (no breaking changes planned) and versioned accordingly
- Packages install against a stock Nephio release without patches, and the pinned release is documented
- CI runs the simulated-radio test suite green on every commit
- Docs cover install, upgrade, and teardown, not just the happy path

Phases 6 and 7 are not 1.0 gates: the closed loop and the cellular leg are research features and can mature after 1.0.

## What research cannot answer (validate by testing)

Desk research is done; it was enough to order the phases and pick the tools. The remaining unknowns are empirical and are resolved inside the phases that touch them:

- Phase 1, answered 2026-08-04: `meshtasticd --sim` behaves well under pod restarts with a PVC (prefs persist, MQTT reconnects unaided). Two new findings: TCP readiness probes force-close the single-client device API, and the MQTT thread starts only at boot, so applies end with an explicit reboot. Still open: Meshtasticator multi-node in containers (stretch).
- Phase 2: does `generic-device-plugin` expose USB cleanly on arm64 k3s on the Orange Pi (there is an open issue about devices not mounting in some environments)? Do `hackrf_sweep` and SoapyHackRF work unchanged against the HackRF Pro (newer hardware than most published containers target)?
- Phase 3: how small can the slim Porch path actually go on the dev PC?
- Phase 4: how disruptive are per-section config reboots in practice, and how good can minimal-diff reconciliation get?

If one of these fails, the research docs list fallbacks (Akri or host-level SoapySDRServer for device access; plain serial sidecars instead of the device plugin; single-node simulation instead of Meshtasticator).

## Phase 0: Foundations (complete; one optional item left)

Goal: a repo worth contributing to.

- [x] Research pass: Nephio extension mechanics, Meshtastic automation surface, containerized SDR, prior art, terminology and legality (`docs/research/`)
- [x] Nephio codebase conventions deep-dive so our code stays upstream-compatible (`docs/research/nephio-codebase.md`)
- [x] README, architecture sketch, this roadmap
- [x] AGENTS.md and CLAUDE.md for AI coding agents
- [x] Detailed plans: Phase 1, Phase 2, CRD API design, engineering conventions (`docs/plans/`)
- [x] Resolve the open decisions flagged in `docs/plans/*.md` (annotated in each plan's open-decisions section, 2026-08-04: owner is blisspixel per the git remote, deletionPolicy enum adopted, secrets story chosen, SPDX/DCO/lib direction set; the nephmesh.io domain check remains a hard precondition for Phase 4)
- [x] Repo scaffolding: CONTRIBUTING, SECURITY, CHANGELOG, license header and style checks (`hack/`), CI workflow, root Makefile, line-ending and ignore rules (kpt render checks arrive with Phase 3 packages)
- [ ] Optional: email brand@linuxfoundation.org to sanity-check the name (low risk: "NephMesh" does not contain the "Nephio" mark, and the README carries a prominent disclaimer)

## Phase 1: Virtual mesh on a single-node cluster ($0) (complete, v0.1.0)

Goal: the smallest end-to-end declarative pipeline. No radios, no Nephio yet. Runs on k3s (Orange Pi 5) or k3d/kind (PC).

- [x] Deployment for `meshtastic/meshtasticd` in simulation mode (`--sim`: official image, real firmware, no radio, device API on TCP 4403); explicit `command` because the image has no entrypoint (validated)
- [x] Persistent volume at `/var/lib/meshtasticd` (the `--fsdir` state root; prefs at `<fsdir>/prefs/`, validated) with a stable `--hwid` for deterministic node identity
- [x] Declarative node config: desired state as YAML (the CLI `--export-config` / `--configure` format), applied by an idempotent Job over TCP (export, subset-compare, apply only on drift, verify after the post-apply reboot)
- [x] In-cluster Mosquitto broker; MQTT module enabled (protobuf topics observed as `msh/2/e/...` on firmware 2.7.26; JSON topics on for demo readability)
- [x] Pipeline validated end to end in Docker against the real image: sendtext reached both protobuf and JSON MQTT topics (see `docs/plans/phase-1-virtual-mesh.md` section 10)
- [x] The 0.1 gate: the full demo script passed on a kind cluster on 2026-08-04, including the idempotency re-run; persistence across pod restarts (V1) also validated (see `demo/phase1/README.md`, gate result)
- [x] Stretch (attempt-and-record per plan OD4): Meshtasticator attempted headless in containers 2026-08-04; script mode needs a terminal emulator and an X display, and under Xvfb plus xterm the spawned nodes refused connections. Recorded in `docs/research/meshtastic.md`; CI stays single-node simulation. Revisit via its Docker mode or upstream headless support

## Phase 2: Real radios and spectrum sensing ($0, uses owned hardware) (not started; depends only on Phase 1)

Goal: the same pipeline drives physical RF, and the mesh is visible in the sensed spectrum.

- [ ] Attach the owned Meshtastic boards (USB serial to the Pi or PC); extend the config pipeline to physical nodes (Python CLI over serial/TCP, same YAML desired-state format)
- [ ] USB device access on k3s via `squat/generic-device-plugin` (advertise devices by USB vendor:product ID, no privileged pods); document host prep (udev rules)
- [ ] Spectrum sensor container using the HackRF Pro, receive-only: `hackrf_sweep` or `soapy_power` sweeping 902 to 928 MHz (SoapySDR keeps the container hardware-agnostic for future RTL-SDR sites)
- [ ] Exporter: parse sweep CSV into per-band aggregate Prometheus gauges (occupancy percent, max/mean dB, not per-bin series). Research found no existing exporter for sweep data; this is novel glue
- [ ] Demo: a Git intent change (for example modem preset LongFast to MediumSlow) propagates to the physical boards; a message crosses real RF; the mesh's own transmissions appear in the occupancy metrics

## Phase 3: Nephio-native packaging ($0) (in progress; packaging done, Porch gate open; depends only on Phase 1)

Goal: the Phase 1 workload becomes a proper Nephio-consumable catalog (the Phase 2 sensor blueprint joins later, when that hardware phase lands).

- [x] Convert to kpt packages: `mesh-gateway` and `mqtt-bridge` blueprints with Kptfile pipeline (set-namespace from package-context, set-labels), `package-context.yaml`, and a placeholder `WorkloadCluster` on the gateway (pkg-example-bp pattern). Both render clean against kpt v1.0.0-beta.67; a render gate (`make check-packages`) is wired into CI. The `spectrum-sensor` blueprint is deferred with Phase 2 (hardware)
- [x] `PackageVariant` and `PackageVariantSet` examples specializing the gateway per site, plus a Porch registration and propose/approve guide (`docs/guides/porch-registration.md`, `packages/examples/`)
- [ ] Slim management path on the PC: kind + Porch + Config Sync (the full Nephio sandbox needs 16 vCPU / 32 GB and stays optional)
- [ ] The 0.3 gate: register this repo with Porch (`porchctl repo register`), then approve a proposed PackageRevision and have it deploy a configured mesh gateway. Needs a running Porch install; the packages and specialization resources are render-validated and derived from the passing Phase 1 demo, but the end-to-end Porch run is not yet done

## Phase 4: The Meshtastic operator ($0)

Goal: true reconciliation instead of one-shot config jobs. The most broadly useful deliverable: no Meshtastic Kubernetes operator exists today.

- [ ] `MeshtasticNode` CRD: region, role, modem preset, channels (PSKs via Secrets), MQTT module, owner info
- [ ] Controller reconcile loop using the Meshtastic Python API (TCP 4403): export live config, diff against spec, apply only drift (each applied section reboots the node, so minimal diffs matter)
- [ ] Status conditions: reachable, config in sync, mesh neighbor count, last-heard telemetry
- [ ] Remote admin: manage radio-only mesh nodes through a gateway (admin channel, `--dest '!nodeid'`), so one managed gateway can reconcile nodes across the mesh
- [ ] Ship as an `operator` kpt package deployed per cluster (the free5gc-operator / oai-operator pattern)

## Phase 5: Multi-site fan-out ($0)

Goal: more than one site, managed from one place.

- [ ] The Pi or Orange Pi becomes a second k3s cluster, registered as a `WorkloadCluster` on the management cluster
- [ ] `PackageVariantSet` fan-out over `WorkloadCluster` labels (for example `nephmesh.io/site-type: mesh-edge`) with per-site specialization
- [ ] Demo: one Git commit configures both sites; each site runs its own gateway (and sensor where hardware is attached)

## Phase 6: Closed-loop spectrum-aware automation ($0)

Goal: sensed spectrum feeds back into intent.

- [ ] Policy controller: watch spectrum-occupancy metrics; occupancy above threshold on the current channel commits a channel or preset intent change to Git, which reconciles out to the mesh. A miniature analog of an O-RAN xApp loop on commodity hardware
- [ ] Interference for testing comes from the mesh itself (traffic generation, board placement) or from attenuated bench setups. Transmit note: the HackRF Pro is not a certified Part 15 transmitter, so over-the-air transmission happens only in an authorized context (for example under an amateur license, on amateur frequencies, unencrypted). Receive-only covers everything this phase needs
- [ ] Security and resilience experiments: dynamic channel and PSK rotation driven by observed interference, as declared policy reconciled to radios
- [ ] Demo: raise occupancy on the active channel, watch the intent change propagate to the radios without human action beyond the Porch approval

## Phase 7: Hybrid cellular leg (optional, most compute)

Goal: the disaster-resilience story end to end, no cellular hardware.

- [ ] Open5GS or free5GC plus UERANSIM (simulated gNB and UE) via the same package pipeline (roughly 4 vCPU / 8 GB floor; runs on the PC; Nephio's Exercise 1 is the template)
- [ ] Mesh to cellular bridge: MQTT protobuf topics (full-fidelity ServiceEnvelope on the private broker) to a service reachable over the 5G data plane
- [ ] Failover demo: kill the simulated gNB (a "cell outage"); an intent promotes the Meshtastic path; messages keep flowing; restore the cell and traffic bridges back
- [ ] Full reproducible demo environment: management plus edge clusters, one script

## Cross-cutting: agent-native from day one (in progress)

The repo should be as easy for AI agents to work on as for humans, and eventually natural language should be a front door to intent.

- [x] AGENTS.md (and CLAUDE.md pointing to it) with project conventions and the facts agents commonly get wrong
- [ ] `.claude/skills/` for repeatable workflows as they stabilize (for example: scaffold a new blueprint the catalog-pattern way; spin up the Phase 1 virtual mesh; render and validate all packages)
- [ ] Natural language to intent (Phase 3 onward): an agent skill that turns "give site X a mesh gateway on MediumSlow with MQTT uplink" into a proposed PackageRevision in Porch. The human approves via Porch's lifecycle: the LLM proposes, the reconciliation loop enforces, the agent is never the control loop
- [ ] MCP server (Phase 5 onward): expose live mesh and spectrum state (nodes, neighbor counts, band occupancy) as MCP tools so any agent can observe the network and draft intent changes against real data

## Later / open questions

- Propose lessons learned (or packages) upstream; `nephio-experimental` exists for exactly this kind of PoC
- Contribution strategy: once Phase 4 ships, the Meshtastic community is the natural first audience (a working operator plus packages), ahead of the Nephio community; treat the operator as the project's flagship deliverable
- Unexplored upside, deliberately left open: the SDR plus LoRa plus intent-automation intersection likely holds ideas this roadmap has not found yet (spectrum-aware mesh optimization, wider LoRa ecosystems beyond Meshtastic, multi-radio transport abstraction); revisit after Phase 2 gives real data
- Akri instead of generic-device-plugin if dynamic SDR discovery and scheduling becomes a feature rather than plumbing
- Direction finding (KrakenSDR) for locating interference sources
- Multi-technology expansion beyond LoRa: receive-only monitoring of CB (27 MHz), GMRS, and amateur bands is just more spectrum sensing and can land any time; *managing* additional radio services only makes sense where a digital control surface exists (ham digital modes, GMRS data), and never analog CB as a transport (see `docs/faq.md`)
- SigMF IQ capture plus IQEngine for post-hoc analysis of interesting events
- Pin Nephio release: R6 today; watch R7's "modularization and easier onboarding for new use cases", a roadmap item aimed at projects like this one
