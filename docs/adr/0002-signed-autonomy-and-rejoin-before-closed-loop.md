# ADR 0002: Define signed autonomy and rejoin semantics before the Phase 6 closed loop

- Status: Proposed
- Date: 2026-08-07
- Context doc: [design doctrine](../design/doctrine.md)

## Context

Phase 6 (closed-loop spectrum-aware automation) is the project's most attractive
feature and its most dangerous one. A loop that senses spectrum and changes channels
in response can be driven by an adversary who can jam, can herd every site onto one
candidate channel, can oscillate, and can burn an emergency reserve. The roadmap
already requires anti-herding controls for it.

The current design routes every spectrum-driven change through Porch approval. That
is sound for high-risk actions but it does not survive loss of the management plane,
which is the exact condition the project exists for. The naive fix, unrestricted
local autonomy, is worse. And a conventional GitOps controller treats locally adapted
state as drift on reconnection and restores central configuration immediately, which
can undo a necessary emergency adaptation, reboot many radios at once, or return
nodes to a jammed channel.

So the closed loop depends on two things that do not exist yet: a way to delegate
bounded autonomy that keeps working when the control plane is gone, and a disciplined
way to rejoin without a destructive drift correction.

## Decision

Do not implement the Phase 6 closed loop until the signed-autonomy and rejoin
semantics are defined and the safety kernel exists. Specifically, the following land
first:

- A signed, content-addressed Intent Capsule the edge can act on with no connectivity:
  hard invariants, mission objectives and priorities, approved degraded modes,
  permitted and prohibited actions, per-action evidence requirements, budgets and an
  emergency reserve, dwell and cooldown minimums, last-known-good reference, rollback
  and rendezvous procedures, and validity behavior.
- A degrading lease, not a binary one: expiry only ever narrows authority (Current to
  Grace to Restricted to Safe hold to Reauthorization required) and never creates it.
  Time-uncertainty behavior follows BPv7's precedent of carrying age when clocks are
  unreliable.
- Risk-tiered autonomy (L0 observe, L1 non-disruptive local care, L2 bounded reversible
  adaptation, L3 high-risk governance, L4 prohibited), with the first autonomous L2
  action being rollback-to-last-known-good, not channel switching.
- An independent, Simplex-style runtime safety kernel that can veto any proposed action
  and return the system to a safe region, validating legal region, transmit limits,
  authority, dwell, budget, reserve, approved candidate set, rollback presence,
  evidence freshness, corroboration, and the SDR-transmit and identity prohibitions.
- An explicit rejoin protocol (a treaty, not drift correction): epoch comparison,
  classification of local divergence, staged change plans, queue drainage under
  budget, and a new signed epoch that revokes the old one.

The authority and lease state machine is not merely a candidate for a small TLA+ or
PlusCal model; that model is a named precondition for enabling any autonomous L2
action (see the gated build order in
[`../design/road-to-safe-autonomy.md`](../design/road-to-safe-autonomy.md)). The
kernel ships first as a pure `Decide` function against an in-memory struct, proven by
an exhaustive bad-action battery; the model-checking gate proves the invariants
(authority monotonicity, no ascent without a fresh capsule, hard invariants in every
reachable state, kernel forward-invariance, non-blocking, epoch integrity) before L2
turns on. Two design corrections carry into implementation: the kernel re-parses the
capsule from raw signed bytes independently of the planner (a shared parse bug
defeats Simplex separation), and the constitutional "never" invariants are provisioned
constants outside the capsule, holding even when the capsule is absent or corrupt.

## Consequences

- Phase 6 moves behind an explicit prerequisite gate rather than being a standalone
  next step. The closed loop, when it arrives, proposes actions that the safety kernel
  authorizes and that carry evidence, a predicted effect, and a rollback.
- Hard safety is locally decidable from signed constraints and never depends on
  reaching peers or the control plane. Consensus is reserved for coordinated actions
  (domain-wide channel switch, credential rotation, steward election).
- Learning, if it is ever added, starts in shadow mode and cannot expand its own
  authorization envelope or define a hard constraint.
- This ADR is Proposed. It becomes Accepted when the safety kernel ships as a pure,
  fail-closed `Decide` function and the model-checking gate passes its invariants,
  before any autonomous L2 action is enabled. Note the reordering: the kernel and its
  proof come first; the signed wire format (COSE/CBOR) is deferred behind an explicit
  unlock condition (capsules refreshing over the air), because a signed capsule with no
  kernel to read it is dead weight. The evidence the kernel acts on is itself an attack
  surface: model-checking secures authority, lease, epoch, and action-set integrity but
  cannot vouch for evidence classification, so the safety claim is conditional, "given
  faithful evidence, the kernel is safe," and the independent corroborator against the
  RF-adjacent adversary is the envelope-authenticated delivery ratio, not the RF-derived
  airtime or SDR signals.

## Alternatives considered

- Ship the closed loop now, gated only by Porch approval. Rejected: it does not
  survive management-plane loss, and an approval-per-action model pushes coherence
  onto the most stressed, least available human at the worst time.
- Allow unrestricted local autonomy when disconnected. Rejected: it removes the only
  safety boundary exactly when oversight is absent and the medium may be adversarial.
