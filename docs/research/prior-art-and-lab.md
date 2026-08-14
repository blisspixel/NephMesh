# Research: Prior art, gap analysis, and the minimal-cost lab path

Researched 2026-08-04, Meshtastic/K8s cousin refreshed 2026-08-13. Prices are observed street prices and fluctuate.

## Prior art map

- **LoRaWAN + K8s:** ChirpStack has community Helm charts and a production EKS deployment guide ([Helium's](https://docs.helium.com/iot/run-an-lns/kubernetes/)) - but LoRaWAN is a star topology whose *centralized network server* is what runs on K8s; dumb gateways backhaul to it. Meshtastic is a decentralized peer-to-peer mesh with **no network server at all**, so managing it from K8s is a different problem: gateway/bridge nodes, MQTT topology, and radio-config intents rather than an LNS.
- **Meshtastic + K8s:** no CRD operator (continuous observe-diff-reconcile of node config) found as of 2026-08-13. Closest: [MeshMonitor](https://meshmonitor.org/) (live dashboard, Helm, remote admin, automation, MeshCore alongside Meshtastic). That is imperative fleet UI, not Git-declared desired state, not an airtime admission gate, and not a spectrum witness. Also: the [official Meshtastic MQTT broker](https://github.com/meshtastic/mqtt) (Docker, no chart).
- **Disaster/resilience mesh:** active academic thread - Meshtastic resilience analysis ([arXiv:2605.17063](https://arxiv.org/abs/2605.17063)), Wi-Fi HaLow post-disaster architectures ([arXiv:2507.07841](https://arxiv.org/abs/2507.07841)), solar LoRa mesh papers - plus practitioner EMCOMM communities (Placer County ARES, community mesh build guides). All imperative and hand-configured; **no orchestration layer**.
- **Nephio extensions:** [nephio-experimental](https://github.com/nephio-experimental) org (PoCs explicitly not TSC-endorsed - a natural eventual home or model for this work); **[INA-Infra](https://arxiv.org/html/2410.09765)** - research framework built on Nephio with OAI + USRP SDRs and E2E slicing. INA-Infra is NephMesh's closest neighbor: it validates the "Nephio + SDR" architecture at ~100× the hardware cost, with no mesh or disaster layer.
- **Intent-driven spectrum:** O-RAN RIC xApps/rApps for dynamic spectrum sharing ([O-DSS](https://arxiv.org/html/2601.02571), [AdapShare](https://arxiv.org/html/2408.16842v1), ChARM, ProSAS, SPARC) - all assume O-RAN E2 interfaces and licensed spectrum. DARPA SC2's durable legacy is the [Colosseum](https://arxiv.org/pdf/2110.10617) emulator at Northeastern.
- **Crowdsourced sensing:** Electrosense (RTL-SDR + Pi + cloud backend, open API) - centralized, not intent-driven, not co-managed with comms.

### The gap

Every pairwise combination exists; this intersection does not:

> **Desired-state KRM for a decentralized ISM mesh, plus commodity-SDR evidence that is not the mesh talking about itself, plus an airtime quota, with the cluster optional at runtime.**

Cousins, one cell each: OpenAirInterface (Nephio catalog pattern, 5G NFs that die with the cluster); [INA-Infra](https://arxiv.org/html/2410.09765) (Nephio plus USRP-class SDR, licensed cellular); MeshMonitor (live Meshtastic fleet UI); Reticulum (crypto mesh, no Kubernetes); Electrosense (commodity sensing, cloud backend); ClusterDuck (disaster firmware, no GitOps). The empty cell is the sentence above, not "fleet management does not exist."

Still true, scoped: (1) no Meshtastic CRD operator; (2) intent-driven spectrum work has not targeted ISM-band mesh with cheap sensors; (3) Nephio's published catalogs have not touched sub-GHz unlicensed infrastructure.

## Lab tiers (verified pricing, 2025–2026)

Note: this research record predates the roadmap rewrite. The "tiers" below were cost tiers for someone starting from nothing; the roadmap now uses dependency-ordered phases instead, and the maintainer's existing hardware makes every phase $0. Rough mapping: Tier 0 covers Phases 1 and 3, Tier 1 covers Phase 2, Tier 2 covers Phases 5 and 6, Tier 3 covers Phase 7.

### Tier 0 - $0 (one PC)
- kind/k3s. Full Nephio sandbox wants 16 vCPU / 32 GB / 200 GB (Ubuntu 22.04); a slim path (kind + Porch + Config Sync + our own controller) runs in far less.
- Virtual nodes: `meshtasticd -s` (official Docker image, real firmware, simulated radio); multi-node RF via [Meshtasticator](https://github.com/meshtastic/Meshtasticator) (known Docker reconnect-loop issue #57).
- MQTT: Mosquitto chart or the official Meshtastic broker.

### Tier 1 - ~$70–120
- 2–3 × Heltec WiFi LoRa 32 V3: ~$18–27 each (V4 @ 28 dBm now out, pin-compatible, worth considering).
- RTL-SDR Blog V4: $29.95–39.95 - **EOL** (R828D stock exhausted; "V4L" successor pending). V3/clones same price band.
- Receive-only: no license needed per our US-scoped research (verify your own jurisdiction; see the repository DISCLAIMER); you sense the 902–928 MHz band your own mesh transmits in.

### Tier 2 - ~$250 used / ~$500 new
- **HackRF One discontinued Sept 2025 → HackRF Pro $400** (supply-constrained). Used HackRF Ones ~$200–250; sub-$150 clones exist, quality unverified.
- Raspberry Pi 5: RAM-shortage inflation (16 GB hit ~$305); the **4 GB ($70) or 8 GB ($95)** models suffice for a k3s edge node; a used Pi 4 (~$50) is a sane substitute.
- Original "$300–400" target only holds with used gear.

### Tier 3 - +$0 (5G leg)
- **free5GC** on kind/minikube: ~4 vCPU / 8 GB floor; needs the gtp5g kernel module (UPF), Multus, 8 Gi PV for MongoDB; [towards5gs-helm](https://github.com/Orange-OpenSource/towards5gs-helm) deploys core + UERANSIM.
- **OAI CN5G**: official Helm charts (~10 pods); no published minimum, community baseline same ~4/8.
- **Open5GS**: lighter-weight alternative ([Gradiant 5g-charts](https://gradiant.github.io/5g-charts/open5gs-oaignb.html)).
- **UERANSIM**: simulated gNB + UE - no cellular hardware ever needed. Practitioner note: co-locating UERANSIM with the UPF on one node caused PDU-session flakiness; separate nodes are more reliable.
- Nephio's [Exercise 1](https://docs.nephio.org/docs/guides/user-guides/usecase-user-guides/exercise-1-free5gc/) (free5GC + UERANSIM via package specialization) is the direct template.

## Per-tier "hello world" demos

- **Tier 0:** a Git-committed intent deploys a virtual node + MQTT bridge; a text sent via CLI round-trips onto `msh/US/...` topics; deleting the file tears it down.
- **Tier 1:** an intent change (LongFast → MediumSlow) propagates to physical radios; a message crosses real RF; the mesh's transmissions appear in RTL-SDR occupancy metrics - the "declared comms + observed spectrum" loop, read-only.
- **Tier 2:** the Pi is a declaratively managed edge site; HackRF injects a controlled ISM test tone; the sensing pipeline triggers a channel-change intent commit - a miniature O-RAN-xApp-style loop on $30 sensors.
- **Tier 3:** kill the simulated gNB ("cell outage") → intent promotes the Meshtastic path → messages keep flowing; restore, and traffic bridges back.
