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

The authority and rejoin state machine is a candidate for a small TLA+ or PlusCal
model before implementation.

## Consequences

- Phase 6 moves behind an explicit prerequisite gate rather than being a standalone
  next step. The closed loop, when it arrives, proposes actions that the safety kernel
  authorizes and that carry evidence, a predicted effect, and a rollback.
- Hard safety is locally decidable from signed constraints and never depends on
  reaching peers or the control plane. Consensus is reserved for coordinated actions
  (domain-wide channel switch, credential rotation, steward election).
- Learning, if it is ever added, starts in shadow mode and cannot expand its own
  authorization envelope or define a hard constraint.
- This ADR is Proposed. It becomes Accepted when the capsule format and the safety
  kernel ship, before any autonomous L2 action is enabled.

## Alternatives considered

- Ship the closed loop now, gated only by Porch approval. Rejected: it does not
  survive management-plane loss, and an approval-per-action model pushes coherence
  onto the most stressed, least available human at the worst time.
- Allow unrestricted local autonomy when disconnected. Rejected: it removes the only
  safety boundary exactly when oversight is absent and the medium may be adversarial.
