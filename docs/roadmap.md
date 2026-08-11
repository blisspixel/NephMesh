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

Each working milestone earns a 0.x release, by demonstrated capability rather than by calendar. Most of the path is hardware-free by design: the flagship operator earns its release validated against a simulated device in CI, and only the real-radio and multi-site gates wait on plugging hardware in. The version path was revised after 0.1 so the operator milestone earns its own release rather than making the flagship wait on hardware; the phase numbers (Phase 1 through 7) are stable identifiers and are not the same sequence as the version numbers.

| Version | Milestone |
|---|---|
| 0.1 | Phase 1 demo: a virtual mesh node deployed, configured, and torn down declaratively (shipped 2026-08-04) |
| 0.2 | The `MeshtasticNode` operator (Phase 4 software): reconciles region, modem preset, role, MQTT (with a broker password read from a Secret), and owner against a live `meshtasticd --sim` in CI; shipped as a kpt package; hardened with an envtest controller tier and assume-breach tests (shipped 2026-08-05) |
| 0.3 | Phase 3 demo: packages consumable by a stock Porch install (this repo registered as a catalog). Packaging and specialization resources done and render-validated; the end-to-end Porch run is the remaining gate |
| 0.4 | Real radios (Phases 2 and 4 on hardware): intent drives physical Meshtastic boards, the operator reconciles drift on a real device, and the mesh is visible in sensed spectrum |
| 0.5 | Phase 5 demo: two sites managed from one Git repo with per-site specialization |
| 0.6 | Phase 6 demo: closed loop from sensed occupancy to reconciled channel change (a hand-driven bench proof-of-concept ran 2026-08-09, see `demo/closed-loop/`; the 0.6 gate is the safe, autonomous form behind ADR 0002) |
| 0.7 | Phase 7 demo: cellular outage fails over to mesh and back |
| 1.0 | See below |

1.0 means someone who is not the maintainer can succeed with it:

- A stranger can reproduce the Phase 1 through 5 demos from the repo alone on a clean machine (no hardware required through Phase 1 and 3; documented hardware list for the rest)
- The `MeshtasticNode` CRD API is stable (no breaking changes planned) and versioned accordingly
- Packages install against a stock Nephio release without patches, and the pinned release is documented
- CI runs the simulated-radio test suite green on every commit
- Docs cover install, upgrade, and teardown, not just the happy path (the operations runbook, `docs/guides/operations.md`, covers declaring a node, observing it through conditions/events/metrics, troubleshooting, and day-2 key rotation, decommission, and upgrade; the one gap it names honestly is that the operator image is not yet published to a registry, so install still builds it)

Phases 6 and 7 are not 1.0 gates: the closed loop and the cellular leg are research features and can mature after 1.0.

## What comes next (recommended order, reviewed 2026-08-09)

The recent hardware milestones (the 0.4 gate essentially demonstrated, plus a Phase 6 mechanism proof of concept) put the engineering ahead of the packaging and the proof. A deliberate multi-perspective review (research, engineering, creative, product) converged on one conclusion: the next work is to make what exists installable and to prove the core claim, not to add capability. The intent-layer frontier below stays frozen at the shipped report-only compiler.

In order:

