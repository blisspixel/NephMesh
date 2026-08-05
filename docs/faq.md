# FAQ

Framing questions that come up when explaining this project. Sourced claims live in [research/](research/).

## What is this in one sentence?

Intent-driven desired state configuration for communications: you declare the comms system you want (mesh gateways, channels, encryption, spectrum monitoring, cellular where available) and the system continuously makes reality match, so that secure communication survives even when there is no carrier.

## Why Kubernetes? Why is that such a big deal here?

Not because "cloud". Kubernetes is the most battle-tested implementation of the desired-state reconciliation loop ever built: controllers watch actual state, compare it to declared state, and correct drift, forever. It is also extensible: custom resource types (CRDs) get that machinery for free. Nephio extends this model to telecom network functions; NephMesh extends it to radios. A `MeshtasticNode` CRD means a mesh gateway that gets rebuilt, reconfigured, and drift-corrected by the same self-healing loop that keeps a 5G core running. The alternative (shell scripts and hand-flashed configs) is exactly what the mesh and SDR communities use today, and it does not scale past a handful of nodes or survive the person who set it up.

A useful shorthand: Nephio is intent-driven desired state configuration for telecom. NephMesh asks whether that model generalizes to any radio system with a programmable control surface.

## Does Nephio support any of this today?

No. Nephio supports Kubernetes cluster lifecycle, 5G core network functions (free5GC, OAI), and O-RAN integration. It has zero native support for SDRs, LoRa, Meshtastic, or any RF and spectrum work. That absence is not a problem; it is the gap this project explores. Nephio's architecture explicitly allows third parties to add new workload types via their own packages, CRDs, and operators (the OpenAirInterface pattern), which is exactly what NephMesh does. See [research/nephio.md](research/nephio.md).

## Will the Nephio community care?

Realistically: core contributors focused on carrier-grade 5G production will mostly not, and that is fine. The project does not need their adoption to be useful. The plausible audiences are different: the `nephio-experimental` org exists precisely for PoCs that test the platform's limits; researchers in intent-driven networking get a reproducible testbed that costs two orders of magnitude less than USRP-based ones; and the resilience, emergency-comms, and private-network communities get fleet management that does not exist today. The strongest standalone artifact, a Meshtastic operator, is valuable to the Meshtastic community regardless of what anyone in telecom thinks.

## Why not just cellular? Why mesh at all?

Because the interesting failure mode is "there is no carrier": disasters, remote areas, overloaded networks, infrastructure failure. LoRa mesh is the tractable off-grid layer (long range, low power, license-free, fully scriptable). The point is not that mesh replaces cellular; it is that a declaratively managed system can hold both, prefer cellular when it exists, and fail over to mesh when it does not, with encryption and channel policy managed as data the whole time.

## Isn't this the same instinct that produced the early internet?

Philosophically, yes. Paul Baran's 1964 RAND work on distributed communications, one of the roots of packet switching, was explicitly about networks that survive the loss of large parts of their infrastructure: no central point of failure, adaptive routing, traffic finds another path. A Meshtastic mesh is that idea in miniature (every node routes for every other node), and NephMesh's hybrid framing (prefer cellular, fail over to mesh, never depend on one system) is the same design instinct applied across radio technologies.

One historical footnote worth getting right: the popular story that "ARPANET was built to survive nuclear war" is an oversimplification. ARPANET itself was built for resource sharing among research sites; it was Baran's survivability work that contributed the distributed packet-switching concept ARPANET adopted. The honest version of the parallel is still strong: the resilience DNA of the internet came from asking "what if the infrastructure is gone", and that is the exact question this project starts from. The scale is obviously different: ARPANET became the internet; this is a small experimental resilience layer. But the motivation is genuinely the same lineage.

## What about CB, GMRS, ham, other radio services?

Honest answer: diversity of technologies is the right instinct, but each service has to earn its way in by having a programmable control surface.

- Meshtastic/LoRa: fully scriptable (API, YAML config, containers). This is why it is first.
- Receive-only monitoring: technically easy, and generally unrestricted in our US-scoped reading (receiving is not licensed the way transmitting is, though interception of certain communications is restricted; see the disclaimer caveats). The SDR can watch CB (27 MHz), GMRS, ham bands, and ISM occupancy; that is just more spectrum sensing.
- CB as a managed transport: poor fit. It is analog voice with essentially no digital control surface; there is nothing to declare or reconcile beyond "a radio exists".
- Ham digital modes and GMRS data: possible future candidates where digital control surfaces exist, with the licensing caveats in [research/terminology-and-legality.md](research/terminology-and-legality.md) (notably: no encryption on amateur bands).

## Is an encrypted off-grid network like this legal?

Our US-scoped research suggests it is, and ordinary: license-free ISM bands under FCC Part 15 are the same rule section Wi-Fi uses, and Part 15 says nothing against encryption; the encryption prohibition people half-remember is an amateur-radio (Part 97) rule. But that is informal non-lawyer research, not legal advice, and it says nothing about other countries or your specific situation. Radio and encryption rules vary by jurisdiction and change. You are responsible for verifying what applies to you. Details and cites in [research/terminology-and-legality.md](research/terminology-and-legality.md); responsibility terms in the [DISCLAIMER](../DISCLAIMER.md).

## Is "INaC" a real industry term?

No, it is this project's shorthand. The established terms are intent-based networking (IBN, RFC 9315), intent-driven management (3GPP TS 28.312, TMF921, ETSI ZSM), and Nephio's own "intent-driven automation with Configuration as Data". Avoid plain "Network as Code": Nokia uses that name for an unrelated API product.
