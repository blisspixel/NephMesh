# Coordinated key rotation and epoch-keyed channels

Status: design direction, not shipped. This is a proposal for a day-2 operation that
is thin today (roadmap Order of operations stage 3, and research backlog item 7). None
of it is built. It reuses the channel apply path and the PSK-hash comparison that stage 1
introduces, and it should be read as a plan to be validated against firmware, not a claim
that the project already rotates keys safely.

## 1. Problem

A NephMesh channel is protected by one shared pre-shared key (PSK). Every node that can
read the channel holds the same key. Two day-2 needs follow from that and are poorly
served today:

- Rotation. An operator wants to replace a channel's PSK across a whole fleet on a
  schedule (routine hygiene, or because a key is suspected exposed) without splitting the
  mesh into a group that already switched and a group still on the old key. A split is the
  main failure to avoid: once half the fleet moves to a new key, the two halves cannot
  decrypt each other and the mesh is partitioned until every node converges.
- Revocation. A member leaves (a lost device, a person who should no longer have access).
  They should lose access to future traffic. With a single shared PSK there is no way to
  remove one holder selectively: the only lever is to rotate the whole channel to a new key
  and withhold the new key from the departed member.

Three project constraints make this hard, and shape the whole design:

- Reboot on apply. Applying channel config reboots the Meshtastic radio (per changed
  section), so every touch of the key material costs an outage. The protocol must minimise
  the number of applies, not just the number of nodes touched (doctrine section 9, least
  change).
- Tiny airtime budget. Airtime, not node count, is the LoRa scaling wall
  (`docs/plans/airtime-budget.md`, and Bor et al. below). Any key material that has to
  travel over LoRa competes with mission traffic. The win here is that pre-distribution
  can travel over the control-plane path (wired or USB or TCP from a powered site) at zero
  LoRa airtime.
- The control plane is not in the field. Deployed nodes run autonomously once configured
  (AGENTS.md). A rotation therefore has to be schedulable in advance and has to survive a
  node being briefly unreachable during the switch window, because there is no live
  controller in the field to coordinate the cutover.

## 2. What Meshtastic gives us, and what it does not

