# Design doctrine: intent as an outcome envelope

This document is a design direction, not a description of shipped code. It records
how the project intends to grow once the core is solid, so that later work does not
quietly collapse the idea back into "automatic knob-turning." Read it as a north
star and a set of invariants, not a feature list.

Status, stated plainly so nothing here is mistaken for a claim:

- What exists today (0.2.0): a `MeshtasticNode` operator that reconciles a device to
  a fixed desired configuration (region, modem preset, role, owner, MQTT with a
  Secret-backed password), a reboot-aware minimal-diff apply loop validated against
  a simulated device in CI and a physical T-Deck over USB, an airtime time-on-air
  model, a Prometheus metrics layer, and assume-breach tests. That is device-level
  reconciliation. It is the foundation, not the thesis.
- What this document describes: almost none of it is built. It is the shape the
  project would take if the foundation earns the right to grow, and the order in
  which the pieces should arrive. Every section that names a resource or a behavior
  is a proposal, not an API.

The single most important idea, and the one everything else follows from:

> NephMesh should reconcile a communications fabric toward a bounded set of viable
> mission outcomes, not merely reconcile individual radios to a fixed configuration.

RFC 9315 (Intent-Based Networking) draws the line this document leans on: genuine
intent expresses desired outcomes and operational goals, distinct from the policies
and low-level configuration that implement them. Under that definition, "configure
this radio as LONG_FAST in US915" is configuration; "preserve delivery of
life-safety traffic while holding an emergency airtime reserve" is intent. The
project already has the configuration layer. The gap is semantic, and it is above
`MeshtasticNode`, not another field inside it.

## 1. `MeshtasticNode` becomes a compiled artifact

The cleanest expression of the reframe is a compiler relationship. A higher-level
`CommunicationIntent` (or `MeshFabricIntent`) states outcomes, constraints, and
priorities. A compiler renders that intent, together with current feasibility
evidence, into the device-level `MeshtasticNode` resources that already exist. The
radio operator keeps doing exactly what it does today.

```text
CommunicationIntent
      |
      v
IntentCompiler  <-- feasibility evidence (what the medium currently permits)
      |
      v
AutonomyPolicy + ChannelBudget
      |
      v
ChangePlan
      |
      v
MeshtasticNode          (the compiled, rendered, device-level intermediate form)
      |
      v
device operator / radio
```

`MeshtasticNode` does not lose value in this picture. It stops being the source
language and becomes the compiled output: a stable, testable, device-level
intermediate representation. The already-planned `MeshTopology` resource is
conceptually close to the higher layer, but it should express outcomes and
constraints rather than fanning out shared settings.

A consequence worth stating early: the compiler may conclude that an objective is
infeasible on current Meshtastic hardware. That should produce a clear
`IntentInfeasible` condition with an explanation, not a silent approximation.
Feasibility reporting is a first-class output, not an error.

## 2. Viability, not target-state convergence

Ordinary Kubernetes reconciliation drives a state `x` toward one declared target
`x*`. A contested wireless fabric is a different class of system: topology,
interference, energy, human priorities, and reachable nodes all change
continuously, and there may be no single configuration that stays correct for more
than a short interval.

The more honest control model keeps the system inside a set of acceptable states
and trajectories `K(I)` implied by intent `I`, and when physics makes that
impossible, gives up the least important objectives first. This is close to the
control-theoretic notion of a viability set: a region within which feedback can
keep a constrained system operating despite changing conditions, rather than a
single optimum to converge on (Aubin, Viability Theory).

The decision rule should be lexicographic, not a weighted sum. A weighted score can
decide that a large enough performance gain justifies crossing a legal, safety, or
security line. A lexicographic controller first rejects every action that violates
a hard invariant, then minimizes mission harm, then minimizes disruption and
resource use. The order is: hard invariants, then mission priority, then disruption
cost, then airtime and energy, then uncertainty.

One guardrail against over-engineering: the first implementation of this is a small
deterministic rule engine or finite-state policy, not an optimizer and not machine
learning. The mathematical framing exists to make the semantics precise and to
permit later verification, not to justify a solver on day one.

## 3. The medium is evidence, not sovereign

