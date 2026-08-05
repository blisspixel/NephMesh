# Plan: agentic AI nodes on the mesh

Status: research-backed design, not yet built. This is a Phase 6+ research direction, deliberately downstream of the core operator and closed loop. It is written now because the idea is load-bearing for the project's mission and shapes earlier decisions (the MQTT bridge, the intent boundary, the secure-channel work).

The idea: when the internet is gone but a LoRa mesh is up, a field node with no connectivity can query an **AI node**, a service reachable over the mesh that helps with what people actually need in a wildfire or earthquake: "where is the nearest open shelter", "how do I control bleeding", "relay this to the coordinator". The AI node lives where there is power and compute (a base, a vehicle, an edge box); the field carries cheap battery radios. This could genuinely help, and it could genuinely harm if done carelessly, so the plan leads with the constraints.

Sources for the claims below are in the research notes gathered 2026-08-05; key figures are cited inline.

## Hard constraints that shape everything

### Bandwidth: this is a telegram, not a chat

LoRa's physical packet is 256 bytes; a Meshtastic packet carries about 200 bytes of usable application data after framing ([meshtastic.org/docs/overview](https://meshtastic.org/docs/overview/)). The default Long Fast preset is roughly 1 kbps of raw airtime, and a small packet is already about 350 ms on air; multi-hop round trips are seconds to tens of seconds, and EU regions enforce a duty cycle (about 100 packets per hour per node under a 1% sub-band). A channel congests above roughly 60 nearby nodes on Long Fast.

Consequences the design must accept, not fight:

- A question is one or two short sentences; an answer targets 200 bytes, and if longer it is explicitly chunked (`1/3`, `2/3`, ...) and paced to respect airtime and duty cycle.
- Terse, structured, templated output is mandatory. Prose is a luxury the medium cannot afford.
- Every byte is expensive and shared. The AI node must be economical by construction, not by request.

### The template-and-code scheme (how to say a lot in 200 bytes)

The proven pattern from emergency radio is to send a code, not the text, and render the localized text at the receiver. This is exactly how the FCC's multilingual Wireless Emergency Alerts will work by 2028 (a template id crosses the air; the phone renders pre-stored text in the user's language, 13 languages plus English plus ASL), and how ARRL Numbered Radiograms have worked for decades (`ARL FORTY SIX` expands to a full standardized message only at delivery).

NephMesh scheme:

- **Coded request grammar:** `<qid> <CATEGORY> <free text?>`, for example `A7 MED bleeding leg`. Category codes are language-neutral (MED, WTR, SHL, EVAC, NAV, RPT).
- **Templated coded responses:** the agent replies with a response code plus minimal slot values; each field node renders the localized or pictographic expansion from a pre-installed template pack. Only codes and variables cross the air.
- **Symbol fallback** (ISO 7010 / CAP hazard pictograms) for low literacy, rendered locally from a code.
- **Language negotiation once:** a node registers a 2-letter language tag; answers are codes; the node renders locally. Translated prose never crosses the mesh. Align hazard categories to Common Alerting Protocol (CAP) taxonomy (its codes, not its verbose XML).

This keeps the system usable for a global, multilingual, sometimes low-literacy population, which the safety research flags as a first-order requirement, not a nicety.

## The integration surface (how an agent attaches to the mesh)

Two clean paths, both already supported by Meshtastic:

- **TCP 4403 sidecar (recommended for a co-located agent):** the `meshtasticd` container exposes the client API on 4403; the AI node is a pod that subscribes (`meshtastic.receive` pub/sub), inspects `to`/`from`/portnum, and replies to a specific node with `sendText(destinationId=..., wantAck=True)`. This fits the NephMesh Kubernetes model directly: the agent is just another declared workload next to the gateway.
- **MQTT bridge (for a base-station or fan-out agent):** subscribe to `msh/REGION/2/e/...` (protobuf ServiceEnvelope, still channel-encrypted) or the JSON topics, and inject replies via `msh/REGION/2/json/mqtt/` with a downlink-enabled `mqtt` channel.

Design choice: AI queries and answers are **direct messages with `wantAck=True`** for real delivery feedback; a broadcast channel is only for public advisories (broadcast ACK is suppressed, so delivery is weaker).

## Reliability over a lossy mesh

Meshtastic already dedupes on packet id, does managed flooding, and resends reliable packets up to three times. The AI-service layer adds an application envelope on top:

- A short correlation id (`qid`) so a node matches an answer to its question, and the agent can dedupe repeated requests with a small LRU and return the cached answer instead of recomputing and re-transmitting.
- Explicit chunking with reassembly by `qid` plus index, paced for duty cycle.
- Generous timeouts (tens of seconds to minutes) with bounded, backed-off retries so a lost answer does not flood a duty-limited channel.
- **Store-and-forward:** Meshtastic's firmware Store and Forward module needs a mains-powered PSRAM node and today covers channels, not direct messages, so NephMesh implements an application-level inbox (a BBS-style queue) on the AI node that answers when a requester reappears. Prior art to borrow: the `meshbot` framework (DM mode, allow-list, built-in BBS).

