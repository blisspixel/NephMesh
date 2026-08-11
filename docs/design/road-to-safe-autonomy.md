# Road to safe autonomy: the gated, evidence-first build plan

Status: design and plan, not shipped. Nothing in the autonomy layer (steward,
safety kernel, capsule, lease) exists yet. This document does one job the
[doctrine](doctrine.md) and the [signed-autonomy plan](../plans/signed-autonomy-and-safety-kernel.md)
do not: it makes the path from what is built today to safe autonomy explicit,
gated, and honest about what can and cannot be proven at this project's scale and
budget. Read the doctrine for the north star and the invariants; read this for the
order of operations and the gates.

It is the output of a structured design review that pressure-tested the plan from
five expert perspectives: provable safety (formal methods and runtime assurance),
adversarial security (red team), disruption-tolerant networking and measurement,
autonomic and intent-based networking, and ruthless minimalism. Their conclusions
converged, which is the reason to trust the shape below.

## 1. The diagnosis, in one sentence

NephMesh has built a first-class Monitor and a first-class Predictor and never
introduced them to each other. The intent compiler predicts channel utilization;
the resilience harness measures delivery ratio and latency; nothing computes the
residual between them. That missing wire is the assurance half of RFC 9315, and it
plus one more artifact (an independent veto) is the whole near-term game. Almost
everything else in the autonomy design is an upward pull, not a blocker.

## 2. Where the loop already exists, and where it does not (MAPE-K)

The honest frame for the site steward is a MAPE-K autonomic loop (Monitor, Analyze,
Plan, Execute over shared Knowledge). Tagging every existing component with its
letter shows exactly which parts are shipped and which are missing:

| Letter | NephMesh component | Status |
|---|---|---|
| Monitor | radio `airUtilTx`/`channelUtilization` telemetry, `spectrum-exporter`, the `resilience` probe | shipped |
| Analyze | `internal/airtime` model, `internal/spectrum.Classify`, `internal/advisor` | shipped, partial |
| Plan | `internal/intent.Compile` (renders nodes), `advisor.Recommendation` (proposes an action) | shipped, thin: no typed action, no predicted effect, no rollback |
| Execute | the operator `Converge` loop | shipped, deliberately gated for intent |
| Knowledge | scattered across CRD status; last-known-good, the capsule, the shared world-model | missing, and unnamed |

Knowledge is the weakest, unnamed letter, and Plan lacks a real contract. The whole
near-term plan is to build the missing seam (a typed action) and close the feedback
edge (predicted versus measured), report-only, before anything actuates.

## 3. The keystone artifact everyone reached from a different door

Five perspectives, one recommendation: build **a typed Evidence-Carrying Action and
a pure, independent safety kernel `Decide` function, report-only, in the repo's
existing pure-function idiom**, before any wire format, lease ladder, or rejoin
protocol. It is the single object that:

- closes the assurance loop, because it carries the predicted effect that the
  harness then measures against (the autonomic lens);
- is the veto, the load-bearing 20 percent of the safety story (the minimalist lens);
- is a provably forward-invariant filter, if written as a pure `Decide` whose guards
  transliterate a small state-machine spec (the formal lens);
- is the seam contract the kernel checks and the adversary attacks (all lenses);
- is measured under attack and over many trials, not asserted once (the red-team and
  DTN lenses).

The action object (report-only; every field maps to something already computed):

```text
EvidenceCarryingAction {
  Claim, Evidence[], Uncertainty, Alternatives    // Monitor/Analyze: advisor + spectrum + airtime
  CapsuleEpoch, Action, Candidate, RiskTier        // Authority: capsule when it exists, epoch 0 report-only
  Predicted { ChannelUtilPercent, DeliveryRatio,   // Plan: airtime model AND the sim twin
              DisruptionSeconds, ControlAirtimeMs }
  RollbackTarget, Verification                      // Knowledge + harness
  Verdict, Residual                                 // kernel + harness, filled after the fact
}
```

The kernel: `func Decide(state State, action EvidenceCarryingAction, clock Clock) Verdict`,
pure and deterministic, verdicts `Allow | Deny | Defer` (fail-closed: anything it
cannot establish is Deny; absent or single-source evidence is Defer). This mirrors
`reconcile.Outcome` and `internal/resilience` exactly: a pure verdict function
validated against synthetic inputs, then run unchanged against real data, actuating
nothing.

## 4. The decisive split: minimal viable safety core vs research frontier

The signed-autonomy plan's own open question ("is any of this overkill?") is here
promoted to a decision. The parts that earn their place now are small; the rest are
gated proposals with named unlock conditions.