It is true that the medium reveals which intents remain viable. It may legitimately
change the system's estimate of achievable delivery, which candidate channels look
usable, how much airtime remains, and which degraded mode to enter. It must never
redefine the legal region, transmit-power limits, root trust, who is authorized, or
whether human life outranks telemetry. The medium informs feasibility; it does not
set values.

That makes the intent process bidirectional. Top-down: here are the desired
outcomes, constraints, priorities, and delegated authority. Bottom-up: given
current evidence, these outcomes are feasible, these are degraded, these are
presently impossible, and here are the least-disruptive alternatives. RFC 9315
frames the same reconciliation between operator goals and what the network can
actually deliver.

## 4. Nested control loops, each with a survival contract

Replace one control loop with four, distinguished by timescale, and make explicit
what each does when its parent disappears.

| Layer | Timescale | Owns | When its parent is gone |
|---|---|---|---|
| Constitutional / strategic | days to months | jurisdiction, hard safety constraints, identity roots, mission objectives, delegation | issues no new strategic intent; lower layers keep a signed, bounded delegation |
| Site steward | seconds to hours | local mode, evidence interpretation, traffic shedding, bounded tactical adaptation | continues inside its autonomy envelope |
| Device reconciliation | seconds to minutes | translation to device config, minimal apply, verification, rollback | holds last-known-good; refuses unauthorized change |
| Radio / application reflex | milliseconds to hours | MAC retries, native forwarding, local queueing, message expiry, store-and-forward | continues firmware-native with the last configuration |

The one rule that keeps this from turning into two controllers fighting over one
knob: no two layers may believe they own the same mutable variable. The strategic
layer should not declare `modemPreset: LONG_FAST` while a local controller sets it
to `MEDIUM_SLOW`. Instead:

- the strategic layer owns the approved set (`allowedModemPresets: [LONG_FAST, MEDIUM_SLOW]`),
- the site steward owns the current tactical selection (`selectedModemPreset: MEDIUM_SLOW`, with a reason),
- the compiler combines the two into the rendered `MeshtasticNode`.

Field ownership must express authority ownership. That is more important than
whether the implementation uses server-side apply, separate CRDs, or an embedded
policy engine.

The missing runtime component is an edge-resident site steward between central
GitOps and the device operator: it caches and verifies a signed intent capsule,
maintains local evidence, selects among preauthorized actions, records a decision,
asks a separate safety kernel for permission, submits a `ChangePlan`, observes the
result, and rolls back or holds safe on failure. It is not an LLM and not a broadly
privileged agent. It starts as a small, deterministic state machine.

## 5. Management-plane dissolution: leave living intention behind

The control plane not being a runtime dependency is already a load-bearing project
claim. To make "leaves living intention behind" mean something, that intention
needs an explicit, portable representation.

An Intent Capsule is a signed, content-addressed object the steward stores locally
and can act on without contacting Git, Porch, a cluster API, or any cloud service.
It carries at least: intent id and epoch, parent digest, issuer and signature, hard
invariants, mission objectives and priority order, approved degraded modes, locally
permitted and prohibited actions, evidence requirements per action, budgets
(airtime, disruption, attention), an emergency reserve, dwell and cooldown minimums,
a last-known-good reference, rollback and rendezvous procedures, validity and
time-uncertainty behavior, rejoin behavior, and retention rules.

Two refinements matter more than they look:

- Use a degrading lease, not a binary one. A hard expiry that says "authority
  expired, stop all service" can be the exact opposite of the mission in a
  disconnected emergency. Expiry should only ever narrow authority, never create
  it: Current (all authorized actions) to Grace (reversible, service-preserving
  actions only) to Restricted (no new topology, channel, key, role, or identity
  changes; rollback and quiet mode remain) to Safe hold (passive receive, essential
  custody, last legal config) to Reauthorization required.
- Clocks are unreliable in the field. Cheap devices lose time across reboots and
  can be denied synchronization. Bundle Protocol v7 (RFC 9171) handles the same
  problem by carrying bundle age when accurate clocks are unavailable rather than
  trusting absolute timestamps. Intent validity should combine signed epochs,
  monotonic residence age where available, recorded boot events, and conservative
  behavior under time uncertainty.

