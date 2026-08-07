# SDR transmit: a gated frontier, not the spine

Status: frontier idea in consideration, deliberately gated, not shipped, and not
started. NephMesh is receive-only by default on the SDR side, and that receive-only
default is the one posture CI mechanically depends on (see
[AGENTS.md](../../AGENTS.md) and the transmit interlocks noted in the
[regulatory matrix](../reference/regulatory-matrix.md)). This document is a
possibility-and-boundaries survey: it names what SDR transmit would open, the honest
hardware reality, and the regulatory and authorization framework that would make any
experiment lawful. It is deliberately not a how-to. It contains no step-by-step
instructions for transmitting, tuning power, or operating on any band, because the
point here is to map the space and its gates, not to cross them. Nothing below is
legal advice. It is informal, non-lawyer research, US-scoped where it names rules, and
legality where you are is your responsibility: read the repository
[DISCLAIMER](../../DISCLAIMER.md) first.

Why write it down at all: the project's stated stance is to name the seductive
frontier ideas openly, with their why and their gate, rather than pretend they do not
exist. SDR transmit is the clearest example. It is genuinely powerful for a resilient
comms project, and it is exactly the capability most likely to cause legal, safety, or
trust harm if it is added casually. Both facts belong in the same document.

## 1. What SDR transmit would open

Today the HackRF is a sensor: NephMesh uses it to observe spectrum, not to occupy it.
Allowing transmit, in an authorized context, would change its category from
instrument to radio. Concretely, it would open the following possibility space.

