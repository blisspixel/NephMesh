# CommunicationIntent and the intent compiler (report-only first)

Status: partial. A report-only compiler shipped (CRD + `internal/intent.Compile` +
RBAC so it cannot create `MeshtasticNode`s). `Feasible` today means renderable.
This plan describes the rest of stage 9: `objectives`, a `ChangePlan`, and
`IntentInfeasible` when physics forbids an outcome. The schema below is
illustrative and explicitly not final. Do not read this file as "nothing exists."

Honesty note on what shipped versus this plan: the report-only compiler that exists
today renders and reports a feasibility verdict, but the shipped
`CommunicationIntentSpec` carries region, approved presets, channels, expected
traffic, and nodes, and has no `objectives` field. So the current `Feasible` means
"renderable" (a known preset, at least one node), not "objectives achievable", and
the stronger `IntentInfeasible` this plan describes (physics forbids an objective) is
not yet computed. Closing that gap is small and closes the assurance loop in one move:
add `objectives` (minDeliveryRatio, maxMessageAge) here, and a measured
`IntentInfeasible` becomes a comparison of those against the delivery ratio and
latency the resilience harness already measures. See
[`../design/road-to-safe-autonomy.md`](../design/road-to-safe-autonomy.md) sections
2 and 5.2.

## 1. The reframe: why `MeshtasticNode` becomes a compiled artifact

The operator shipped in 0.2.0 reconciles a device to a fixed desired
configuration: region, modem preset, role, owner, channels, MQTT. In the
vocabulary of [RFC 9315, Intent-Based Networking: Concepts and
Definitions](https://www.rfc-editor.org/rfc/rfc9315.html), that is configuration,
the lowest abstraction level, device-level settings without any higher-order
goals. RFC 9315 defines intent separately, as "a set of operational goals (that a
network should meet) and outcomes (that a network is supposed to deliver) defined
in a declarative manner without specifying how to achieve or implement them," and
it draws a third line for policy, "a set of rules that governs the choices in
behavior of a system," typically event-condition-action. Under those definitions,
"configure this radio as LONG_FAST in US915" is configuration; "preserve delivery
of life-safety traffic while holding an emergency airtime reserve" is intent.

The project already has the configuration layer. The gap is semantic and it sits
above `MeshtasticNode`, not as another field inside it. ADR 0001 records the
decision: treat intent as a bounded envelope of acceptable mission outcomes, and
make `MeshtasticNode` the compiled output of a higher-level `CommunicationIntent`
rather than the source of truth. `MeshtasticNode` loses nothing in this picture.
It stops being the source language and becomes a stable, independently testable,
device-level intermediate representation, the analog of compiled output. The
radio operator keeps doing exactly what it does today.

This also gives the project a concrete test for scope creep, straight from ADR
0001: if a proposed change turns `MeshtasticNode` into an outcome API, or gives
two control layers write access to one field, it violates the ADR. The
already-planned `MeshTopology` idea (see `crd-api-design.md`, section 4, sketched
there as a mesh-wide resource that fans out a shared channel set, region, and PSK
rotation policy into per-node CRs) is conceptually close to the higher layer, but
under this reframe it should express outcomes and constraints, not merely
broadcast shared settings. `CommunicationIntent` is that higher layer done as
intent rather than as fan-out.

## 2. A conceptual CommunicationIntent schema (NOT FINAL)

The following is a thinking aid, not an API. Field names, grouping, and even the
resource boundary are expected to change once stages 6 and 7 (`ChannelBudget`,
mission traffic classes) land and inform what the intent layer needs to express.

The intent carries five kinds of content:

- Objectives per traffic class: the outcomes, stated as measurable targets. A
  minimum delivery ratio and a maximum useful message age per class (life-safety,
  command and coordination, situational reports, telemetry, best effort), matching
  the traffic classes named in the doctrine and airtime work.
- Hard invariants: constraints no optimization or learned behavior may trade
  away. Region and transmit limits, the prohibition on autonomous power, identity,
  or root-key change, receive-only SDR, and an emergency airtime reserve. These
  are values, not physics; the medium never edits them.
- An approved operating set: the bounded sets a lower layer may select from.
  Allowed modem presets and allowed channels. This is the strategic layer's
  authority, and it is a set, not a single value (see section 5).
- Degraded modes: the ordered, named semantic states the fabric may enter when
  full service is infeasible (Normal, Constrained, Contested, Quiet, and so on
  from the doctrine), so that shedding is a declared behavior rather than an
  improvisation.
