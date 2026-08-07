# A second radio driver: proving the driver seam

Status: research and design direction, not shipped. Nothing here is built. This
doc evaluates candidates for a second NephMesh radio driver and proposes the
minimal driver contract every radio must satisfy, so the "radio-agnostic seam"
claim can be tested against a real second implementation rather than asserted.

## 1. Why a second driver matters now

NephMesh states, in `AGENTS.md` and `docs/architecture.md`, that Meshtastic is
the first driver, not the only one, and that the design keeps a radio-agnostic
seam so other radios can be reconciled behind the same intent model. Today that
seam has exactly one implementation, `device.Client` in the Meshtastic operator
(`operators/meshtastic-operator/internal/device/device.go`). A seam with one
implementation is an assertion, not an abstraction: any interface fits a single
caller. The only honest way to show the seam is real is to reconcile a second,
structurally different radio through the same reconcile loop and see which
assumptions survive.

A well-chosen second driver does three things. It validates the claim cheaply,
in simulation, before any hardware. It surfaces where the current interface
silently encodes Meshtastic-specific behavior (reboot-on-apply, single-client
access, a flat channel model). And it de-risks the roadmap: the resilient-comms
landscape research already lists a second driver as a mid-term backlog item
(`docs/research/resilient-comms-landscape.md`, ranked backlog item 8), and names
MeshCore and ChirpStack as the two mature, structurally different LoRa control
surfaces with no existing operator.

## 2. The minimal driver interface

The current Meshtastic `device.Client` interface exposes four methods:
`ExportConfig`, `Apply`, `Reboot`, and `Info`, plus a sentinel `ErrUnreachable`
that lets the reconciler distinguish "retry shortly" from a hard failure. That
shape came from one radio, so part of this exercise is separating the general
verbs from the Meshtastic-specific ones.

The general verbs every radio driver must expose, derived from what the
reconciler actually needs, are:

- Export the currently-live configuration, decoded into a comparable form, so
  the reconciler can diff desired against actual and apply only drift (the
  minimal-diff rule in `AGENTS.md`).
- Apply a set of changes, ideally only the drift, returning cleanly even when
  the device becomes briefly unreachable as a result.
- Read telemetry (airtime, channel utilization, battery, link stats) for status
  and for the airtime-budget invariant.
- Read identity (a stable node id, and where available a public key and firmware
  version) so status and inventory can key on something durable.
- Report reachability, so the loop can requeue instead of erroring when a device
  is mid-reboot, asleep, or behind an intermittent link.

A conceptual Go-shaped contract, deliberately not tied to Meshtastic:

```go
// RadioDriver is the minimal control surface the reconciler needs from any
// radio. Methods return ErrUnreachable when the device cannot be reached, so
// the caller can requeue rather than fail.
type RadioDriver interface {
    // ExportConfig returns the device's live, desired-relevant config in a
    // form the reconciler can diff. Not every field, only what intent governs.
    ExportConfig(ctx context.Context) (Config, error)

    // Apply writes the drift (desired minus actual). The driver decides how to
    // actuate it; the reconciler never issues device-specific commands.
    Apply(ctx context.Context, drift Config) (ApplyResult, error)

    // Telemetry returns the radio's own operational metrics for status and the
    // airtime-budget invariant. Absent metrics are absent, not zero.
    Telemetry(ctx context.Context) (Telemetry, error)

    // Identity returns durable identity: a stable id, and where available a
    // public key and firmware version.
    Identity(ctx context.Context) (Identity, error)

    // Reachable is a cheap liveness probe that never mutates the device.
    Reachable(ctx context.Context) error
}
```

`ApplyResult` is where the Meshtastic-specific behavior moves out of the method
list and into data: it can report "this apply caused a reboot, expect
unreachability for N seconds" instead of every driver being forced to expose a
`Reboot` verb it may not have. This is the first thing a second driver would
teach us (see section 6).

## 3. Candidate: MeshCore