| Minimal viable safety core (build now, $0, report-only) | Research frontier (gated proposal, and why it waits) |
|---|---|
| The veto: pure `Decide` -> Allow/Deny/Defer, fail-closed | COSE/CBOR wire format. Unlock: capsules ever refresh over LoRa. Until then the capsule is provisioned once at deployment, so bytes are free and signed canonical JSON suffices. |
| Hard invariants as provisioned constants (region/frequency, TX power and duty ceiling, SDR-transmit prohibition, identity and root-key prohibition), enforced even when the capsule is absent or garbage | Biscuit/macaroon attenuation. Unlock: a real steward-to-steward field-delegation requirement. There is none; a fixed signed grant is all the edge needs. |
| A minimal signed capsule: a Go struct (approved candidate set, budgets, reserve, dwell/cooldown, rollback reference, epoch), Ed25519 over canonical bytes, SHA-256 content address, integer-epoch supersession, verified offline against a provisioned key | The full five-state degrading lease. Collapse to three (Current, Restricted, Reauthorization-required) preserving the monotonic-narrowing invariant; the finer gradations are false precision nothing can act on yet. |
| The typed Evidence-Carrying Action seam contract | The rejoin treaty. Unlock: an actuating L2 loop exists, so state can diverge and there is something to rejoin. There is nothing to reconcile yet. |
| Objectives on the CRD (minDeliveryRatio, maxMessageAge) and a report-only assurance reducer that compares them to measured delivery | The multi-source corroboration engine. It is the Defer path for now (single-source or absent evidence -> Defer); building it needs an evidence pipeline that does not exist. |
| A small TLA+/Apalache model of the lease/authority machine as a gate before enabling any L2 action | Control barrier functions as a build target. Reclassify to framing only: there is no continuous LoRa-delivery model to write a barrier over; the real artifact is a discrete inductive invariant. |

Building the frontier prematurely is not neutral: a signed COSE/CBOR capsule with no
kernel to read it is dead weight, a five-state lease is more transitions to get
subtly wrong, and a rejoin protocol for adaptations that cannot occur is a state
machine that rots. Over-scoping is the doctrine's named number-one failure mode, and
this table is how the plan refuses it.

## 5. The two hard truths the review surfaced

These are honesty corrections, not features. Both change how the safety story must
be told.

### 5.1 Evidence integrity is the Achilles heel, and the SDR is not the trust anchor against the assumed adversary

The kernel requires corroboration (multiple independent sources agree) before a
disruptive action. But the two sources the project actually has for "is this channel
congested", the radio's own airtime figures and the SDR's occupancy, are both
functions of the same RF medium the adversary controls. An RF-adjacent attacker
floods; the radio reports high utilization; the SDR faithfully hears the same energy
and reports high occupancy; both agree; corroboration is "satisfied"; the kernel
permits the exact channel switch the attacker was herding toward. That is not
independence, it is two thermometers in the same fire.

This scopes a claim the threat model currently overstates. The receive-only SDR is a
genuine out-of-band anchor against the PSK-holding insider (who cannot make the SDR
hear a transmission that did not happen), but it is fully spoofable by the
transmitter-holding, RF-adjacent adversary, who is the assumed adversary the whole
closed loop exists to survive. The honest independent corroborator is the one signal
the attacker cannot forge without breaking crypto: the application delivery ratio,
authenticated by the message-authentication envelope. That reframes the auth envelope
from one item among many into the load-bearing corroboration source, and it aligns
with the formal boundary below: model-checking buys authority, lease, epoch, and
action-set integrity; it does not vouch for evidence classification, which the
adversary owns. State the boundary plainly: given faithful evidence, the kernel is
safe.

Corollary hardening, both from the formal lens: the kernel must re-parse and
re-validate the capsule from raw signed bytes independently (a shared parse bug fools
guard and planner identically, defeating Simplex separation), and the constitutional
"never" invariants must live as provisioned constants outside the capsule so they
hold when the capsule is absent or corrupt.

### 5.2 The shipped CommunicationIntent has no objectives, so it is not yet intent

`CommunicationIntentSpec` carries region, approved presets, channels, expected
traffic, and nodes. It has no objectives field. By the project's own ADR 0001 and
RFC 9315 definition, an approved set plus expected traffic fanned out to nodes is
policy plus configuration, not intent: it states means, not outcomes. Consequently
the compiler's `Feasible` condition means "renderable" (at least one node, a known
preset), not "objectives achievable", and the stronger `IntentInfeasible` verdict the
doctrine describes (physics forbids an objective) is never computed.

