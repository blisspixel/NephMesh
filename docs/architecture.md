# NephMesh Architecture

High-level design. This evolves with the roadmap; details for each layer are in [research/](research/).

## The core idea

Nephio's model: intent lives in Git as kpt packages of plain KRM YAML; Porch orchestrates package lifecycle (Draft → Proposed → Published); `PackageVariant`/`PackageVariantSet` clone and specialize a blueprint per site by injecting each site's `WorkloadCluster` facts; a GitOps agent (Config Sync / Argo / Flux) pulls published packages onto workload clusters; operators on those clusters reconcile intent CRs into running workloads.

NephMesh adds nothing to that machinery. It supplies **new blueprints, CRDs, and operators for radio workloads** and registers its own catalog repo with Porch - the same third-party pattern OpenAirInterface uses for its RAN packages.

## Radio-agnostic by design

The project is about radio systems in general, not one product. The intent model treats a radio as a **driver**: a CRD plus an operator that knows how to reconcile one kind of control surface. Meshtastic is the first driver because its control surface is the most complete (a scriptable API, config round-trip, simulation mode), and the `MeshtasticNode` operator doubles as the reference for what a driver looks like. The seam is meant to widen:

- **Wider LoRa:** LoRaWAN (for example ChirpStack, whose network server is already cloud-native) and other mesh firmwares can become additional drivers behind the same propose-approve-reconcile flow.
- **SDR as a co-equal pillar:** spectrum sensing is a first-class workload today (a `SpectrumScan` driver over SoapySDR, hardware-agnostic by design), with a larger SDR possibility space later, not a mesh accessory.
- **Multi-transport adaptation:** the longer arc is a fabric that keeps secure comms alive across whatever transport is currently available (cloud and cellular and satellite backhaul when present, mesh when not), adapting channels, keys, and routing as conditions change. The north star and its honest status are in `docs/faq.md`; the building blocks (PACE tiers, the closed loop, control-plane-independent nodes) are on the roadmap.

A new radio earns inclusion by having a control surface worth reconciling; analog services with no digital control (for example CB voice) stay out of scope as managed transports, though the SDR side can still monitor them.

## Topology

```
┌─────────────────────────────────────────────────────────────┐
│ Management cluster (kind / Nephio sandbox)                  │
│                                                             │
│  Porch ── registers ──► NephMesh catalog (this repo, Git)   │
│  PackageVariantSet ──► one specialized pkg per WorkloadCluster
│  Config Sync / Argo ──► pushes published pkgs to edges      │
│  (later) spectrum-policy controller: metrics → Git commits  │
└──────────────┬──────────────────────────┬───────────────────┘
               │                          │
   ┌───────────▼───────────┐   ┌──────────▼──────────────────┐
   │ Edge site A (k3s/Pi)  │   │ Edge site B (kind, virtual) │
   │                       │   │                             │
   │ meshtastic-operator   │   │ meshtasticd -s (simulated)  │
   │ meshtasticd + radio ──┼─RF┼── mesh peers                │
   │ spectrum-sensor pod   │   │ MQTT broker (Mosquitto)     │
   │  └─ RTL-SDR / HackRF  │   │ (later) 5G core + UERANSIM  │
   └───────────────────────┘   └─────────────────────────────┘
```

## Components

### Packages (kpt, in `packages/` eventually)

| Package | Contents |
|---|---|
| `mesh-gateway` blueprint | meshtasticd Deployment (official image), config PVC, Service on TCP 4403, `MeshtasticNode` intent CR, placeholder `WorkloadCluster` for injection |
| `meshtastic-operator` | The operator (Phase 4), deployed per-cluster via PVS - free5gc-operator pattern |
| `spectrum-sensor` blueprint | soapy_power scanner + CSV→metrics exporter sidecar, device-plugin resource request |
| `mqtt-bridge` | Mosquitto (or the official Meshtastic packet-aware broker) + bridge config |
| `demo/*` | PackageVariant/PVS examples, sandbox scripts |

### CRDs (planned)

- **`MeshtasticNode`** - desired radio state: region, modem preset, role, channels (PSKs referenced from Secrets), MQTT module config, owner. Reconciled over the TCP 4403 device API by export → diff → apply-only-drift (config applies reboot the node per section, so minimal diffs matter). Can target remote radio-only nodes through a gateway via Meshtastic remote admin.
- **`SpectrumScan`** - band(s) to sweep, bin width, interval, output (Prometheus aggregates and/or MQTT full spectra), which SDR (SoapySDR driver string - same CR works for RTL-SDR and HackRF).
- Later: a policy CR closing the loop (occupancy threshold → channel-change intent).