"Graceful amnesia" should be selective forgetting, not amnesia. Complete forgetting
would be unsafe. Memory has classes with different retention: constitutional memory
(region, prohibited transmit actions, root trust) is never forgotten by autonomous
operation; safety memory (last-known-good, rollback point, cooldown state) is
long-lived and cleared only by a superseding epoch; mission custody (undelivered
messages) is held until delivered, expired, or shed under declared storage
pressure; tactical memory (recent channel scores) decays over minutes to hours;
optimization memory (learned thresholds) decays and never overrides a hard
constraint; sensitive operational memory (locations, contacts) carries an explicit
privacy TTL; audit summaries are compact and tamper-evident and offloaded on
rejoin. Amnesia then has a precise purpose: stale optimization assumptions
disappear, while safety, authority, custody, and accountability persist.

Autonomy cannot live equally in every radio. Most Meshtastic leaf devices are not
intent interpreters. The honest devolution ladder: central plane lost, edge cluster
alive, the steward continues bounded adaptation; edge cluster lost, gateway alive,
the device holds last-known-good and native mesh behavior; gateway lost, leaf mesh
remains, firmware-native routing continues but higher-order adaptation is gone; all
stewards lost, the fabric degrades to the static intention last compiled into the
radios. That is not a failure of the philosophy; it is an honest mapping of
intention onto available computation.

## 6. Airtime as a commons

The project's airtime work (a time-on-air model by modem preset, `airUtilTx` and
`channelUtilization` read from the radio, Prometheus metrics, an observational
`AirtimeHealthy` condition) is the strongest existing bridge from philosophy to
enforceable behavior. The next step is to govern airtime as a budgeted common-pool
resource.

Three design commitments:

- Scope budgets by interference domain, not by fleet. A global fleet percentage is
  too coarse: two distant groups can reuse the same spectrum, while two nearby
  groups run by different clusters collide. A `ChannelBudget` needs a
  `collisionDomain` scope, operator-declared at first, later informed by who can
  hear whom (gateway reception overlap, SDR occupancy correlation, packet
  provenance). The LoRa scalability literature is clear that capacity is bounded by
  shared-medium interference, with no node-count-independent abundance (Bor et al.,
  "Do LoRa Low-Power Wide-Area Networks Scale?").
- Budget by meaning. Define traffic classes (life-safety, command and coordination,
  acknowledgements and control, situational reports, position and presence,
  telemetry, best effort), each with a protected minimum share, a normal maximum,
  message lifetime, retransmission ceiling, aggregation rules, reserve eligibility,
  and degradation behavior. An emergency message may borrow from the reserve; that
  borrowing creates airtime debt, after which lower-priority transmissions are
  suppressed until the reserve is restored. Emergency traffic is still bounded
  against malfunction and abuse.
- Account for reconfiguration airtime. A remote reconfiguration is not free just
  because the control logic runs in Kubernetes: admin messages, acknowledgements,
  retries, reboot verification, and fallback announcements all consume scarce
  channel time. A `ChangePlan` (section 8) estimates this before acting.

Hysteresis, dwell, cooldown, and anti-herding are necessary but not sufficient.
BGP route-flap damping (RFC 2439) was designed to suppress instability, and
operational experience showed that naive damping could suppress valid reachability
and make convergence worse (later analyses led to revised, less aggressive
recommendations). The lessons transfer: damp near the source of instability,
distinguish observed instability from its propagated consequences, keep an
emergency escape from damping, expose the accumulated penalty and its reason, and
avoid identical deterministic responses at every site. Anti-herding needs
randomized candidate selection and site-specific tie-breaking, with a verification
phase before broad rollout.

## 7. A contingency layer needs different semantics, not just less throughput

A low-bandwidth contingency layer should not try to preserve the interaction model
of an always-on IP network. It should preserve meaning, priority, custody, and
justified silence.

An application-level `MessageIntent` envelope would carry message class, creation
time or accumulated age, maximum useful age, destination scope, required
acknowledgement strength, whether custody is requested, transmission and hop
ceilings, privacy classification, an aggregation key, a semantic encoding or
template id, whether delayed delivery is still useful, and whether silence is
preferable to uncertain delivery. BPv7 already offers useful semantics (explicit
lifetimes, accumulated age for clockless nodes, hop counts against forwarding
loops) that gateways and applications can adopt without forcing a full DTN stack
onto constrained firmware. The agent-mesh-node design already moves this way with
terse template-and-code messages, correlation ids, deduplication, and
application-level store-and-forward.