The fix is small and closes the loop in one move, because the objectives are
literally the quantities the harness already emits. Add `objectives`
(minDeliveryRatio, maxMessageAge) to the CRD; then a measured `IntentInfeasible` is
just `resilience.Window.DeliveryRatio < objective.minDeliveryRatio ||
Window.LatencyMsP50 > objective.maxMessageAge`: one comparison, over two typed values
already produced, that turns the report-only renderer into a report-only closed
assurance loop. The utilization-to-delivery transfer function needed to predict
before measuring already has its first two points, captured by hand in the survival
demo (about 19 percent utilization to 100 percent delivery, about 56 percent to 50
percent); fitting that curve replaces the folklore 25 percent ceiling constant with
an evidence-derived knee.

### 5.3 And two smaller ones

- Single-run numbers are not evidence. Every headline resilience figure is n=1, and
  messages within a run are correlated, so the experimental unit must be the run,
  with K trials reported as a median and a bootstrap confidence interval. The
  reproducibility chain is also broken at the root: the sim image is pinned to a
  moving tag, so pin a digest.
- Degrade-down under time uncertainty is a weaponizable availability attack. The
  lease correctly narrows authority when time is unknown, but "narrow" means no
  adaptation, so a time-denier can drive the whole fabric to a hold state and then
  jam the held channel, using the safety mechanism to disable the mesh. The
  "forbidden actions equals zero" scoreboard registers this as green. The plan needs
  a floor of L1 self-care that survives even the hold state, and a bounded-liveness
  property: under bounded clock disruption, a node still holding a valid capsule does
  not descend below Restricted.

## 6. The gated build ladder

Each rung names the gate that must be green to earn the next. Rungs 1 to 5 are
report-only and hardware-free; autonomy turns on only at rung 8, and only behind the
model-checking gate.

1. **Objectives and measured assurance.** Add `objectives` to the CRD; a report-only
   reducer compares measured `Window` to objectives and emits a measured
   `IntentInfeasible`. Gate: the assurance verdict is computed from harness data, not
   compile-time renderability.
2. **The typed Evidence-Carrying Action.** Replace the bare `advisor.Recommendation`
   at the proposer seam with the ECA contract. Gate: the advisor emits an ECA the
   kernel could check (predicted effect, rollback target, evidence provenance present).
3. **The sim as a pre-actuation twin.** Run a proposed candidate through the
   UDP-multicast sim before proposing it, filling `Predicted.DeliveryRatio`. Gate: a
   proposal carries a twin-predicted effect (honest scope: airtime and delivery, not
   RF relocation).
4. **The pure kernel `Decide`, with the bad-action battery.** Independent, fail-closed,
   Allow/Deny/Defer, hard invariants as provisioned constants, re-parsing the capsule
   from raw bytes. Gate: a table-driven battery of bad actions is refused (SDR
   transmit, region or power change, over-budget or reserve-spending change, no
   rollback target, repeat inside cooldown, candidate outside the approved set,
   superseded or tampered capsule -> Deny; single-source or absent evidence -> Defer;
   a clean L1 action and a rollback-to-last-known-good -> Allow), and CI enforces that
   the kernel package has no import edge to the planner.
5. **Minimal signed capsule and the three-state lease as pure functions.** Ed25519
   over canonical JSON, offline verification, integer-epoch supersession; the lease as
   a pure function of epoch, residence age, and boot events, conservative under
   uncertainty with the bounded-liveness floor. Gate: capsule verification and lease
   descent are exhaustively table-tested.
6. **Prove it under attack, and over a distribution.** Add an adversarial peer to the
   harness (a rogue node on the sim network with the channel key) and a partition
   primitive (data-plane-only network cut). The marquee scenario is the herder A/B:
   model candidate channels as distinct keys so channel choice is a measurable
   delivery partition; show the naive loop follows the herder and delivery craters
   while the guarded loop holds last-known-good and keeps delivering. Report K-trial
   distributions with confidence intervals; add a partition-tolerance and
   time-to-heal scenario reusing `ReducePhases` with connected/partitioned/rejoined
   phases. Gate: a `check-under-attack` CI target keeps the red-team metrics (channel
   switches per hour, superseded-capsule-accepted equals zero, disruptive-actions-on-
   single-source-evidence equals zero) inside threshold.
