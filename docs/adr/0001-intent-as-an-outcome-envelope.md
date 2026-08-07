# ADR 0001: Intent is an outcome envelope; `MeshtasticNode` is a compiled artifact

- Status: Proposed
- Date: 2026-08-07
- Context doc: [design doctrine](../design/doctrine.md)

## Context

The operator shipped in 0.2.0 reconciles a device to a fixed desired configuration:
region, modem preset, role, owner, and MQTT. That is device-level configuration in
the sense RFC 9315 (Intent-Based Networking) uses the word: an implementation choice,
not an outcome. The temptation, as the CRD grows, is to keep adding fields to
`MeshtasticNode` (channel management, richer status, connection options, then
spectrum-driven overrides) until it quietly becomes a configuration API with an
adaptive loop bolted on.

That path has two failure modes the project wants to avoid. First, a contested
wireless fabric has no single configuration that stays correct for long, so a
target-state-convergence model fights the medium instead of working with it. Second,
once a closed loop and a central plane both write the same fields, two controllers
end up fighting over one actuator.

## Decision

Treat intent as a bounded envelope of acceptable mission outcomes, distinct from the
configuration that implements it, and make `MeshtasticNode` the compiled output of a
higher-level intent rather than the source of truth.

Concretely:

- A higher-level `CommunicationIntent` (working name) expresses outcomes,
  constraints, priorities, approved operating sets, and degraded modes.
- A compiler renders that intent, plus current feasibility evidence, into the
  `MeshtasticNode` resources the operator already reconciles. `MeshtasticNode`
  becomes a stable device-level intermediate representation, analogous to compiled
  output, not the language the operator writes in.
- Control is governed by viability (keep the fabric inside the acceptable set, and
  when physics forbids that, shed the least important objectives first) using a
  lexicographic decision order: hard invariants, then mission priority, then
  disruption cost, then resource cost. The first implementation is a deterministic
  rule engine, not an optimizer or a model.
- The medium informs feasibility, never values. `IntentInfeasible` is a legitimate
  reported condition, not a controller error.
- Authority ownership equals field ownership: a given mutable variable has exactly
  one writer. The strategic layer owns approved sets (for example
  `allowedModemPresets`); a lower layer owns the current selection.

### Invariants this establishes

1. Hard constraints (legal region, transmit power, SDR receive-only, root trust,
   identity) are never traded away by any optimization or learned behavior.
2. No two control layers own the same mutable field.
3. Infeasibility is reported, not silently approximated.
4. `MeshtasticNode` remains a valid, independently testable device API even as the
   intent layer is added above it.

## Consequences

- The near-term roadmap is unchanged in order but clearer in intent: the core
  (channels and keys, multi-site packaging, key rotation, reproducible demos) comes
  first, and the intent layer arrives in report-only mode before it ever actuates.
- New resources are proposed above `MeshtasticNode` (`CommunicationIntent`,
  `ChannelBudget`, `AutonomyPolicy`, `ChangePlan`), not new fields inside it. High-rate
  observations stay in metrics and logs; CRDs carry consequential summaries and
  decisions.
- The project gains a clear test for scope creep: if a proposed change turns
  `MeshtasticNode` into an outcome API or gives two layers write access to one field,
  it violates this ADR.
- This ADR is Proposed. It becomes Accepted when the first `CommunicationIntent`
  compiler ships in report-only mode.

## Alternatives considered

- Keep extending `MeshtasticNode` with outcome-like fields and an inline adaptive
  loop. Rejected: it collapses the intent/configuration distinction and leads to
  multiple writers on one field.
- Jump straight to a closed-loop, optimizing controller. Rejected: it front-loads the
  hardest and least-safe part before the core is solid, and an optimizer that can
  trade a hard constraint for performance is a safety hazard. See ADR 0002.
