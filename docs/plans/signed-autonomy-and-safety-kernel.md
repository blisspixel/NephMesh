# Signed autonomy and the safety kernel

Status: design direction only, gated by [ADR 0002](../adr/0002-signed-autonomy-and-rejoin-before-closed-loop.md), not shipped. Nothing described here is built; this document explores a shape and an order, so the eventual closed loop cannot be reduced to unguarded knob-turning.

This note develops five pieces named in the [design doctrine](../design/doctrine.md) and required by ADR 0002: the signed Intent Capsule, the degrading lease, the deterministic site steward, the independent Simplex-style safety kernel, and risk-tiered autonomy L0 to L4. It leans on outside precedent where precedent exists, and it tries to be honest about which precedents are mature and which parts of them would be overkill for tens to low hundreds of LoRa nodes.

## 1. Why signed autonomy and a safety kernel come before any closed loop

Two facts about NephMesh force this ordering, and they are not negotiable design preferences.

First, the control plane is not in the field. The Kubernetes, Porch, and GitOps layer provisions the mesh from a powered site and is explicitly not a runtime dependency: deployed nodes must keep working with the cluster gone (see `AGENTS.md`). So the authority to act, and the limits on that authority, cannot live in the cluster at decision time. They have to be carried to the edge, in a form the edge can verify and act on with no Git, no Porch, no cluster API, and no cloud. That is what the Intent Capsule is for.

Second, the RF medium is untrusted and possibly adversarial. A jammer or a compromised node can manufacture the very observations that a naive sense-and-react loop treats as ground truth: it can make a good channel look congested, herd every site onto one candidate, oscillate a controller, burn an emergency reserve, or expose a fallback channel. The doctrine states this plainly in sections 8 and 10. A loop that wires observation to actuation in that environment is not resilient, it is a remote-controlled attack surface. So a closed loop needs a component whose job is to refuse actions, independent of the planner that proposes them.

The conclusion in ADR 0002 follows: define the capsule, the lease, the tiers, the safety kernel, and rejoin before Phase 6, not alongside it. This document covers all but rejoin, which has its own note.

## 2. The Intent Capsule (conceptual)

Conceptual. No wire format is committed here. The goal is to pick a direction and record the tradeoffs.

### 2.1 What it is

An Intent Capsule is a signed, content-addressed object the steward stores locally and evaluates offline. It is the field-resident residue of central intent: the thing "living intention left behind" has to actually be, if that phrase is to mean anything (doctrine section 5). It is authorization plus constraints plus procedures, not configuration. It does not say "set LONG_FAST"; it says which actions are permitted, under what evidence, within what budget, and what to do when its own authority lapses.

### 2.2 Fields

Carried, at least (doctrine section 5, ADR 0002):

- Identity and lineage: intent id, epoch, parent digest, issuer, signature.
- Hard invariants: legal region and frequency plan, transmit-power ceiling, the SDR-transmit prohibition, the identity and root-key-change prohibition.
- Mission: objectives and their priority order (lexicographic, not weighted; doctrine section 2), approved degraded modes.
- Action authority: permitted actions and prohibited actions, keyed to the risk tier (section 7 below), with the approved candidate set for each (for example the preapproved channel or preset list).
- Evidence requirements: per action, what must corroborate before it is legal (which sources, minimum freshness, minimum agreement).
- Budgets: airtime and disruption budgets, plus an emergency reserve that lower-priority traffic may not spend.
- Timing: minimum dwell and cooldown, so an authorized action cannot be repeated faster than the invariant allows.
- Recovery: last-known-good reference, rollback procedure, rendezvous procedure for coordinated changes.
- Validity: signed epoch, lease behavior, and explicit time-uncertainty behavior (section 4).
- Rejoin behavior and retention rules (retention classes per doctrine section 5).

### 2.3 Format direction: COSE-signed CBOR, not a JWT

The capsule is a signed authorization object for constrained, sometimes disconnected nodes. Two candidate families exist.