1. **Publish the operator image**, signed and digest-pinned, with an SBOM (the Phase 4 release step and a named 1.0 gap). It unblocks reproducible install, every demo, and a first user, and it is hardware-free. Land the supply-chain hardening (trivy/hadolint/pip-audit, SBOM, signing, digest-pinned bases) with it rather than splitting them.
2. **Prove control-plane independence**: bring up a configured mesh, delete the entire management cluster, and show the message delivery ratio is unchanged (the 0.5 / Phase 5 proof). This is the single most load-bearing unproven claim; until it runs, "resilient" is asserted, not shown. A first hardware-free demonstration landed (2026-08-10): `demo/resilience` stands up a multi-node UDP-multicast sim mesh (the data plane), has the operator's real reconcile loop configure a node (the management plane), measures delivery, then destroys the management plane mid-run; delivery held at 100% before and after, reduced to a before/after verdict by the pure, tested `internal/resilience` (`nephmeshctl resilience`). That builds the reusable delivery-ratio measurement harness Phase 6 and 7 need and operationalizes the claim on a sim substrate. What remains for the full proof: the two-kind-cluster form with an SBC as the surviving second cluster, and the carrier-outage MDR on real hardware.
3. **Finish the core on hardware and in the observability plane**: confirm role and channels round-trip on the physical board (the last 0.4 piece, bench time, and channels apply through a distinct path so the sim is weaker evidence there); the spectrum exporter is now wired onto Prometheus in-cluster (the `packages/spectrum-exporter` kpt package with a `ServiceMonitor` and report-only rules, simulation-first so it proves the integration hardware-free, landed 2026-08-10), leaving only the device-plugin USB host for live in-cluster capture; and pair server-side apply for status with the k8s v0.36 / controller-runtime v0.24 upgrade so the correctness fix and the version bump touch the status path once.
4. **Close the 0.3 Porch gate** end to end (register, propose, approve, pull, apply), the credibility proof that Nephio-native is real rather than aspirational. Hardware-free (WSL2).
5. **Make the claims independently verifiable**: a one-take "three witnesses" capture (the intent change, the reconcile loop, and the SDR peak physically moving, with the airtime agreeing across the model, the radio's telemetry, and the sensed spectrum); a Grafana panel over the existing `nephmesh_spectrum_*` and node exporters; and a short agent-driven-planning recording. The substance of the first already exists on disk.
6. **Then, and only as research behind the core**: spectrum classification (mine versus neighbor versus jammer) built on a shared labeled IQ corpus, so the loop reads cause and not merely occupancy (and its honest deliverable is a false-positive curve, not a jammer-detector claim). An experimental first slice exists (`internal/spectrum.Classify` separates packet-shaped LoRa from continuous/wideband interference by time-frequency shape, and `nephmeshctl advise` has a local LLM reason over it, report-only and guarded, validated on real hardware including an edge model on the Jetson; see `demo/edge-advisor/`); the labeled IQ corpus, frame decode for mine-versus-neighbor, and the false-positive characterization remain. plus the cheap, decisive firmware probes several designs currently assume unverified (whether a per-node identity key can sign an application payload for message auth, whether the frequency slot derives only from the primary channel name for epoch channels) and a rebroadcast-aware airtime model measured on a multi-node mesh so `AirtimeWithinBudget` can graduate from an advisory floor to an enforceable budget.

Held frozen until the above lands: actuation from `CommunicationIntent`, the autonomy stack (safety kernel, signed capsules, degrading lease, rejoin), an enforceable `ChannelBudget` admission gate, a second radio driver, and Phases 6 and 7. Over-scoping into that frontier before the core is packaged and proven is the doctrine's named first failure mode; the report-only compiler and the closed-loop proof of concept already signal the direction convincingly.

## Design direction: intent as an outcome envelope (research frontier, gated behind the core)

There is a larger shape this project could grow into, written up in full in the
[design doctrine](design/doctrine.md) and recorded as decisions in
[docs/adr/](adr/). For the sequenced, gated build order from what exists today to
safe autonomy, with the decisive split between the minimal viable safety core and
the research frontier, see [`design/road-to-safe-autonomy.md`](design/road-to-safe-autonomy.md);
it turns this direction into an order of operations with explicit gates. It is a
direction, not a plan with dates, and almost none of it is
built. It is here so that later work does not quietly collapse the idea back into
automatic knob-turning, and so the ordering stays honest.

The reframe, in one sentence: NephMesh should reconcile a communications fabric
toward a bounded set of viable mission outcomes, not merely reconcile individual
radios to a fixed configuration. In RFC 9315 terms, "configure this radio as
LONG_FAST in US915" is configuration; "preserve delivery of life-safety traffic while
holding an emergency airtime reserve" is intent. The operator that exists today is the
configuration layer. The direction is to add an intent layer above it, so that
`MeshtasticNode` becomes the compiled output of a higher-level `CommunicationIntent`
rather than the source of truth (ADR 0001), and to define signed-autonomy and rejoin
semantics before the Phase 6 closed loop rather than after (ADR 0002).

This changes nothing about what comes next. It sharpens why the core comes first. The
rock-solid core, the thing that has to be exceptional before any of the frontier earns
its complexity, is a tight list: complete channel and PSK support on real hardware
(the last flagship config surface, Phase 4), multi-site packaging and fan-out
(Phase 5), day-2 key and channel rotation, and reproducible sim-plus-physical demos
that make the operational win visible. Ship that before expanding surface area. The
honest boundaries are stated plainly in the doctrine: physics caps the useful envelope
at tens to low hundreds of nodes for contingency traffic, the audience is
organized-operator resilience rather than hobby use, and over-scoping is the main way
this fails.

When the core is solid, the frontier arrives in this disciplined order (each step
elaborated in the doctrine), report-only before anything actuates, safety before
autonomy, rejoin before broad autonomy:

1. Write down the doctrine and invariants (this section, the doctrine, the ADRs; done).
2. A `CommunicationIntent` API in report-only mode: parse objectives, evaluate
   feasibility, emit proposed `MeshtasticNode`s and a `ChangePlan`, explain, never
   actuate.
3. Make disruption explicit: field-level planned deltas, last-known-good storage,
   estimated reboot and airtime cost, rollback, rate and dwell limits.
4. Mission traffic classes and a `ChannelBudget` scoped by interference domain, so the
   existing channel behaves better before any frequency is changed. This is the same
   airtime-invariant work already listed first in the backlog below, now with a
   commons framing.
5. A site steward (a small deterministic state machine, not an agent) with L1
   non-disruptive actions only.
6. An independent, Simplex-style runtime safety kernel that can veto any action.
7. One L2 action: rollback-to-last-known-good, before any autonomous channel switch.
8. The detached-epoch and rejoin protocol, before broad autonomy.
9. Model-check the authority and rejoin state machine (TLA+ or PlusCal).
10. Learning last, in shadow mode, unable to define hard constraints or invent actions.

The Phase 6 closed loop in this roadmap is step 7-and-beyond of that sequence, not a
standalone next step; ADR 0002 makes the prerequisite explicit. The full build queue,
core first, is the Order of operations directly below.

## Order of operations

One dependency-ordered build queue, from where the project is now (0.2.0 plus the
observability, airtime-model, and doctrine work just landed) to the research frontier.
No dates and no effort estimates: an item is ready when the items it depends on are
done, and done when its check passes. The spine below is ordered; the parallel tracks
at the end have no dependency on the spine and can be built whenever there is attention
for them.

The one priority rule, from the doctrine: the core (stages 1 through 8) must be
rock-solid before the intent-layer frontier (stage 9 onward) earns its complexity.
That is a sequencing rule, not a throttle. The hardware-free work can move as fast as
it can be made correct, and most of this is hardware-free.

### Core spine (in order)

1. Channels and PSKs on real hardware, which finishes Phase 4. Decode the device's
   channel set (the Meshtastic library exposes it per index over USB and TCP), compare
   by PSK hash so raw keys never move through stdout, apply drift through a distinct
   channel path (the export encodes channels as a single `channel_url`, not discrete
   fields), resolve PSKs from Secrets through the existing redacting path, and
   round-trip every channel field against `meshtasticd --sim` and the T-Deck. This is
   the flagship's last configuration surface and the concrete secure-private-channels
   feature. Status: done in software and validated against `meshtasticd --sim` across
   the three slices (hash-based diff, the observational `ChannelsInSync` condition, and
   the converging per-channel apply with keys kept off the command line); the only
   piece left is a destructive on-hardware confirmation, which the sim safely stands in
   for. See Phase 4 for detail.
2. Richer, mission-aware status conditions. Keep today's device-management signals
   (Reachable, ConfigInSync, RebootPending, Ready) and add the mission-level ones the
   doctrine names (MissionViable, IntentionallyQuiet, MessageCustodyHealthy) using
   three-valued healthy/unhealthy/unknown reasoning, so an intentionally quiet node
   never reads as a failure. Design: [contingency semantics and silence-as-a-mode](plans/contingency-semantics-dtn.md).
3. Day-2 operations, the part real deployments live in and thin today: coordinated
   channel and PSK rotation as declared policy (rotate at a scheduled epoch so the
   change is atomic across the fleet), a fleet firmware-upgrade path, and node
   decommission (the Wipe deletionPolicy). Design: [coordinated key rotation and epoch-keyed channels](plans/key-rotation-and-epochs.md).
4. Publish the operator image to a registry with a pinned digest, so the operator kpt
   package deploys end to end (a release step Phase 4 already lists).
5. A reproducible demo path: a stranger can run the Phase 1 mesh and the operator
   reconcile (sim, and hardware where present) from the repo alone on a clean machine,
   scripted. A 1.0 gate, and the thing that makes the operational win visible rather
   than asserted. The operator half is landed as `demo/operator/`: a scripted,
   hardware-free run that declares intent, converges a simulated radio including a
   secure channel, and proves idempotence, with the real transcript in its README.
6. A `ChannelBudget` resource with an enforced admission gate: predict a change's
   airtime cost with the existing time-on-air model, scope the budget to an
   interference domain (operator-declared at first), refuse a fleet change that
   oversubscribes it, and hold an emergency reserve. This turns airtime from an
   observed number into a governed invariant, the clearest thing a declarative
   system would offer over hand-tuning, and the Phase 6 prerequisite. Design: [airtime budget](plans/airtime-budget.md).
   First step shipped (2026-08-08): the intent compiler estimates a fleet's channel
   utilization from declared `expectedTraffic` and reports an `AirtimeWithinBudget` verdict
   (a conservative floor that ignores rebroadcast, so over-budget is authoritative). This is
   the observed-to-governed move in report-only form; refusing an oversubscribing change and
   holding a reserve is the remaining enforcement step.
7. Mission traffic classes and reserve accounting: protected shares per class, airtime
   debt when an emergency borrows the reserve, and reconfiguration-airtime accounting
   so a remote change budgets its own admin traffic. Design: [traffic classes and degraded modes](plans/contingency-semantics-dtn.md).
8. Multi-site and the load-bearing resilience proof (Phase 5): a second cluster as a
   `WorkloadCluster`, `PackageVariantSet` fan-out with per-site specialization, then
   the control-plane-independence test, bring up both configured sites, delete the
   entire management cluster, and show message delivery ratio is unchanged. Until that
   passes, "the control plane is not a runtime dependency" is unproven, and it is the
   crudest, most important form of management-plane dissolution.

### Intent-layer frontier (only after the core spine is solid)

9. A `CommunicationIntent` CRD and a compiler in report-only mode: parse objectives and
   constraints, evaluate current feasibility, emit proposed `MeshtasticNode`s and a
   `ChangePlan`, report `IntentInfeasible` honestly, and never actuate. Here
   `MeshtasticNode` formally becomes the compiled artifact and ADR 0001 moves from
   Proposed to Accepted. Design: [CommunicationIntent and the report-only compiler](plans/communication-intent.md).
   Shipped (report-only), 2026-08-08: the `intent.nephmesh.io/v1alpha1`
   `CommunicationIntent` CRD and a pure compiler (`internal/intent`) that renders proposed
   `MeshtasticNode` specs on status with a `Feasible` verdict, selects the preset from an
   approved set, and reports a fleet-wide `AirtimeWithinBudget` verdict (the fleet airtime
   check only the intent layer can make). Report-only is enforced by RBAC (no create/write
   on `MeshtasticNode`) and by an envtest admission suite, not just intended. Still open
   before ADR 0001 moves to Accepted: the `ChangePlan` artifact (item 10) and the richer
   feasibility surface. Also exposed to agents offline via `nephmeshctl plan` and the
   `nephmesh-mcp` MCP server (`docs/guides/agent-interface.md`).
10. A `ChangePlan` resource and least-change actuation: field- or section-level deltas,
    a last-known-good cache, a disruption score, rate and dwell limits, and a rendezvous
    procedure for channel and preset changes (an announced switch epoch and predeclared
    fallback, because a radio channel change can isolate an ordinary canary from the
    mesh).
11. A signed, content-addressed Intent Capsule the edge can act on with no connectivity,
    with a degrading lease (expiry only ever narrows authority) and clock-uncertainty
    behavior. Design: [signed autonomy and the safety kernel](plans/signed-autonomy-and-safety-kernel.md), which also covers stages 12 to 14.
12. A site steward, a small deterministic state machine, running L1 non-disruptive
    actions only (shed telemetry, aggregate reports, prioritize queues, enter a declared
    quiet mode).
13. An independent, Simplex-style safety kernel that can veto any proposed action
    against the signed constraints.
14. The first L2 autonomous action: rollback-to-last-known-good, proven before any
    autonomous channel switch.
15. The detached-epoch and rejoin protocol (a treaty, not drift correction): epoch
    comparison, classification of local divergence, staged change plans, queue drainage
    under budget, and a new signed epoch that revokes the old. Design: [rejoin as a treaty and self-stabilization](plans/rejoin-and-self-stabilization.md), which also covers stage 16.
16. Model-check the authority and rejoin state machine (TLA+ or PlusCal); ADR 0002 moves
    to Accepted.
17. Phase 6 closed loop: sensed occupancy proposes a channel change that the safety
    kernel authorizes and Porch approves, with anti-herding from the start (randomized
    candidate selection, site-specific tie-breaking, dwell, cooldown, and treating
    "every candidate is congested" as evidence of jamming). The first actuating L2
    channel switch lives here. A hand-driven mechanism proof-of-concept ran on the
    bench (2026-08-09, `demo/closed-loop/`): the SDR sensed the mesh's channel peak
    at -17.2 dB, a threshold policy decided to relocate, the operator reconciled the
    preset (which moved the channel slot), and a second sweep confirmed the mesh
    peak had moved from 906.5 to 903.5 MHz, then restored it. That proves the
    mechanism; what item 17 adds is the safety envelope (the kernel, Porch approval,
    and anti-herding) that lets it run unattended, which is the actual gate.
18. Phase 7 hybrid backhaul: a cellular outage fails over to the mesh and back, reported
    as message delivery ratio and time-to-failover, with UERANSIM standing in for the
    RAN.
19. Learning last, in shadow mode only, unable to define a hard constraint or invent an
    action.

### Parallel tracks (no dependency on the spine)

- Nephio packaging gate (Phase 3, the 0.3 milestone): register the repo with Porch and
  run propose/approve/pull/apply. Hardware-free; needs WSL2. Independent of the spine.
- Hardware enablement (Phase 2, and the RF-real half of 0.4): the USB device plugin on
  k3s, the HackRF receive-only sweep exporter, and the real-RF demo. Gated only on
  plugging boards and the SDR in, not on the software spine.
- The coherence-metrics vector: grow the current readiness and airtime gauges into the
  mission, commons-protection, control-stability, epistemic, and human-coherence
  measures the doctrine lists, demoting readiness from the definition of success to one
  input among many.
- Continuous hardening: server-side apply for status, the NetworkPolicy cross-namespace
  proof, reproducible-build flags and digest-pinned base images, trivy/hadolint/pip-audit,
  then SBOM and signing at image publish; the deliberate k8s v0.36 and
  controller-runtime v0.24 upgrade; agent-native skills and an MCP server (an offline
  first slice shipped 2026-08-08: `nephmeshctl` and `nephmesh-mcp` expose the report-only
  compiler as a CLI and an MCP tool; the richer live-state MCP layer remains ahead); and the
  regulatory matrix kept as a living reference.

### Is everything captured?

Close, and now closer. The phases, the version path, the doctrine's ten-step frontier,
and the research backlog were all already present; this section threads them into one
order and pulls a few doctrine-derived items that were only implicit into explicit
lines: the mission-aware status conditions (stage 2), day-2 rotation and decommission
elevated into the near-term core (stage 3), the enforced `ChannelBudget` (stage 6), the
MessageIntent and degraded-mode semantics (folded into stages 2 and 7), and the
coherence-metrics vector (a parallel track). The deliberately-open entries in "Later /
open questions" below are real research forks, not omissions: they are things to revisit
when the phases that touch them produce data.

## Resilience, defined (so "survives" is measurable, not a slogan)

"Secure communication survives when there is no carrier" is only meaningful if it is measurable, so the project commits to concrete metrics rather than an adjective:

- **Message delivery ratio (MDR) during a simulated outage:** with the carrier path killed, the fraction of messages sent that are received across the mesh. The failover demos report a number, not "it worked."
- **Time to failover:** wall-clock from carrier loss to the first message delivered over the mesh path.
- **Control-plane independence:** the mesh's MDR must not change when the Kubernetes control plane is killed. This is the load-bearing claim, that the control plane provisions but is not a runtime dependency, and it gets its own validation: bring up a configured mesh, delete the entire management cluster, and show messages keep delivering. Until that test passes, "resilient" is unproven. A first hardware-free demonstration landed 2026-08-10 (`demo/resilience/independence.sh`, MDR unchanged across a killed management plane); see "What comes next" item 2.
- **Airtime-commons survival:** the shared channel has a finite airtime budget; offering traffic beyond it collapses MDR, and admission control (pacing offered load back within the budget) restores it. Demonstrated hardware-free 2026-08-10 (`demo/resilience/survival.sh`): baseline (within budget) 100%, degraded (over budget) 50%, adapted (paced back) recovered, reduced to a per-phase verdict by the pure, tested `internal/resilience.ReducePhases`. It grounds the airtime model against measured delivery (the over-budget verdict is authoritative) and records the honest finding that the recovery lever in the sim is admission control, not a preset change (the per-node send rate is capped by a firmware broadcast cadence, not the PHY airtime).

These metrics become gate criteria for Phases 6 and 7 and appear in the 1.0 definition below.

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
- [x] Resolve the open decisions flagged in `docs/plans/*.md` (annotated in each plan's open-decisions section, 2026-08-04: owner is blisspixel per the git remote, deletionPolicy enum adopted, secrets story chosen, SPDX/DCO/lib direction set; the `nephmesh.io` API group is a provisional placeholder, kept at v1alpha1 so a rename stays cheap, and revisited only before a public or 1.0 release; it does not block Phase 4)
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
- [ ] Spectrum sensor container using the HackRF Pro, receive-only: `hackrf_sweep` or `soapy_power` sweeping 902 to 928 MHz (SoapySDR keeps the container hardware-agnostic for future RTL-SDR sites). Capture path built and validated on real hardware (2026-08-09): a receive-only capture helper (`hack/spectrum-sweep.sh`, integrating several seconds of `hackrf_sweep` passes) and its analysis ran end to end against a HackRF on a Jetson Orin Nano over SSH, the 915 band reading a stable ~32 to 34 percent occupancy; runbook at `docs/guides/spectrum-validation.md`. Remaining: package it as a container with `squat/generic-device-plugin` USB access (still needs an in-cluster device host) rather than the current host-tools-over-SSH form
- [x] Exporter: parse sweep CSV into per-band aggregate Prometheus gauges (occupancy percent, peak/noise dB, not per-bin series). Research found no existing exporter for sweep data; this is novel glue. Built and validated on real hardware (2026-08-09): `cmd/spectrum-exporter` runs `hackrf_sweep` on a loop, reduces via `internal/spectrum`, and serves `nephmesh_spectrum_*` gauges (occupancy, peak dBm, peak freq, noise floor) on `/metrics`; cross-compiled for arm64 and run on the Jetson against the HackRF Pro, `curl /metrics` returned live per-band values with uncovered bands correctly absent. In-cluster wiring landed (2026-08-10): `packages/spectrum-exporter` is a hardened, receive-only kpt package with a Deployment, a `ServiceMonitor` so the Prometheus Operator scrapes it, and a report-only `PrometheusRule` (no-sensor, stale-sweep, sweep-error, band-congested). It is simulation-first, like the mesh-gateway node: the Deployment runs with a new exporter `-replay` flag over a bundled sample sweep, so it publishes the same `nephmesh_spectrum_*` series with no radio, proves the Prometheus integration hardware-free in `kind`, and passes the manifest security gate. Remaining: the live in-cluster capture still needs a device-plugin USB host (a live sweep needs the SDR mounted, which the gate forbids in a shipped manifest), so on-host capture-and-scrape stays the real-sensor path for now
- [ ] Demo: a Git intent change (for example modem preset LongFast to MediumSlow) propagates to the physical boards; a message crosses real RF; the mesh's own transmissions appear in the occupancy metrics. The spectrum-visibility half is validated (2026-08-09): with a Meshtastic T-Deck transmitting on its US LongFast channel and a HackRF Pro sensing 902-928 MHz, the mesh appeared as a 24 dB peak (idle -40.9 dB to -17.1 dB) at 906.5 MHz, exactly the LongFast channel frequency, with 1367 strong (>-30 dB) hits there during the burst versus 0 idle. Two findings: per-band peak power, not aggregate occupancy, is the sensitive detection signal (occupancy barely moved, since one transmitter lights up one channel), and the peak landing at the correct frequency confirms the post-firmware-update mapping is honest. The actuation-and-sensing link is now demonstrated end to end (2026-08-09): the operator's reconcile loop changed the T-Deck's preset from LONG_FAST to LONG_MODERATE, and the HackRF Pro saw the transmission frequency move from ~906.5 to ~902.5 MHz (the 250 to 125 kHz bandwidth change shifts the channel slot), while the airtime rose in three independent measurements that agree, the operator's model (~559 to ~1642 ms per frame), the radio's own channelUtilization telemetry (0.0 to 3.2 percent), and the sensed spectrum (more strong hits per bin). Intent change caused an observable, correctly-attributed RF change. The last piece of the demo is wrapping this into a Git-driven, scripted run rather than a hand-driven one

## Phase 3: Nephio-native packaging ($0) (in progress; packaging done, Porch gate open; depends only on Phase 1)

Goal: the Phase 1 workload becomes a proper Nephio-consumable catalog (the Phase 2 sensor blueprint joins later, when that hardware phase lands).

- [x] Convert to kpt packages: `mesh-gateway` and `mqtt-bridge` blueprints with Kptfile pipeline (set-namespace from package-context, set-labels), `package-context.yaml`, and a placeholder `WorkloadCluster` on the gateway (pkg-example-bp pattern). Both render clean against kpt v1.0.0-beta.67; a render gate (`make check-packages`) is wired into CI. The `spectrum-sensor` blueprint is deferred with Phase 2 (hardware)
- [x] `PackageVariant` and `PackageVariantSet` examples specializing the gateway per site, plus a Porch registration and propose/approve guide (`docs/guides/porch-registration.md`, `packages/examples/`)
- [ ] Slim management path on the PC (research complete, 2026-08-05): Porch installs standalone on a single kind cluster via its released kpt package (`porch-kpt-package.tar.gz`, v1.6.2), about 1 vCPU and under 1 GB with the default CR cache, no full 16 vCPU / 32 GB Nephio sandbox and no Config Sync needed for the gate. Windows note: there is no `porchctl` Windows binary, so run it from WSL2; a local gitea gives a fully offline loop. Details in the Phase 3 research notes
- [ ] The 0.3 gate: register this repo with Porch (`porchctl repo register`), clone/propose/approve a PackageRevision, then `rpkg pull` plus `kubectl apply` to actuate a configured mesh gateway (Config Sync and a second cluster are only needed for cross-cluster GitOps, out of scope for the gate). Write `PackageVariant` as `config.porch.kpt.dev/v1alpha1` and `PackageVariantSet` as `v1alpha2` (the current served version). Packages are render-validated and derived from the passing Phase 1 demo; the end-to-end Porch run is the remaining work

## Phase 4: The Meshtastic operator ($0)

Goal: true reconciliation instead of one-shot config jobs. The most broadly useful deliverable: no Meshtastic Kubernetes operator exists today.

- [x] `MeshtasticNode` CRD: region, role, modem preset, channels (PSKs via `secretKeyRef`), MQTT module, owner, plus a `connection` oneof (tcp/serial/viaGateway) enforced by a CEL rule, open-string enums for firmware-churn tolerance, and status conditions. Lives in the `api/` Go module (`mesh.nephmesh.io/v1alpha1`); builds, vets, and tests clean (hand-written logic at 100% coverage), with a generated CRD and a `go` CI job
- [x] Controller reconcile loop (`operators/meshtastic-operator`): a non-blocking, reboot-aware state machine (export live config, diff against spec, apply only drift, mark RebootPending, re-verify after the device returns) driven by `RequeueAfter` so the worker never blocks. A CLI-backed device client execs the Meshtastic CLI; the core is fully unit-tested against an in-memory fake and a fake Kubernetes client, every package above the 80% floor (the pure config and reconcile logic near 100%), all hardware-free
- [x] Status conditions: Reachable, ConfigInSync, RebootPending, Ready (plus NodeID, firmware, neighbor count, last-heard), managed with `metav1.Condition` and observedGeneration
- [x] USB serial transport, validated against a real board. First real-hardware contact (2026-08-05): the operator's `Converge` loop ran against a physical Meshtastic T-Deck (firmware 2.7.26, node `!0c3a5f2c`) over USB and reached `Ready` read-only. Real-hardware testing surfaced that the CLI's `--export-config` hangs over serial (it re-requests config over admin messages that time out on the device; `--info` and the connect-streamed config are reliable), so the device client uses a bundled exporter (`hack/mesh-export.py`) reading the connect-streamed config for both serial and TCP
- [x] Apply drift on real hardware (the 0.4 gate's operator half). Verified against the T-Deck (2026-08-05): the operator detected an owner drift, applied it over serial, the device rebooted, and it re-verified to Ready, then restored the original owner. The client treats the USB re-enumeration during a reboot as transient unreachable
- [ ] Verify the remaining fields round-trip over serial on real hardware the way owner does. modemPreset is now confirmed on the physical T-Deck (2026-08-09): the operator's reconcile loop changed the preset from LONG_FAST to LONG_MODERATE (applied drift, device rebooted, re-verified to Ready), then restored it. role and channels remain validated against `meshtasticd --sim` only; a physical-board apply of each is the outstanding hardware confirmation. viaGateway transport is still a graceful "unsupported" that reports unreachable
- [x] Ship as an `operator` kpt package (`packages/meshtastic-operator`): CRD, least-privilege RBAC, ServiceAccount, and Deployment, rendering clean through the render gate (set-namespace correctly namespaces the ServiceAccount and Deployment while leaving the CRD and ClusterRole cluster-scoped and fixing the binding subject). A multi-stage Dockerfile bundles the pinned CLI the operator execs
- [x] Integration test against real firmware: a build-tagged test drives the real `Converge` loop through the CLI-backed device client against a live `meshtasticd --sim`, reaching convergence and reporting the real node id, including a Secret-backed secondary channel reconciled through the same exporter and channel-apply helpers the packaged operator uses (so channel convergence is checked on every commit, not just manually). Runs in a CI job (Docker sim device plus the pinned CLI and helpers) and passes locally in under 20 seconds. This is the "executed against real firmware" bar the fake cannot provide; the controller layer stays covered by fake-client unit tests
- [x] Reconcile modem preset, device role, and owner (long and short name), each field's export path verified to round-trip against `meshtasticd --sim` in the integration test (so it cannot silently never-converge). Region, modem preset, role, MQTT, and owner are now managed
- [x] Read Secrets through a redacting type behind a namespaced RBAC Role (get only, uncached, never a cluster-wide Secret grant), proven by a no-secret-in-logs assume-breach test that shows a sentinel password reaching the device but never a log line or the stored status. The MQTT broker password is reconciled this way (username and password into the device's MQTT module config)
- [x] Reconcile channels (channel PSKs through the same Secret path): the flagship's last configuration surface, done in three slices. The operator projects and diffs channels by key hash (so a key never enters the diff, logs, or status), surfaces a `ChannelsInSync` condition, applies drift through a distinct path (per-channel through a bundled helper that keeps keys in a file, not on the command line, because the CLI's `--configure` is unreliable for channels, meshtastic/python #891), gates `Ready` on channel convergence, and bounds the apply so an unconvergeable channel degrades rather than loops. The default-key shorthand (the single `0x01` byte the device stores) is normalized, so a default channel does not read as permanent drift. Validated end to end against `meshtasticd --sim` (export emits the channel set and apply writes it, both over TCP), covered by the CI integration test on every commit (it reconciles a Secret-backed secondary channel to `Ready` through the same helpers the packaged operator uses), and unit-tested through the fake (apply, reboot, re-verify to converged). Remaining is only a destructive confirm on a physical board, deferred to avoid disrupting the maintainer's device, for which the sim is the safe equivalent
- [ ] Publish the operator image to a registry with a digest so the `operator` package deploys end to end (release step); the image already bundles the exporter and channel-apply helpers

## Phase 5: Multi-site fan-out ($0)

Goal: more than one site, managed from one place.

- [ ] The Pi or Orange Pi becomes a second k3s cluster, registered as a `WorkloadCluster` on the management cluster
- [ ] `PackageVariantSet` fan-out over `WorkloadCluster` labels (for example `nephmesh.io/site-type: mesh-edge`) with per-site specialization
- [ ] Demo: one Git commit configures both sites; each site runs its own gateway (and sensor where hardware is attached)
- [ ] Control-plane-independence validation (the load-bearing resilience claim): with both sites configured and messaging, delete the entire management cluster and show message delivery ratio is unchanged. Proves the control plane provisions but is not a runtime dependency

## Phase 6: Closed-loop spectrum-aware automation ($0)

Goal: sensed spectrum feeds back into intent.

- [ ] Policy controller: watch spectrum-occupancy metrics; occupancy above threshold on the current channel commits a channel or preset intent change to Git, which reconciles out to the mesh. A miniature analog of an O-RAN xApp loop on commodity hardware. Must be built with anti-herding controls from the start (hysteresis, minimum dwell time, rate-limited changes, and treating "every candidate channel is congested" as evidence of jamming rather than a reason to keep hopping), because an attacker who can jam can otherwise drive the loop; see the induced-reconfiguration entry in `docs/security/threat-model.md`. The loop only ever proposes through Porch approval and never keys a transmitter
- [ ] Interference for testing comes from the mesh itself (traffic generation, board placement) or from attenuated bench setups. Transmit note: the HackRF Pro is not a certified Part 15 transmitter, so over-the-air transmission happens only in an authorized context (for example under an amateur license, on amateur frequencies, unencrypted). Receive-only covers everything this phase needs
- [ ] Security and resilience experiments: dynamic channel and PSK rotation driven by observed interference, as declared policy reconciled to radios
- [ ] Demo: raise occupancy on the active channel, watch the intent change propagate to the radios without human action beyond the Porch approval

## Phase 7: Hybrid backhaul leg (optional, most compute)

Goal: the disaster-resilience story end to end, no cellular hardware. Cellular is the concrete first implementation of a broader idea: a Primary backhaul tier that also covers cloud and satellite links (for example Starlink) when they exist, with the mesh as the always-present floor beneath them. UERANSIM stands in for the cellular leg so this is demonstrable without hardware; the same bridge pattern generalizes to other backhaul.

- [ ] Open5GS or free5GC plus UERANSIM (simulated gNB and UE) via the same package pipeline (roughly 4 vCPU / 8 GB floor; runs on the PC; Nephio's Exercise 1 is the template)
- [ ] Mesh to cellular bridge: MQTT protobuf topics (full-fidelity ServiceEnvelope on the private broker) to a service reachable over the 5G data plane
- [ ] Failover demo, reported as numbers not adjectives: kill the simulated gNB (a "cell outage"); an intent promotes the Meshtastic path; measure message delivery ratio during the outage and time to failover; restore the cell and traffic bridges back (see "Resilience, defined")
- [ ] Full reproducible demo environment: management plus edge clusters, one script

## Cross-cutting: exceptional engineering (in progress)

The bar is code a critical-infrastructure or mission user could trust; the full standard and honest gaps are in `docs/CODE-QUALITY-STANDARDS.md`. Landed: golangci-lint (v2.12.2) and an 80% meaningful-coverage gate in CI, native fuzzing of the untrusted-input parsers, the race detector and `govulncheck` in CI (which caught real reachable vulnerabilities, fixed by bumping `golang.org/x/text`/`x/net` and pinning CI to Go 1.25.12), ShellCheck on the gate scripts, a real sim-device integration test, the fix for the production logger defaulting to dev mode, the envtest controller tier (real API server and etcd) proving hostile-CR rejection at admission and reconciler idempotency, assume-breach control-proving tests (the RBAC least-privilege golden test and a no-secret-in-logs proof of the redacting-secret path), and a supply-chain pass: SHA-pinned GitHub Actions and least-privilege workflow tokens enforced by `hack/check-actions.sh`, a checksum-verified kpt download, and Dependabot keeping dependencies current. Prioritized remaining, highest leverage first:

- [ ] Server-side apply for status (the envtest controller tier and a two-call idempotency test are done)
- [ ] Remaining assume-breach control-proving tests: no-secret-in-logs, NetworkPolicy blocks cross-namespace (with a positive control), no default credentials in any shipped package (the RBAC least-privilege golden test and hostile-CR rejection at admission are done)
- [ ] Remaining supply chain: reproducible-build flags and digest-pinned base images, trivy and hadolint and pip-audit in CI, then SBOM and signing when the image is published (SHA-pinned Actions, least-privilege workflow tokens, and the checksum-verified kpt download are done)
- [x] Architecture Decision Records (`docs/adr/`): the record format and index, plus ADR 0001 (intent as an outcome envelope, `MeshtasticNode` as a compiled artifact) and ADR 0002 (signed autonomy and rejoin before the Phase 6 closed loop), both Proposed pending their first implementing slice
- [ ] A coordinated-disclosure process and signed release tags

## Cross-cutting: agent-native from day one (in progress)

The repo should be as easy for AI agents to work on as for humans, and eventually natural language should be a front door to intent.

- [x] AGENTS.md (and CLAUDE.md pointing to it) with project conventions and the facts agents commonly get wrong
- [ ] `.claude/skills/` for repeatable workflows as they stabilize (for example: scaffold a new blueprint the catalog-pattern way; spin up the Phase 1 virtual mesh; render and validate all packages)
- [ ] Natural language to intent (Phase 3 onward): an agent skill that turns "give site X a mesh gateway on MediumSlow with MQTT uplink" into a proposed PackageRevision in Porch. The human approves via Porch's lifecycle: the LLM proposes, the reconciliation loop enforces, the agent is never the control loop
- [ ] MCP server (Phase 5 onward): expose live mesh and spectrum state (nodes, neighbor counts, band occupancy) as MCP tools so any agent can observe the network and draft intent changes against real data

## In consideration (the seductive frontier, named with its why)

This is an exploratory, experimental project, so it names the big ideas it is
deliberately not building yet, with the reason each is compelling and the reason each
is held back, rather than pretending they do not exist or diving in and wrecking the
core. Nothing here is on the core spine (see the Order of operations). Each is a real
option kept in view. No dates: an item moves onto the spine only when its unlock
condition is met and the core has earned the complexity. The format for each is: what
it opens, the honest constraint, and what would unlock it.

### The SDR pillar, past receive-only occupancy

Today the SDR does one thing, a coarse energy sweep that reports how busy a band is.
That is a small fraction of what a software defined radio can do, and the rest is a
genuine frontier. Most of it is available receive-only, legal, and $0.

- **Signal classification and interference ground-truth (receive-only; mostly unlocked
  already).** What it opens: decode and classify what is actually on the band, so "the
  channel is busy" carries a cause, my own mesh, another LoRa network, an unrelated ISM
  device, or a jammer. Those call for opposite responses, and the closed loop and the
  safety kernel are only as good as this ground truth. The honest constraint: CPU and
  sample-rate cost, a decoder generally handles one preset at a time, and RF
  classification on cheap SDR is a probabilistic corroborator, not proof. What would
  unlock it: little, it is receive-only and $0, and can be prototyped against recorded
  or synthetic IQ before any hardware is plugged in. Research: [spectrum
  classification](research/spectrum-classification.md).
- **A multi-band situational-awareness receiver (receive-only).** What it opens: one
  containerized receiver watching more than the mesh band (ADS-B, AIS, weather, GMRS and
  amateur and CB, other ISM) for a "what is on the air around me" picture that is
  valuable in a disaster in its own right, separate from the mesh. The honest
  constraint: legality of reception varies by band and country (reception is broadly
  permissive but not universal), and each band wants its own antenna. What would unlock
  it: receive-only and $0; SoapySDR keeps it hardware-agnostic across the HackRF and
  cheaper receivers. Research: [situational-awareness
  receiver](research/situational-awareness-receiver.md).
- **SDR transmit, the real frontier.** What it opens: the single biggest expansion in
  the project. An SDR that transmits can be the radio, a software-defined LoRa node or
  gateway with no dedicated LoRa hardware, and beyond LoRa it can carry custom and
  jam-resistant waveforms (frequency hopping, LR-FHSS), cross-band relays, and the full
  cognitive-radio loop where the same device senses and acts. It is also the honest path
  to the SDR being a first-class radio driver at the physical layer, not just a sensor.
  The honest constraint: this is where law and hardware bite hardest. The HackRF is not
  a certified Part 15 unlicensed transmitter; clean transmit needs filtering and
  amplification to avoid spurious emissions; and every option is bounded by regional
  rules (in the US, unlicensed Part 15 versus amateur Part 97, which forbids encryption
  on the air), by power, duty cycle, and band. What would unlock it: an authorized
  context, an amateur license operating unencrypted on amateur frequencies, a shielded
  or attenuated bench, or a coordinated test range. Until then receive-only stays the
  default and the only posture CI depends on, and it is a safety and trust posture as
  much as a legal one. Research: [SDR transmit
  considerations](research/sdr-transmit-considerations.md).

### The big swings already named elsewhere, held the same way

These are in-consideration for the same reason, compelling, gated, and off the core
spine, and are described where they arise rather than duplicated here: the closed-loop
spectrum policy (Phase 6, gated by ADR 0002's autonomy and safety work), agentic AI
mesh nodes (`docs/plans/agent-mesh-nodes.md`), the off-world disruption-tolerant north
star, the cellular and Starlink hybrid backhaul tier (Phase 7), direction finding for
locating interference (KrakenSDR), and ATAK interop for the existing emergency-response
audience. The "Later / open questions" list below is the fuller backlog.

## Later / open questions

- Research-informed backlog (from the August 2026 landscape sweep, full synthesis and sources in `docs/research/resilient-comms-landscape.md`), highest leverage first: (1) an airtime/duty-cycle budget as an enforced reconciler invariant, the one idea backed by two independent research streams and the canonical scaling literature, and the clearest thing a declarative intent system can offer over hand-tuned Meshtastic (the time-on-air model foundation is landed in `internal/airtime`, and the predictive advisory `AirtimeBudget` condition (predict a declared preset change's channel-utilization effect from the radio's measured load and warn when it would oversubscribe) is landed; the enforced fleet admission gate, a Porch validator that sums a channel's nodes and fails an over-budget PR, is the next step); (2) a mesh-observability layer, the prerequisite for the resilience numbers already committed to (a Prometheus exporter for the KPIs the operator already reads, readiness, apply attempts, channel utilization, transmit airtime, is landed in `internal/metrics`; MeshMonitor integration, delivery-ratio and neighbor-churn metrics, and the control-plane-independence harness remain); (3) an application-layer authentication and freshness envelope on automation-triggering mesh packets (the RF medium is untrusted: Meshtastic is AES-CTR with a shared key, no per-sender auth, no replay protection; design in [message-auth-envelope](plans/message-auth-envelope.md), which recommends a compact Ed25519 authentication-and-freshness envelope on the rare automation-triggering packets, with clock-free monotonic freshness); (4) the receive-only SDR as an out-of-band "claim vs air" trust anchor (duplicate-node-id, impossible-mobility, Sybil detection), plus a receive-only LoRa decoder alongside the energy sweep; research in [sdr-trust-anchor](research/sdr-trust-anchor.md), which finds the two cheap logic detectors (duplicate-id, impossible-mobility) the right first move and is honest that commodity-SDR LoRa decode is real but overstated in the wild; (5) an anti-oscillation debouncer with unpredictable-destination selection for the Phase 6 closed loop; (6) a written Reticulum position (candidate driver, not competitor) and a crypto-mesh prior-art update; (7) epoch-keyed channels for forward secrecy and revocation at zero airtime (design in [key-rotation-and-epochs](plans/key-rotation-and-epochs.md), a make-before-break fleet rotation with the honest limit that a revoked member keeps the current epoch key until the next rotation); (8) a second radio driver to prove the seam is real (research in [second-radio-driver](research/second-radio-driver.md), which derives a minimal driver contract and recommends MeshCore first, then Reticulum, then ChirpStack as the topology stress-test)
- Propose lessons learned (or packages) upstream; `nephio-experimental` exists for exactly this kind of PoC
- Contribution strategy: once Phase 4 ships, the Meshtastic community is the natural first audience (a working operator plus packages), ahead of the Nephio community; treat the operator as the project's flagship deliverable
- Scale envelope (state it plainly, it is a credibility point): a Meshtastic channel is airtime-limited to roughly tens of active nodes, not thousands. NephMesh is a resilience layer for small teams and sites, not a carrier network, and that is a respectable thing to be. The control plane can manage many sites; each mesh stays small by physics
- Day-2 operations (the part real deployments live in): fleet firmware upgrade of nodes, coordinated channel and PSK rotation, and node decommissioning. Design these before claiming production-readiness; they are thin today
- Air-gapped and offline operation as a first-class path: mirror every container image, pre-provision nodes onto SD-card images, never ship a default key, and never require kubectl-from-the-field. The $0 local-first posture already points this way; make it explicit and tested
- ATAK integration: the emergency and defense audience already uses ATAK (Android Team Awareness Kit), and Meshtastic has an ATAK bridge. Provisioning that bridge would let NephMesh speak the operator's existing tooling. High-value interop, later
- Mesh and telecom KPIs beyond spectrum: packet delivery ratio, neighbor churn, last-heard age, exported as first-class metrics (MeshMonitor is prior art to integrate rather than reinvent)
- Unexplored upside, deliberately left open: the SDR plus LoRa plus intent-automation intersection likely holds ideas this roadmap has not found yet (spectrum-aware mesh optimization, wider LoRa ecosystems beyond Meshtastic, multi-radio transport abstraction); revisit after Phase 2 gives real data
- Agentic AI nodes on the mesh (Phase 6+, research-backed plan in `docs/plans/agent-mesh-nodes.md`): an offline, retrieval-grounded, advice-only AI node reachable over the mesh to help when the internet is gone (the wildfire and earthquake case). Designed with hard constraints up front, terse template-and-code messages within a ~200-byte budget, the model at a powered base with cheap field radios, air-gapped from every transmit and config path, human-decides, and global by construction. Not near-term, and deliberately downstream of the reliable core
- Disruption-tolerant and off-world north star: treat the terrestrial-disaster case and the space case as one disruption-tolerant problem differing only in latency and RF, so a later Delay/Disruption-Tolerant Networking (Bundle Protocol v7) or CCSDS-adjacent extension is credible. Honest scope in `docs/plans/agent-mesh-nodes.md`: the control-plane and store-and-forward semantics map onto space; the LoRa and SDR radio layer does not
- Akri instead of generic-device-plugin if dynamic SDR discovery and scheduling becomes a feature rather than plumbing
- Direction finding (KrakenSDR) for locating interference sources
- Multi-technology expansion beyond LoRa: receive-only monitoring of CB (27 MHz), GMRS, and amateur bands is just more spectrum sensing and can land any time (the design is in [situational-awareness receiver](research/situational-awareness-receiver.md)); *managing* additional radio services only makes sense where a digital control surface exists (ham digital modes, GMRS data), and never analog CB as a transport (see `docs/faq.md`)
- SigMF IQ capture plus IQEngine for post-hoc analysis of interesting events
- Pin Nephio release: R6 today; watch R7's "modularization and easier onboarding for new use cases", a roadmap item aimed at projects like this one. The compatibility posture (where the code stays Nephio-consumable, where it diverges on purpose, and the current upstream API-group and library versions) is tracked in [nephio-compatibility](reference/nephio-compatibility.md), checked 2026-08-07: NephMesh matches the separate-api-module, domain-suffixed-v1alpha1, Configuration-as-Data shape, and interops at the KRM and package boundary rather than by importing Nephio Go types, so it can run newer k8s libraries than upstream's `api` module without friction