This is the same store-carry-forward problem, at a human scale, that the space section treats at planetary scale.

## The safety boundary (the most important section)

An AI that gives emergency advice is high-stakes. The research is blunt: hallucination is the central hazard and may be irreducible (WHO 2024 guidance on large multi-modal models; NIST's Generative AI Profile names "confabulation" as a core risk), errors in medical or safety advice can cause direct physical harm, and humans over-trust AI outputs (automation bias). The design must earn trust, not assume it.

Do and do not, adopted as design rules:

- **Ground answers in a curated, signed, versioned, region-specific dataset** (shelters, evacuation routes, official contacts, widely accepted first aid) and quote with source and timestamp. Prefer retrieval over generation. Never present frozen training-data facts as current situational truth, and never fabricate a route, frequency, phone number, or shelter that is not in the dataset.
- **Defer, do not diagnose.** General first-aid guidance framed as such is acceptable; a definitive medical diagnosis or a medication dose is not. Signal uncertainty; say "I am not certain, here is what the official dataset says, updated on <date>".
- **Persistent disclaimers** that this is not professional advice and to verify with authorities when possible.
- **Log every consequential answer** for later review (respecting privacy, see data sovereignty), so accountability starts from evidence.

### The agent never actuates (a hard architectural boundary, not a prompt)

The single firmest rule: the AI advises; a human decides and acts. In a comms system that must be enforced by architecture.

- **Air-gap the model from the transceiver control path.** The inference process has no capability, API, or permission to key the radio, change `lora.region` / frequency / power, or reconfigure the mesh. It emits text into a reply buffer only.
- **Privilege separation via the platform.** In the NephMesh model this is RBAC and NetworkPolicy: the AI pod may reach the 4403 text interface to send replies, and has no access to the intent controllers or admin channel that reconfigure radios. The agent's credentials are kept off the admin channel entirely, so it is cryptographically incapable of issuing config or transmit-control commands.
- **The agent proposes, never commits.** If the agent ever suggests a network change, it writes an intent *draft* to a proposed queue that a human approves through the normal Porch lifecycle before any controller reconciles it. This is the same "LLM proposes, reconciliation loop enforces, agent is never the control loop" principle already stated in the roadmap, applied to life-safety, where the literature is emphatic that a rubber-stamp human step is a design failure (Therac-25 is the warning), so the human must have genuine authority and the action must be interruptible and logged.

This boundary also aligns with radio law: in most jurisdictions a licensed human control operator is responsible for every transmission, so an autonomous transmitter is both unsafe and often unlawful.

## Global by construction, not US-first

The safety and global research is clear that a US-centric design fails abroad, and the failures are legal, not cosmetic:

- **Region is a legal constraint, encoded as configuration, never assumed.** Frequency plans differ worldwide (EU868, US915, AS923, IN865, and about 24 Meshtastic regions), and region controls frequency, duty cycle, and legal power. Never hardcode US915, English, 911, FEMA, or imperial units.
- **Encryption legality varies and can be dangerous to get wrong.** Some countries restrict encryption or mandate decryption assistance, and some prohibit encrypted radio messages outright; amateur bands internationally forbid obscuring meaning. NephMesh must let operators choose whether and how to encrypt per local law, and must not assume amateur-band operation is a safe default.
- **Data sovereignty is a safety feature.** Local-only, no cloud, no phone-home, air-gapped operation reduces what a hostile regime can compel or intercept, which the humanitarian data-responsibility frameworks (OCHA, IASC) and the encryption-law reality both push toward.
- **Align to humanitarian standards** where it clarifies "useful and safe": Sphere and the Core Humanitarian Standard (accountability to affected people), ITU emergency telecom and the Tampere Convention, and Humanitarian OpenStreetMap for offline, per-region basemaps and points of interest.

## The honest space and DTN north star

The user's instinct that this could matter off-world is real, but only for the right layer. The rigorous version:

- **Genuine parallel:** node autonomy independent of the control plane, store-and-forward, disruption tolerance, and intent-driven reconciliation on reconnect are exactly what Delay/Disruption-Tolerant Networking solves. NASA runs DTN operationally (Bundle Protocol v7, RFC 9171; PACE used it for routine telemetry with tens of millions of bundles at 100% success), and LunaNet names BPv7 as its interoperable networking layer. Kubernetes-style edge orchestration is already being flown (Axiom's on-orbit data center on Red Hat Device Edge; Spaceborne Computer-2). NephMesh's "control plane provisions, autonomous nodes survive its loss, reconcile when a link returns" is the same disruption-tolerant control philosophy at a terrestrial scale.
- **Stretch to avoid claiming:** LoRa RF and commodity SDR do not reach orbit; space uses licensed, radiation-hardened, CCSDS-standardized links (Proximity-1 for surface, optical and X-band for backhaul). A low-power mesh maps, at most, onto the short-range surface tier, and even there the radio would be CCSDS, not LoRa.
- **The useful framing:** treat the terrestrial-disaster case and the space case as one disruption-tolerant problem differing only in round-trip time and RF. If the architecture is designed that way (disconnection is normal, not a failure; store-and-forward is first-class; intent reconciles eventually), then speaking real DTN (BPv7 via a stack like DTN7 or uD3TN) later is an extension, not a rewrite. That is the honest reason NASA or a serious operator might find the control-plane semantics interesting.

## Phasing (where this sits)

This is not near-term. Order of operations:

1. The operator and closed loop first (the reliable, boring, deterministic core). An agent is only useful once there is a reliable surface to act on.
2. The MQTT bridge and secure channels (already designed) are the substrate the agent attaches to.
3. A minimal, offline, retrieval-grounded, advice-only AI node as a declared workload, DM plus BBS, template-and-code output, air-gapped from all transmit and config paths, is the first experiment.
4. Disruption-tolerant framing throughout, so a later DTN or space extension is credible rather than bolted on.

## Edge inference reality (what actually runs offline)

The research here is encouraging and, helpfully, it points at the same architecture the rest of the project already uses.

- **Put the model at a powered base, not in the field.** Per-query inference energy is trivial everywhere (under about 0.25 Wh); the design driver is idle power over 24 hours. So the efficient design is a mains- or solar-powered base station running the model, with field units that are sub-watt ESP32 Meshtastic radios relaying queries in and answers out. This is the same "control plane provisions from a powered site, cheap autonomous nodes in the field" split the whole project is built on. A Pi-class always-on field AI node is feasible (roughly a 100 W panel and 500 Wh battery, or far less with a wake-on-demand MCU front-end) but only worth it if inference must survive base-station loss.
- **A bare small model is the wrong tool for safety-critical answers; grounding is the tool.** The strong recommendation is a curated, authoritative, versioned knowledge base plus strict retrieval-grounded generation: every claim cites a retrieved passage, the model abstains and refuses ("not in my sources, contact <official channel>") when retrieval is empty, and the highest-risk content (first aid, evacuation steps, phone numbers) is pre-authored and answered extractively rather than free-generated. The knowledge base is the source of truth; the model only synthesizes and routes. This lines up exactly with the safety section above.
- **What runs:** the smallest genuinely useful model for terse Q&A and JSON extraction is around 0.5 to 0.6B (Qwen2.5-0.5B / Qwen3-0.6B) with grammar-constrained decoding at temperature 0, with about 1.7B (Qwen3-1.7B) as a safe default; architecture and constrained decoding matter more than raw size. Hardware paths: a Raspberry Pi 5 with llama.cpp (simplest, CPU-only, roughly 7 to 18 tokens/sec on 1 to 3B models at about 3 to 8 W), an Orange Pi 5 / RK3588 with Rockchip RKLLM on the NPU (roughly double the throughput per watt on Qwen models, but a fragile W8A8-only toolchain), or a Jetson Orin Nano Super (the low-power standout, 40 to 165 tokens/sec on sub-1.5B models at 5.6 to 8.5 W). Offline retrieval on a Pi is cheap: a small embedding model (nomic-embed-text or EmbeddingGemma) plus sqlite-vec or LanceDB, with hybrid keyword-plus-vector search. Kiwix ZIM packs (WikiMed, WikEM) and offline OSM maps (OsmAnd, Organic Maps) are the pre-loadable knowledge base, with the important caveat that offline maps have no live hazard or road-closure data, so evacuation routing must be built on a curated shelter and hazard dataset.

## Prior art (this is not greenfield, which sharpens the gap)

Several offline mesh-plus-LLM projects already exist, which validates the idea and clarifies what NephMesh would add: Blackbox Node (fully local llama.cpp command post queried over the mesh), MeshClaw (an agent-to-Meshtastic bridge targeting disaster and air-gapped use), MESH-AI (an off-grid AI-plus-mesh router whose author explicitly warns against emergency reliance in beta), and academic cautions like the DORA benchmark, which found large reliability failures for LLM agents on long emergency-operations pipelines. The gap NephMesh fills is not "connect an LLM to a mesh," which exists, but doing it as a declaratively managed, retrieval-grounded, air-gapped-from-transmit, globally-applicable, human-decides workload with the same reliability discipline as the rest of the fleet, and honestly, most existing efforts flag themselves as not-for-mission-critical, which is exactly the bar this project cares about.

## Open questions and to verify

- Whether curated regional datasets (shelters, routes, first aid, official contacts, offline maps) can be assembled, signed, and maintained per region, and by whom. This is arguably the hardest and most important part, harder than the model.
- The exact Meshtastic admin-key model in current firmware, to confirm the agent can be made cryptographically incapable of admin actions.
- CAP taxonomy mapping for the category and response codes.
- Reliability evidence: the DORA-style finding that agents degrade badly on long pipelines argues for keeping the agent's job narrow (retrieve and relay grounded facts, not multi-step planning) and measuring it before trusting it.