- Autonomy delegation: which risk-tier actions a lower layer may take
  autonomously, under what evidence, matching the risk-tiered autonomy ladder in
  the doctrine. In report-only mode this is recorded and rendered into plans, but
  nothing acts on it yet.

Illustrative YAML, conceptual only:

```yaml
# CONCEPTUAL. Not a shipped API. apiVersion is a placeholder.
apiVersion: intent.nephmesh.io/v1alpha1
kind: CommunicationIntent
metadata:
  name: valley-relay-mission
spec:
  # Outcomes, per traffic class. What the fabric should deliver, not how.
  objectives:
    - class: life-safety
      minDeliveryRatio: 0.99
      maxMessageAge: 60s
    - class: command
      minDeliveryRatio: 0.95
      maxMessageAge: 120s
    - class: telemetry
      minDeliveryRatio: 0.50
      maxMessageAge: 900s
  # Hard invariants. Never traded for performance. Values, not evidence.
  invariants:
    region: US
    autonomousPowerChange: forbidden
    autonomousIdentityChange: forbidden
    autonomousRootKeyChange: forbidden
    sdrTransmit: forbidden
    emergencyAirtimeReservePercent: 10
  # Approved operating set. Bounded choices a lower layer may select from.
  approvedSet:
    allowedModemPresets: [LONG_FAST, MEDIUM_SLOW]
    allowedChannels: [primary, coord, safety]
  # Ordered degraded modes. Declared shedding behavior.
  degradedModes: [Normal, Constrained, Contested, Quiet, Isolated]
  # Delegation. Which autonomy tier a lower layer may exercise. Report-only for now.
  autonomy:
    maxLevel: L1        # L1 = non-disruptive local care only, per the doctrine ladder
    requireCorroboration: true
```

The `intent.nephmesh.io` group is provisional, consistent with the placeholder
note on `nephmesh.io` in `crd-api-design.md`. Whether the intent group is a third
API group or folds into `mesh.nephmesh.io` is an open decision, not a commitment.

## 3. The compiler

The compiler is the translation function RFC 9315 calls out as part of intent
fulfillment (its stages are ingestion, translation, and orchestration). NephMesh
scopes the first version tightly: it does translation and reporting only. It does
not do orchestration, because report-only mode by definition never applies
anything.

Inputs:

- A `CommunicationIntent` (the outcomes, invariants, approved set, degraded modes,
  delegation).
- Feasibility evidence: what the medium currently permits. At stage 9 this is the
  evidence the project already produces or has planned, the airtime time-on-air
  model and the observed `airUtilTx` / `channelUtilization`, the `ChannelBudget`
  admission math from stage 6, mission-aware status conditions from stage 2, and,
  where present, SDR occupancy from `SpectrumScan`. The compiler consumes evidence;
  it never lets evidence rewrite an invariant.

Outputs:

- Proposed `MeshtasticNode` resources: the compiled, rendered device-level form.
  In report-only mode these are emitted as proposals (for review or diffing, for
  example as a dry-run render or a separate proposed object), not applied to
  devices.
- A `ChangePlan`: the field- or section-level deltas the proposal implies against
  current live config, with the estimated reboot and control-airtime cost, per the
  doctrine's least-change section and roadmap stage 10. In report-only mode the
  plan is produced and explained but not executed.
- Conditions, including `IntentInfeasible`. Feasibility reporting is a first-class
  output, not an error. If no configuration in the approved set can meet the
  objectives given current evidence, the compiler says so, in plain language, with
  which objective failed and why, rather than silently approximating.

The decision order is lexicographic, not a weighted sum, exactly as the doctrine
and ADR 0001 require: hard invariants first, then mission priority, then
disruption cost, then resource cost (airtime and energy), then uncertainty. The
compiler rejects every candidate that violates a hard invariant before it
considers any performance gain. A weighted score can decide that a large enough
throughput gain justifies crossing a legal, safety, or security line; a
lexicographic order structurally cannot. That property is the whole point.

Why a deterministic rule engine first, not an optimizer or a model: the doctrine's
guardrail against over-engineering. The mathematical framing (a viability set, a
lexicographic controller) exists to make the semantics precise and to permit later
verification, not to justify a solver on day one. A deterministic rule engine is
testable with golden tests, explains itself by construction, cannot trade away an
invariant, and is legible to a reviewer. An optimizer that can exchange a hard
constraint for performance is a safety hazard (ADR 0001, alternatives
considered). Learning, if it ever arrives, arrives last and in shadow mode, unable
to define hard constraints or invent actions.