MeshCore is a newer LoRa mesh firmware, open source under the MIT license,
written as a portable C++ library, with firmware roles of Companion, Repeater,
and Room Server and support for multi-hop routing with configurable hop limits
([MeshCore site](https://meshcore.co.uk/),
[MeshCore on GitHub](https://github.com/meshcore-dev/MeshCore)). It advertises
30,000 plus users across 80 plus countries and 50 plus supported devices, so it
is past the abandoned-prototype stage that killed disaster.radio and Commotion
(noted in `docs/research/resilient-comms-landscape.md`).

Control surface: MeshCore has two relevant surfaces. Repeaters and Room Servers
expose a CLI (a "Repeater and Room Server CLI Reference" in the docs), and there
is a documented Companion Radio binary protocol used by the phone, web, Python,
and NodeJS clients ([MeshCore docs](https://docs.meshcore.io/)). The companion
protocol has concrete framed commands that map onto the driver verbs: an initial
`CMD_APP_START`, a `CMD_DEVICE_QUERY` for firmware version and capabilities, a
`PACKET_SELF_INFO` response carrying TX power, public key, radio frequency,
bandwidth, and spreading factor, `CMD_GET_CHANNEL` and `CMD_SET_CHANNEL` for
channel config, and `CMD_GET_BATTERY` for battery and storage telemetry
(command names from the companion protocol reference). That set is close to a
one-to-one match with ExportConfig (self-info plus channels), Apply
(set-channel), Telemetry (battery), and Identity (device query plus public key).

Topology and security: MeshCore is still a LoRa mesh, but its routing differs
from Meshtastic's managed flood. It uses a hybrid model (flood for discovery,
then more directed forwarding) with explicit non-repeating roles like Companion
to avoid poor paths, which is a different reconciliation target: role assignment
and repeater placement become first-class config, matching the "sparse,
well-placed routers" policy idea in the landscape research. It runs on the same
hardware as Meshtastic, so the hardware and $0-before-hardware story is shared.

Simulator: this is the weak point. No dedicated network simulator is documented.
The repo does support native unit tests via PlatformIO
(`pio test --environment native`), which gives a hardware-free path for driver
logic but not a full mesh emulator equivalent to Meshtasticator. Status here is
uncertain and worth checking directly before committing.

Maturity: active and widely used, but younger than Meshtastic; the docs are
mid-migration (the wiki now redirects to `docs.meshcore.io`), so the control
surface is documented but moving.

## 4. Candidate: ChirpStack

ChirpStack is a mature, open-source LoRaWAN Network Server (v4), MIT-licensed,
with a component architecture of Concentratord, MQTT Forwarder, Gateway Bridge,
and the Core network server ([ChirpStack architecture](https://www.chirpstack.io/docs/architecture.html)).
It is a LoRaWAN server, not a mesh firmware, so it is the most different
candidate and the most interesting stress test of the abstraction.

Control surface: ChirpStack is gRPC-first, with a gRPC-web/REST gateway on top,
and an object model of Tenants, Applications, Device Profiles, Devices, and
Gateways ([ChirpStack API](https://www.chirpstack.io/docs/chirpstack/api/index.html),
[gRPC API](https://www.chirpstack.io/docs/chirpstack/api/grpc.html)).
Authentication is a bearer API token in gRPC metadata, and official SDKs exist
for Go, Python, JavaScript, and Rust. This is the cleanest control surface of
the three: a CRUD object API over provisioning records maps almost directly onto
CRDs and a reconcile loop, and the landscape research notes there is a genuinely
empty automation gap here (no operator, Helm chart, or Terraform provider).

Topology and security: this is where ChirpStack differs hardest. LoRaWAN is a
star-of-stars: end devices talk to gateways, gateways backhaul to the network
server, and there is no device-to-device mesh at all. "Reconciling a device"
here does not mean pushing config to a radio over the air; it means reconciling a
provisioning record (device EUI, keys, device profile, application assignment)
in the network server's database. The device itself is often not reachable on
demand (Class A devices only receive just after they transmit). Security is
per-device AES with a proper join procedure (OTAA), which is stronger and more
structured than Meshtastic's shared-PSK channel model.

Simulator: strong. The `chirpstack-docker` repo brings up the full v4 stack
(core, gateway bridge, MQTT, PostgreSQL, Redis) with Docker Compose and no
hardware ([chirpstack-docker](https://github.com/chirpstack/chirpstack-docker)),
so the entire control plane runs for $0 in CI. Simulated uplinks can be injected
at the MQTT / gateway-bridge edge; a separate device simulator has existed
historically, but that specific project was not re-verified this pass, so treat
the "inject simulated traffic" path as the reliable one and the standalone
simulator as unconfirmed.

Maturity: high. ChirpStack is production LoRaWAN infrastructure with a stable v4
API and multiple language SDKs.

## 5. Candidate: Reticulum

Reticulum is a cryptography-based, transport-agnostic networking stack, already
named in NephMesh docs as the closest philosophical neighbor and a candidate
managed driver, not a competitor (`docs/research/resilient-comms-landscape.md`).
It provides self-sovereign addressing (an address is a hash of a public key),
encryption by default with forward secrecy, and next-hop self-healing routing
over media ranging from LoRa (via RNode firmware) to TCP/IP
([reticulum.network](https://reticulum.network/)).

Control surface: Reticulum has a real remote-management plane. Config lives in a
directory (default `~/.reticulum`) defining interfaces and transport. Utilities
include `rnsd` (the daemon), `rnstatus` (interface status and traffic stats,
with JSON output), `rnid` (identity management), `rnpath` (path and announce
inspection), `rnx` (remote command execution), `rnsh` (interactive remote
shell), and `rnodeconf` (RNode firmware and EEPROM configuration)
([Reticulum manual, using](https://markqvist.github.io/Reticulum/manual/using.html)).
Remote management is gated by an identity-hash allow-list
(`enable_remote_management` plus `remote_management_allowed`), which is exactly
the authenticated, allow-listed remote-admin model a reconcile loop wants. There
is also a Python RNS API. The mapping is workable: `rnstatus --json` and
`rnid`/`rnpath` cover telemetry and identity, the config directory plus
`rnodeconf` cover export and apply, and reachability is `rnpath`.

Topology and security: Reticulum is the security high point. Self-sovereign
addressing is the real answer to the shared-key impersonation weakness in
Meshtastic (the landscape research makes this point directly). Its routing is
next-hop self-healing rather than flood. The one honest caveat carried in the
existing docs: Reticulum links use AES-256-CBC plus HMAC, not a modern AEAD, and
group destinations use a shared symmetric key, the same tradeoff Meshtastic has.

Simulator: strong, and effectively free. A full Reticulum network can run
entirely in software over TCP, UDP, or AutoInterface with no radio at all; only
the RNodeInterface needs physical LoRa hardware
([Reticulum interfaces](https://markqvist.github.io/Reticulum/manual/interfaces.html)).
Multiple `rnsd` instances on one host form a testable multi-node network, which
is an excellent $0 CI story.

Maturity: the stack is mature and actively developed, though the ecosystem is
smaller and more single-maintainer-centric than Meshtastic or ChirpStack.

## 6. Others, briefly

Plain LoRaWAN gateways and other mesh firmwares are worth noting but weaker
first picks. A bare LoRaWAN gateway (Basics Station or Semtech UDP) has a thin
config surface and is really a component of a ChirpStack deployment, not an
independent driver. Other mesh firmwares exist but either lack a documented,
stable control surface or, like disaster.radio and Commotion, have lost momentum
(`docs/research/resilient-comms-landscape.md`), which is precisely the failure
mode NephMesh avoids by managing thriving firmware rather than building new
hardware.

## 7. Comparison and recommendation

| Dimension | MeshCore | ChirpStack | Reticulum |
| --- | --- | --- | --- |
| What it is | LoRa mesh firmware | LoRaWAN network server | Crypto-native transport stack |
| Control surface | CLI plus companion binary protocol | gRPC / REST CRUD object API | Config dir plus rnx/rnsh/rnstatus, Python API |
| Maps to export/apply/telemetry/identity | Close (self-info, set-channel, battery, device-query) | Cleanest (CRUD records) | Workable (rnstatus/rnid/rnodeconf) |
| $0 simulator for CI | Weak (native unit tests, no mesh sim) | Strong (docker-compose full stack) | Strong (software-only network, no radio) |
| Topology vs Meshtastic | Similar (LoRa mesh, hybrid routing, roles) | Very different (star-of-stars, no mesh) | Different (next-hop self-healing mesh) |
| Security vs Meshtastic | Similar shared-key family | Stronger (per-device OTAA keys) | Stronger (self-sovereign addressing) |
| Reconcile target | Radio config over the air | Provisioning record in a database | Node config plus identity allow-list |
| License | MIT | MIT | Reticulum License (MIT-style) |
| Maturity | Growing, docs in flux | Production-grade | Mature, smaller ecosystem |

Recommendation: implement MeshCore first, with Reticulum as a strong second and
ChirpStack as the deliberate stress-test third.

The reasoning follows the goal, which is to stress-test the seam without
choosing something so different it breaks it on the first try. MeshCore is the
same physical medium and the same rough problem shape as Meshtastic (a LoRa mesh
you configure over a companion link), but with different routing and an
independent control protocol. That difference is enough to expose Meshtastic
assumptions baked into the interface (does ExportConfig assume Meshtastic's YAML
shape? does Apply assume reboot-on-write?) while keeping the reconcile loop, the
airtime invariant, and the hardware-free story intact. Its main weakness is the
missing full simulator, which is real but bounded: the companion protocol can be
exercised against a fake in the same way `device.Fake` already stands in for
Meshtastic, and the native unit-test path covers driver logic.

Reticulum ranks a close second precisely because its $0 story is excellent (a
full software-only network) and it is already the project's stated candidate
managed driver, so building it pays down a documented credibility gap. It ranks
below MeshCore only because its remote-management verbs (`rnx`, `rnsh`) are
shell-shaped rather than config-object-shaped, so the export/apply mapping needs
more adaptation.

ChirpStack is the most valuable long-term stress test and the cleanest API, but
it should come after a mesh driver, because it changes so much at once (no mesh,
no over-the-air apply, a database record as the reconcile target) that doing it
first risks the interface bending to fit LoRaWAN rather than staying radio-
neutral. It is the right candidate for the moment the team wants to prove the
seam spans star-of-stars as well as mesh.

## 8. What the second driver would teach us

The point of the exercise is to find where MeshtasticNode-specific assumptions
leak. Likely leaks, ranked by confidence:

- Reboot-on-apply is Meshtastic-specific and currently sits in the interface as
  a first-class `Reboot` method and an implicit "expect unreachability after
  Apply." MeshCore and Reticulum may not reboot on every config change, so this
  belongs in `ApplyResult` data (does this apply require a restart, and for how
  long), not in the method set.
- Single-client access is a Meshtastic constraint (the TCP 4403 API is
  single-client). ChirpStack's gRPC API is happily concurrent, so any locking
  the driver layer assumes should live in the Meshtastic driver, not the
  interface.
- The config shape. ExportConfig returning `map[string]any` decoded from
  Meshtastic YAML leaks a Meshtastic representation. A second driver forces a
  neutral `Config` type or an agreed canonical form to diff against.
- Identity assumptions. `Info` today carries a node id parsed from the CLI plus
  airtime metrics. MeshCore and Reticulum lead with a public key as identity,
  and ChirpStack keys on a device EUI, so identity is richer than a single
  string and telemetry is not universally airtime-shaped.
- The reconcile target itself. For ChirpStack, "the device" is a database
  record, not a radio. If the interface can express that without contortion, the
  seam is genuinely radio-agnostic; if it cannot, that is the most valuable
  finding of all.

## 9. How it maps to the roadmap

This is a research-backlog item, not a committed phase. It corresponds to backlog
item 8 in `docs/research/resilient-comms-landscape.md` ("A second radio driver:
MeshCore first, or ChirpStack scoped to Gateway-Mesh relay-role intent") and to
the radio-agnostic-seam thesis stated in `AGENTS.md`. Sequenced against the
existing plan, the natural order is: first stabilize the general driver
interface as its own package (extract the general verbs from `device.Client`,
move reboot semantics into `ApplyResult`), then build a MeshCore driver plus fake
behind it, proving the reconcile loop and the airtime invariant work unchanged.
Nothing here should be started before the interface-extraction step, since the
whole value is measuring what the second driver forces the interface to change.

## 10. Open questions

- Does MeshCore have, or plan, a network simulator comparable to Meshtasticator?
  If not, is a companion-protocol fake plus native unit tests a sufficient $0
  bar, matching how `device.Fake` stands in for Meshtastic today?
- Is the MeshCore companion protocol stable enough to depend on while its docs
  are mid-migration, or should the driver target the Repeater/Room Server CLI
  instead?
- For ChirpStack, is reconciling a provisioning record (not a radio) still
  "reconciling a radio" for NephMesh's purposes, or does it belong to a separate
  "LoRaWAN provisioning" driver class with its own intent shape?
- For Reticulum, do the shell-shaped verbs (`rnx`, `rnsh`) plus the Python RNS
  API give a clean enough export/apply, or would a driver need a thin
  Reticulum-side agent to expose config as data?
- Which single `Config` representation can all three diff against without one
  radio's model dominating the type?

## 11. Sources

- [MeshCore site](https://meshcore.co.uk/)
- [MeshCore on GitHub](https://github.com/meshcore-dev/MeshCore)
- [MeshCore documentation](https://docs.meshcore.io/)
- [ChirpStack architecture](https://www.chirpstack.io/docs/architecture.html)
- [ChirpStack API overview](https://www.chirpstack.io/docs/chirpstack/api/index.html)
- [ChirpStack gRPC API](https://www.chirpstack.io/docs/chirpstack/api/grpc.html)
- [chirpstack-docker (docker-compose stack)](https://github.com/chirpstack/chirpstack-docker)
- [Reticulum](https://reticulum.network/)
- [Reticulum manual: using Reticulum](https://markqvist.github.io/Reticulum/manual/using.html)
- [Reticulum manual: interfaces](https://markqvist.github.io/Reticulum/manual/interfaces.html)
- [NephMesh resilient-comms landscape](./resilient-comms-landscape.md)
