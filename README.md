# NephMesh

**Intent-driven desired state configuration for communications: declare the comms system you want, and keep it running even when there is no carrier.**

Cellular networks fail: disasters, remote areas, overload, infrastructure loss. The radio technologies that survive those failures (LoRa mesh, license-free ISM bands, software defined radio) are configured by hand today, one device at a time. NephMesh explores managing them the way modern telecom manages 5G: as declared, version-controlled, continuously reconciled desired state, using the [Nephio](https://nephio.org) model of Kubernetes-native, intent-driven, Configuration-as-Data automation. It is the same design instinct that shaped early packet-switching research: assume the infrastructure can be lost, and build communications that route around the loss.

> **Disclaimer:** NephMesh is an independent, experimental open-source project. It is not affiliated with, endorsed by, or part of the official Nephio project, LF Networking, or The Linux Foundation. It consumes Nephio components as an ordinary third-party user, the same pattern other projects use to ship their own Nephio-ready packages.

## The idea

Declare in Git: *"edge site X shall run a mesh gateway on channel preset Y, bridge it to MQTT, prefer cellular when available, and report spectrum occupancy."* The system makes it so, keeps it so, and feeds sensed RF conditions back into the intent loop. Three workload types, none of which telecom automation has touched before:

- **[Meshtastic](https://meshtastic.org) LoRa mesh gateways** as declarative, reconciled network functions. No Meshtastic Kubernetes operator exists today; building the first one is this project's most broadly useful deliverable.
- **SDR spectrum sensing** (HackRF, RTL-SDR) as containerized, fleet-managed edge workloads. Receive-only by default.
- Later, a **lightweight 5G core** with a simulated RAN as the cellular leg of a hybrid failover fabric.

Our research found every pairwise combination of these ideas in the wild, but not the three-way intersection: see the [gap analysis](docs/research/prior-art-and-lab.md). Whether any of it deserves to exist is part of what the experiment is for.

## Status

Pre-alpha. The 0.1 gate has shipped: a virtual Meshtastic node deployed, declaratively configured, observed on MQTT, and torn down on a plain Kubernetes cluster, with an idempotent applier and persistence across pod restarts. Transcript and findings: [demo/phase1](demo/phase1/README.md). Demo captures land in this section as each roadmap gate ships. Contributions, questions, and skepticism are welcome.

## Start here

| Doc | What it covers |
|---|---|
| [Roadmap](docs/roadmap.md) | Phased plan in dependency order, the version path to 1.0, and how much runs with zero hardware (most of it) |
| [FAQ](docs/faq.md) | Why Kubernetes, why mesh, the resilient-comms lineage, legality, what the industry calls this |
| [Architecture](docs/architecture.md) | Components, planned CRDs, data flows, design principles |
| [Plans](docs/plans/) | Implementation plans: Phase 1, Phase 2, CRD API design, engineering conventions |
| [Research](docs/research/) | Sourced research: Nephio mechanics and codebase conventions, Meshtastic, SDR, prior art, terminology and legality |
| [AGENTS.md](AGENTS.md) | Conventions for AI coding agents; the repo is agent-native from day one |

## Cost and legality, briefly

Everything runs at $0 first: simulated radios, a simulated RAN, and kind/k3s on machines you already own. Real hardware (a ~$20 LoRa board, a ~$35 receive-only SDR) enters only when you want RF to be real. Spectrum sensing is receive-only and requires no license in the US, and encrypted mesh networking on license-free ISM bands is legal under the same FCC rules as Wi-Fi. Details and sources: [terminology and legality](docs/research/terminology-and-legality.md).

## License

[Apache 2.0](LICENSE)