7. **The model-checking gate.** A small TLA+/Apalache model of the single-node
   lease/authority/kernel machine, checked under an adversarial environment, proving
   the named invariants: authority monotonicity, no ascent without a fresh capsule,
   hard invariants in every reachable state, kernel forward-invariance (Allow implies
   the successor state is safe), non-blocking (every safe state admits rollback or
   safe-hold), epoch integrity, no action outside the approved set, and budget,
   reserve, and dwell safety. The Evidence-Carrying-Action log doubles as the
   conformance trace. Gate: the model checks clean, and it is a precondition to
   enabling any L2 action.
8. **L1, then one L2 action.** Wire the kernel into a deterministic steward for L1
   self-care only; then enable exactly one L2 action, rollback-to-last-known-good, the
   safest possible move and the one that makes the safe set control-invariant.
   Channel and preset switching with the rendezvous procedure come later, still behind
   the kernel, and need hardware to exercise the frequency lever the sim cannot model.

## 7. What is honestly out of reach here, and why

Say it plainly so the plan does not imply more than it can show:

- Custody and store-carry-forward, the actual DTN differentiator, are not
  demonstrable on `meshtasticd --sim`: Meshtastic's store-and-forward is
  server-hardware-only, so a message sent during a partition is never replayed on
  rejoin. Demonstrating custody needs a `MessageIntent` gateway shim, a real build,
  not harness reuse. Until then, partition tests measure transport recovery of the
  live path, which is real and worth doing, but it is not custody.
- The frequency-relocation recovery lever needs hardware: the flat UDP sim caps
  per-node rate by a firmware broadcast cadence, not PHY time-on-air, so a faster
  preset does not recover delivery and only admission control does. The sim shows half
  the airtime story.
- Interference ground truth (jammer versus neighbor versus another network) needs a
  labeled IQ corpus and real RF; the advisor and the kernel's evidence classification
  are only as good as that, and it is genuinely gated.
- The capsule and lease deliver their value only when a node is detached across
  reboots on real target hardware whose monotonic-clock behavior cannot be verified in
  sim. Build the pure functions; do not claim the capsule story until it is exercised
  on a board.
- A mechanized refinement proof (the Go provably implements the spec) and a full
  self-stabilization or CRDT-convergence proof are overkill at tens of nodes;
  property-based conformance buys most of the assurance for a fraction of the effort.

## 8. What this changes about the other docs

This plan does not replace the doctrine or the ADRs; it sequences them and fixes
specific overclaims. The concrete edits it calls for:

- [ADR 0002](../adr/0002-signed-autonomy-and-rejoin-before-closed-loop.md): upgrade
  "candidate for a TLA+ model" to a named precondition. Proposed becomes Accepted when
  the kernel ships and the model-checking gate (rung 7 invariants) passes, not merely
  when a capsule format exists.
- [Signed-autonomy plan](../plans/signed-autonomy-and-safety-kernel.md): promote open
  question 8.6 to the decision in section 4 here; rewrite the build order to lead with
  the veto against an in-memory struct (capsule wire format last, not first); reframe
  the twelve-check kernel list as "locally decidable, build now" versus
  "evidence-dependent, Defer path"; state three verdicts, not five (rollback and
  safe-hold are steward responses); reclassify control barrier functions as framing
  only.
- [Threat model](../security/threat-model.md): scope the SDR out-of-band-trust-anchor
  claim to the PSK-holding insider and caveat it explicitly for the RF-adjacent
  adversary, per section 5.1.
- [Communication-intent plan](../plans/communication-intent.md): note that the shipped
  CRD has no objectives and that report-only assurance means comparing measured
  delivery to objectives, per section 5.2.
- [Roadmap](../roadmap.md): add the assurance-loop and Evidence-Carrying-Action work as
  a stage between the report-only compiler and actuation; move the model-checking stage
  earlier, as a gate before L2; rewrite the "Resilience, defined" section into a
  measurable standard (estimators with confidence intervals, a measured/designed/needs-
  hardware status tag per metric, envelope curves, and a reproducibility ledger keyed to
  an image digest).

## 9. The through-line

The project is one typed object and one comparison away from turning a report-only
renderer into a report-only closed assurance loop, and one pure function away from an
independent veto. Build those first, prove the veto by making it refuse a battery of
bad actions and resist a herder in the sim already shipped, measure over trials with
intervals, and hold everything else (signed wire formats, the full lease, rejoin,
heavy formal machinery) as gated proposals with named unlock conditions. That is the
honest, exceptional path: raise the bar of the claim to "provably and measurably
safe under attack," and keep the discipline of shipping the smallest correct thing
that earns the next rung.
