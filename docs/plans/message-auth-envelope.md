# Application-layer message authentication and freshness envelope (plan)

Status: this is a design direction, not shipped. Nothing here is built in 0.2.0. It
explores how NephMesh would authenticate the small class of mesh packets that trigger
automation, and where that work would slot into the roadmap. Every recommendation is
scoped to NephMesh's constraints (tiny airtime, no online PKI, unreliable field clocks,
control plane absent at runtime).

## 1. Problem and threat

The threat model records the load-bearing gap plainly (`docs/security/threat-model.md`,
sections 4 and 8): a Meshtastic channel is AES-256-CTR with a shared per-channel PSK.
That gives confidentiality with a non-default key, but CTR is a stream mode with no
integrity tag (bit-flips go undetected), no per-sender authentication (any channel
member can impersonate any other), and no replay protection (dedup is only a 32-bit
packet id plus a 3-bit hop limit). Meshtastic's own documentation states it "is trivial
to impersonate anyone else if you have access to the channel key" and that it "does not
verify the integrity of channel messages"
([Meshtastic encryption](https://meshtastic.org/docs/overview/encryption/)).

Most mesh traffic (position, telemetry, chat) can tolerate that. The narrow class that
cannot is packets that drive automation: a config change, an intent update, a command.
If automation believes a forged or replayed packet off the air, an attacker who is a
channel member, or who merely recorded an earlier legitimate command, can reconfigure or
herd the fleet. This is the trust decision the threat model calls the highest-leverage
remaining security work: not the Kubernetes admission boundary, but "the moment
automation believes a packet off the air." A run of 2025 firmware CVEs around key
provenance and PKI downgrade
([firmware security advisories](https://github.com/meshtastic/firmware/security/advisories))
sharpens the point: a control that lives above the firmware and is testable holds even on
unpatched or vendor-cloned firmware.

Scope discipline matters here. This envelope defends automation-triggering packets first,
not all mesh traffic. Signing every telemetry frame would spend airtime the project has
already argued is the real scaling wall (`docs/plans/airtime-budget.md`).

## 2. Constraints that shape the design

- Airtime budget. A Meshtastic frame carries only a small payload (the firmware caps the
  total packet near 256 bytes; the usable data payload is roughly 200 to 237 bytes after
  framing, firmware-defined and worth re-verifying against the constant before coding).
  Overhead is counted in bytes, not percentages.
- No online PKI. Nodes cannot assume they can reach a CA or key server in the field. They
  can be provisioned with key material by the operator via Kubernetes Secrets before
  deployment (0.2.0 already provisions per-node key material this way).
- Unreliable clocks. Cheap devices lose time across reboots and can be denied
  synchronization, so freshness cannot rest on wall-clock timestamps. This is the same
  problem Bundle Protocol v7 solves by carrying elapsed age rather than absolute time
  ([RFC 9171](https://www.rfc-editor.org/rfc/rfc9171), Bundle Age block).
- Control plane not in the field. Provisioning happens from a powered site; verification
  at runtime must need nothing online.

## 3. Prior art and primitives, with byte costs

- Symmetric MAC (HMAC). An HMAC-SHA256 tag is 32 bytes at full length. NIST permits
  truncation, and short-tag designs are common: SFrame defines cipher suites down to a
  4-byte tag (`AES_128_CTR_HMAC_SHA256_32`)
  ([RFC 9605](https://www.rfc-editor.org/rfc/rfc9605)). Truncating to t bits bounds a
  single blind forgery attempt at 2^-t, so an 8-byte (64-bit) tag is a defensible
  automation-packet choice and a 4-byte tag is the aggressive floor. A MAC authenticates
  only between parties that do not share the verifying key: anyone holding the key can
  both make and check tags, so a MAC cannot protect against a compromised verifier.
- Public-key signature (Ed25519). A signature is 64 bytes, a public key 32 bytes, a
  private seed 32 bytes ([RFC 8032](https://www.rfc-editor.org/rfc/rfc8032)). Only the
  holder of the private seed can sign; verifiers hold public keys only, so a captured
  verifier cannot forge. The cost is 64 bytes on every signed packet and a per-MCU verify
  time (tens of milliseconds on ESP32/nRF52-class parts, feasible but a CPU-DoS surface
  under a flood).
- Monotonic counters and sequence numbers. A per-sender counter that only increases,
  with the receiver storing the last value it accepted and rejecting anything less than
  or equal, is the classic replay defense and needs no clock. Cost is a few bytes on air
  plus a small persisted table at the receiver. BPv7's creation timestamp pairs a time
  with exactly such a monotonic sequence number to disambiguate bundles
  ([RFC 9171](https://www.rfc-editor.org/rfc/rfc9171)).
- TESLA broadcast authentication. TESLA authenticates broadcast with symmetric MACs plus
  delayed disclosure of one-way key-chain keys, so a receiver trusts a MAC only after the
  key is later revealed ([RFC 4082](https://www.rfc-editor.org/rfc/rfc4082)). It is
  attractive for broadcast meshes because it avoids per-packet asymmetric cost, but it
  requires loose time synchronization (each receiver must bound its clock lag relative to
  the sender) and it authenticates with a delay of one disclosure interval or more.
  Per-packet cost is a MAC (say 8 bytes) plus a disclosed key (16 to 32 bytes), plus
  receiver buffering.
- SFrame and MLS as reference points. SFrame is an end-to-end media framing that assumes
  an underlying hop-by-hop transport and an external group key-management layer such as
  MLS ([RFC 9605](https://www.rfc-editor.org/rfc/rfc9605)). MLS is a group key agreement
  and ratcheting protocol ([RFC 9420](https://www.rfc-editor.org/rfc/rfc9420.html)). Both
  are worth reading for their short-tag and epoch ideas, and both are too heavy to run on
  Meshtastic firmware: MLS handshake messages and its tree state dwarf a 200-byte frame,
  and NephMesh has no path to fork firmware to carry them. The useful borrowings are
  narrow: SFrame's truncated-tag suites and MLS's notion of a key epoch.
- Meshtastic's own primitives. Firmware 2.5+ added public-key cryptography for direct
  messages: DMs are encrypted to the recipient's Curve25519 public key and signed with the
  sender's key, giving per-sender authentication for DMs (but not for channel broadcasts)
  ([Meshtastic encryption](https://meshtastic.org/docs/overview/encryption/)). Whether an
  application portnum can reuse that per-node identity key for a raw sign/verify over an
  arbitrary payload is not verified here and is an open question (section 7).

## 4. Options and tradeoffs

Each option below authenticates the automation payload and adds freshness. Byte counts
are the on-air envelope overhead, on top of the command payload, inside whatever
Meshtastic channel encryption is already configured.

### Option A: short-tag symmetric MAC with per-sender keys plus a counter

Layout: 1 byte version/flags, 1 byte key-id (which sender key), 4 bytes freshness counter,
8 bytes truncated HMAC-SHA256. About 14 bytes. Sender key material (32 bytes per sender)
is provisioned by the operator, never sent on air.

Strengths: smallest of the real options, no asymmetric math on the MCU, no clock.
Weakness that is decisive for NephMesh: a symmetric MAC only helps between parties that do
not share the verifying key. To let the whole fleet verify sender S, every node must hold
S's MAC key, so any captured field node can then forge as S. That collides directly with
the project's assume-breach stance. It is genuinely strong only for a fixed pair (for
example gateway-to-gateway) where the verifying key is not spread across the fleet.

### Option B: signature-based (Ed25519) with a counter

Layout: 1 byte version/flags, 1 byte epoch, 4 bytes freshness counter, 64 bytes Ed25519
signature. About 70 bytes. Issuers hold a private seed; the fleet holds only 32-byte
public keys.

Strengths: only an authorized issuer can produce a valid command, and no captured receiver
can forge one, which is exactly the property assume-breach wants. Public keys are safe to
distribute widely (nothing to steal that enables forgery). Weakness: 64 bytes of airtime
per signed packet, and signature verification is a CPU cost that a flood of bogus packets
could abuse.

### Option C: TESLA delayed disclosure

Layout: roughly 8 bytes MAC plus 16 to 32 bytes disclosed key, about 24 to 40 bytes, plus
receiver buffering. Rejected for NephMesh on two grounds the constraints make fatal: it
assumes loose time synchronization, which field nodes that lose time across reboots cannot
guarantee, and it authenticates with a delay, which is the wrong property for a command
that should be acted on or rejected now, not one interval later.

## 5. Recommended direction

Recommend Option B (Ed25519 signatures) as the primary envelope for the highest-consequence
automation packets (config changes, intent updates, commands), with Option A (pairwise
short-tag HMAC) named as an acceptable lighter alternative only for fixed-pair automation
signals where the verifying key stays off the fleet. The reasoning is specific to
NephMesh, not a general preference for signatures:

- The set of nodes authorized to issue automation is tiny (a few gateway or command nodes),
  and automation packets are rare relative to telemetry, so 64 bytes of airtime on a rare
  packet is affordable under a command-and-control traffic class (the doctrine already
  budgets airtime by meaning, `docs/design/doctrine.md` section 6).
- The project assumes breach. Only signatures keep a captured field node from forging
  commands; a fleet-shared MAC key does not.
- Provisioning is public-key distribution, which needs no online PKI: the fleet holds only
  public keys, and a leaked public key does not enable forgery.

Concrete minimal envelope (fields prepended to the automation payload, then the whole
Meshtastic packet is channel-encrypted as usual):

```text
version/flags   1 byte    envelope version, algorithm selector, epoch-present flag
epoch           1 byte    monotonic authority epoch (from the provisioned capsule)
freshness       4 bytes   bootCount (2) || seqWithinBoot (2), both monotonic, persisted
signature      64 bytes   Ed25519 over (portnum-context || epoch || freshness || payload)
```

About 70 bytes of overhead, leaving roughly 130 to 165 bytes for the command itself, which
is ample for terse template-and-code automation messages. The sender node number is already
in the Meshtastic header, so the verifier selects the issuer public key from it at no extra
on-air cost.

Freshness under unreliable clocks. The freshness field is a monotonic triple, never a
wall-clock time: (epoch, bootCount, seqWithinBoot), compared lexicographically. Each
receiver stores the highest triple it has accepted per issuer and rejects anything less
than or equal to it.

- seqWithinBoot increments per automation packet within one power cycle.
- bootCount is a persisted counter the node increments once per boot, so a reboot that
  loses the in-RAM sequence still advances the ordering and cannot replay an old sequence.
  This is the clock-free analogue of BPv7 carrying elapsed age rather than an absolute
  timestamp ([RFC 9171](https://www.rfc-editor.org/rfc/rfc9171)): ordering comes from
  monotonic local state, not from a synchronized clock.
- epoch is bumped by the operator on reprovisioning or membership change and only ever
  advances; it defeats a counter-reset attack after a reflash and provides coarse
  revocation (a node reprovisioned to epoch N rejects everything signed under epoch N-1).

Optionally the sender may include a BPv7-style elapsed age (milliseconds since it created
the command, measured on its own monotonic clock, not wall time) so a receiver can also
reject a command that is simply too old to be useful. That is a staleness bound, secondary
to the replay defense, and is left out of the minimal envelope to save bytes.

Key-provisioning story via the operator and Secrets. The `MeshtasticNode` operator already
provisions per-node key material through redacting Kubernetes Secrets. This design extends
that path, entirely from the powered site, offline-friendly:

- For each authorized issuer node, the operator generates an Ed25519 keypair and stores the
  32-byte private seed in that node's Secret (redacted secret type, never inline, never
  logged), consistent with the CRD redaction control the threat model already mandates.
- The corresponding 32-byte issuer public keys are distributed to the fleet as part of node
  provisioning (a small trust list baked in before deployment), which fits the signed
  Intent Capsule the doctrine describes (`docs/design/doctrine.md` section 5: issuer,
  signature, epoch) and ADR 0002 on signed autonomy.
- epoch and the freshness counters are initialized at provisioning. Rotation is a
  reprovisioning event (new epoch, redistributed public keys), which is a day-2 operation
  the roadmap already lists as thin and needing design.

This keeps the mesh independent of the control plane at runtime: once provisioned, nodes
verify commands with local state and a local trust list, with nothing online required.

## 6. How it maps to the roadmap and threat model

- It is research backlog item 3 in `docs/research/resilient-comms-landscape.md`
  ("Application-layer authentication and freshness envelope on automation-triggering mesh
  packets"), described there as "the honest completion of assume-breach."
- It closes the first of the two controls named in the threat model's untrusted-RF-medium
  boundary (`docs/security/threat-model.md` section 8), and turns the OPEN "spoofing and
  replay" item in boundary 4 into a testable control (forge or replay a command, assert
  ingest rejects it). The receive-only SDR "claim vs air" monitor (backlog item 4) is the
  complementary out-of-band check and is out of scope for this doc.
- It gives the doctrine's Intent Capsule and Evidence-Carrying Action their on-air
  authentication primitive (`docs/design/doctrine.md` sections 5 and 8): the signature and
  epoch are what make a capsule's authority checkable at the edge.
- It respects the airtime doctrine by scoping to a rare command class rather than all
  traffic (`docs/plans/airtime-budget.md`, and doctrine section 6 traffic classes).

## 7. Open questions

- Reuse Meshtastic's PKC identity key, or run a parallel keyset? Firmware 2.5+ already holds
  a per-node Curve25519/Ed25519 identity. Whether an application portnum can invoke a raw
  sign/verify over an arbitrary payload, rather than only the built-in DM path, is not
  verified. Reuse would halve provisioning work; a parallel keyset is more portable across
  firmware forks. This needs a firmware-capability check before committing.
- Revocation without online PKI. epoch bumps give coarse revocation only at the next
  reprovisioning. Is that fast enough, and what is the reprovisioning path for a node that
  is in the field and only intermittently reachable?
- Counter durability and flash wear. Persisting bootCount and accepted-counter state per
  issuer costs flash writes. What is the write budget, and is there a safe bound on the
  per-issuer receiver table for a fleet of tens of nodes?
- Signature verification DoS. An attacker can flood valid-looking signed packets to burn
  MCU cycles. Cheap pre-checks (portnum, epoch and counter window) before the Ed25519
  verify, plus rate limiting, are the likely mitigation; the exact ordering needs
  measurement on target hardware.
- Tag length and algorithm agility. Is 64-bit the right floor for the symmetric fallback,
  and should the version/flags byte carry an algorithm selector so a future move (for
  example to a shorter post-quantum-aware scheme) does not require a wire-format break?
- Relationship to encryption. This envelope is authentication and freshness only; it does
  not add confidentiality and assumes Meshtastic channel encryption stays configured. It
  sits inside the encrypted payload. Confirm that ordering (authenticate-then-encrypt vs
  the reverse) against the actual firmware path.
- Multi-hop semantics. The envelope is end-to-end at the application layer and independent
  of relay nodes, but the exact interaction with hop-limit dedup and any store-and-forward
  path should be validated so a legitimately delayed command is not misread as a replay.

## 8. Sources

- [Meshtastic encryption overview](https://meshtastic.org/docs/overview/encryption/)
- [Meshtastic firmware security advisories](https://github.com/meshtastic/firmware/security/advisories)
- [RFC 8032, Edwards-Curve Digital Signature Algorithm (EdDSA)](https://www.rfc-editor.org/rfc/rfc8032)
- [RFC 4082, TESLA: Timed Efficient Stream Loss-Tolerant Authentication](https://www.rfc-editor.org/rfc/rfc4082)
- [RFC 9171, Bundle Protocol Version 7](https://www.rfc-editor.org/rfc/rfc9171)
- [RFC 9605, Secure Frame (SFrame)](https://www.rfc-editor.org/rfc/rfc9605)
- [RFC 9420, The Messaging Layer Security (MLS) Protocol](https://www.rfc-editor.org/rfc/rfc9420.html)
- Internal: `docs/security/threat-model.md` (sections 4 and 8), `docs/research/resilient-comms-landscape.md` (backlog item 3), `docs/design/doctrine.md` (sections 5, 6, 8), `docs/plans/airtime-budget.md`.