Degraded modes are semantic states of the fabric, not just modem presets: Normal
(all classes admitted), Constrained (telemetry reduced, messages aggregated),
Contested (only essential traffic sent immediately, the rest stored), Quiet
(receive and retain; transmit only life-safety or explicitly authorized traffic),
Isolated (local-only operation and custody), Recovery (queues drained by priority
and age under a controlled airtime ramp), and Quiescent (no useful traffic, no
sufficient evidence to act, deliberate silence).

Silence must not read as failure. Today's `Ready` is based on reachability, config
sync, and reboot state, which is right for device management but insufficient for
mission semantics: an intentionally quiet node may be operating exactly as intended
while a manager sees it as unreachable. Separate at least `ManagementReachable`,
`ConfigInSync`, `MissionViable`, `EvidenceSufficient`, `AuthorityValid`,
`IntentionallyQuiet`, and `MessageCustodyHealthy`, and use three-valued reasoning
(healthy, unhealthy, unknown) where appropriate. Unknown must not be silently
converted to either healthy or failed.

## 8. Observation and action as a single, evidence-carrying gesture

"Observation and action become one gesture" should not mean wiring a sensor
directly to an actuator; in a contested medium that is dangerous. It means every
action carries the observation, authority, predicted consequence, and verification
plan that make it intelligible. Represent each autonomous decision as an
Evidence-Carrying Action: a claim with scope, the evidence and its age, an
uncertainty estimate with alternative explanations, the authorizing capsule epoch
and permitted action, the selected candidate, the predicted effect (delivery
improvement, disruption seconds, control airtime), a rollback target, and a
verification window with success criteria.

The control sequence: observe; form a claim with uncertainty; identify authorized
alternatives; estimate resource and disruption cost; pass the action through the
safety kernel; enact the smallest admissible change; sense in a way conditioned on
the action just taken; verify; then retain, roll back, or abstain.

Sensing should be action-conditioned. Before a channel switch, confirm the
candidate is not merely momentarily quiet, listen long enough to estimate hidden
activity, cross-check application delivery, and confirm the sensor itself is
healthy. After, listen for expected peers and verify real delivery rather than
merely lower occupancy; a switch that only improved the sensing site while remote
nodes vanished should roll back.