### Device access

USB radios (RTL-SDR `0bda:2838`, HackRF `1d50:6089`, CH341 LoRa sticks) are exposed to pods with `squat/generic-device-plugin` - advertised as named resources (`nephmesh.io/rtlsdr: 1`), no privileged pods. Host prep (documented per phase): udev rules, `dvb_usb_rtl28xxu` blacklist for RTL-SDR, group permissions. Akri is the upgrade path if dynamic discovery/scheduling becomes a feature. Escape hatch: SoapySDRServer/`sdr-server` on the host, sensing pods connect over TCP.

### Data flow

1. **Intent down:** Git commit → Porch package revision → PV/PVS specialization (WorkloadCluster injection) → GitOps sync → operator reconciles radios.
2. **Mesh traffic up:** LoRa mesh → gateway node → MQTT module → private broker (`msh/REGION/2/e/...` protobuf ServiceEnvelope; JSON topics for convenience - note JSON is lossy and unsupported on nRF52 nodes). Consumers dedupe on packet `id` (multiple gateways duplicate).
3. **Spectrum up:** SDR → soapy_power sweep CSV → exporter → Prometheus (per-band aggregates) / MQTT (full spectra).
4. **Closed loop (Phase 6):** policy controller reads spectrum metrics → commits intent changes → flow 1 repeats. Humans stay in the loop via Porch's propose/approve lifecycle.

## Design principles

- **Provider-neutral, local-first.** The core runs on kind, k3s, and bare single-board computers; CI depends on nothing else. No cloud provider (GCP, AWS, Azure) is imported or assumed anywhere in core packages, CRDs, or operators. If cloud-specific material ever exists, it lives in separated distro directories, mirroring the Nephio catalog's `distros/` layout where GCP is one distro among several rather than a dependency.
- **Upstream-compatible by construction.** Code follows the Nephio codebase conventions documented in `docs/research/nephio-codebase.md` (module layout, condkptsdk, golden tests, license headers, DCO), so that if any of this ever makes sense to integrate into the Nephio ecosystem, the rework is minimal. This is planning ahead, not presumption: the project is an experiment in what an extension might look like.
- **Global by configuration, never US-first.** Radio region, frequency, duty cycle, power, language, emergency contacts, units, and legal posture (including whether and how to encrypt) are data, never hardcoded assumptions. LoRa frequency plans and encryption law differ by country, and a US-centric default is illegal or useless elsewhere; the design keeps all of it region-configurable and defaults to nothing that assumes a jurisdiction.
- **Disruption-tolerant, a terrestrial cousin of DTN.** Disconnection is normal, not a failure mode: the control plane provisions and reconciles when a link returns, and nodes store and forward in the meantime. This is the same problem NASA solves with Delay/Disruption-Tolerant Networking (Bundle Protocol v7), at a terrestrial scale and latency. The honest scope: the architecture and network semantics map onto space needs (autonomous nodes, store-and-forward, intent reconciliation), while the LoRa/SDR physical layer does not (space uses licensed, radiation-hardened, CCSDS links). Designing disruption tolerance in as a first-class invariant keeps a future DTN or space extension credible rather than bolted on. See `docs/plans/agent-mesh-nodes.md` for how this connects to the agentic-node vision.

- **Consume Nephio, never fork it.** Pin to a release (R6 now); track R7's modularization work.
- **Everything must run at $0.** Every feature lands first against simulated radios (`meshtasticd -s`, Meshtasticator, UERANSIM) so CI and newcomers need no hardware.
- **Hardware-agnostic radio code.** SoapySDR abstraction; a driver string is config, not code.
- **Receive-only by default.** Transmit paths are explicit opt-in, documented with the regulatory notes in [research/sdr-spectrum-sensing.md](research/sdr-spectrum-sensing.md). This governs SDR transmit and power; a Meshtastic mesh node transmits by design as its normal, legal ISM function.
- **Secure private channels are a core goal, not a bolt-on.** A private encrypted channel is first-class intent: the `MeshtasticNode` API declares channels whose PSKs come from Kubernetes Secrets (`pskSecretRef`), never inlined or committed, so separate groups get separate encrypted channels and keys can be rotated as declared policy (rotation in Phase 6). The design never weakens the firmware's encryption and never reuses the public default channel key for anything private. The honest limits (symmetric shared keys, unprotected metadata) are stated in the threat model rather than hidden.
- **Minimal-diff reconciliation.** Radio nodes reboot on config apply; the operator must never blindly re-apply full config.