Meshtastic encrypts each packet payload with AES256-CTR using a per-channel key, and
leaves the header in the clear so relays can forward
([Meshtastic encryption overview](https://meshtastic.org/docs/overview/encryption/)). A
channel PSK is 0 bytes (no crypto), 16 bytes (AES128), or 32 bytes (AES256), with a
`random` option that generates a secure 256-bit key
([Meshtastic channels](https://meshtastic.org/docs/configuration/radio/channels/)). The
primary channel ships with a well-known default key (`AQ==`) that the docs say users must
change. The whole channel set (each channel with a name and PSK and settings) is serialised
as a single protobuf `ChannelSet`, base64-encoded into one shareable channel URL, and
`--seturl` replaces the entire set at once (`docs/research/meshtastic.md`). This is why the
operator's channel path is different from every other field: the export encodes channels as
one URL, not discrete per-channel entries.

Two properties Meshtastic explicitly does not provide, both load-bearing here:

- No forward secrecy within a channel. AES-CTR with a static PSK means everything sent
  under a key is readable by anyone who ever holds that key, including recorded traffic
  decrypted later ("harvest now, decrypt later"). The docs state this plainly. Rotation
  gives forward secrecy only across epoch boundaries, never inside an epoch.
- No per-sender authentication and no replay protection. Anyone with the channel key can
  impersonate any node ([encryption overview](https://meshtastic.org/docs/overview/encryption/);
  and backlog item 3 in `docs/research/meshtastic.md`). This matters for how a switch is
  signalled, below.

A node can hold several channels at once (one primary plus up to seven secondary). All
channels share one LoRa PHY: the frequency slot is derived from the primary channel name
together with region and modem preset, and secondary channels are logically multiplexed on
that same slot and told apart by a channel-hash byte, so a node decrypts whichever channels
it holds the PSK for. This multi-channel behaviour is the mechanism that makes an overlap
possible, and the exact frequency-slot derivation is a firmware detail this design should
confirm against `meshtasticd --sim` and a real board (roadmap stage 1, attempt-and-record).

## 3. Prior art, scoped honestly to broadcast LoRa

- Signal Double Ratchet. The reference point for per-message forward secrecy and
  post-compromise recovery
  ([Double Ratchet spec](https://signal.org/docs/specifications/doubleratchet/)). It is a
  pairwise two-party protocol: each pair maintains sending and receiving chains and a
  per-message Diffie-Hellman ratchet, and the specification does not address group or
  broadcast messaging. It is the wrong tool here. A LoRa broadcast channel is one-to-many
  over a shared static PSK, there is no back-channel per pair, and per-message DH ratcheting
  would multiply both state and airtime. We borrow the goal (old keys should not open future
  traffic) but not the mechanism.
- Messaging Layer Security (MLS, RFC 9420). MLS organises a group into epochs: each
  membership change advances the epoch and derives a fresh epoch secret from new entropy that
  excluded members cannot access, and a removed member cannot decrypt messages after the epoch
  in which they are removed ([RFC 9420](https://datatracker.ietf.org/doc/rfc9420/)). The
  epoch idea maps well. The machinery (a ratchet tree, Commit and Welcome messages, per-member
  key packages) does not: it assumes a reliable ordered delivery channel and a way to send
  O(log N) targeted updates, neither of which a duty-cycle-limited broadcast mesh offers
  cheaply. We take the epoch and the "removed members are excluded at the next epoch" semantics,
  and leave the tree.
- Group rekeying literature, LKH (RFC 2627). The Logical Key Hierarchy reduces the cost of
  removing one member from O(N) to O(log N) by giving each member a path of keys in a tree and
  replacing only keys on the departed member's path
  ([RFC 2627](https://datatracker.ietf.org/doc/rfc2627/)). This is the honest scoping point:
  Meshtastic has exactly one symmetric PSK per channel and no key tree, so selective, cheap
  member removal is not available to us at all. Every revocation is a full-group rekey. What
  NephMesh can add is not cryptographic selectivity, it is orchestrating that full rekey across
  a reboot-sensitive fleet without splitting the mesh.
- Make-before-break versus break-before-make. Borrowed from cellular handover: make-before-break
  (soft handover) establishes the new connection before releasing the old and preserves
  continuity, while break-before-make (hard handover) releases the old first and risks a gap
  ([Handover](https://en.wikipedia.org/wiki/Handover)). A naive key rotation is break-before-make
  and is exactly the split we want to avoid. The design below is make-before-break: the new
  epoch key is resident everywhere before anyone relies on it.
- The doctrine's rendezvous idea. `docs/design/doctrine.md` section 9 already argues that a
  channel change needs a rendezvous, not an ordinary canary (an announced switch epoch, repeated
  pre-change notices, a predeclared fallback channel, a rollback timeout, and verification from
  both sides of the expected partition boundary), because a radio channel change can isolate an
  ordinary canary from the mesh. Section 10 lists rotating shared channel credentials as an action
  that quorum may gate, while insisting hard safety stays locally decidable. This document is the
  concrete rotation protocol those sections point at.

## 4. A rotation protocol for NephMesh

Model. Reserve the primary channel as a stable, long-lived carrier and rendezvous channel
whose name never changes, so the LoRa frequency slot stays constant across rotations. Put the
rotating operational traffic on an epoch-named secondary channel: `ops-e14`, `ops-e15`, and so
on, each with its own independent random 256-bit PSK. Because epoch channels are secondary, they
share the carrier's frequency slot, so epoch N and epoch N+1 traffic coexist on the same PHY and
each node decrypts whichever it holds. The operator pre-generates the epoch key sequence and
stores each key in a Kubernetes Secret, read through the existing redacting type
(`docs/plans/phase-4-operator.md`), so raw keys never pass through logs or status.

Roles of the standing channels:

- Carrier and rendezvous (primary, stable key rotated rarely and out of band): always present on
  every node, sets the frequency slot, and is the recovery path for a node that missed a rotation.
- Epoch channel (secondary, rotated): carries the actual operational traffic for one epoch.

Rotation from epoch N to epoch N+1, as a step sequence:

1. Pre-generate. The operator mints `K_{N+1}` (random 256-bit) into a Secret ahead of time. No
   device is touched, no airtime is spent.
2. Distribute the new epoch (the "make"). The operator reconciles each reachable node to also
   carry `ops-e{N+1}` alongside `ops-e{N}`. This is one channel apply, so one reboot per node,
   scheduled in a low-traffic window, and minimal-diff so a node already carrying both is left
   untouched. Where a node is reached from a powered gateway over USB or TCP, the new key travels
   the control-plane path at zero LoRa airtime. Where a node can only be reached over the mesh by
   admin messages, that push does cost airtime and must be budgeted as reconfiguration airtime
   (doctrine section 6); it is not zero, and the doc should not pretend otherwise.
3. Confirm the make converged. The operator reads each node's channel set and compares by PSK hash,
   never by raw key (roadmap stage 1), and sets a per-node condition, for example `EpochStaged(N+1)`.
   The fleet is make-converged when every managed and reachable node reports `ops-e{N+1}` present.
   Convergence, not a timer, is the gate to the next step. Quorum policy (doctrine section 10) can
   require, for example, 100 percent of reachable nodes plus an operator approval before proceeding.
4. Switch senders to N+1 (the scheduled epoch). At a predeclared switch epoch, applications begin
   sending operational traffic on `ops-e{N+1}` instead of `ops-e{N}`. Because every converged node
   already holds `K_{N+1}`, a node that switches early or late still shares a key with its peers, so
   there is no partition. During the overlap window both channels are live and readable. See section
   4.1 for how the switch is signalled without a field control plane.
5. Hold the overlap (last-known-good). Keep `ops-e{N}` resident as a secondary channel through a
   declared grace window. It is the last-known-good: a node that rebooted late, or was briefly
   unreachable during step 2 and only just received `K_{N+1}`, can still reach and be reached on the
   old epoch until it settles. Rollback during this window is cheap because the old key is still
   present.
6. Confirm mission convergence. Verify from both sides of the expected partition boundary (doctrine
   section 9): that expected peers are heard on `ops-e{N+1}` and that real delivery, not merely
   presence of the channel, is happening. Only then consider retiring the old epoch.
7. Retire the old epoch (the "break", deferred and reversible until here). Reconcile nodes to remove
   `ops-e{N}`. This is a second reboot per node. After it, epoch-N traffic is no longer decryptable
   by the fleet and any holder of only `K_N` (including a revoked member, section 5) is excluded from
   current traffic. Retiring is what actually enforces revocation; staging alone does not.

Reboot budget: two applies per node per rotation in the normal path (stage the new epoch, later
retire the old). The scheduled switch in step 4 is a convention about which channel senders use, not
a third reboot, as long as both channels are already resident. This is the concrete meaning of
"least change" for key rotation: the disruptive part is bounded to two reboots and the cutover itself
costs none.

Handling a node that missed the window. Make-before-break covers the common case: a node that received
`K_{N+1}` in step 2 is safe even if it reboots late or drops out briefly, because it holds both keys. The
hard case is a node unreachable through the entire staging window that never got `K_{N+1}`. Do not retire
epoch N (step 7) while such a node is still expected and reachable-in-principle, which is why step 3's gate
is convergence and not a clock. When a long-absent node reappears holding only `K_N` after the fleet has
retired it, it is partitioned from operational traffic but still reachable on the stable carrier and
rendezvous channel that every node always carries, and the operator (or a powered gateway) re-keys it there.
A node that is offline for the whole rotation and never returns to any reachable path cannot be silently
rescued: it must be re-provisioned when it comes back, and the design says so rather than implying magic.

### 4.1 Signalling the switch without a field control plane

Step 4 needs the fleet to agree on when to start using `ops-e{N+1}`. There is no live controller in the
field and Meshtastic has no per-sender authentication, so this is genuinely awkward and worth stating as
an open tension rather than a solved problem. Two candidate mechanisms, each with a cost:

- Scheduled time. Encode the switch epoch as a wall-clock time in the signed intent the node already
  holds. This costs no airtime but assumes clocks, and field devices lose time across reboots and can be
  denied synchronisation. The doctrine's answer (section 5, borrowing Bundle Protocol v7, RFC 9171) is to
  combine signed epoch counters with monotonic residence age and recorded boot events and to behave
  conservatively under time uncertainty, rather than trusting an absolute timestamp.
- In-band beacon. Announce the switch over the mesh. This costs airtime and, worse, is spoofable because
  the channel has no per-sender authentication, so an attacker on the channel could trigger or suppress a
  switch. This is the same gap backlog item 3 (an application-layer authentication and freshness envelope)
  is meant to close, and a rotation trigger is a strong argument for that envelope.

Because make-before-break makes an early or late switch non-fatal (everyone holds both keys during the
overlap), the switch epoch can be soft: a target time, not a hard barrier. Getting the switch exactly
simultaneous is neither achievable nor necessary; getting every node to hold both keys before anyone
switches is the part that must be exact, and that part is confirmable by the operator (step 3).

## 5. Epoch-keyed channels: what "zero airtime" can and cannot do

Epoch keying gives NephMesh the group-security properties MLS names, scoped to a single shared PSK:

- Forward secrecy across epochs. A member removed before epoch N+1 does not receive `K_{N+1}` and cannot
  read epoch-N+1 traffic, matching RFC 9420's "cannot decrypt after the epoch in which they are removed."
- Backward secrecy across epochs. A member who joins at epoch N+1 and is given only `K_{N+1}` cannot read
  epoch-N traffic, provided the old epoch keys are not also handed over.

What zero-airtime revocation genuinely achieves: the new epoch key can be pre-distributed over the wired or
USB control-plane path from a powered site, spending no LoRa airtime, so routine forward rotation is cheap on
the medium even though it is not cheap in reboots.

What it cannot achieve, stated plainly:

- A revoked member who already holds `K_N` can still read every epoch-N message, including traffic they
  recorded earlier, forever. Meshtastic has no forward secrecy inside an epoch. Rotation does not claw back
  what a legitimate holder already has; it only stops them reading the next epoch.
- Revocation therefore takes effect only at the next epoch boundary, and only once the old epoch is actually
  retired (step 7). Until you rotate and retire, a leaver keeps reading current traffic. There is no
  instant mid-epoch cutoff.
- "Zero airtime" holds only for the pre-distribution leg when it travels out of band. Rescuing a straggler
  over the mesh, pushing a key to a node reachable only by LoRa admin messages, or announcing a switch in
  band all cost airtime.
- No selective removal. Because there is one PSK per channel and no key tree, you cannot remove one member
  without rotating the whole channel (contrast LKH, section 3). Every revocation is a full-group epoch
  advance.

The honest summary: epoch-keyed channels buy periodic, orchestrated, low-airtime rotation and clean
membership boundaries at epoch granularity. They do not buy per-message forward secrecy, instant revocation,
or selective member removal, and the doc should never imply they do.

Slot budget. A node holds at most eight channels. A practical resident set is the carrier, the current
epoch, and the next epoch (three slots), so you cannot keep an unbounded history resident and the overlap
window is bounded by slot count as well as by airtime and reboot cost.

## 6. How this maps to the roadmap

- Order of operations stage 3 (day-2 operations) names exactly this: "coordinated channel and PSK rotation
  as declared policy (rotate at a scheduled epoch so the change is atomic across the fleet)." This document
  is the design for that line. It also depends on stage 1 (channels and PSKs on real hardware, compare by
  PSK hash so raw keys never move through stdout), which provides the channel apply path and the hash
  comparison that steps 2 and 3 reuse.
- Research backlog item 7 ("epoch-keyed channels for forward secrecy and revocation at zero airtime") is the
  origin of this work. This doc keeps the ambition but corrects the framing: the airtime saving is real for
  pre-distribution over the control-plane path, and the honest limits in section 5 are part of the deliverable,
  not a footnote.
- Doctrine section 9 (rendezvous, announced switch epoch, predeclared fallback, rollback timeout) and section
  10 (quorum may gate rotating shared channel credentials, hard safety stays locally decidable) are the
  invariants this protocol implements: the stable carrier is the predeclared fallback, the overlap window is
  the rollback path, and step 3's convergence gate plus quorum approval is the coordinated-action gate.
- Later, in the intent-layer frontier, a `CommunicationIntent` compiler would emit a rotation as a `ChangePlan`
  (doctrine section 8) scored against a change budget, and a rejoin (section 11) would issue a new signed epoch
  that revokes the old. Those are downstream. Stage 3 is the manually declared, operator-approved version, and
  it must be solid before any of that earns its complexity.

## 7. Open questions

- Frequency-slot derivation. Does the LoRa slot really come only from the primary channel name (plus region and
  preset), leaving secondary epoch channels co-resident on one PHY? The whole overlap depends on this; validate
  against `meshtasticd --sim` and a real board before trusting it.
- Reboot cost in practice. How disruptive is adding, then later removing, a secondary channel, and can the two
  applies be batched with other pending config to avoid extra reboots (doctrine section 9)? This is the same
  empirical question the roadmap defers to stage 4 hardware work.
- Switch signalling. Which of scheduled-time or authenticated in-band beacon (or both) is the default, and does
  the switch trigger become the first concrete motivation to ship backlog item 3 (an authentication and freshness
  envelope), since an unauthenticated trigger is spoofable?
- Clockless scheduling. How exactly do signed epoch counters, monotonic residence age, and boot events (RFC 9171)
  combine into a switch decision on a node with no reliable clock, and what is the conservative default when time
  is uncertain?
- Key erasure. When the old epoch channel is removed (step 7), does the firmware promptly and irrecoverably erase
  `K_N` from the device, or does it linger where a later attacker could recover it?
- Straggler and quorum policy. How long should old-epoch retention run before a persistently absent node is
  declared re-provision-on-return rather than rescued, what counts as "converged enough" to retire an epoch
  across a partition-prone mesh, and who approves that (doctrine section 10)?

## 8. Sources

- [Meshtastic encryption overview](https://meshtastic.org/docs/overview/encryption/) (AES256-CTR, default key,
  no forward secrecy, no per-sender authentication).
- [Meshtastic channels](https://meshtastic.org/docs/configuration/radio/channels/) (PSK sizes 0 / 16 / 32 bytes,
  random 256-bit, primary versus secondary, channel name).
- [Signal Double Ratchet specification](https://signal.org/docs/specifications/doubleratchet/) (pairwise two-party
  forward secrecy and post-compromise recovery; no group or broadcast mode).
- [RFC 9420, Messaging Layer Security](https://datatracker.ietf.org/doc/rfc9420/) (epochs, epoch secret, removed
  members excluded at the next epoch).
- [RFC 2627, Key Management for Multicast](https://datatracker.ietf.org/doc/rfc2627/) (group rekeying, Logical Key
  Hierarchy, O(log N) member removal).
- [Handover, make-before-break versus break-before-make](https://en.wikipedia.org/wiki/Handover).
- [RFC 9171, Bundle Protocol Version 7](https://datatracker.ietf.org/doc/rfc9171/) (bundle age for clockless nodes),
  cited via the doctrine.
- M. Bor, U. Roedig, T. Voigt, J. Alonso, "Do LoRa Low-Power Wide-Area Networks Scale?" (MSWiM 2016), on the
  airtime-not-node-count scaling wall.
- Internal: `docs/design/doctrine.md` sections 5, 6, 9, 10, 11; `docs/roadmap.md` (Order of operations stage 3,
  backlog item 7); `docs/research/meshtastic.md`; `docs/plans/airtime-budget.md`.

These are pointers to the ideas this design leans on, not claims that NephMesh already implements them.
