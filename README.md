# NephMesh

**Intent-driven desired state configuration for communications: declare the comms system you want, and keep it running even when there is no carrier.**

Cellular networks fail, and not only in rare emergencies. Wildfire seasons and earthquakes knock out cell service somewhere every year; add hurricanes, floods, and ordinary grid outages, and "no signal" is a routine condition, not a doomsday one. The radio technologies that keep working when the towers do not (LoRa mesh, license-free ISM bands, software defined radio) are configured by hand today, one device at a time. NephMesh explores managing them the way modern telecom manages 5G: as declared, version-controlled, continuously reconciled desired state, using the [Nephio](https://nephio.org) model of Kubernetes-native, intent-driven, Configuration-as-Data automation. It is the same design instinct that shaped early packet-switching research: assume the infrastructure can be lost, and build communications that route around the loss.

Secure communication is a core goal, not an afterthought. Meshtastic channels are AES-256 encrypted, and NephMesh treats private channels as first-class: a team, a family, or a response group gets its own encrypted channel, its pre-shared key lives in a Kubernetes Secret and never in Git, and keys can be rotated as declared policy. Be precise about what that buys: a shared channel key gives confidentiality with a non-default key, but not per-sender authentication (any channel member can impersonate any other), and the [threat model](docs/security/threat-model.md) states these limits plainly rather than implying "encrypted" means "authenticated." Open public channels (the free community relief net anyone in range can join) and private, authenticated group coordination are both first-class: the same declarative machinery provisions either, and security is core precisely because it has to hold across that whole range.

> **Disclaimer:** NephMesh is an independent, experimental open-source project. It is not affiliated with, endorsed by, or part of the official Nephio project, LF Networking, or The Linux Foundation. It consumes Nephio components as an ordinary third-party user, the same pattern other projects use to ship their own Nephio-ready packages.

## The idea

Declare in Git: *"edge site X shall run a mesh gateway on channel preset Y, bridge it to MQTT, prefer cellular when available, and report spectrum occupancy."* The system makes it so, keeps it so, and feeds sensed RF conditions back into the intent loop. The scope is radio systems broadly, not one product:

- **LoRa mesh, starting with [Meshtastic](https://meshtastic.org)** as the first driver, because it has the most complete programmable control surface. The abstraction is meant to widen to other LoRa ecosystems (LoRaWAN, other mesh firmwares) over time. No Meshtastic Kubernetes operator exists today (last verified 2026-08-05 against the Meshtastic ecosystem; the closest projects are apps or blind-push scripts, none doing a declarative observe-diff-reconcile loop); building the first one is the most broadly useful near-term deliverable, and it doubles as the reference for what a radio "driver" looks like here.
- **Software defined radio** (the HackRF Pro and cheaper receivers) as a co-equal pillar, not a mesh accessory: containerized, fleet-managed spectrum sensing today, with room for a much larger SDR possibility space later. Receive-only by default.
- Later, a **lightweight 5G core** with a simulated RAN as the cellular leg of a hybrid failover fabric.

The through-line is a radio-agnostic intent model: any radio with a control surface worth reconciling can become a driver. Our research found every pairwise combination of these ideas in the wild, but not this specific three-way intersection (declarative KRM intent, a decentralized LoRa mesh, and co-located commodity-SDR sensing feeding the loop): see the [gap analysis](docs/research/prior-art-and-lab.md) and the [landscape synthesis](docs/research/resilient-comms-landscape.md). The closest philosophical neighbor is [Reticulum](https://reticulum.network), a crypto-native, transport-agnostic mesh that already delivers much of the resilience story; what NephMesh adds is the Kubernetes and GitOps management plane and the SDR feedback loop, and Reticulum is a candidate managed driver rather than a competitor. Whether any of it deserves to exist is part of what the experiment is for.

One thing to be clear about up front, because it is easy to misread: the Kubernetes control plane is not in the field. It provisions and manages the mesh from a powered site; the deployed Meshtastic nodes run autonomously once configured and keep carrying traffic even if the cluster, and the site running it, are gone. NephMesh is a management layer, not a runtime dependency of the mesh. In PACE terms (Primary, Alternate, Contingency, Emergency), it prepares and maintains the contingency tier before you need it.

## Status

Pre-alpha, and further along than that sounds. **0.2.0** shipped the flagship: the `MeshtasticNode` operator, built in Go and validated against a live `meshtasticd --sim` device in CI (it reconciles region, modem preset, role, owner, and MQTT with a broker password read from a Secret), packaged as a kpt blueprint, and hardened with an envtest controller tier and assume-breach control-proving tests. 0.1.0 shipped the Phase 1 virtual mesh demo (a node deployed, configured, observed on MQTT, and torn down declaratively; transcript in [demo/phase1](demo/phase1/README.md)). The Go code holds lint, coverage, race-detector, and vulnerability-scan floors, with SHA-pinned CI actions. Everything so far is hardware-free; real-radio validation is the next hardware gate. Demo captures land here as each milestone ships.

![NephMesh's reconcile loop driving a live meshtasticd sim to convergence](docs/media/operator-reconcile.png)

*NephMesh's real reconcile loop (the `Converge` state machine and CLI-backed device client the controller uses), run against a live `meshtasticd --sim` via the `cmd/reconcile-demo` tool. Step 1 detects drift, applies only the minimal config, and reboots the device; step 2 re-verifies and reports `Ready` with the device's real node id. Captured from an actual run, not a mock-up.* Contributions, questions, and skepticism are welcome.

## Start here

| Doc | What it covers |
|---|---|
| [Roadmap](docs/roadmap.md) | Phased plan in dependency order, the version path to 1.0, and how much runs with zero hardware (most of it) |
| [FAQ](docs/faq.md) | The north star (a self-adapting multi-transport fabric), why Meshtastic first, secure private channels, power and autonomy, PACE/DIL, legality |
| [Architecture](docs/architecture.md) | Components, the radio-driver seam, planned CRDs, data flows, design principles |
| [Plans](docs/plans/) | Implementation plans: Phase 1, Phase 2, the operator, CRD API design, engineering conventions, and agentic AI nodes on the mesh |
| [Research](docs/research/) | Sourced research: Nephio mechanics and codebase conventions, Meshtastic, SDR, prior art, terminology and legality, and a [resilient-comms landscape synthesis](docs/research/resilient-comms-landscape.md) (DTN, adversarial mesh, LoRa prior art, Reticulum/MeshCore) |
| [Guides](docs/guides/) | How-to guides, for example registering the packages with Nephio Porch |
| [Code quality standards](docs/CODE-QUALITY-STANDARDS.md) | The engineering bar (race, fuzzing, govulncheck, envtest, assume-breach tests), what is enforced, and honest gaps |
| [Agent playbook](docs/agent-playbook.md) | Tool-agnostic commands and entry points for any coding agent or human |
| [AGENTS.md](AGENTS.md) | Conventions for AI coding agents; the repo is agent-native from day one |
| [DISCLAIMER](DISCLAIMER.md) | Research-project and lawful-use terms: legality is your responsibility |
| [Threat model](docs/security/threat-model.md) | Security-first analysis with unmitigated risks named honestly |

## Cost, legality, and responsibility

Everything runs at $0 first: simulated radios, a simulated RAN, and kind/k3s on machines you already own. Real hardware (a ~$20 LoRa board, a ~$35 receive-only SDR) enters only when you want RF to be real.

This is a research project. Its SDR side is receive-only, and nothing here uses a software defined radio to transmit or to raise power. A Meshtastic mesh node transmits by design on license-free bands as its normal function, which the operator configures. Radio and encryption rules vary by country, band, and licensing, they change, and no one here claims to know the laws that apply to you. **You are solely responsible for ensuring that anything you do with this code and any radio hardware is legal where you are.** Please read the [DISCLAIMER](DISCLAIMER.md); any legality notes in the docs are informal, US-scoped, non-lawyer research, not legal advice. Security posture and known gaps: [threat model](docs/security/threat-model.md).

## License

[Apache 2.0](LICENSE)
