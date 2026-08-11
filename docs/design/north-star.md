# North star: a runnable safety case for resilient autonomy

Status: aspiration and framing, not a roadmap and not shipped. This is the
companion to the [doctrine](doctrine.md) (the invariants) and
[road-to-safe-autonomy.md](road-to-safe-autonomy.md) (the gated build plan). It
exists to name the biggest honest thing this project could become, and it is
written under a hard rule: it adds zero roadmap items. Over-scoping is the
doctrine's named number-one failure mode, so a visionary document that appended
features would be the disease, not the cure. The reframing below lives entirely in
the claim the project makes and the order it already committed to, not in new work.
It is the output of a five-perspective elevation review (deep-space / disruption-
tolerant networking, anti-fragile reliability engineering, honest scale, flight-grade
code, and a tenth-power reframing), which converged, which is the reason to record it.

## 1. The reframing: the product is trust, the radio is a driver

Physics caps the radios and the doctrine says so plainly (section 15): tens to low
hundreds of low-bandwidth nodes per collision domain, forever, and claiming
telecom-scale value would be dishonest. So the elevation is not in the radios. It is
in the discipline above them, and it is a discipline the field almost never ships.

The thing that does not exist anywhere, open or closed, is a runnable, checkable
safety case for autonomous resilient communication under active attack: an
independent veto you can model-check, hard invariants that hold when the signed
authority object is absent or corrupt, and a resilience claim measured over trials
with confidence intervals instead of asserted with an adjective. That reframes the
project:

> NephMesh is the reference implementation of a provably-and-measurably-safe
> autonomic control plane for communications that must keep working when their
> infrastructure is gone and an adversary is present. The product is trust; the
> radio is a swappable driver; LoRa is only the first driver that had a programmable
> control surface.

The one-line form, to earn and not yet to claim: the safety case for the last radio
standing, as code you can run and check.

The escalator is honest because it has a real first rung. The most rigorous open
resilient-comms testbed in existence is nearly true today, and it is the credible
on-ramp to the provably-safe control plane that a disaster, defense, or space
research group would actually adopt, precisely because it is the most rigorous open
thing in the category, not because it moves the most bytes. "Transport-agnostic
internet that survives anything" oversells the physical layer; "the standard way
humanity manages contingency comms" is an adoption outcome a solo project can earn
but not plan. Both stay north stars. The control plane being transport-agnostic (the
radio-driver seam, with MeshCore and then Reticulum as candidate drivers) is the
demonstrable version of the first, and it is a rung, not a slogan.

## 2. The convergence: five threads are one autonomic loop with a missing wire

Tag the shipped and designed pieces with the letters of a MAPE-K autonomic loop
(Monitor, Analyze, Plan, Execute over shared Knowledge; Kephart and Chess, IBM, 2003):

- Monitor: the SDR witness, radio airtime telemetry, and the resilience probe. Shipped.
- Analyze and Plan: the airtime time-on-air model and the intent compiler, the
  Predictor. Shipped.
- Measure, and, run before acting, a digital twin: the simulation harness. Shipped,
  one flag from pre-actuation use.
- Execute made trustworthy: the independent safety kernel, the veto. Designed.
- Knowledge, the weak unnamed letter: the Intent Capsule and last-known-good.

The unifying object the doctrine already named is the Evidence-Carrying Action, which
carries a claim, its evidence and age, the predicted effect, the authorizing epoch,
a rollback target, and a verification plan together, and is then closed by the
residual (predicted delivery minus measured delivery) and filtered by the veto. Alone
each thread is a feature; wired through that one object they become an autonomic loop
whose audit trace is also its conformance trace, which is trustworthy autonomy rather
than automatic knob-turning.

Airtime-as-a-commons is what forces the loop to be governed rather than optimized.
Because a shared channel is a common-pool resource (Ostrom, Governing the Commons),
the objective cannot be a scalar a learner maximizes; it must be lexicographic (hard
invariants, then mission priority, then disruption, then resource, then uncertainty).
You cannot safely optimize a commons, so you govern it, and governance needs a veto:
the commons framing and the safety kernel are one idea, not two. The collapse the
survival demo measured is Hardin's tragedy of the commons; the recovery it found,
admission control pacing offered load back inside budget, is Ostrom's answer that
governed access beats both open access and privatization.

## 3. The highest-leverage spine, and it is already seeded

