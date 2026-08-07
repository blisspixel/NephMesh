# Plan: contingency semantics and borrowed DTN store-and-forward

Status: design direction, not shipped. This explores the shape of a contingency
messaging layer for NephMesh. Nothing here is built, and every resource or field it
names is a proposal, not an API. It elaborates doctrine section 7 (a contingency layer
needs different semantics, not just less throughput) and connects to the roadmap Order
of operations stages 2 and 7.

## 1. Why a contingency layer needs different semantics, not just lower throughput

The tempting mistake is to treat a LoRa contingency channel as a slow IP link and
reach for the same interaction model: connect, request, wait, retransmit, expect a
timely answer. That model assumes an always-on path and fast feedback. A contested
mesh violates both assumptions as a matter of course, not as a failure. Intermittent
connectivity is the normal case here.

Two bodies of practice already reject the always-on assumption on purpose.

PACE (Primary, Alternate, Contingency, Emergency) is a US Army planning method that
designates the order in which an element moves through available communication systems
until contact is established ([PACE methodology](https://en.wikipedia.org/wiki/PACE_(communication_methodology))).
The relevant point for NephMesh is not the four tiers themselves but their doctrine:
each tier is independent, the emergency tier is the slowest but the most available (a
courier, in the classic example), and moving down a tier is a planned, graceful
degradation rather than a fault. NephMesh sits at the Contingency and Emergency end of
someone else's PACE plan. A layer built for that end should optimize for meaning
surviving disruption, not for throughput or latency.

Delay-Tolerant Networking (DTN) makes the same rejection at the protocol level. The
DTN architecture (RFC 4838) does not expect links to be always available or reliable,
and instead expects nodes to store bundles until a transmission opportunity arises
([RFC 4838](https://datatracker.ietf.org/doc/html/rfc4838)). It replaces stream-based,
packet-switched delivery with a message-level abstraction and custody transfer, so a
source can hand off responsibility and release resources rather than hold a session
open across a long partition.

The consequence for NephMesh is that the unit of value is a message with explicit
semantics, not a packet on a session. A contingency layer should preserve four things an
always-on network takes for granted and therefore never encodes: the MEANING of a
message (what it is and how to render it), its PRIORITY relative to scarce airtime,
CUSTODY (who is currently responsible for delivering it), and JUSTIFIED SILENCE (the
difference between "nothing to say" and "failed"). None of these follow from lowering the
bit rate.

## 2. The MessageIntent envelope

`MessageIntent` is an application-level envelope that travels with a message and states
what the message is for, not how to route it. It is the message-scale analogue of the
`CommunicationIntent` the doctrine proposes at the fabric scale. It exists so that
gateways, stewards, and queues can make priority, aggregation, custody, and expiry
decisions without parsing payloads or guessing.

### Byte-cost awareness

Every field costs airtime on a shared, slow medium. A Meshtastic packet carries roughly
200 bytes of usable application data after framing, and a single small packet is already
hundreds of milliseconds on air ([Meshtastic overview](https://meshtastic.org/docs/overview/),
via `docs/plans/agent-mesh-nodes.md`). An envelope that spends 60 bytes on metadata has
spent nearly a third of the packet before any content. So the envelope is small
enumerations and varints, most fields optional with sane defaults, and several fields
carried only at gateways where bytes are cheaper. The design target is a common-case
on-air envelope of a handful of bytes, not a rich header.

### Fields

The envelope carries the fields doctrine section 7 names. Byte cost and residence are
called out for each. "Leaf" means it must be cheap enough to live in constrained
Meshtastic firmware or a thin companion; "gateway" means it can live at a
PSRAM-class gateway or the application and need not cross the constrained air hop.

| Field | Purpose | Rough cost | Lives on |
|---|---|---|---|
| `class` | traffic class (see below) | 3-4 bits | leaf (on air) |
| `createdAt` or `age` | creation time, or accumulated age for clockless nodes | 0-4 bytes | leaf (age), gateway (absolute) |
| `maxUsefulAge` | after this, delivery is pointless; drop | 1-2 bytes | leaf (on air) |
| `destScope` | self, neighbor, site, named group, any | 1-2 bytes | leaf (on air) |
| `ackStrength` | none, implicit-overheard, explicit-routing, end-to-end custody | 2-3 bits | leaf (on air) |
| `custodyRequested` | ask a gateway to take custody | 1 bit | leaf flag, gateway honors |
| `txCeiling`, `hopCeiling` | retransmission and hop limits | ~1 byte | leaf (on air) |
| `privacy` | classification, drives what may be logged or bridged | 2-3 bits | leaf flag, gateway enforces |
| `aggKey` | aggregation key, groups mergeable reports | 1-2 bytes | gateway (aggregation) |
| `template` | semantic encoding or template id | 1-2 bytes | leaf (on air), rendered at receiver |
| `delayStillUseful` | is late delivery still worth airtime | 1 bit | leaf flag, gateway honors |
| `silencePreferred` | prefer silence over uncertain delivery | 1 bit | leaf flag |

### Traffic classes

The classes are the ones the doctrine and roadmap stage 7 already name, highest priority
first: life-safety; command and coordination; acknowledgements and control; situational
reports; position and presence; telemetry; best effort. The class is the single most
load-bearing field: it drives admission under scarcity, reserve eligibility, aggregation,
and what each degraded mode suppresses. It is a small enumeration precisely because it
must always be on air.

### What lives where

The honest split follows the doctrine's devolution ladder. Constrained leaves carry only
the fields needed to decide, locally and cheaply, whether to send, store, or drop:
class, age or `maxUsefulAge`, scope, ack strength, the two ceilings, the template id, and
the small boolean flags. Everything that benefits from memory, a clock, or compute lives
at the gateway or application: absolute timestamps, aggregation by `aggKey`, custody
bookkeeping, privacy-driven bridging decisions, and history. This keeps the envelope that
actually crosses the slow air hop close to a handful of bytes, and lets the expensive
semantics accrete where there is power and PSRAM.

### Illustrative conceptual structure

Illustrative only, not an API. Field names and encodings are placeholders.

```text
MessageIntent {
  class:            enum  // lifeSafety | commandCoord | ackControl |
                          // sitrep | position | telemetry | bestEffort
  age:              varint ms      // accumulated age; preferred on clockless leaves
  createdAt:        opt timestamp  // absolute; added at a gateway with a trusted clock
  maxUsefulAge:     varint ms      // drop when age exceeds this
  destScope:        enum  // self | neighbor | site | group:<id> | any
  ackStrength:      enum  // none | overheard | routing | custody
  custodyRequested: bool
  txCeiling:        uint8          // max retransmissions
  hopCeiling:       uint8          // max forwards; mirrors BPv7 hop limit
  privacy:          enum  // open | restricted | sensitive
  aggKey:           opt bytes      // gateway aggregation; usually omitted on air
  template:         opt uint16     // semantic template id, rendered at receiver
  delayStillUseful: bool
  silencePreferred: bool
}
```

## 3. Degraded fabric modes

Degraded modes are semantic states of the fabric, not modem presets. A mode declares
what the fabric is currently willing to do with meaning, priority, and airtime. A modem
preset may change as a consequence, but the mode is the decision and the preset is one
possible effect. The site steward (roadmap stages 12 onward) would own the current mode
inside its signed authority envelope; the mode is evidence-driven and always reversible.

The seven modes, and what each admits, aggregates, stores, or suppresses:

- Normal. All classes admitted. Aggregation optional. Entry: airtime reserve healthy,
  evidence fresh, no contention signal. Exit: any pressure or evidence signal below.
- Constrained. Telemetry reduced (longer cadence or dropped), situational reports and
  position aggregated by `aggKey`, higher classes still immediate. Entry: airtime
  pressure or rising channel utilization. Exit: pressure clears for a dwell period.
- Contested. Only essential traffic (life-safety, command and coordination, control)
  sent immediately; the rest stored for later. Entry: sustained high utilization,
  delivery-ratio drop, or a contention or interference signal corroborated by more than
  one source. Exit: corroborated improvement, then a controlled ramp (see Recovery).
- Quiet. Receive and retain. Transmit only life-safety or explicitly authorized traffic.
  All other classes are stored, not dropped. Entry: an authority signal (declared quiet
  policy) or strong evidence that transmitting is unsafe or unwise. This is a deliberate,
  authorized posture, not a fault. Exit: authorization lifts or the triggering evidence
  clears.
- Isolated. Local-only operation and custody. The node cannot reach the wider fabric, so
  it serves local scope and holds custody of everything bound outward. Entry: partition
  detected (no gateway, no expected peers). Exit: connectivity restored, hand off to
  Recovery.
- Recovery. Stored queues drained by priority then age, under a controlled airtime ramp
  so a partition healing does not dump a backlog into a fragile channel. Entry: pressure
  or partition clears from Constrained, Contested, Quiet, or Isolated. Exit: queues
  drained within budget and reserve restored, return to Normal.
- Quiescent. No useful traffic to send and no sufficient evidence to act, so deliberate
  silence. This is distinct from Quiet (which is authorized suppression of a mostly-full
  queue) and from Isolated (which is a partition). Quiescent is the healthy resting state
  of a node with nothing to say and no reason to probe. Entry: empty useful queue plus
  insufficient evidence. Exit: new traffic or new evidence.

Two invariants hold across all modes, from the doctrine. Custody is never silently
discarded to relieve pressure; shedding stored traffic is an explicit, declared action
under storage pressure, not a side effect of a mode change. And hard invariants (legal
region, transmit limits, the SDR-transmit prohibition, authority) do not change with the
mode; a mode may only narrow what is sent, never authorize the otherwise forbidden.

## 4. Adopting BPv7 semantics at gateways without a full DTN stack on leaves

Bundle Protocol v7 (RFC 9171) solves several of the exact problems a contingency layer
faces. The design intent is to borrow four specific ideas as semantics and place their
machinery where it can afford to live, not to run BPv7 or a bundle agent on leaf
firmware. Constrained Meshtastic nodes keep their native, firmware-level behavior; the
DTN-flavored bookkeeping accretes at PSRAM-class gateways and the application.

The four borrowed ideas ([RFC 9171](https://www.rfc-editor.org/rfc/rfc9171.html)):

- Explicit lifetime. BPv7's primary block carries a Lifetime field, the time past
  creation after which the payload is no longer useful; once age exceeds lifetime, nodes
  need no longer retain or forward the bundle and it should be deleted. This maps
  directly onto the envelope's `maxUsefulAge`. It lives partly on the leaf (a leaf can
  cheaply drop an over-age message it holds) and is enforced authoritatively at the
  gateway that manages stored queues.
- Accumulated age for clockless nodes. Cheap radios lose time across reboots and can be
  denied synchronization. BPv7's Bundle Age extension block carries the milliseconds
  elapsed since creation, and the spec directs that bundle age must be obtained from that
  block when the creation timestamp is zero or the local clock's accuracy is unknown.
  This is the mechanism behind the envelope's `age` field, and it is why age is preferred
  over an absolute `createdAt` on leaves. A gateway with a trusted clock can convert age
  to an absolute time when it takes custody.
- Hop count against forwarding loops. BPv7's Hop Count block carries a hop limit and a
  hop count, and a bundle whose hop count exceeds its limit should be deleted for "hop
  limit exceeded." NephMesh's `hopCeiling` mirrors this. Meshtastic already enforces a
  hop limit natively, so on the leaf this is largely the existing behavior; the envelope
  makes the ceiling explicit and per-message so higher classes can be allowed more reach
  than best-effort traffic.
- Custody-style semantics. This is where honesty matters most. RFC 9171 itself does not
  define custody transfer; it leaves securing and custody handling to other
  specifications, and the custodian model is described in the older DTN architecture
  ([RFC 4838](https://datatracker.ietf.org/doc/html/rfc4838)), where a custodian accepts
  responsibility for forward progress so the source can release resources. NephMesh
  should borrow the idea, not the protocol: `custodyRequested` asks a gateway to become
  the responsible party for a message, take it off the originator's conscience, and keep
  trying under the queue's priority and age rules. Custody lives entirely at gateways.
  Leaves only raise the flag.

Explicitly out of scope for leaves: bundle formats, CBOR bundle encoding, bundle
security (BPSec), convergence-layer adapters, and custodian signaling. Putting any of
that on a constrained node would spend the scarce resource (airtime and firmware
complexity) to buy semantics the gateway can provide more cheaply. The seam is the
envelope: leaves emit intent, gateways implement DTN-flavored behavior against it.

### What Meshtastic already provides, so NephMesh does not reinvent it

Meshtastic ships a Store and Forward module, and the design should sit on top of it
rather than duplicate it. The module runs a server on an ESP32 device with PSRAM (a
T-Beam or T3S3, roughly 11,000 records by default) that retains received text messages
and replays them to a client on request; a client asks with an "SF" direct message or
automatically on reconnect, bounded by a History Return Max (default 25 messages) and a
History Return Window (default 240 minutes), and the server sends periodic heartbeats so
clients can find it ([Meshtastic Store and Forward module](https://meshtastic.org/docs/configuration/module/store-and-forward-module/)).
Two honest limits shape where NephMesh adds value: the server cannot track which
messages a given client already saw, so duplicate delivery is possible, and it stores
text messages, without message-class, age, custody, or priority semantics. That is
exactly the gap the `MessageIntent` envelope and gateway custody fill: NephMesh would
use the existing store-and-forward transport and add meaning, priority, dedup by
correlation id, and expiry on top, at the gateway, rather than building a new
store-and-forward mechanism.

For acknowledgements, Meshtastic already offers a graded model that `ackStrength` should
map onto rather than replace: no ack, implicit acknowledgement by overhearing a
neighbor rebroadcast a flooded packet, and an explicit routing acknowledgement or
negative acknowledgement when a message requests one. The envelope's job is to say which
strength a given message actually needs, so control-plane chatter is spent in proportion
to a message's class and not by default. (The precise Meshtastic ack and flood-routing
behavior should be re-verified against firmware before implementation; treat this
paragraph as design intent, not a firmware spec.)

## 5. Silence is not failure

An intentionally quiet node, or a Quiescent fabric, can be operating exactly as
intended while a naive manager reads it as unreachable and therefore failed. Today's
device-management `Ready` signal is built on reachability, config sync, and reboot
state, which is correct for managing a device and wrong for judging a mission. This is
the reason the roadmap's stage 2 adds mission-aware status conditions alongside the
device ones.

The contingency layer feeds those conditions directly:

- MissionViable answers whether the fabric can still deliver the classes that matter,
  independent of whether any given node is currently transmitting. A fabric in Quiet or
  Quiescent mode can be MissionViable and silent at the same time.
- IntentionallyQuiet is asserted by the mode itself. When the steward has entered Quiet
  or Quiescent, this condition tells a manager that silence is a decision, so absence of
  traffic is not evidence of failure.
- MessageCustodyHealthy answers whether custody is coherent: messages under custody are
  either progressing, or expired by policy, or explicitly shed under declared storage
  pressure, with none silently lost.

All three use three-valued reasoning: healthy, unhealthy, or unknown. Unknown is a
first-class value and must not be silently collapsed into either healthy or failed. A
node the manager cannot currently hear is MissionViable=unknown, not MissionViable=false.
The `silencePreferred` envelope flag and the Quiet and Quiescent modes are the
mechanisms that let the system distinguish "chose not to speak" from "could not," which
is the whole point of encoding justified silence.

## 6. How it maps to the roadmap

This design is deliberately downstream of the core spine and does not reorder it.

- Doctrine section 7 is the parent. This document is the detailed elaboration of the
  contingency-semantics paragraph there, and it should not contradict it.
- Order of operations stage 2 (richer, mission-aware status conditions) is the nearest
  concrete hook. The `MissionViable`, `IntentionallyQuiet`, and `MessageCustodyHealthy`
  conditions with three-valued reasoning are a prerequisite that this layer both depends
  on and gives meaning to. Section 5 is written to that stage.
- Order of operations stage 7 (mission traffic classes and reserve accounting) supplies
  the traffic classes, protected shares, and reserve semantics this layer uses for
  admission and for what each degraded mode suppresses. The classes in section 2 are the
  same classes stage 7 defines; this document does not invent a second taxonomy.
- The gateway custody and store-and-forward work sits later still, alongside or after the
  multi-site resilience proof (stage 8) and the intent-layer frontier, because it needs a
  real gateway and the airtime-governance machinery to be trustworthy first.

The `MessageIntent` envelope is the interface between all of these: stage 7 fills its
class and reserve semantics, stage 2 consumes its silence and custody signals, and the
later gateway work implements the borrowed BPv7 behavior against it.

## 7. Open questions

- Envelope encoding. What is the minimum on-air envelope that still carries enough to
  decide send/store/drop on a leaf? Is it a few packed bytes prepended to the payload, or
  a Meshtastic portnum with a compact schema? The byte budget is unforgiving.
- Age versus timestamp in practice. Accumulated age needs a monotonic source across
  reboots; how reliable is that on the target hardware, and what is the conservative
  default when even age is untrustworthy?
- Dedup identity. Custody and replay need a correlation id, but ids cost bytes and leak
  structure. What is the smallest id that gives acceptable collision resistance across a
  low-hundreds-of-nodes fabric?
- Mode authority and consensus. Entering Quiet is an authority decision; entering
  Contested is evidence-driven. Which mode transitions must be locally decidable during a
  partition, and which may wait for or require coordination? The doctrine's rule is that
  hard safety must never depend on reaching peers.
- Aggregation semantics. `aggKey` implies mergeable messages, but merging situational
  reports can lose information. What merges are safe, and which must never be merged
  (life-safety, for one)?
- Interop and firmware honesty. If a real BPv7 or DTN deployment is ever adjacent, is the
  gateway a translation point, and does the borrowed-not-implemented stance hold at that
  seam? And the Meshtastic ack and store-and-forward behavior described here must be
  validated against current firmware in simulation before any of it is treated as settled.

## 8. Sources

- [RFC 9171, Bundle Protocol Version 7](https://www.rfc-editor.org/rfc/rfc9171.html):
  lifetime field, Bundle Age extension block for clockless nodes, Hop Count block, and
  the absence of custody transfer from this spec.
- [RFC 4838, Delay-Tolerant Networking Architecture](https://datatracker.ietf.org/doc/html/rfc4838):
  store-and-forward under intermittent connectivity, and the custodian model.
- [PACE communication methodology](https://en.wikipedia.org/wiki/PACE_(communication_methodology)):
  Primary, Alternate, Contingency, Emergency, and graceful degradation across
  independent tiers.
- [Meshtastic Store and Forward module](https://meshtastic.org/docs/configuration/module/store-and-forward-module/):
  server and client roles, PSRAM record store, history request bounds, heartbeat, and the
  duplicate-delivery limit.
- [Meshtastic overview](https://meshtastic.org/docs/overview/): usable per-packet payload
  and on-air cost, via `docs/plans/agent-mesh-nodes.md`.
- RFC 9315, Intent-Based Networking (via `docs/design/doctrine.md`): the outcome-versus-
  configuration distinction the envelope and modes lean on.

These are pointers to the ideas this design leans on, not claims that NephMesh implements
them. They are here so a reader can check the reasoning rather than take it on faith.