## 4. One writer per field: approved sets versus current selection

The single rule that keeps this from becoming two controllers fighting over one
knob (doctrine section 4, ADR 0001 invariant 2): no two layers may believe they
own the same mutable variable. Authority ownership equals field ownership.

The split for a preset:

- The strategic layer (the `CommunicationIntent`) owns the approved set:
  `allowedModemPresets: [LONG_FAST, MEDIUM_SLOW]`.
- A lower layer (a site steward, when it exists; in report-only mode, the compiler
  proposing a default) owns the current selection: `selectedModemPreset:
  MEDIUM_SLOW`, with a recorded reason.
- The compiler combines the two into the rendered `MeshtasticNode`, whose
  `spec.modemPreset` is the single concrete value the device operator reconciles.

`MeshtasticNode` therefore still has exactly one writer for each of its fields, the
compiler, even though the value is derived from two authorities above it. The
approved set constrains; the selection chooses within the constraint; the render
resolves to one value. No layer writes the same field as another.

How the render enforces this is an implementation choice, not a doctrine
commitment. Kubernetes server-side apply with distinct field managers is one
option: the strategic layer could manage the fields that encode the approved set
and the compiler could manage the resolved `MeshtasticNode` fields, so the API
server itself rejects a second writer to a field. That is worth prototyping, but
the design should not over-commit to it. Separate CRDs, an embedded policy engine,
or a plain single-writer render step could all express the same authority
structure. What matters is the invariant (one writer per mutable field), not the
mechanism.

## 5. Feasibility reporting and the bidirectional loop

RFC 9315 frames intent as a reconciliation between operator goals and what the
network can actually deliver, through fulfillment (translation of goals downward)
and assurance (monitoring, compliance assessment, corrective action, reporting
back up). NephMesh keeps the reporting half and defers the corrective-action half:
report-only mode does fulfillment-as-translation and assurance-as-reporting, but
not assurance-as-correction.

The loop is bidirectional, matching doctrine section 3 (the medium is evidence,
not sovereign):

- Top-down: the `CommunicationIntent` states desired outcomes, constraints,
  priorities, and delegated authority.
- Bottom-up: given current feasibility evidence, the compiler reports which
  objectives are feasible, which are degraded, which are presently impossible, and
  what the least-disruptive alternatives are.

`IntentInfeasible` is the honest name for the bottom-up finding that physics
currently forbids an objective inside the approved set. It is a legitimate
reported condition, not a controller failure (ADR 0001 invariant 3). The compiler
should attach which class or objective is unmet, the evidence that shows it, and,
where one exists, the degraded mode that would keep the higher-priority classes
alive. This is where the lexicographic order becomes visible to a human: the
report says which objectives were shed and in what order, so the trade-off is
explicit rather than buried in a score. Silence and intentional quiet must not
read as failure here either, which is why stage 2's mission-aware conditions
(MissionViable, IntentionallyQuiet) are a prerequisite: an intentionally quiet
node is not an infeasible intent.

## 6. How a compiler fits Configuration-as-Data and kpt/Porch