Attenuable capability tokens. [Macaroons](https://research.google/pubs/pub41892/) and [Biscuit](https://www.biscuitsec.org/) let a holder narrow a token offline by appending caveats, with the result still verifiable without contacting the issuer. Biscuit adds a Datalog authorization language and public-key (Ed25519) verification; macaroons use chained HMAC. [SPIFFE/SVID](https://spiffe.io/docs/latest/spiffe-about/overview/) solves workload identity but assumes far more online infrastructure than a detached LoRa node has. The attenuation model is genuinely attractive for delegation, and it is worth keeping in view for the steward-to-steward case. For the central-to-edge capsule it is probably overkill: NephMesh does not need holders to mint narrower sub-capsules in the field, it needs a fixed signed grant the edge reads and the safety kernel checks. Attenuation logic is attack surface we would rather not run on a constrained node.

Signed structured claims. A [JWT](https://datatracker.ietf.org/doc/html/rfc7519) is JSON and verbose. Its CBOR-native sibling, the [CBOR Web Token (CWT), RFC 8392](https://datatracker.ietf.org/doc/html/rfc8392), carries the same claim model but is signed with [COSE (RFC 9052)](https://datatracker.ietf.org/doc/html/rfc9052) over [CBOR (RFC 8949)](https://datatracker.ietf.org/doc/html/rfc8949), which is compact and was designed for exactly this class of device. [PASETO](https://paseto.io/) removes JWT's algorithm-agility footguns but stays JSON-first.

Direction: a COSE-signed CBOR object, shaped like a CWT with NephMesh-specific claims. Reasons: CBOR is compact where airtime and flash are scarce; COSE fixes the signature algorithm per deployment and avoids JWT-style algorithm confusion; the format is an IETF standard with constrained-device intent, not a bespoke encoding. This is a direction, not a commitment; if field delegation between stewards becomes a real requirement, Biscuit's attenuation model should be revisited for that edge only.

### 2.4 Signing, content addressing, offline verification

- Content addressing: the capsule id is a hash of its canonical serialization (a self-describing multihash-style digest). Epoch and parent digest form a hash-linked chain, so a steward can tell newer from older and detect a replay of a superseded capsule without asking anyone.
- Signing: the issuer signs the canonical bytes with the constitutional root key. The public key (or its trust anchor) is provisioned onto the node offline, before deployment, consistent with the project's air-gapped-first stance. There is no default key and no network-from-the-field fetch of trust material.
- Offline verification: on receipt the steward checks the signature against the provisioned anchor, checks the digest against the content, checks epoch ordering against what it already holds, and only then treats the capsule as authority. All of this is local arithmetic. None of it contacts the cluster.

## 3. The degrading lease

A binary lease is wrong for this project. "Authority expired, stop all service" can be the exact opposite of the mission during a disconnected emergency, which is the condition the whole project exists for. The invariant is simpler and safer to state as a monotonic one.

> Lease expiry only ever narrows authority. It never creates authority, and it never triggers a new action that was not already permitted.

### 3.1 The state ladder

From the doctrine (section 5) and ADR 0002:

| State | What remains authorized |
|---|---|
| Current | all actions the capsule permits, within budget and evidence rules |
| Grace | reversible, service-preserving actions only; no irreversible or disruptive change |
| Restricted | no new topology, channel, key, role, or identity change; rollback and quiet mode remain |
| Safe hold | passive receive, essential custody, hold last legal config; no autonomous change |
| Reauthorization required | no autonomous authority; wait for a fresh signed capsule |

Each step down is a strict subset of the one above. Nothing becomes newly permitted by descending. A node deep in Safe hold is still doing the mission-critical thing (receiving, holding custody, keeping the last legal config), it has simply lost the right to change anything.

### 3.2 Transition rules

- Descent is driven by elapsed validity relative to the signed epoch, tempered by time uncertainty (below). Descent is always allowed and is the safe default.
- Ascent (regaining authority) happens only on receipt and verification of a fresh, higher-or-equal-epoch capsule. Elapsed time alone can never move a node up the ladder.
- Hard invariants (region, transmit-power ceiling, SDR-transmit and identity prohibitions) do not degrade with the lease. They hold in every state, including Reauthorization required, because they are locally decidable from signed constraints and must not depend on reaching anyone (doctrine section 10).

### 3.3 Clock-uncertainty behavior

Cheap field devices lose time across reboots and can be denied synchronization, so an absolute wall-clock expiry is not trustworthy. [Bundle Protocol v7 (RFC 9171)](https://datatracker.ietf.org/doc/html/rfc9171) faces the same problem and carries bundle age when accurate clocks are unavailable rather than trusting absolute timestamps. The capsule should follow that precedent: combine the signed epoch, monotonic residence age since receipt where the hardware offers a monotonic counter, recorded boot events, and conservative behavior when time is unknown. Conservative means: when the node cannot establish how much validity remains, it degrades rather than assumes authority. Uncertainty pushes down the ladder, never up. This is deliberately asymmetric, because the cost of wrongly keeping authority in an adversarial medium is higher than the cost of wrongly dropping it.

## 4. The site steward

The steward is the edge-resident component that actually decides, between central GitOps and the device operator (doctrine section 4). Its most important property is what it is not.

### 4.1 It is a deterministic state machine, not an agent

The steward is a small deterministic state machine. It is not an LLM, not a solver, and not a broadly privileged agent. That is a deliberate constraint, for three reasons:

- Reviewability: a finite-state controller can be read, tested with golden cases, and (per doctrine section 11) is a candidate for a TLA+ or PlusCal model. A learned policy cannot be audited the same way.
- Adversarial robustness: the medium is an adversarial input surface. A deterministic machine has a bounded, enumerable response to any input; it cannot be argued into an action outside its table.
- Authority containment: an agent that can reason its way to new actions is exactly the self-expansion of authority the doctrine forbids (section 10, L4). Determinism makes the authority envelope a property of the code, not of a prompt.

Learning, if it ever arrives, sits outside the steward in shadow mode and can only propose; it never defines a constraint or adds an action (doctrine section 10). This document assumes no learning.

### 4.2 Inputs and outputs

Inputs: the verified Intent Capsule and its current lease state; local evidence (radio airUtilTx and channelUtilization, application delivery signals, neighbor counts, and where present SDR occupancy); the current lease/time state; last-known-good; cooldown and budget state.

Outputs: a proposed action drawn only from the capsule's permitted set, packaged as an Evidence-Carrying Action (doctrine section 8) with its evidence, predicted effect, and rollback target; a decision record for the audit summary; and, after the safety kernel and any actuation, a verify-then-retain-or-rollback outcome.

### 4.3 Order of operation

The steward runs L1 non-disruptive actions first, before considering anything reversible-but-disruptive. It prefers traffic shaping and semantic degradation over RF reconfiguration. Every proposed action, even an L1 one, is offered to the safety kernel; the steward never actuates on its own say-so.

## 5. The safety kernel

The kernel is the independent veto. It is the [Simplex architecture](https://ieeexplore.ieee.org/document/936249) applied to radio autonomy: Lui Sha's insight is that a complex, possibly optimizing controller can retain performance while a simpler, verified safety controller retains authority to override it and return the system to a safe region. In control-theoretic terms the same role is played by [control barrier functions](https://arxiv.org/abs/1903.11199) and the broader [runtime assurance](https://ntrs.nasa.gov/citations/20160006526) and [runtime verification](https://link.springer.com/article/10.1007/s10703-011-0114-4) literature: a monitor with a forward-invariant safe set that filters a nominal controller's proposed action.

### 5.1 Why it must be independent of the planner

If the same component both proposes and approves, a flaw in the planner is a flaw in the guard. The whole value of Simplex is separation: the planner may be a heuristic today and a statistical or learned model tomorrow, and the safety argument must not have to be re-made each time the planner changes. So the kernel is a distinct, small, verified module with its own copy of the hard constraints (from the capsule and the provisioned constitution), and it treats the planner as untrusted. It fails safe: if the kernel cannot establish that an action is legal, it denies.

### 5.2 What it validates

For each proposed action the kernel checks, at minimum (doctrine section 8, ADR 0002):

- legal region and frequency
- transmit limits (power, duty)
- action authority for the current lease state and tier
- capsule validity (signature, epoch, not superseded)
- minimum dwell since the last change of this kind
- change budget not exceeded
- emergency reserve preserved (the action does not spend reserve for non-emergency traffic)
- the proposed candidate is in the approved candidate set
- a rollback target is present and reachable
- evidence freshness meets the per-action requirement
- required corroboration is present (multiple independent sources agree; doctrine section 8)
- the SDR-transmit prohibition (never)
- the identity and root-key-change prohibition (never)

### 5.3 Verdicts

The kernel returns one of:

- Allow: all checks pass; the steward may actuate the smallest admissible change.
- Deny: at least one check fails; the action is refused and the reason recorded.
- Defer: evidence is insufficient or conflicting; the action waits for corroboration. Abstention is an active safety behavior, not a failure (doctrine section 8).
- Rollback: the current state is unsafe or a prior change did not verify; return to last-known-good.
- Safe hold: no legal action and no safe forward path; drop to passive receive on the last legal config (the lease Safe hold state).

Deny, Defer, Rollback, and Safe hold are all first-class successful outcomes. A kernel that never refuses is not doing its job.

## 6. Risk-tiered autonomy L0 to L4

The tiers bound what may ever run without a human, with concrete Meshtastic examples (doctrine section 12).

| Level | Meshtastic examples | Authority |
|---|---|---|
| L0 Observe | read airUtilTx and channelUtilization, delivery ratio, battery, neighbor count | automatic |
| L1 Non-disruptive local care | shed telemetry, aggregate position reports, slow scan cadence, reprioritize the send queue, store instead of send, enter a declared quiet mode | automatic inside the signed envelope |
| L2 Bounded reversible adaptation | roll back to last-known-good, then switch among preapproved channels or modem presets, alter approved reporting cadence | automatic only with corroborated evidence, cooldown, rollback, and post-change verification |
| L3 High-risk governance | change region, transmit power, root credentials, membership, routing role, firmware, factory wipe, persistent topology change | explicit human or central approval |
| L4 Prohibited | SDR transmit, autonomous power escalation, bypassing legal limits, any self-expansion of authority | never |

Classification can vary by driver and deployment: a modem-preset change might be L2 in a lab and L3 in a large deployed mesh, because a preset change can partition the network.

### 6.1 Why the first L2 action is rollback-to-last-known-good

The first bounded reversible action the project enables is rollback to last-known-good, deliberately not channel switching. The reasons:

- A channel switch is the action an adversary most wants to trigger. Rollback is not; it returns toward a known-safe configuration rather than chasing observations that a jammer may be manufacturing.
- Rollback exercises the entire L2 machinery (corroborated evidence, cooldown, a rollback target, post-change verification) on the safest possible move, so the machinery is proven before it is trusted with a move that can isolate the mesh.
- A channel change is not an ordinary software canary: a radio channel change can cut the changed node off from its peers, so it needs the rendezvous procedure (announced switch epoch, repeated pre-change notice, predeclared fallback, rollback timeout, two-sided verification; doctrine section 9). That is more machinery, and it should land after rollback, not before.

## 7. Mapping to the roadmap and a build order

This work sits under ADR 0002 and matches the doctrine's Order of operations (section 16), stages 11 to 14: build the steward with L1 actions only, add the independent safety kernel, add one L2 action (rollback-to-last-known-good) before channel switching, then the detached-epoch and rejoin protocol, then model-check the authority and rejoin state machine. Rejoin and model-checking are their own notes; this document covers the capsule, lease, steward, kernel, and tiers.

Suggested build order within this topic:

1. Fix the capsule schema and the offline verification path (COSE-signed CBOR, content addressing, provisioned trust anchor). Nothing acts until a capsule can be verified with the cluster gone.
2. Implement the degrading lease as a pure function of epoch, residence age, and boot events, with conservative behavior under time uncertainty. Test descent and the no-ascent-without-capsule rule.
3. Build the deterministic steward restricted to L0 and L1 only. No reversible RF change yet.
4. Add the independent safety kernel with the full check list and the five verdicts, exercised first against L1 proposals.
5. Enable exactly one L2 action, rollback-to-last-known-good, behind the kernel, with cooldown and post-change verification.
6. Only then design channel and preset switching with the rendezvous procedure, still behind the kernel.

Each stage is testable in simulation (`meshtasticd -s`, Meshtasticator) with no hardware and no cluster, which keeps it on the project's $0, offline-first path.

## 8. Open questions

- Capsule size versus expressiveness. Full per-action evidence rules, candidate sets, and budgets in one COSE object may be large for a flash-constrained node and pointless to move over LoRa. How much is provisioned once at deployment versus refreshed, and what is the realistic byte budget?
- Trust anchor rotation offline. If the constitutional root key must rotate, how is the new anchor provisioned to nodes that are already detached, without a network-from-the-field dependency and without weakening the identity prohibition?
- Monotonic time availability. The lease leans on a monotonic residence counter and recorded boot events. Which target hardware actually provides a trustworthy monotonic source across reboots, and what is the fallback where it does not?
- Where the kernel runs. Is it a separate process, a separate module in the steward, or ideally a separate device? Simplex's separation is strongest when the guard is physically independent, but a LoRa node may not have the room. What independence is achievable and sufficient?
- Corroboration with one gateway. Required corroboration assumes multiple evidence sources. A lone gateway with no SDR and few neighbors may never reach corroboration, so it should mostly Defer. Is permanent L1-only operation the honest answer for such nodes?
- Is any of this overkill at this scale? For tens of nodes, some of this apparatus may cost more than it saves. The parts that clearly earn their place are the hard invariants, the offline-verifiable capsule, and the veto. Attenuable tokens, a solver, and heavy formal machinery should stay proposals until a concrete need appears.

## 9. Sources

- [RFC 9315, Intent-Based Networking: Concepts and Definitions](https://datatracker.ietf.org/doc/html/rfc9315)
- [RFC 9171, Bundle Protocol Version 7](https://datatracker.ietf.org/doc/html/rfc9171)
- [RFC 8949, Concise Binary Object Representation (CBOR)](https://datatracker.ietf.org/doc/html/rfc8949)
- [RFC 9052, CBOR Object Signing and Encryption (COSE)](https://datatracker.ietf.org/doc/html/rfc9052)
- [RFC 8392, CBOR Web Token (CWT)](https://datatracker.ietf.org/doc/html/rfc8392)
- [RFC 7519, JSON Web Token (JWT)](https://datatracker.ietf.org/doc/html/rfc7519)
- [PASETO, Platform-Agnostic Security Tokens](https://paseto.io/)
- [Macaroons: Cookies with Contextual Caveats for Decentralized Authorization in the Cloud (Birgisson et al., Google)](https://research.google/pubs/pub41892/)
- [Biscuit authorization tokens](https://www.biscuitsec.org/)
- [SPIFFE and the SPIFFE Verifiable Identity Document (SVID)](https://spiffe.io/docs/latest/spiffe-about/overview/)
- [L. Sha, "Using Simplicity to Control Complexity," IEEE Software (2001)](https://ieeexplore.ieee.org/document/936249)
- [A. Ames et al., "Control Barrier Functions: Theory and Applications" (2019)](https://arxiv.org/abs/1903.11199)
- [Runtime Assurance framework, NASA technical report](https://ntrs.nasa.gov/citations/20160006526)
- [M. Leucker, C. Schallhart, "A brief account of runtime verification"](https://link.springer.com/article/10.1007/s10703-011-0114-4)

These are pointers to the ideas this note leans on, not claims that NephMesh implements them. They are here so the reasoning can be checked rather than taken on faith.