One technical spine delivers every elevation at once, and the repository already
contains its seed: `internal/resilience` is a pure, deterministic verdict function,
and the boundary between the messy world and that oracle is a flat JSONL event
schema. That is exactly the architecture of deterministic simulation testing, the
technique behind systems famous for surviving anything (FoundationDB, TigerBeetle,
and the commercial descendant Antithesis): a single-threaded deterministic core, a
simulated world, a seeded fault scheduler, a checker, millions of runs, and every
failure reproducible from a seed.

The spine: build a pure in-memory mesh model and a seeded, adversarial fault
scheduler, run the real safety kernel and the real reducer against millions of
adversarial histories in CI, and gate the model with a conformance test against the
Docker harness's measured airtime knee so the simulation can never drift into proving
things about a fantasy mesh. That single spine is simultaneously:

- anti-fragile: "it just keeps working" becomes provable over millions of seeded
  histories including node loss, partition, clock chaos, and a channel herder, with
  every failure a replayable seed and every survival a distribution, not a single run;
- flight-grade code: a small, model-checked, import-isolated trusted computing base
  (the veto), with fuzz, property, metamorphic, and mutation tests that prove the
  tests bite (the pattern seL4 sets as the bar for a proven-small TCB, and ASTM F3269
  runtime assurance sets for bounding an untrusted controller with a verified monitor);
- the disruption-tolerant autonomy substrate: the same veto, custody, and
  contact-graph-routing problem NASA and JPL solved for deep space (ION, CGR,
  Bundle Protocol v7), whose control and store-and-forward semantics map onto the
  disconnected edge even though the LoRa physical layer never will;
- honest scale: federate thousands of verified cells rather than grow one, "BGP for
  small meshes," promoting the collision domain to a first-class autonomous system
  that advertises a signed viability summary and routes between domains over a contact
  graph, with every cell staying inside physics.

The claim it earns, and why it is not marketing: we do not assert NephMesh is
resilient; we build an adversary whose entire job is to find a bounded-fault history
where the veto executes a forbidden action, authority ascends without a fresh capsule,
the mesh follows a herder, or self-stabilization fails to converge; we run it for
millions of histories; and we publish the seeds it found, the fixes, and the honest
boundary where the proof stops. That boundary is fixed and stated everywhere: the
simulation proves discrete control, delivery, and the airtime commons; it never
proves RF propagation, frequency relocation, or custody, and the safety claim is
conditional, given faithful evidence the kernel is safe, because evidence
classification is owned by the RF adversary. The independence failure that makes this
honest ("two thermometers in the same fire": radio airtime and SDR occupancy are both
functions of the medium the adversary controls) is a textbook common-cause failure,
and it is why the envelope-authenticated application delivery ratio, not the
RF-derived signals, is the load-bearing independent corroborator.

## 4. Elevation moves by lens, each with its honest boundary

Every move is a rung on the existing ladder or a near-term seed, not a new subsystem.

| Lens | The move (all $0, sim, mostly report-only) | The honest boundary it must never cross |
|---|---|---|
| Reliability | A pure `meshsim` plus a seeded, coverage-guided adversarial fault searcher running the real kernel and reducer over millions of histories; the herder A/B and partition scenarios with K-trial confidence intervals; a `check-under-attack` CI gate | Models control, delivery, and the airtime commons only; never RF, frequency relocation, or custody. Gated by a conformance test to the measured airtime knee. |
| Flight-grade code | Fuzz and property/metamorphic tests on the reducer (turn its invariant comments into checks), mutation testing to prove coverage bites, a build-enforced kernel-isolation import gate, a TLA+/Apalache model of the authority machine, reproducible builds | Model-check the abstract machine, not a refinement proof of the Go; scope Power-of-Ten rigor to the kernel package; defer SBOM and signing until images publish. |
| Disruption tolerance | A custody store-carry-forward gateway shim measured over partition; a pure contact-graph-routing function over a synthetic schedule; light-time delay injection to stress the clockless bundle-age path | Custody needs a real gateway build, not sim reuse (Meshtastic store-and-forward is server-hardware-only); the LoRa PHY is not a deep-space link and never claims to be. |
| Honest scale | A multi-domain sim (several collision domains, per-domain budgets, a cross-domain gateway) proving one domain's collapse does not touch another; a second bearer with PACE failover measured as delivery ratio and time-to-failover; a hierarchy-of-stewards toy proving the constitutional invariant holds at a leaf with the issuer deleted | Scale lives in the number of domains, the federation, and multi-bearer routing; never in a single cell's node count or throughput. Any capacity number needs hardware. |