Nephio already expresses intent as Configuration-as-Data: all artifacts are plain
KRM YAML mutated by KRM functions in a `Kptfile` pipeline, with no templating, and
specializer functions read a package's context and emit derived resources, running
repeatedly because specialization converges iteratively (see
`docs/research/nephio.md`, and the [Nephio PackageVariant
docs](https://docs.nephio.org/docs/porch/package-variant/) and
[kpt.dev](https://kpt.dev)). A `PackageVariant` clones an upstream package
downstream with mutations; a `PackageVariantSet` fans that out across clusters by
label selector.

The intent compiler is the same shape of idea, one altitude up, and this is the
prior art the project should lean on rather than inventing a new mechanism. A
specializer function lowers a higher-level package plus injected cluster facts into
concrete derived resources; the NephMesh compiler lowers a `CommunicationIntent`
plus feasibility evidence into concrete `MeshtasticNode`s. Both are declarative to
imperative lowering, what a compiler does: a stable rendering from a higher-level
description and some context into a lower-level intermediate form. Nephio's
specializers are pure and idempotent and run in a pipeline; the report-only
compiler should share those properties, which is also what makes golden testing
straightforward. Whether the first compiler runs as a controller reconciling a
`CommunicationIntent` CR, as a KRM function in a kpt pipeline, or both, is an open
question (section 8), but the Configuration-as-Data model means either can emit the
same `MeshtasticNode` KRM the operator already consumes.

Broader prior art on declarative-to-imperative lowering (network configuration
synthesis, policy compilers) supports the same conclusion, that a deterministic
rendering with explicit infeasibility reporting is a well-trodden pattern. Treat
that literature as reassurance for the approach, not as a source of a specific
algorithm, because the LoRa-specific feasibility math (airtime, collision domains)
is where the real work is, not the compiler scaffolding.

## 7. How it maps to the roadmap

This is roadmap stage 9, the first item of the intent-layer frontier, gated behind
the core spine (stages 1 through 8: channels and keys on hardware, mission-aware
conditions, day-2 rotation, the published operator image, a reproducible demo,
`ChannelBudget`, mission traffic classes, and the multi-site control-plane
independence proof). The doctrine's sequencing rule is that the frontier does not
earn its complexity until that core is rock-solid.

Stage 9 is the point where ADR 0001 moves from Proposed to Accepted: the ADR
itself says it becomes Accepted "when the first `CommunicationIntent` compiler
ships in report-only mode." Stage 10 (a `ChangePlan` resource and least-change
actuation) is the first stage that could act, and it is deliberately after this
one.

Report-only de-risks the whole frontier. Because the compiler emits proposals and
never touches a radio, it can be built, tested, and reviewed against the real
`MeshtasticNode` API and against simulated devices with zero risk of a bad render
rebooting a fleet or spending the emergency reserve. The infeasibility reporting,
the lexicographic order, and the one-writer-per-field render can all be validated
as pure functions with golden tests before any autonomy exists to consume them.
This matches the project's standing bar that everything shipped is executed first:
a report-only compiler is fully exercisable (parse an intent, feed it evidence,
diff the proposed `MeshtasticNode`s and `ChangePlan`) without hardware and without
actuation. Only once that is solid do the later stages (the Intent Capsule, the
site steward, the safety kernel, and rejoin) add the ability to act.

## 8. Open questions

- Resource boundary: is `CommunicationIntent` one CRD, or does the outcome layer
  split across `CommunicationIntent` (mission charter) and the already-sketched
  `MeshTopology` (fabric shape)? The doctrine names both `CommunicationIntent` and
  `MeshFabricIntent` as working names; the boundary is not settled.
- API group: a third group (`intent.nephmesh.io`) versus folding into
  `mesh.nephmesh.io`. The two-group rationale in `crd-api-design.md` argues for
  per-area groups, which would favor a distinct intent group, but this is unforced.
- Runtime form: controller reconciling a CR, KRM function in a kpt pipeline, or
  both. Configuration-as-Data supports either; the choice affects how proposals are
  surfaced for review.
- How proposed `MeshtasticNode`s are represented in report-only mode: dry-run
  render, a separate proposed object, a status field, or a diff artifact. This
  needs to be reviewable without being mistaken for a desired-state write.
- Feasibility evidence contract: what exact shape the compiler consumes from the
  airtime model, `ChannelBudget`, and `SpectrumScan`, and how stale evidence is
  handled (the doctrine's three-valued healthy/unhealthy/unknown reasoning should
  extend here, so unknown feasibility is not silently read as feasible).
- Objective expressibility: whether min delivery ratio and max message age are
  enough, or whether coverage of declared critical endpoints and custody
  completion need first-class objective fields.
- Server-side apply field ownership: whether it is the right enforcement mechanism
  for one-writer-per-field, or over-constrains the render. Prototype first.

## Sources

- [RFC 9315, Intent-Based Networking: Concepts and
  Definitions](https://www.rfc-editor.org/rfc/rfc9315.html) (intent versus policy
  versus configuration; fulfillment and assurance).
- [Nephio PackageVariant
  documentation](https://docs.nephio.org/docs/porch/package-variant/) and
  [kpt.dev](https://kpt.dev) (Configuration-as-Data, KRM, specializer rendering).
- `docs/research/nephio.md` (Nephio building blocks, PackageVariant/PackageVariantSet,
  specializer KRM functions, as used in this project).
- `docs/design/doctrine.md` (viability, lexicographic order, one-writer-per-field,
  degraded modes, risk-tiered autonomy, deterministic rule engine first).
- `docs/adr/0001-intent-as-an-outcome-envelope.md` (the decision and its invariants).
- `docs/plans/crd-api-design.md` (the existing `MeshtasticNode` API and the
  `MeshTopology` sketch).
- `docs/roadmap.md` (Order of operations, stage 9).