Use corroboration, not a single threshold. High channel utilization can be
legitimate managed traffic, unmanaged traffic, an interfering technology, a jammer,
a hidden terminal, a sensor artifact, or a successful emergency that must not be
interrupted. Low utilization can be an open channel, a receiver fault, a partition,
intentional silence, or an adversary waiting for migration. Disruptive actions
should therefore require combinations of evidence (radio utilization, SDR
occupancy, application delivery ratio, acknowledgement behavior, neighbor count,
RSSI/SNR distributions, another site's view, sensor health). When evidence
conflicts, abstention is an active safety behavior.

Wrap the whole planner in a Simplex-style runtime safety boundary. Treat the
adaptive planner, whether heuristic, statistical, or a future model, as fallible. A
small, independent safety kernel validates each proposed action against legal
region and frequency, transmit constraints, action authority, capsule validity,
minimum dwell, change budget, emergency reserve, the approved candidate set, the
presence of a rollback, evidence freshness, required corroboration, the prohibition
on SDR transmit, and the prohibition on identity or root-key change. The Simplex
architecture (Sha) is the precedent: an advanced controller may optimize, but a
simpler verified supervisor retains authority to veto it and return the system to a
safe region.

## 9. Least change as a first-class invariant

The reconciler already ignores unspecified live fields when diffing, which is a
good start. The actuation path can be stronger. Introduce a `ChangePlan` before
every disruptive apply: the exact fields changing and why, values before and after,
nodes and collision domains affected, expected reboots and outage, control airtime,
required coordination messages, a fallback path, a rollback target, validation
checks, the authorizing source, and a disruption score that consumes a change
budget. The coefficients in any disruption model are not universal truths; their
value is to force explicit accounting.

Required behaviors: generate field- or section-level patches where the device
interface allows; batch settings that would otherwise cause multiple reboots;
distinguish reboot-requiring settings from the rest; cache last-known-good; verify
identity before applying; limit to a maximum number of nodes or one failure domain
at a time; rate-limit disruptive changes; suppress repeated application of the same
failed plan; treat no-op and abstention as valid outcomes; and prefer traffic
shaping and semantic degradation before RF reconfiguration.

Channel and preset changes need a rendezvous, not an ordinary canary. A software
canary deploys to a few instances while the rest stay reachable; a radio channel
change can isolate the canary from the mesh. A coordinated change needs an announced
switch epoch, repeated pre-change notices, a predeclared fallback channel, a
rollback timeout, an out-of-band path where available, staggered gateway changes
only where dual connectivity exists, and verification from both sides of the
expected partition boundary. Least change is therefore not "change fewer nodes" but
"minimize expected disruption to mission continuity."

## 10. A constitutional hierarchy for policy, and learning that cannot expand authority

"Policy learns from real radio experience" is useful only if the kinds of policy are
separated into layers that change at different rates and by different processes:

1. Constitution: jurisdiction, prohibited actions, identity roots, safety
   invariants, data governance. Slow, explicitly approved, signed.
2. Mission charter: desired outcomes, service classes, priorities, acceptable
   degradation, emergency reserve.
3. Delegation: which local actor may take which actions, under what evidence, for
   what period and scope.
4. Tactics: current local choices inside the delegation.
5. Evidence and learning: observations, estimates, confidence, model updates,
   proposed amendments.

Evidence may change tactics immediately when authorized, may propose a revision to
the mission charter, and may never rewrite the constitution.

Online learning must not expand authority. The RF environment is an adversarial
input surface: a jammer or compromised node can deliberately produce the
observations that make a controller abandon a good channel, herd every site onto one
candidate, burn the emergency reserve, reboot repeatedly, or expose a fallback. So
learning starts in shadow mode; training data keeps provenance; single-source or
anomalous observations have limited influence; models are versioned and replayed
against past incidents and adversarial scenarios; the learned component proposes and
the safety kernel authorizes; and a model can never add actions to its own
authorization envelope.

Do not require distributed consensus for safety. During a partition, global
agreement may be impossible, so hard safety must be locally decidable from signed
constraints. Consensus or quorum may gate coordinated actions (switching a whole
collision domain, rotating shared channel credentials, changing a routing role,
declaring a new steward), but "do not exceed legal power," "do not transmit from the
SDR," and "do not spend the reserve on telemetry" must never depend on reaching
peers.

## 11. Rejoin as a treaty, not drift correction

The most dangerous moment may be reconnection. A conventional GitOps controller
sees locally adapted state as drift and immediately restores the central
configuration, which can undo a necessary emergency adaptation, reboot many radios
at once, return nodes to a jammed channel, discard custody, replay stale commands,
or dump queued telemetry into a recovering channel.

Rejoin should be an explicit state machine: Managed to Detached to LocallyGoverned
(which can branch to Degraded or Quiescent) to LinkRestored to RejoinPending to
EpochCompared, which resolves to AdoptLocalState, StagedRollback, or CompileNewState,
then VerifiedManaged. The sequence: detect restored connectivity; freeze
non-safety autonomous change; exchange intent and authority epoch digests; reject
replayed or superseded directives; upload the detached decision and evidence
summary; classify local divergence (unauthorized, authorized and still useful,
authorized but obsolete, safety rollback, unknown); generate a `ChangePlan` for any
required transition; drain stored traffic under class and airtime budgets; apply in
a staged order; verify mission behavior; issue a new signed capsule and revoke the
old epoch; and compact the detached history into a durable audit summary.

Use CRDTs selectively. Local-first research shows replicas can accept updates
without synchronization and later converge (Kleppmann et al.), which fits
append-only observations, decision ids, acknowledgements, and a grow-only incident
log. It does not fit arbitrary radio configuration: channel A versus B, router
versus client, key epoch 14 versus 15, quiet mode versus emergency transmit are
semantic conflicts that generic last-writer-wins or set merge cannot resolve safely.
And do not run continuous CRDT gossip over LoRa merely because it converges; the
convergence traffic itself spends the common resource.

Aim for self-stabilization as the formal goal: from any reachable state caused by
bounded faults, local rules eventually return the fabric to a legitimate state
without complete global knowledge (Dijkstra, EWD426). "Legitimate" here means hard
invariants hold, at most one current authority epoch per scope, no forbidden action
pending, coherent custody, nodes either sharing a rendezvous or explicitly marked
detached, change-rate limits respected, and the system in a declared mode. The
capsule, authority, and rejoin protocols are good candidates for a small TLA+ or
PlusCal model before implementation.

## 12. Risk-tiered autonomy

| Level | Examples | Authority |
|---|---|---|
| L0 Observe | measure occupancy, delivery, airtime, battery, neighbors | automatic |
| L1 Non-disruptive local care | shed telemetry, aggregate reports, change scan cadence, prioritize queues, store rather than send, enter declared quiet mode | automatic inside the signed envelope |
| L2 Bounded reversible adaptation | switch among preapproved channels or presets, roll back to last-known-good, alter approved reporting cadence | automatic only with corroborated evidence, cooldown, rollback, and verification |
| L3 High-risk governance | region, power, root credentials, membership, routing role, firmware, wipe, persistent topology change | explicit human or central approval |
| L4 Prohibited | SDR transmit, autonomous power escalation, bypassing legal limits, self-expansion of authority | never |

The exact classification can vary by driver: a modem-preset change might be L2 in a
lab and L3 in a large deployed mesh, because of partition risk.

## 13. Measuring coherence when no one is watching

Do not collapse success into one resilience score; a scalar hides unacceptable
trade-offs. Use a vector of measures, with the existing readiness and config-sync
signals demoted to low-level inputs rather than the definition of success:

- Safety and authority (target zero): forbidden actions executed, actions outside
  the current capsule, legal or regional violations, stale-epoch actions accepted,
  unauthorized authority expansion, failed rollback without safe hold.
- Mission service: delivery ratio by message class, end-to-end message age,
  fraction delivered before expiry, isolated mission groups, custody completion,
  coverage of declared critical endpoints, time meeting minimum objectives.
- Commons protection: residual airtime reserve, reserve consumed by class, managed
  versus unmanaged occupancy, retransmission amplification, control-plane airtime,
  per-class fairness, airtime debt and repayment time.
- Control stability: disruptive changes per hour, reboots per node, transition time,
  rollback rate, repeated failed plans, oscillation or herding events, fraction of
  actions that produced the predicted improvement.
- Epistemic quality: observation freshness, evidence-source diversity, sensor
  disagreement, confidence calibration, false congestion and false jamming
  classifications, disruptive actions on insufficient evidence, cases where
  abstention would have been better.
- Human coherence: actionable alerts per hour, duplicated alarms, time to
  reconstruct current state, interventions, overrides, successful handoffs, actions
  whose reason could not be reconstructed.
- Detachment and rejoin: time operating without central management, objectives
  sustained while detached, local decisions within delegated authority, stale
  commands rejected after rejoin, time to verified managed state, burst airtime
  during queue drainage, locally authorized adaptations wrongly overwritten.

The fabric should also be legible to a stressed human. Acute stress does not
uniformly worsen cognition, but controlled studies find impaired working memory
under acute stress, so the system should carry coherence a person should not have
to reconstruct under pressure. For every consequential state it should be able to
answer, in plain language: what mission objective is at risk, what changed in the
medium, how certain we are, what has already been tried, what state is known safe,
what action is authorized, what happens automatically next, what needs a human, what
happens if no one responds, and how to reverse it. Human-on-the-loop beats
human-in-every-loop: requiring a synchronous approval for every reversible
load-shedding action pushes coherence back onto the most stressed, least available
component.

## 14. Language, made precise

The prose that inspired this doctrine is worth keeping as project voice, but four
phrases need normative engineering counterparts so they cannot be misread as magic:

- "The medium teaches the operators" becomes: observations update feasibility
  estimates and surface conflicts between objectives and physics; they do not alter
  constitutional constraints or mission values.
- "Graceful amnesia" becomes: optimization and tactical state decay by explicit
  retention class, while safety, authority, custody, and provenance are retained.
- "Observation and action are one gesture" becomes: every action is
  evidence-carrying, with pre-action sensing, authorization, predicted impact,
  post-action verification, and rollback.
- "Adaptive regulation" becomes: radio experience may generate evidence-backed
  policy amendments, but legal, safety, identity, and authority constraints change
  only through their defined governance process.

The "living medium" is a productive metaphor, but the system model stays precise:
the medium is stochastic, partially observed, shared, and sometimes adversarial.
Some participants have agency; physics does not. That distinction matters when
assigning authority and blame.

## 15. Honest boundaries

None of this repeals physics or manufactures a large audience.

- Physics wins. LoRa airtime, range, and capacity are not software problems. The
  useful envelope is tens to low hundreds of nodes carrying low-bandwidth traffic
  for contingency, not primary, communications. There is no telecom-scale value
  here, and claiming otherwise would be dishonest.
- The user intersection is small. The people who most need fleet management of
  resilient radios are not always the people who want to run Porch, operators, and
  GitOps. Complexity is the main way this fails. For one person with five nodes, the
  phone app is better; the value appears only across multiple sites, multiple
  operators, key rotation, coordinated channel changes, and configuration that must
  stay correct under stress.
- This is specialized infrastructure, not a platform play. "Meshtastic but with
  Kubernetes" is a losing framing. "Desired-state management for resilient radio
  fleets, with spectrum awareness and hybrid contingency" is accurate and
  defensible.
- Over-scoping kills it. The full vision (closed-loop spectrum policy, multi-transport,
  agentic nodes, 5G hybrid) is intellectually attractive and is exactly what must
  wait. The highest-probability path to usefulness is a focused, rock-solid core
  with the exotic pieces treated as research extensions and an upward pull, never as
  blockers.

## 16. What this changes about the plan

The doctrine does not reorder the near-term work; it sharpens why the near-term work
comes first. The disciplined implementation order, elaborated in the roadmap's
"Design direction" section:

1. Write down the doctrine and invariants (this document and the ADRs).
2. Add an outcome-level API (`CommunicationIntent`) in report-only mode: it parses
   objectives, evaluates feasibility, emits proposed `MeshtasticNode`s and a
   `ChangePlan`, explains itself, and never actuates.
3. Make disruption explicit: field-level planned deltas, last-known-good storage,
   estimated reboot and airtime cost, rollback, rate and dwell limits.
4. Implement mission classes and `ChannelBudget`, so the existing channel behaves
   better before any frequency is ever changed.
5. Build the site steward with L1 actions only.
6. Add the independent runtime safety kernel.
7. Add one L2 action, rollback-to-last-known-good before channel switching.
8. Implement the detached-epoch and rejoin protocol before broad autonomy.
9. Model-check the authority and rejoin state machine.
10. Introduce learning last, in shadow mode, unable to define hard constraints or
    invent actions.

The single most valuable next design decision is to make `MeshtasticNode` the
compiled output of a higher-level `CommunicationIntent`, and to define the signed
autonomy and rejoin semantics before implementing the Phase 6 closed loop. Until the
core (channels and keys, multi-site packaging, day-2 key rotation, reproducible
demos) is rock-solid, none of the rest earns its complexity.

## Sources

- RFC 9315, Intent-Based Networking: Concepts and Definitions.
- RFC 9171, Bundle Protocol Version 7.
- RFC 2439, BGP Route Flap Damping (and the later operational reassessment of
  aggressive damping).
- J.-P. Aubin, Viability Theory.
- M. Bor, U. Roedig, T. Voigt, J. Alonso, "Do LoRa Low-Power Wide-Area Networks
  Scale?" (MSWiM 2016).
- L. Sha, "Using Simplicity to Control Complexity" (the Simplex architecture).
- E. W. Dijkstra, "Self-stabilizing systems in spite of distributed control"
  (EWD426).
- M. Kleppmann et al., local-first software and CRDTs.
- Human-factors literature on acute stress and working memory.

These are pointers to the ideas the doctrine leans on, not claims that the project
implements them. They are here so a reader can check the reasoning rather than take
it on faith.
