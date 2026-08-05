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

## Is this only for rare, dramatic scenarios?

No, and that is the point. The failures NephMesh targets are common and recurring, not hypothetical. Wildfire seasons force evacuations and take out cell sites every year; earthquakes drop service across whole regions; hurricanes and floods do the same on a schedule; and ordinary grid or backhaul outages leave a valley or a neighborhood with no bars for hours. In all of these, low-power LoRa mesh keeps carrying text and location when the towers are dark. The extreme adversarial case (a hostile actor degrading infrastructure) sits at the far end of the same spectrum and uses the same tool, but the everyday driver is the wildfire and the earthquake, which is where the design should be judged. Resilience here is measured, not asserted: the roadmap defines message delivery ratio during an outage and time to failover as gate criteria.

## What makes the comms secure, and can channels be private?

Secure, private channels are a core capability, built on mechanisms that mostly exist already:

- Meshtastic channels are AES-256 encrypted by default; a channel's traffic is readable only by holders of its pre-shared key.
- NephMesh treats a private channel as first-class intent. The `MeshtasticNode` custom resource declares channels whose PSKs are referenced from Kubernetes Secrets (`pskSecretRef`), never inlined in a resource or committed to Git. Different teams, families, or response groups get separate encrypted channels, so coordination stays within the group.
- Keys are managed as declared state, so key rotation becomes a policy the reconciliation loop enforces rather than a manual re-flash of every radio (rotation lands with the closed loop in Phase 6).
- The honest limits: a channel's key is symmetric and shared, so any member can impersonate any other within that channel, and the default channel's key is public (never use it for anything private). Metadata (that a transmission happened, roughly where) is never hidden. These are properties of the medium, stated plainly in the [threat model](security/threat-model.md).

So open broadcast is supported and useful, but the harder, more valuable case, provisioning and maintaining private encrypted channels for a group across a fleet of radios, is exactly what the intent model is for.

## Why not just cellular? Why mesh at all?

Because the interesting failure mode is "there is no carrier": disasters, remote areas, overloaded networks, infrastructure failure. LoRa mesh is the tractable off-grid layer (long range, low power, license-free, fully scriptable). The point is not that mesh replaces cellular; it is that a declaratively managed system can hold both, prefer cellular when it exists, and fail over to mesh when it does not, with encryption and channel policy managed as data the whole time.

## Isn't this the same instinct that produced the early internet?

Philosophically, yes. Paul Baran's 1964 RAND work on distributed communications, one of the roots of packet switching, was explicitly about networks that survive the loss of large parts of their infrastructure: no central point of failure, adaptive routing, traffic finds another path. A Meshtastic mesh is that idea in miniature (every node routes for every other node), and NephMesh's hybrid framing (prefer cellular, fail over to mesh, never depend on one system) is the same design instinct applied across radio technologies.

One historical footnote worth getting right: the popular story that "ARPANET was built to survive nuclear war" is an oversimplification. ARPANET itself was built for resource sharing among research sites; it was Baran's survivability work that contributed the distributed packet-switching concept ARPANET adopted. The honest version of the parallel is still strong: the resilience DNA of the internet came from asking "what if the infrastructure is gone", and that is the exact question this project starts from. The scale is obviously different: ARPANET became the internet; this is a small experimental resilience layer. But the motivation is genuinely the same lineage.

## Wait, Kubernetes needs power and a datacenter. How is that "resilient" when the grid is down?

This is the most important thing to understand about the architecture, and it is easy to miss: the Kubernetes control plane is not in the field. It provisions and manages the mesh from a powered site (a homelab, an emergency operations center, a vehicle with power, a cloud region while one exists). The deployed Meshtastic nodes run the real firmware and, once configured, operate completely autonomously. They do not need the cluster, the operator, Porch, or any network back to them to keep carrying traffic.

So the resilience story is honest and specific: NephMesh is a management and provisioning layer, not a runtime dependency of the mesh. Kill the cluster (grid loss, flood, the manager site is gone) and every already-configured node keeps meshing. The cluster is how you configure fifty nodes consistently and reconfigure them when you can reach them again; it is not the thing the field depends on. A validation for this is explicit on the roadmap: kill the control plane and prove the mesh keeps delivering messages.

Two consequences the docs commit to: the mesh nodes are low-power by design (Meshtastic runs for days on a small battery), while the control plane is not something you run off a solar panel in a tent; and field bootstrapping must not require kubectl-from-a-canoe, so an offline, air-gapped path (mirrored images, pre-provisioned SD cards, no default keys) is a first-class goal, not an afterthought.

## What is the north star, the fully realized version?

A self-adapting, multi-transport communications fabric that keeps working while its pieces fail around it. The declared intent is not "run this radio" but "keep secure communication available here," and the system chooses how: use cloud and cellular backhaul when they exist (the Primary tier, and satellite links like Starlink fit here too), bridge onto the mesh when they degrade, and fall all the way to a local off-grid mesh when there is no telco at all. As conditions change, it adapts: the closed loop shifts channels away from interference, rotates keys and channels on a security policy, and re-homes traffic across whatever transport is currently alive.

The design property to aim for is what you might call cockroach resilience: no single point of failure, graceful degradation rather than collapse, and autonomous survival of the edge. Because the control plane is not in the field (see the power question above), the mesh keeps carrying traffic even when the managing site, the cloud, and the carrier are all gone. Every tier that is present makes the system better; no tier being present is required for it to keep working at some level.

Be clear about status: this is the destination, not a claim about today. The building blocks are already on the roadmap (PACE tiers, the spectrum-to-intent closed loop, control-plane-independent nodes, dynamic channel and PSK rotation, the cellular-and-cloud bridge). The current phases build toward it one honest, demoable gate at a time, and much of it is still research. The point of the experiment is to find out how much of the cockroach actually holds up.

## Is there a doctrine this maps to?

Yes, and using its vocabulary makes the project far clearer to the people who need it. Emergency and defense communicators plan comms as PACE: Primary, Alternate, Contingency, Emergency. Cellular is Primary; NephMesh's managed mesh is a Contingency or Emergency tier that is already configured and waiting. The environment these operators name is DIL: Disconnected, Intermittent, Limited. NephMesh is a tool for DIL conditions, and its receive-only-by-default posture doubles as emission control (EMCON): a radio that only listens does not advertise its position to direction finding. Transmitting is emitting, and emitting is detectable; that trade is the operator's to make deliberately, which is exactly why transmit is opt-in.

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
