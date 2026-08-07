# Rejoin as a treaty, and self-stabilization as the formal goal

Status: this is a design direction, not shipped code. It elaborates doctrine section
11 ([design doctrine](../design/doctrine.md)) and [ADR 0002](../adr/0002-signed-autonomy-and-rejoin-before-closed-loop.md),
and maps onto Order of operations stages 15 and 16 in the [roadmap](../roadmap.md).
Nothing here is implemented. It exists so that when the rejoin protocol is built, it
is a controlled reconciliation and not an ordinary GitOps drift sync.

## 1. The problem: reconnection is the most dangerous moment

NephMesh assumes the Kubernetes control plane provisions from a powered site and is
not a runtime dependency. Nodes, and any edge site steward, keep operating with the
cluster gone and reconnect later. That reconnection, not the outage, is where the
most damage can happen.

A conventional GitOps controller has one job on reconnect: make the live state match
the declared state in Git, immediately. Argo CD calls this self-heal, and its
default behaviour on detecting a difference between desired and live state is to
correct it ([Argo CD auto-sync and self-heal](https://argo-cd.readthedocs.io/en/stable/user-guide/auto_sync/)).
Flux reconciles on an interval toward the source of truth in the same spirit
([Flux core concepts](https://fluxcd.io/flux/concepts/)). For stateless workloads
that is exactly right: local changes are usually mistakes or manual drift, and
snapping back to Git is the safe default.

For a contested radio fabric the same reflex is destructive. While detached, a site
steward may have taken bounded, signed-and-authorized local actions (doctrine
sections 4 and 12): rolled back to a last-known-good channel, entered a declared
quiet mode, shed telemetry, or held custody of undelivered life-safety messages.
Treating all of that as drift and restoring the central configuration in one sync
can, in the worst case:

- undo a necessary emergency adaptation and return nodes to a channel that was
  jammed at the moment the outage began,
- reboot many radios at once (Meshtastic applies config with a reboot; a fleet-wide
  apply is a fleet-wide outage),
- replay stale or superseded directives that were valid when Git last saw the fabric
  but are wrong now,
- discard message custody by overwriting queues, and
- dump queued telemetry into a channel that is still recovering, spending the exact
  airtime the recovery needs.

So rejoin cannot be a sync. It has to be a negotiated handover: compare what each
side believes, decide deliberately which locally adapted state to keep, and stage
the transition under the same airtime and disruption budgets that govern any other
change. The doctrine names this a treaty rather than drift correction.

## 2. The rejoin state machine

The states below are the doctrine's. The machine is deliberately explicit so that
"reconnected" is never a single edge that triggers a full apply.

```text
        Managed
           |  (management plane lost, lease still valid)
           v
        Detached
           |  (steward begins bounded local governance)
           v
      LocallyGoverned ------------------> Degraded
           |        \                        |
           |         \--> Quiescent          |  (no useful traffic, deliberate silence)
           |              (insufficient       |
           |               evidence to act)   |
           |  (connectivity to control plane detected)
           v                                  |
      LinkRestored <--------------------------/
           |  (freeze non-safety autonomous change)
           v
      RejoinPending
           |  (exchange intent and authority epoch digests)
           v
      EpochCompared
        /    |     \
       /     |      \
      v      v       v
AdoptLocal  Staged   CompileNewState
State      Rollback  (central epoch supersedes; render new plan)
      \      |       /
       \     |      /
        v    v     v
        VerifiedManaged
           |  (new signed capsule issued, old epoch revoked)
           v
        Managed   (loop closed)
```

Transition triggers, stated as guards rather than timers where possible:

- Managed to Detached: the steward cannot reach the control plane and its cached,
  signed Intent Capsule (doctrine section 5) is still within a usable lease tier.
- Detached to LocallyGoverned: the steward begins acting inside its autonomy
  envelope. Under a degrading lease, authority only ever narrows over time (Current
  to Grace to Restricted to Safe hold), it never grows.
- LocallyGoverned to Degraded or Quiescent: evidence-driven mode changes. Degraded
  means constrained but still transmitting essential traffic; Quiescent means there
  is not enough evidence to act safely and silence is the chosen behaviour. Both are
  legitimate detached states, not faults.
- LocallyGoverned to LinkRestored: connectivity to the control plane is observed.
  The trigger is observed reachability, not a clock.
- LinkRestored to RejoinPending: the steward freezes all non-safety autonomous
  change. Safety actions (rollback to last-known-good, entering a safer mode) remain
  permitted, because freezing them could be the unsafe choice.
- RejoinPending to EpochCompared: both sides exchange intent-epoch and
  authority-epoch digests and the steward rejects any replayed or superseded
  directive before comparing.
- EpochCompared to one of three resolutions, per divergence class (section 4):
  AdoptLocalState keeps the local adaptation, StagedRollback returns to central
  configuration in stages, CompileNewState renders a fresh plan when neither the
  local nor the old central state is right.
- resolution to VerifiedManaged: the chosen ChangePlan is applied in staged order
  and mission behaviour is verified, not just config sync.
- VerifiedManaged to Managed: a new signed capsule is issued, the old epoch is
  revoked, and the detached history is compacted into a durable audit summary.

## 3. The rejoin sequence, step by step

1. Detect restored connectivity. Reachability to the control plane is the trigger,
   not a clock; field clocks are unreliable, which is why the capsule already reasons
   about time the way Bundle Protocol v7 does (section 5).

2. Freeze non-safety autonomous change. Enter RejoinPending. Safety rollbacks and
   moves to a safer mode stay allowed.

3. Exchange epoch digests. Both sides publish digests of their current intent and
   authority epochs and their parent chain. This is a comparison, not yet a transfer
   of state.

4. Reject replayed or superseded directives. Any directive whose epoch is older than
   the steward's current authority, or already superseded, is refused. This is the
   split-brain guard: the newest signed epoch fences the older one, like a fencing
   token that stops a delayed or resumed actor from acting on stale authority
   ([Kleppmann, How to do distributed locking](https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html)).
   Hard safety must stay locally decidable and must not wait on peers (doctrine
   section 10).

5. Upload the detached decision and evidence summary: the steward's
   Evidence-Carrying Actions (doctrine section 8), what it did, on what evidence,
   under which capsule epoch and permitted action, with predicted and observed
   effect. This is what makes the treaty auditable.

6. Classify local divergence into one of five classes (section 4).

7. Generate a ChangePlan for any required transition. Reuse the existing ChangePlan
   machinery (doctrine section 9): exact fields changing, before and after values,
   expected reboots and outage, control airtime, a fallback and a rollback target, a
   disruption score against a budget. No transition is applied without one.

8. Drain stored traffic under class and airtime budgets. Queued telemetry does not
   get dumped into a recovering channel. Custody and life-safety traffic drain
   first; best-effort telemetry waits, and may be aggregated or dropped under
   declared storage pressure. The airtime commons still governs (doctrine section 6).

9. Apply in a staged order. Prefer traffic shaping and semantic de-escalation before
   RF reconfiguration. Batch reboot-causing settings. Limit to a bounded number of
   nodes or one failure domain at a time. A channel or preset change needs a
   rendezvous, not an ordinary canary, because a channel change can isolate the
   canary from the mesh (doctrine section 9).

10. Verify mission behaviour, not just config sync. Confirm real delivery and that
    expected peers are present, from both sides of any partition boundary. A change
    that only made the local view look healthy while remote nodes vanished must roll
    back.

11. Issue a new signed capsule and revoke the old epoch. There is now exactly one
    current authority epoch for the scope. This is the state that self-stabilization
    (section 6) treats as legitimate.

12. Compact detached history into a durable audit summary. Per the retention classes
    in doctrine section 5, safety, authority, custody, and provenance persist; stale
    tactical and optimization state is allowed to decay.

### Divergence classification

Step 6 is where the treaty earns its name. Each locally diverged field is placed in
one class, and the class, not a diff, decides the resolution:

| Class | Meaning | Default resolution |
|---|---|---|
| Unauthorized | change the steward was not permitted to make | StagedRollback to central state, flagged in audit |
| Authorized and still useful | permitted adaptation that remains correct now | AdoptLocalState, promoted into the new epoch |
| Authorized but obsolete | permitted at the time, no longer the right state | CompileNewState or StagedRollback |
| Safety rollback | a rollback to last-known-good or a safer mode | AdoptLocalState, then reassess whether to advance |
| Unknown | cannot be classified from available evidence | hold safe, do not auto-apply, surface to a human |

Unknown is a first-class outcome. Converting unknown into either "keep" or "revert"
silently is the failure mode this classification exists to prevent (doctrine section
7 uses the same three-valued healthy/unhealthy/unknown reasoning for status).

## 4. Selective CRDTs

Local-first research shows replicas can accept writes while disconnected and later
converge without a central server, using Conflict-free Replicated Data Types
([Kleppmann et al., Local-first software](https://www.inkandswitch.com/local-first/);
[Shapiro et al., Conflict-free Replicated Data Types](https://inria.hal.science/inria-00609399/document)).
That property is genuinely useful for NephMesh, but only for the data where merge is
semantically safe. It is actively unsafe for radio configuration, and running it as
continuous gossip over LoRa spends the very resource the project is trying to
protect.

| Data | Merge model | Why |
|---|---|---|
| Observations (occupancy, delivery, RSSI/SNR samples) | CRDT, append-only or grow-only set | order-independent, additive, never needs a winner |
| Decision ids and their evidence | CRDT, add-only | each decision is a distinct immutable fact |
| Acknowledgements and delivery receipts | CRDT, add-only or OR-set | more acks only ever strengthen the record |
| Incident log | CRDT, grow-only log | append-only history; concatenation is the merge |
| Custody state (message held vs delivered vs expired) | epoch and lifetime resolved, not LWW | a set-merge could resurrect a delivered or expired item; BPv7 lifetime and age semantics decide this instead |
| Active channel (A vs B) | epoch-resolved | two channels are not a set to union; one is correct |
| Role (router vs client) | epoch-resolved | a node cannot be both; merge has no safe meaning |
| Key epoch (14 vs 15) | epoch-resolved, newest signed wins under fencing | last-writer-wins on keys is a security hole |
| Mode (quiet vs emergency transmit) | epoch-resolved under the safety kernel | opposite intents; merging them is nonsensical |

The rule of thumb: if two concurrent values can both be true at once, a CRDT is a
good fit; if exactly one must be true, the conflict is semantic and belongs to epoch
resolution and the safety kernel, not to last-writer-wins or set-merge. Custody sits
deliberately on the epoch side. BPv7 gives bundles an explicit lifetime and, for
clockless nodes, a Bundle Age block to reason about expiry without a trusted clock
([RFC 9171](https://www.rfc-editor.org/rfc/rfc9171.html)). Worth noting honestly:
BPv7 removed the custody-transfer mechanism that BPv6 carried
([RFC 5050](https://www.rfc-editor.org/rfc/rfc5050)), so custody here means the
project's own application-level store-and-forward using BPv7-style lifetime and age
semantics, not a standard BPv7 custody feature.

Why continuous CRDT gossip over LoRa is the wrong default: CRDTs converge by
exchanging updates until every replica has seen every update. That anti-entropy
traffic is cheap on the internet and expensive on a shared LoRa channel where
airtime is the scaling wall (doctrine section 15; [Bor et al., Do LoRa Low-Power
Wide-Area Networks Scale?](https://dl.acm.org/doi/10.1145/2988287.2989163)).
Convergence for its own sake would spend the common resource the airtime budget
exists to protect. So CRDT state is reconciled opportunistically, at rejoin and when
a link already exists, under the same class-and-airtime budgets as any other
traffic, never as a background chatter that runs because it can.

## 5. Self-stabilization as the formal goal

The formal target for the fabric is self-stabilization, from Dijkstra's
"Self-stabilizing systems in spite of distributed control"
([EWD426, CACM 1974](https://dl.acm.org/doi/10.1145/361179.361202)). A system is
self-stabilizing if, starting from any state (including states produced by faults),
it reaches a legitimate state in a finite number of steps using only local rules,
and thereafter stays legitimate. Crucially, no node needs complete global knowledge.

Stated for NephMesh: from any reachable state caused by bounded faults (an outage, a
partition, a stale directive, a lost steward, a rejoin that adopted the wrong
class), local rules eventually return the fabric to a legitimate state without any
node needing a global view.

A concrete definition of a legitimate NephMesh state, so the goal is checkable:

- all hard invariants hold (legal region and frequency, transmit constraints, the
  SDR-transmit prohibition, no self-expansion of authority),
- at most one current authority epoch per scope (no split-brain authority),
- no forbidden action is pending,
- message custody is coherent (each message is held, delivered, or expired, never
  duplicated across a resurrected queue),
- every node is either sharing a rendezvous or is explicitly marked detached (no
  node is silently lost),
- change-rate, dwell, and cooldown limits are respected, and
- the system is in a declared mode (one of Normal, Constrained, Contested, Quiet,
  Isolated, Recovery, Quiescent), not an implicit one.

Self-stabilization is the right frame because it does not assume the system starts
clean. A treaty-based rejoin plus a degrading lease plus a safety kernel is a set of
local rules; the claim to test is that those rules always drive the fabric back into
the legitimate set, and never park it in an illegitimate one (two live authority
epochs, custody duplicated, a forbidden action pending).

### Where a small TLA+ or PlusCal model would pay off

Formal methods are not free and much of NephMesh does not need them. The rejoin and
authority state machine is the exception, because its dangerous behaviours are
concurrency and ordering bugs that tests rarely reach: a superseded directive
applied after a newer one, two stewards both believing they hold the current epoch,
a rejoin that adopts an obsolete channel, custody double-counted across a partition.
These are exactly the interleavings a model checker enumerates and a unit test does
not.

A small TLA+ or PlusCal model ([Lamport, TLA+](https://lamport.azurewebsites.net/tla/tla.html);
[PlusCal](https://lamport.azurewebsites.net/tla/pluscal.html)) scoped to the epoch
and rejoin protocol could state the legitimate-state predicate above as an invariant
and check, over all interleavings of outage, partition, delayed delivery, and
rejoin, that:

- safety (invariant): the system is never in an illegitimate state as defined above,
  in particular never two current authority epochs for one scope and never a
  forbidden action pending, and
- liveness (temporal): from any reachable faulted state, the system eventually
  reaches VerifiedManaged (this is the self-stabilization claim made checkable).

Honest scope. Model checking is finite-state and proves properties of the model, not
of the running Go and the radios; its value is catching protocol-level design bugs
before implementation, which is why the roadmap places it as its own stage (16)
right after the protocol is defined and before Phase 6 turns on any actuating loop.
It does not replace the assume-breach and integration testing the codebase already
does, and it will not model RF physics.

## 6. How this maps to the roadmap

- Order of operations stage 15, the detached-epoch and rejoin protocol, is this
  document's sections 2 through 4: the state machine, the sequence, divergence
  classification, staged ChangePlans, and queue drainage under budget.
- Order of operations stage 16, model-checking the authority and rejoin state
  machine, is section 5: at that point ADR 0002 moves from Proposed to Accepted.
- This sits behind the core spine (stages 1 through 8) and behind the earlier
  frontier stages it depends on: the ChangePlan and least-change actuation (stage
  10), the signed Intent Capsule and degrading lease (stage 11), the site steward
  (stage 12), the safety kernel (stage 13), and the first L2 rollback action (stage
  14). Rejoin is deliberately placed before the Phase 6 closed loop (stage 17): a
  fabric that cannot rejoin safely has no business running an autonomous channel
  loop, which is the whole point of ADR 0002.

The reused pieces are not new inventions: the ChangePlan, the airtime budget and
traffic classes, the Evidence-Carrying Action, the degrading lease, and the safety
kernel all come from the doctrine and already have roadmap homes. Rejoin composes
them into a protocol rather than adding a parallel mechanism.

## 7. Open questions

- Epoch digest format and revocation. What is in an epoch digest, how the parent
  chain is represented, and how an old epoch is revoked so a returning steward cannot
  act on it (a signed revocation list, a monotonic counter, both).
- Steward election after a lost steward. Legitimacy requires at most one current
  authority epoch per scope. Electing a replacement is a coordinated action that may
  need quorum, while hard safety must still be locally decidable without it (doctrine
  section 10). The election protocol is unspecified here.
- Clock uncertainty at rejoin. How much rejoin leans on absolute time given the
  capsule's signed-epoch plus residence-age plus boot-event reasoning, and the
  conservative behaviour when two sides disagree about the detachment's duration.
- Partial rejoin and repeated flapping. A link that comes and goes must not thrash
  the machine between LinkRestored and LocallyGoverned or replay drainage each time.
  The doctrine's warning about aggressive route-flap damping (RFC 2439) applies: damp
  near the source, keep an emergency escape.
- Custody reconciliation semantics. Since BPv7 provides no custody transfer, the
  application-level rules for merging two custody views without duplication or loss
  need writing down and, ideally, inclusion in the model.
- CRDT type choice per data kind, and how compaction of the grow-only incident log
  interacts with the durable audit summary without losing tamper-evidence.
- What the model checker should not attempt: the line between what the TLA+ model
  asserts and what only integration tests and field trials can validate, so a green
  result is never overclaimed.

## Sources

- E. W. Dijkstra, "Self-stabilizing systems in spite of distributed control," Communications of the ACM 17(11), 1974 (EWD426). [ACM](https://dl.acm.org/doi/10.1145/361179.361202)
- M. Kleppmann, A. Wiggins, P. van Hardenberg, M. McGranaghan, "Local-first software: you own your data, in spite of the cloud." [Ink and Switch](https://www.inkandswitch.com/local-first/)
- M. Shapiro, N. Preguica, C. Baquero, M. Zawirski, "Conflict-free Replicated Data Types," INRIA. [INRIA HAL](https://inria.hal.science/inria-00609399/document)
- M. Kleppmann, "How to do distributed locking" (fencing tokens against stale actors). [martin.kleppmann.com](https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html)
- RFC 9171, Bundle Protocol Version 7 (lifetime and Bundle Age block for clockless nodes). [RFC Editor](https://www.rfc-editor.org/rfc/rfc9171.html)
- RFC 5050, Bundle Protocol Specification (the earlier custody-transfer mechanism BPv7 removed). [RFC Editor](https://www.rfc-editor.org/rfc/rfc5050)
- RFC 9315, Intent-Based Networking: Concepts and Definitions. [RFC Editor](https://www.rfc-editor.org/rfc/rfc9315.html)
- Argo CD, automated sync and self-heal (the GitOps drift-correction default this document deliberately does not use). [Argo CD docs](https://argo-cd.readthedocs.io/en/stable/user-guide/auto_sync/)
- Flux, core concepts (interval-based reconciliation toward a source of truth). [fluxcd.io](https://fluxcd.io/flux/concepts/)
- L. Lamport, TLA+ and PlusCal. [TLA+](https://lamport.azurewebsites.net/tla/tla.html), [PlusCal](https://lamport.azurewebsites.net/tla/pluscal.html)
- M. Bor, U. Roedig, T. Voigt, J. Alonso, "Do LoRa Low-Power Wide-Area Networks Scale?" MSWiM 2016. [ACM](https://dl.acm.org/doi/10.1145/2988287.2989163)

These are pointers to the ideas this design leans on, not claims that NephMesh
implements them. They are here so the reasoning can be checked rather than taken on
faith.