- The SDR as an actual radio node. LoRa's physical layer has been reverse engineered
  and implemented in software defined radio. The widely cited
  [gr-lora](https://github.com/rpp0/gr-lora) project decodes LoRa across spreading
  factors and coding rates on commodity SDRs (HackRF, USRP, RTL-SDR, LimeSDR), and its
  authors consider the modulation "fully reverse engineered." That project is receive
  only, but it establishes that the LoRa waveform is well understood in the SDR
  domain. A transmit-capable equivalent would let one HackRF act as a software-defined
  LoRa node or gateway with no dedicated SX126x/SX127x hardware at all: the modem
  becomes code, not a soldered part.

- Custom and more jam-resistant waveforms. A general-purpose transmitter is not locked
  to one modem's fixed modes. It could, in principle, run frequency-hopping schemes or
  LR-FHSS (Long Range Frequency Hopping Spread Spectrum), a LoRa-family modulation
  designed for robustness and capacity that Meshtastic's current presets do not use
  (Meshtastic today runs fixed LoRa presets on spreading factors 7 to 12, per the
  [Meshtastic radio settings](https://meshtastic.org/docs/overview/radio-settings/)).
  Waveform agility is the main thing a purpose-built LoRa transceiver cannot give you
  and an SDR can.

- Cross-band relays. A device that can both receive and transmit across 1 MHz to
  6 GHz could bridge two otherwise incompatible allocations or bands, subject to every
  band's own rules applying independently at each end. This is a capability that a
  single fixed-function radio cannot express.

- The full cognitive-radio loop on one device. Spectrum sensing plus transmit on the
  same hardware closes the sense-and-act loop: observe occupancy, then choose when and
  where to emit. This is the natural end state of the spectrum-sensing work, and it is
  also the point at which the safety surface grows the most, because the same device
  now decides both what it hears and what it says.

- The SDR as a first-class PHY-layer radio driver. In the existing driver model
  (see [the second-radio-driver survey](./second-radio-driver.md)), every radio is
  reconciled behind the same intent seam. Today the SDR sits outside that seam as a
  sensor/exporter. Transmit is what would let it become a real radio driver at the
  physical layer, not just a source of telemetry.

None of this is free. Every item above also enlarges the set of ways the project can
cause interference, break a rule, or be misused, which section 5 treats as a
first-class cost, not a footnote.

## 2. Hardware reality

The HackRF is a superb wideband instrument and a mediocre transmitter, and both halves
of that sentence matter here.

- Half-duplex, by design. Great Scott Gadgets describes the HackRF One as a
  "half-duplex transceiver" ([HackRF One product page](https://greatscottgadgets.com/hackrf/one/)).
  It cannot transmit and receive at the same instant. Any protocol that assumes
  simultaneous listen-and-talk (full-duplex, or tight listen-before-talk timing) has
  to be reshaped around that constraint.

- 8-bit, wideband, modest dynamic range. The HackRF uses "8-bit quadrature samples
  (8-bit I and 8-bit Q)" at "up to 20 million samples per second" over "1 MHz to 6 GHz"
  ([HackRF One product page](https://greatscottgadgets.com/hackrf/one/)). Eight bits of
  resolution is generous bandwidth traded against dynamic range: it is excellent for
  broad survey work and comparatively coarse for clean, high-fidelity signal
  generation next to strong adjacent signals.

- Not a certified Part 15 unlicensed transmitter. The HackRF is "designed to enable
  test and development of modern and next generation radio technologies"
  ([HackRF One product page](https://greatscottgadgets.com/hackrf/one/)). It is test
  equipment. It carries no FCC equipment authorization as an unlicensed intentional
  radiator, so pointing it at an ISM band and transmitting does not inherit Part 15
  compliance (section 4 develops this). A purpose-built LoRa module is often sold as a
  certified or pre-certified module; a bare SDR is not.

- Clean transmit needs external filtering (and often amplification done carefully).
  The HackRF documentation itself advises that when adding an external amplifier you
  "should also use an external bandpass filter for your operating frequency"
  ([HackRF documentation](https://hackrf.readthedocs.io/en/latest/hackrf_one.html)),
  which is a direct acknowledgement that the raw output is not band-limited for you.
  A general-purpose direct-conversion transmitter can emit harmonics and spurious or
  out-of-band products; keeping emissions inside the intended band and within spurious
  limits is the operator's job, achieved with band-pass filtering and careful gain
  staging, not something the device guarantees.

- Modest and frequency-dependent output power. Typical maximum transmit power is not
  flat across the range; the vendor lists roughly 5 to 15 dBm across much of the HF
  through low-microwave range, dropping to around -10 to 0 dBm at the top of the band
  ([HackRF documentation](https://hackrf.readthedocs.io/en/latest/hackrf_one.html)).
  That is low power by design, which bounds range and means any serious link would want
  an amplifier, which loops back to the filtering requirement above.

Contrast with purpose-built LoRa transceivers. The Semtech SX126x/SX127x family
implements the LoRa PHY in dedicated silicon: defined modulation, integrated PA with
known spectral behavior, and modules that are frequently sold pre-certified for the
relevant regional band. For actually running a LoRa mesh, that hardware is cheaper,
cleaner, lower power, and closer to compliant out of the box than an SDR. The SDR's
advantage is not being a better LoRa radio; it is being able to be radios that no fixed
part implements. That is the whole reason transmit is interesting and also the reason
it is not urgent: the core spine already has good, cheap, compliant LoRa hardware.

## 3. Regulatory reality and authorization paths

This section names rules to make the shape of the constraint visible. It is
US-scoped, informal, and not legal advice. Verify every specific against the primary
source before acting, and read the [DISCLAIMER](../../DISCLAIMER.md).

Two regimes matter most for the bands NephMesh touches.

- Part 15 (unlicensed ISM). FCC Part 15.247 governs frequency-hopping and digitally
  modulated intentional radiators in the 902 to 928 MHz, 2400 to 2483.5 MHz, and
  5725 to 5850 MHz bands, with structured requirements: minimum hop-channel counts,
  dwell-time limits, a maximum conducted output power (commonly cited as 1 watt /
  30 dBm under the qualifying conditions), antenna-gain and EIRP provisions, and
  spurious-emission limits that require out-of-band radiated power well below the
  in-band level ([47 CFR 15.247, Cornell LII](https://www.law.cornell.edu/cfr/text/47/15.247)).
  The load-bearing point for this document: those allowances attach to authorized
  equipment operating within all of those conditions. A general-purpose SDR
  transmitting on an ISM band is not automatically Part 15 compliant just because it is
  on an ISM frequency. Compliance is a property of the whole emission (power, hopping,
  spurious content, certification status), not of the band alone, and an uncertified
  test transmitter with unfiltered output can fail the spurious and equipment-authorization
  requirements even while nominally "on" a license-free band.

- Part 97 (amateur). Under an amateur license, an operator may transmit on amateur
  allocations at higher power than Part 15 allows, which is one of the cleanest lawful
  paths to experiment with custom waveforms. The significant catch for a comms project
  that values confidentiality: amateur rules prohibit "messages encoded for the purpose
  of obscuring their meaning"
  ([47 CFR 97.113(a)(4), Cornell LII](https://www.law.cornell.edu/cfr/text/47/97.113)).
  In practice, operating under Part 97 typically forfeits the encrypted-channel model
  NephMesh treats as first-class elsewhere (the same tradeoff the
  [regulatory matrix](../reference/regulatory-matrix.md) records for running Meshtastic
  on amateur bands). Amateur experimentation is real and lawful; it is just
  unencrypted and license-gated.

- The regions differ. This is US framing. ETSI/EU rules (EN 300 220 on the 863 to
  870 MHz and 433 MHz bands) impose duty-cycle ceilings and different power limits, and
  other regions differ again. The per-region shape lives in the
  [regulatory matrix](../reference/regulatory-matrix.md), which is the document any
  transmit design must consult, because duty cycle in particular is a hard airtime
  invariant, not a soft preference.

Authorization paths that make experiments lawful. The honest summary is that lawful SDR
transmit is entirely possible; it just runs through an authorization, never around one.
The main legitimate paths, at the level of what makes them lawful rather than how to
execute them, are:

- An amateur license, operating unencrypted, on amateur frequencies, within the
  license's power and band conditions.
- Fully shielded or attenuated bench setups: a wired/conducted test into a load or a
  shielded enclosure, where the emission is contained rather than radiated over the
  air.
- Coordinated test ranges or experimental authorizations, where transmit happens under
  an explicit grant and coordination.
- In every case, proper band-pass filtering and gain staging so that spurious and
  out-of-band emissions stay within limits, because the hardware does not do this for
  you.

What this document deliberately does not do is explain how to configure any of the
above to emit over the air. The authorization is the prerequisite, and obtaining it is
the operator's responsibility.

## 4. Unlock conditions, and why receive-only stays the default

Receive-only is not only a legal default. It is a safety and trust posture, and it
should stay the default until specific conditions are met, not merely until someone
wants the feature.

The conditions that would need to be true before transmit moved from "considered" to
"experimentally allowed," roughly in order:

- A concrete lawful context exists for the specific experiment: an amateur license and
  amateur-band unencrypted operation, or a shielded/attenuated bench, or an
  experimental authorization or coordinated range. The context is named and documented,
  not assumed.
- A filtering and spurious-emission plan for the actual hardware, so the emission stays
  within limits rather than trusting the raw SDR output.
- The transmit path is explicit opt-in, off by default, and impossible to enable by
  accident, preserving the CI-enforced receive-only interlock as the safe resting
  state. The default posture the pipeline depends on does not change; a narrow, gated,
  clearly-marked path is what would open.
- The threat and safety surface is re-examined honestly. A device that can transmit can
  interfere, can be misused, and raises the stakes of any bug in the sense-and-act
  loop. That review is a precondition, not a follow-up.

Why default-off rather than default-on-with-warnings: the failure modes are asymmetric.
A receive-only device that has a bug produces bad data. A transmit-capable device that
has a bug, or a careless default, can emit unlawfully or harmfully into shared
spectrum, and unlike a bad log line that cannot be taken back off the air. Keeping
receive-only as the mechanical default means the project's worst realistic accident
stays "we mis-measured," which is the posture a lawful, experimental, trust-dependent
project should want.

## 5. Where SDR transmit would sit in the driver model, later

If and when it is unlocked, SDR transmit has a natural home in the existing design, and
naming that home now is useful precisely because it shows the feature does not require a
new architecture, only a new gate.

The [second-radio-driver survey](./second-radio-driver.md) defines the minimal driver
contract every radio reconciles behind: export live config, apply drift, report
telemetry, report identity, report reachability. A transmit-capable SDR could become a
radio driver behind that same seam: a software-defined LoRa node whose "config" is its
waveform, frequency plan, and power, reconciled by the same intent loop that reconciles
a Meshtastic node. The airtime-budget invariant and the duty-cycle ceilings from the
[regulatory matrix](../reference/regulatory-matrix.md) would apply to it exactly as
they apply to any other transmitting radio, which is a feature: the constitutional
constraints are already modeled.

What would have to be true first, beyond section 4's legal gate: the general driver
interface would need to be extracted and stable (the survey's prerequisite step), and
the SDR's PHY-layer behavior (half-duplex timing, filtering, power) would have to be
expressible as reconcilable config rather than ad-hoc operator action. Until both the
legal gate and the interface are in place, the SDR stays a sensor and exporter, which
is exactly where the roadmap keeps it.

## 6. Open questions

- Which single lawful context is the right first experiment: an amateur-licensed
  unencrypted waveform test, or a fully shielded bench setup that avoids the encryption
  question entirely? They unlock different subsets of the possibility space.
- Can a transmit-capable software-defined LoRa PHY be built and validated entirely in
  simulation and on a shielded bench first, honoring the $0/simulation-first rule, so
  that no over-the-air step is needed to prove the driver seam accepts an SDR?
- Does half-duplex operation break any assumption in the airtime-budget or
  listen-before-talk model that the current receive-only design has not had to confront?
- What is the minimal filtering and measurement setup that would let the project make an
  honest spurious-emission claim, rather than assuming the raw HackRF output is clean?
- Should a transmit-capable SDR be a distinct driver class from the Meshtastic-style
  managed radios (because its "config" is a waveform, not a firmware setting), or does
  the same contract genuinely stretch to cover it?
- How much of the cognitive-radio sense-and-act loop can be prototyped with sensing
  plus a simulated transmitter, deferring any real emission until an authorization
  exists?

## 7. Sources

- [NephMesh AGENTS.md](../../AGENTS.md)
- [NephMesh DISCLAIMER](../../DISCLAIMER.md)
- [NephMesh regulatory matrix](../reference/regulatory-matrix.md)
- [NephMesh second-radio-driver survey](./second-radio-driver.md)
- [HackRF One product page (Great Scott Gadgets)](https://greatscottgadgets.com/hackrf/one/)
- [HackRF One documentation (transmit power, input limits, filtering note)](https://hackrf.readthedocs.io/en/latest/hackrf_one.html)
- [gr-lora: LoRa receiver for GNU Radio (rpp0)](https://github.com/rpp0/gr-lora)
- [Meshtastic radio settings and LoRa presets](https://meshtastic.org/docs/overview/radio-settings/)
- [47 CFR 15.247, Cornell Law LII](https://www.law.cornell.edu/cfr/text/47/15.247)
- [47 CFR 97.113, Cornell Law LII](https://www.law.cornell.edu/cfr/text/47/97.113)
