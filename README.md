# NephMesh

**Intent-driven desired state configuration for communications: declare the comms system you want, and keep it running even when there is no carrier.**

Cellular networks fail, and not only in rare emergencies. Wildfire seasons and earthquakes knock out cell service somewhere every year; add hurricanes, floods, and ordinary grid outages, and "no signal" is a routine condition, not a doomsday one. The radio technologies that keep working when the towers do not (LoRa mesh, license-free ISM bands, software defined radio) are configured by hand today, one device at a time. NephMesh explores managing them the way modern telecom manages 5G: as declared, version-controlled, continuously reconciled desired state, using the [Nephio](https://nephio.org) model of Kubernetes-native, intent-driven, Configuration-as-Data automation. It is the same design instinct that shaped early packet-switching research: assume the infrastructure can be lost, and build communications that route around the loss.

Secure communication is a core goal, not an afterthought. Meshtastic channels are AES-256 encrypted, and NephMesh treats private channels as first-class: a team, a family, or a response group gets its own encrypted channel, and its pre-shared key lives in a Kubernetes Secret and never in Git. Single-node rotation is a Secret update the operator already reconciles; coordinated fleet rotation is designed, not built. Be precise about what that buys: a shared channel key gives confidentiality with a non-default key, but not per-sender authentication (any channel member can impersonate any other), and the [threat model](docs/security/threat-model.md) states these limits plainly rather than implying "encrypted" means "authenticated." Open public channels (the free community relief net anyone in range can join) and private, authenticated group coordination are both first-class: the same declarative machinery provisions either, and security is core precisely because it has to hold across that whole range.

> **Disclaimer:** NephMesh is an independent, experimental open-source project. It is not affiliated with, endorsed by, or part of the official Nephio project, LF Networking, or The Linux Foundation. It consumes Nephio components as an ordinary third-party user, the same pattern other projects use to ship their own Nephio-ready packages.

## The idea

Declare in Git: *"edge site X shall run a mesh gateway on channel preset Y, bridge it to MQTT, prefer cellular when available, and report spectrum occupancy."* The system makes it so, keeps it so, and feeds sensed RF conditions back into the intent loop. The scope is radio systems broadly, not one product:

- **LoRa mesh, starting with [Meshtastic](https://meshtastic.org)** as the first driver, because it has the most complete programmable control surface. The abstraction is meant to widen to other LoRa ecosystems (LoRaWAN, other mesh firmwares) over time. No Meshtastic Kubernetes operator (a CRD plus a continuous observe-diff-reconcile loop) exists today (last verified 2026-08-13). The operational cousin is [MeshMonitor](https://meshmonitor.org/): a live dashboard with Helm, remote admin, and automation. It is not Git-declared desired state, not an airtime admission gate, and not a spectrum witness. Building the first reconcile operator is the most broadly useful near-term deliverable, and it doubles as the reference for what a radio "driver" looks like here.
- **Software defined radio** (the HackRF Pro and cheaper receivers) as a co-equal pillar, not a mesh accessory: containerized, fleet-managed spectrum sensing today, with room for a much larger SDR possibility space later. Receive-only by default.
- Later, a **lightweight 5G core** with a simulated RAN as the cellular leg of a hybrid failover fabric.

The through-line is a radio-agnostic intent model: any radio with a control surface worth reconciling can become a driver. Our research found every pairwise combination of these ideas in the wild, but not this specific intersection: desired-state KRM for a decentralized ISM mesh, commodity-SDR evidence that is not the mesh talking about itself, and an airtime quota, with the cluster optional at runtime. See the [gap analysis](docs/research/prior-art-and-lab.md) and the [landscape synthesis](docs/research/resilient-comms-landscape.md). Closest cousins: [Reticulum](https://reticulum.network) (crypto-native mesh, a candidate driver), MeshMonitor (live Meshtastic fleet UI), OpenAirInterface's Nephio packages (the catalog pattern, for 5G NFs that die with the cluster), and [INA-Infra](https://arxiv.org/html/2410.09765) (Nephio plus USRP-class SDR). Whether any of it deserves to exist is part of what the experiment is for.

One thing to be clear about up front, because it is easy to misread: the Kubernetes control plane is not in the field. It provisions and manages the mesh from a powered site; the deployed Meshtastic nodes run autonomously once configured and keep carrying traffic even if the cluster, and the site running it, are gone. NephMesh is a management layer, not a runtime dependency of the mesh. In PACE terms (Primary, Alternate, Contingency, Emergency), it prepares and maintains the contingency tier before you need it.

The accurate framing of the value, and the honest limits, is "desired-state management for resilient radio fleets, with spectrum awareness and hybrid contingency," not "Meshtastic with Kubernetes." Physics caps the useful envelope at tens to low hundreds of nodes carrying low-bandwidth contingency traffic, and the audience is organized-operator resilience rather than a single hobbyist with five nodes. Where the project would grow if the core earns it, and why the core comes first, is written up as a design direction in the [doctrine](docs/design/doctrine.md); most of it is deliberately not built yet.

## Status

Pre-alpha. Shipped: **0.1.0** (virtual mesh), **0.2.0** (operator on `meshtasticd --sim`), **0.2.1** (channels, airtime, doctrine). Everything after 0.2.1 is Unreleased. The flagship config surface (region, preset, role, owner, MQTT, channels) is proven on sim in CI and on owned hardware. The engineering is still ahead of installability: the operator image is unpublished, and the Porch register/propose/approve/pull/apply gate has not been run. Next work, in order: [roadmap, What comes next](docs/roadmap.md#what-comes-next-recommended-order-reviewed-2026-08-13).

| You can run today | What it proves |
|---|---|
| [`demo/phase1`](demo/phase1/) | A simulated node deployed, configured, observed on MQTT, torn down |
| [`demo/operator`](demo/operator/) | The real reconcile loop, including a secure channel, no hardware |
| `nephmeshctl plan` / [`nephmesh-mcp`](docs/guides/agent-interface.md) | Report-only `CommunicationIntent` (renderable, not "objectives met") |
| [`demo/resilience`](demo/resilience/) | Measured control-plane independence and airtime-commons collapse on a UDP-sim mesh |
| [`demo/meshtoad-gateway`](demo/meshtoad-gateway/) | Two-radio LoRa text, channels, role. Needs owned hardware and `SENSOR_SSH` |
| [`demo/closed-loop`](demo/closed-loop/) | Hand-run three-witness: intent, reconcile, SDR peak move. Not autonomy |
| [`demo/edge-advisor`](demo/edge-advisor/) | Report-only local-model advice. The model never actuates |

Held frozen: `CommunicationIntent` actuation, an enforceable `ChannelBudget`, a second radio driver, Phases 6 and 7. Hardware applies on the bench used `reconcile-demo` (TCP or serial), not an in-cluster `MeshtasticNode` with `connection.serial`. The deployed operator's wired transport is TCP 4403.

![NephMesh's reconcile loop driving a live meshtasticd sim to convergence](docs/media/operator-reconcile.png)

*NephMesh's real reconcile loop (the `Converge` state machine and CLI-backed device client the controller uses), run against a live `meshtasticd --sim` via the `cmd/reconcile-demo` tool. Step 1 detects drift, applies only the minimal config, and reboots the device; step 2 re-verifies and reports `Ready` with the device's real node id. Captured from an actual run, not a mock-up.*

![The operator applying a config change to a physical Meshtastic T-Deck over USB and re-verifying to Ready](docs/media/operator-hardware-apply.png)

*The same loop against a physical handheld over USB serial (`reconcile-demo -serial`). The board rebooted and re-verified to `Ready`; the node id in the capture is the test fixture, not a lab device. The in-cluster operator's wired transport is TCP 4403. USB serial is a `reconcile-demo` path, not a shipped in-cluster `connection.serial`.* Contributions, questions, and skepticism are welcome.

## Start here

| Doc | What it covers |
|---|---|
| [Roadmap](docs/roadmap.md) | Phased plan in dependency order, the version path to 1.0, and how much runs with zero hardware (most of it) |
| [Design doctrine](docs/design/doctrine.md) | The design direction (mostly not built): intent as an outcome envelope, `MeshtasticNode` as a compiled artifact, airtime as a commons, and the honest boundaries. Read as a north star, not a feature list |
| [North star](docs/design/north-star.md) and [road to safe autonomy](docs/design/road-to-safe-autonomy.md) | What the project could honestly become (a runnable safety case for resilient autonomy), and the gated, evidence-first build order that gets there without over-scoping. See also [10x creative thinking](docs/design/10x-creative-thinking.md) for the first-principles framing (the surprise economy: shrink the demand, do not only ration it) |
| [Decision records](docs/adr/) | The significant, hard-to-reverse decisions and why, in Context/Decision/Consequences form |
| [FAQ](docs/faq.md) | The north star (a self-adapting multi-transport fabric), why Meshtastic first, secure private channels, power and autonomy, PACE/DIL, legality |
| [Architecture](docs/architecture.md) | Components, the radio-driver seam, planned CRDs, data flows, design principles |
| [Regulatory matrix](docs/reference/regulatory-matrix.md) | Informal per-region band, duty-cycle, power, and encryption-legality notes; verify against primary sources |
| [Nephio compatibility](docs/reference/nephio-compatibility.md) | Where the code stays Nephio-consumable and where it diverges on purpose, with a dated check of upstream API-group and library versions |
| [Plans](docs/plans/) | Implementation and design plans: the phases and the operator, plus the intent-layer frontier (CommunicationIntent and the compiler, signed autonomy and the safety kernel, rejoin as a treaty, key rotation, message authentication, contingency semantics) |
| [Research](docs/research/) | Sourced research: Nephio mechanics and codebase conventions, Meshtastic, SDR, prior art, terminology and legality, and a [resilient-comms landscape synthesis](docs/research/resilient-comms-landscape.md) (DTN, adversarial mesh, LoRa prior art, Reticulum/MeshCore) |
| [Operations runbook](docs/guides/operations.md) | Install, declare a node, observe (conditions, events, metrics), troubleshoot, and day-2 (key rotation, decommission, upgrade) |
| [Guides](docs/guides/) | How-to guides, for example registering the packages with Nephio Porch |
| [Examples](examples/) | Starting-point `MeshtasticNode` resources: a basic node and a secure-private-channel node with its Secret |
| [Code quality standards](docs/CODE-QUALITY-STANDARDS.md) | The engineering bar (race, fuzzing, govulncheck, envtest, assume-breach tests), what is enforced, and honest gaps |
| [Agent playbook](docs/agent-playbook.md) | Tool-agnostic commands and entry points for any coding agent or human |
| [Agent plugin](agent-plugin/) | Agent Plugins 1.0 wrap: `plugin.json`, stdio MCP, skills (report-only) |
| [MeshToad bench](demo/meshtoad-gateway/) | Two-radio LoRa replay: `meshtasticd` USB gateway plus a serial handheld |
| [Resilience harness](demo/resilience/) | Hardware-free MDR: control-plane independence and airtime-commons collapse |
| [Closed-loop PoC](demo/closed-loop/) | Hand-run three-witness: sense, actuate, verify. Not autonomy |
| [AGENTS.md](AGENTS.md) | Conventions for AI coding agents; the repo is agent-native from day one |
| [DISCLAIMER](DISCLAIMER.md) | Research-project and lawful-use terms: legality is your responsibility |
| [Threat model](docs/security/threat-model.md) | Security-first analysis with unmitigated risks named honestly |

## Cost, legality, and responsibility

Everything runs at $0 first: simulated radios, a simulated RAN, and kind/k3s on machines you already own. Real hardware (a ~$20 LoRa board, a ~$35 receive-only SDR) enters only when you want RF to be real.

This is a research project. Its SDR side is receive-only, and nothing here uses a software defined radio to transmit or to raise power. A Meshtastic mesh node transmits by design on license-free bands as its normal function, which the operator configures. Radio and encryption rules vary by country, band, and licensing, they change, and no one here claims to know the laws that apply to you. **You are solely responsible for ensuring that anything you do with this code and any radio hardware is legal where you are.** Please read the [DISCLAIMER](DISCLAIMER.md); any legality notes in the docs are informal, US-scoped, non-lawyer research, not legal advice. Security posture and known gaps: [threat model](docs/security/threat-model.md).

## License

[Apache 2.0](LICENSE)