## 5. The seed to start today

It is almost embarrassingly small, and it is already the first rung of the build plan:
add an `objectives` field (minDeliveryRatio, maxMessageAge) to the
`CommunicationIntent` CRD, and write one pure, report-only reducer that computes the
residual between the compiler's predicted delivery and the harness's measured
delivery, emitting a measured `IntentInfeasible`. One typed field, one subtraction,
two values the system already produces.

It is the whole vision in a seed because it is the first moment the system checks a
claim about itself against measurement instead of asserting it: today `Feasible` means
"renderable", after this seed `IntentInfeasible` means "physics forbids the outcome you
asked for, and here is the delivery number that proves it". And it composes upward with
nothing wasted: the residual is the first field of the Evidence-Carrying Action, the
Evidence-Carrying Action is what the kernel vetoes, the kernel is what model-checking
certifies, and model-checked authority is what lets the word "provably" be used without
lying. A grace note that captures the spirit: the survival demo already sampled two
points of the utilization-to-delivery curve by hand (about 19 percent utilization to
100 percent delivery, about 56 percent to 50 percent); fitting that curve replaces the
folklore 25 percent ceiling the mesh community repeats with an evidence-derived knee.

## 6. North star, not roadmap

The discipline that keeps this from becoming the over-scoping that kills the project:
everything in the left column is a rung already on the ladder; everything in the right
column stays a north star and must never become a dated item.

| Credible near-term ($0, sim, already gated) | Inspiring but a north star, and why it must not be a roadmap item |
|---|---|
| The measured assurance loop (objectives, residual, measured `IntentInfeasible`) | "The internet that survives anything" at scale: physics forbids it (doctrine section 15) |
| The typed Evidence-Carrying Action and the pure kernel `Decide` with a bad-action battery | Off-world / interplanetary operation: the control and store-and-forward semantics map, the LoRa/SDR physical layer does not; a metaphor, not a milestone |
| A content-addressed signed capsule and a three-state degrading lease as pure functions | "The standard way humanity manages contingency comms": an adoption outcome, earned, never planned |
| The herder A/B and partition scenarios with confidence intervals; a `meshsim` deterministic tester | Demonstrated custody and store-carry-forward: needs a real gateway build, gated |
| A TLA+/Apalache model of the single-node authority machine | Full self-stabilization or CRDT-convergence proofs, or ML/learned control: overkill or last-and-shadow-only |
| A second radio driver in sim, to make the transport-agnostic claim demonstrated | SDR transmit or full cognitive radio: a legal and hardware wall; receive-only stays a safety, trust, and legal posture |

## The through-line

The biggest honest thing NephMesh could be is not a bigger radio network. It is the
place where autonomous communication under duress becomes checkable: an open, runnable
safety case with a small verified veto, a commons it governs rather than optimizes, and
a resilience claim carried by a confidence interval instead of an adjective. The radios
keep it free and concrete; the discipline is what could matter on the scale of
infrastructure. And the seed that starts pulling toward it today is one struct field and
one subtraction, predicted minus measured. Build that wire, and the report-only renderer
quietly becomes the smallest honest instance of the thing worth becoming.

## Sources

Autonomic computing and MAPE-K: Kephart and Chess, "The Vision of Autonomic Computing"
(IEEE Computer, 2003). Runtime assurance and the verified-monitor pattern: Sha, "Using
Simplicity to Control Complexity" (Simplex); ASTM F3269 (run-time assurance for bounding
untrusted controllers). Deterministic simulation testing: FoundationDB and TigerBeetle
simulation testing; Antithesis. Proven-small trusted computing base: the seL4 verified
microkernel. Commons governance: Ostrom, Governing the Commons. Deep-space disruption
tolerance: NASA/JPL ION and Contact Graph Routing; RFC 9171 (Bundle Protocol v7).
Inter-domain federation and its instability lessons: BGP and RFC 2439 route-flap damping
with its later operational reassessment. Transport-agnostic addressing: Reticulum. These
are pointers to the ideas this framing leans on, not claims that NephMesh implements them.
