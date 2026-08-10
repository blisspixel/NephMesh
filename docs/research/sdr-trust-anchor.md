# Research: the receive-only SDR as an out-of-band claim-vs-air trust anchor

Status: research and design direction, not shipped. Everything here is receive-only by policy (the SDR never transmits, per the transmit interlock in [`../security/threat-model.md`](../security/threat-model.md)), and is meant to be prototyped $0/simulation-first against recorded or synthetic IQ before any hardware.

## The gap this addresses

Meshtastic channels are AES-256-CTR with a shared channel key. With a non-default key that gives confidentiality, but the model has no per-sender authentication, no integrity MAC (CTR is malleable), and no replay protection. A consequence, stated plainly in the threat model and the landscape sweep, is that a node's on-mesh claims about itself (its node id, its reported position, its identity) can be forged by anyone holding the shared key, including a compromised insider. See [`../research/resilient-comms-landscape.md`](../research/resilient-comms-landscape.md) and section 8 of [`../security/threat-model.md`](../security/threat-model.md).

There are two complementary answers, and they are complementary on purpose:

- The in-band complement: an application-layer authentication and freshness envelope on any mesh packet that drives automation (a per-node signature plus a monotonic counter, reject unsigned/stale/replayed at ingest). This is the primary control and is tracked separately. It neutralizes impersonation and replay above the firmware, and it holds even on unpatched or vendor-cloned firmware.
- The out-of-band complement, and the subject of this doc: the receive-only SDR. Because the SDR is not a mesh member and holds no channel key, a compromised insider cannot lie to it. It observes what was actually transmitted over the air and reconciles that against what nodes claim on the mesh. This reframes the co-located SDR from a spectrum instrument into a control the mesh cannot spoof by holding the shared key.

The envelope authenticates the message; the SDR corroborates the physical event. Neither is sufficient alone. The envelope cannot see a transmitter that never sends a valid envelope (a foreign or jamming emitter), and the SDR cannot read encrypted payload semantics. Together they bound both "is this claim signed" and "did the claimed thing actually happen on the air."

An honest scope note up front: none of this defeats jamming, direction-finding of our own nodes by an adversary, or traffic analysis. Those are inherent to any unlicensed sub-GHz mesh and are accepted, not solved, here.

## Candidate detectors, cheapest and most feasible first

The ordering is deliberate. The first two are mostly logic over telemetry the mesh already produces, corroborated by coarse RF presence the current energy sweep can already give. The later ones need real DSP, calibration, or extra hardware, and their reliability on commodity receivers is genuinely contested in the literature. Do not lead with the exciting ones.

### 1. Duplicate-node-id detection (cheapest)

What it detects: two transmitters asserting the same Meshtastic node id, the simplest Sybil or clone signature. In a healthy mesh a node id maps to one physical radio.

How it works on receive-only commodity SDR: this is mostly logic over existing mesh telemetry (packets seen via the device API or an MQTT bridge), with the SDR adding an independent physical channel. Even the current energy sweep (occupancy percent per band via `hackrf_sweep`/`soapy_power`, see [`sdr-spectrum-sensing.md`](sdr-spectrum-sensing.md)) can corroborate simultaneity: if the same node id is claimed in two frames whose airtimes overlap, or fall within one time-on-air of each other, one is not that node. A receive-only LoRa decoder (below) sharpens this by attaching a decode timestamp and a measured RSSI/CFO to each on-air frame, so two "same id" frames with incompatible physical parameters are flagged.

Cost: near zero. This is table logic plus the existing airtime model in `internal/airtime`. It fits $0/simulation-first cleanly: generate a synthetic packet stream with an injected duplicate and assert the detector fires.

False-positive risks: legitimate role changes and repeater/relay behavior can make one id appear from different apparent directions; a naive "two directions therefore Sybil" rule is wrong. Keep the cheap version to strict simultaneity (overlapping airtime is physically impossible for one half-duplex radio) and treat direction as a later, weaker signal.

### 2. Impossible-mobility detection (cheap)

What it detects: a node whose claimed position moves faster than physically plausible, or teleports, which flags a spoofed position field or a second transmitter reusing an id from elsewhere.

How it works: pure logic over the position telemetry nodes already broadcast, bounded by a max-velocity gate (claimed displacement divided by time between reports must stay under a configured ceiling). No SDR is strictly required for the claim-vs-claim version. The SDR upgrades it to claim-vs-air: if the energy sweep or decoder shows the transmitter was on the air here at time t, a claim to be tens of kilometers away moments later is inconsistent. RSSI trend from a fixed receiver is a coarse but real distance proxy (a node claiming to move far while its received power at a static sensor stays flat is suspicious).

Cost: near zero, logic plus telemetry. Simulation-first is natural: replay a track with a teleport injected.

False-positive risks: GPS jitter and multipath make RSSI a noisy range estimate, so RSSI-based mobility checks must be advisory, not hard gates. Set the velocity ceiling generously (well above any real vehicle) so the detector only fires on the physically impossible, not the merely fast. This keeps precision high at the cost of missing subtle spoofs, which is the right tradeoff for a first detector feeding a human.

### 3. Receive-only LoRa decode alongside the energy sweep

Today the SDR does energy detection only (occupancy percent), it does not decode packets. Decoding LoRa on commodity SDR is real but frequently overstated, so here is the honest version.

What is genuinely possible: open-source receivers demodulate LoRa from raw IQ. The most complete is [`gr-lora_sdr`](https://github.com/tapparelj/gr-lora_sdr) (Tapparel et al., EPFL), a GNU Radio 3.10 out-of-tree module implementing full synchronization (sample-timing and carrier-frequency-offset estimation and correction), Gray mapping, deinterleaving, Hamming decode, CRC, spreading factors 5 to 12, coding rates, and soft-decision decoding, reported to work at low SNR. [SDRangel's ChirpChat](https://github.com/f4exb/sdrangel/blob/master/plugins/channelrx/demodchirpchat/readme.md) is an interactive LoRa/LoRa-adjacent demodulator usable with HackRF and RTL-SDR. The older [`gr-lora`](https://github.com/rpp0/gr-lora) (rpp0) is widely cited but is a reverse-engineering effort with known reliability gaps; prefer `gr-lora_sdr`.

Sample-rate and CPU reality: LoRa channel bandwidths are 125/250/500 kHz, and these receivers oversample the channel (commonly by a factor of several), so required capture rates are on the order of hundreds of kHz to a few MHz for a single channel. Both RTL-SDR (about 2.4 MHz usable) and HackRF (up to 20 MHz) clear this easily on bandwidth; the constraint is not the radio, it is CPU and configuration. A single known preset on a modern laptop-class CPU is tractable in real time. The honest limits:

- A decoder instance is configured for one spreading factor, bandwidth, and coding rate at a time. Meshtastic uses named presets (for example LongFast, which in the US region is a specific SF/BW), so you decode the preset you configured or you scan presets, which multiplies cost.
- Decoding recovers the Meshtastic frame, but the payload is still AES-CTR under the shared key. Decode does not by itself authenticate a sender. Its value is independent ground truth (a frame that was actually on the air, with a physical timestamp, RSSI, CFO, and SF), plus the ability to notice frames that the mesh's own telemetry never surfaces.
- Keeping up with all channels and all spreading factors simultaneously, in a container, alongside the sweep, is the expensive part. Watching one active preset is cheap; watching everything is not.

Which receiver: for this specific job an RTL-SDR is enough. It covers 915 MHz ISM and its roughly 2.4 MHz usable bandwidth comfortably captures a single 125 to 500 kHz LoRa channel at healthy oversampling, and it is receive-only by construction (no transmit path to gate). The HackRF buys wideband sweeps and the option to watch several adjacent channels at once, at higher CPU cost. That tradeoff table is in [`sdr-spectrum-sensing.md`](sdr-spectrum-sensing.md). The trust-anchor use does not need the HackRF; the sweep and the decode can share one cheap dongle for a single active preset.

Fit with $0/simulation-first: strong. `gr-lora_sdr` ships transmitter and receiver flowgraphs, so you can generate synthetic LoRa IQ, decode it, and build the claim-vs-air reconciler entirely without a radio or a transmit path. Recorded IQ (SigMF) is the natural test fixture, consistent with the IQEngine/SigMF tooling already noted in [`sdr-spectrum-sensing.md`](sdr-spectrum-sensing.md). The generated-IQ path also means the whole reconciler can live in CI with no hardware, which matches the project's rule that every feature works against simulation before any hardware variant.

### 4. RF fingerprinting (physical-layer device identification)

What it promises: identify an individual transmitter by hardware imperfections (carrier frequency offset from crystal-oscillator tolerance, transient turn-on shape, I/Q imbalance, so-called RF-DNA), so that two frames claiming one id but coming from two different physical radios can be told apart even when the payload is identical. [Carrier frequency offset](https://en.wikipedia.org/wiki/Carrier_frequency_offset) is attractive because oscillators are never identical and the offset is device-specific and directly measured by a LoRa sync stage. LoRa-specific physical-layer identification has been demonstrated in the literature ([radio fingerprinting for LoRa, WiSec 2017](https://dl.acm.org/doi/10.1145/3098243.3098267)).

Reliability caveats, and they are serious on cheap SDR:

- Published fingerprinting accuracy is usually measured in a fixed lab: same receiver, same day, high SNR, closed set of known devices. Robustness degrades with SNR, temperature (oscillator drift is temperature-dependent, so a device's own fingerprint moves), multipath, and receiver changes.
- The receiver's own imperfections contaminate the measurement. A commodity RTL-SDR/HackRF adds its own CFO and I/Q imbalance, so fingerprints are partly a property of your sensor, not just the target.
- It is fundamentally a probabilistic corroborator, not a hard authenticator. The landscape sweep already records this stance: RF fingerprinting on commodity SDR is a probabilistic corroborator, never a hard authenticator. Treat any fingerprint match or mismatch as weak evidence that raises or lowers confidence, never as proof of identity.

Fit with $0/simulation-first: partial. CFO estimation can be prototyped against synthetic and recorded IQ (measure the offset the decoder already estimates, cluster per claimed id). But a fingerprinting claim only becomes credible with a real multi-device, multi-condition capture campaign, which is hardware-and-time expensive and outside a $0 first pass. Keep this research-only for now.

### 5. TDOA, direction finding, and multilateration (most speculative here)

What it promises: physically locate a transmitter, so a claimed position can be checked against an actual bearing or fix, and impossible mobility becomes a hard geometric contradiction rather than a heuristic.

The commodity reference point is [KrakenSDR](https://www.krakenrf.com/), five coherent RTL-SDR receivers on one board for phase-coherent angle-of-arrival direction finding (roughly 100 MHz to 1 GHz, which covers 915 MHz ISM), receive-only, with open [DoA software](https://github.com/krakenrf/krakensdr_doa). A single station gives a bearing; a fix needs multiple stations (multi-station triangulation) or a moving station. Time-difference-of-arrival multilateration with independent sensors is the alternative geometry.

Cost and honesty:

- This is the one detector that needs specific extra hardware (a coherent multi-channel receiver, five matched antennas with real spacing, and per-run phase calibration), so it breaks the "one commodity dongle" assumption. A single non-coherent RTL-SDR or HackRF cannot do angle-of-arrival.
- Accuracy in the field is limited by multipath (sub-GHz in cluttered terrain is unkind to DF), antenna geometry, and calibration discipline. Bearings can be tens of degrees off in bad conditions.
- It remains receive-only, which fits policy, but the setup and calibration cost push it well down the list.

Fit with $0/simulation-first: weak in the near term. AoA/TDOA geometry and the multilateration math can be simulated, and that is a legitimate paper study, but a useful hardware result needs the coherent receiver and a calibrated antenna array. Research-only for now.

### Where Sybil detection sits across the above

Sybil attack detection is the framing that ties detectors 1, 4, and 5 together. The classic result is that Sybil identities cannot be bounded by protocol logic alone; you need either a trusted certifying authority or a distinct resource that a single physical device cannot cheaply multiply ([Douceur, The Sybil Attack, 2002](https://www.microsoft.com/en-us/research/publication/the-sybil-attack/); [Newsome et al., The Sybil Attack in Sensor Networks, IPSN 2004](https://dl.acm.org/doi/10.1145/984622.984660)). Radio-resource tests (received signal strength, and by extension timing, CFO, and bearing measured at an independent receiver) are a known family of physical Sybil detectors precisely because one radio occupies one place and one set of hardware imperfections at one time. The receive-only SDR is exactly such an independent physical channel: the cheap version (simultaneity and coarse presence) is the robust part; the fancy version (fingerprint or bearing per claimed id) is the corroborating part with the caveats above.

### Summary comparison

| Detector | What it detects | Cost on receive-only commodity SDR | Main false-positive risk | Sim-first fit |
|---|---|---|---|---|
| Duplicate node id (simultaneity) | Two overlapping transmissions claiming one id (clone/Sybil) | Near zero (logic plus existing airtime model plus the sweep) | Relay/repeater paths for one id; naive direction rules | Strong (inject a duplicate into a synthetic stream) |
| Impossible mobility (max velocity) | Spoofed or teleporting claimed position | Near zero (logic over position telemetry; RSSI advisory) | GPS jitter, multipath making RSSI a noisy range proxy | Strong (replay a track with a teleport) |
| Receive-only LoRa decode | Frames actually on air, with timestamp/RSSI/CFO/SF; frames the mesh telemetry never shows | Low for one known preset on a modern CPU; high to cover all presets/channels | Wrong preset means missed frames, not false alarms | Strong (`gr-lora_sdr` TX+RX flowgraphs, recorded SigMF IQ) |
| RF fingerprinting (CFO, RF-DNA) | Two physical radios reusing one id | Medium to high; needs a multi-device, multi-condition capture campaign to be credible | SNR, temperature drift, receiver contamination; lab numbers do not transfer | Partial (CFO clustering on recorded IQ; real claim needs hardware) |
| TDOA / AoA / multilateration | Actual bearing or fix vs claimed position | High; needs a coherent multi-channel receiver and calibrated antenna array | Multipath, geometry, calibration error (bearings off by tens of degrees) | Weak near term (geometry simulates; a useful result needs hardware) |

## What NephMesh should try first

Scoped to receive-only, commodity hardware, and $0/simulation-first:

1. Duplicate-node-id detection, simultaneity variant. Pure logic plus the existing airtime model, corroborated by the energy sweep. Prototype against synthetic packet streams. Highest confidence, lowest cost.
2. Impossible-mobility detection, max-velocity variant. Pure logic over position telemetry, with RSSI as an advisory (not gating) corroborator. Prototype against synthetic tracks.
3. Receive-only LoRa decode of a single active preset, using `gr-lora_sdr` against synthetic and recorded IQ, to turn the two detectors above from claim-vs-claim into claim-vs-air. This is the point where the SDR stops being only an energy meter. Still $0: no radio needed to build and test the reconciler.

Research-only for now (track, do not build yet):

- RF fingerprinting (CFO clustering can be explored on recorded IQ, but a credible detector needs a hardware capture campaign).
- TDOA/AoA/multilateration (needs a coherent receiver and calibrated array; simulate the geometry, defer the hardware).

The pattern is consistent with the project's posture: lead with deterministic logic over data you already have, add the cheapest independent physical signal next, and treat the probabilistic physical-layer methods as corroborators that raise or lower confidence rather than as authenticators.

## Hardware attempt log: over-the-air Meshtastic decode (2026-08, partial, parked)

This section records an actual hardware attempt honestly, as a partial and mostly negative result, so the finding is resumable and not overstated. It does not change the ordering above: full frame decode remains item 3, behind the two cheap logic detectors, exactly as this document already recommended.

What was built and is proven (CI-green, in the tree behind tests):

- A Meshtastic clear-text packet-header parser (`internal/meshframe`, 100 percent covered) and a `nephmeshctl decode` subcommand that reads payload hex into who-sent-what-to-whom. Verified against a known node id.
- A portable receive-only decode toolchain: `hack/install-lora-decode.sh` builds GNU Radio, the HackRF source, and `gr-lora_sdr` for whatever Python GNU Radio actually uses, and `hack/lora-decode.py` is a receive-only `gr-lora_sdr` RX flowgraph parameterized entirely by flags (so an RTL-SDR works as well as a HackRF). Three real portability bugs were found and fixed along the way (Python selection, pybind11 ABI pinning, scheduler buffering). The module imports and the flowgraph runs clean on the Jetson.
- The SDR already witnesses the mesh at the energy and occupancy layer on real hardware: the sensed peak tracked a commanded preset change and the airtime model agreed across three independent measurements. That is the honest, shipped form of "the SDR as an independent witness." It reads that a transmission happened, not its contents.

What did not land: over-the-air `gr-lora_sdr` frame lock on the live Meshtastic signal. After confirming the PHY parameters against firmware (SF11, BW 250 kHz, CR 4/5, explicit header, CRC on, preamble 16, sync word 0x2b, which `gr-lora_sdr` maps to net-id symbols [16, 88]) and confirming the flowgraph is structurally correct, the likely root cause is a carrier-frequency offset beyond the receiver's capture range. `frame_sync` corrects CFO only within roughly plus or minus BW/2 (about 125 kHz), but the signal was consistently measured at about 907.206 MHz while the decoder was tuned to the computed LONG_FAST default of 906.875 MHz, an offset of about 331 kHz. That is larger than the capture range and larger than plausible crystal error, so preamble energy is present but lock never occurs, which is consistent with every sync-word and preamble variant failing identically.

Resume recipe, if this is ever picked up (kept so the time already spent is not lost):

1. Wire a lock indicator first. Put a `blocks.tag_debug` (or a counting sink) on the `frame_sync` to `fft_demod` link and set `header_decoder(print_header=True)`, so "no lock" is distinguished from "no decode" instead of conflated.
2. Remove the frequency offset. Tune to the measured center (about 907.2 MHz) rather than the theoretical default, or tune slightly off and recenter digitally to also move the HackRF DC spike out of band. Sweep a small residual-CFO range if needed.
3. Simplify the rate path. Sample at 2 MHz and use `os_factor=8` (8 times 250 kHz) with no resampler, rather than decimating to 1 MHz with `os_factor=4`.
4. Bisect with a loopback and a cross-check oracle. Feed `gr-lora_sdr`'s own TX flowgraph output into this RX to prove the instantiation; and decode the same recorded IQ with SDRangel's ChirpChat demod (scriptable via its REST API) as an independent confirmation that the capture is decodable.

Why it is parked rather than finished: it is off every committed path (it maps only to item 3 here, which this document already places behind the cheap detectors), it does not move the project's load-bearing unproven claim (control-plane independence), and a live retry needs coordinated two-machine transmit-and-capture over a remote link, which is high-variance work for a corroborating witness. The energy-layer witness is the real, shipped result; decode-layer witness is an open research fork, honestly negative so far. No user-facing feature depends on the parser or toolchain, so they stay as research artifacts and are not claimed in the README.

## How a detection would surface

A detection is evidence, not an action. Concretely:

- It surfaces as an observation feeding the independent, Simplex-style safety kernel (the runtime component that can veto a proposed action against the signed constraints, roadmap items 6 and 13) and as a status condition on the relevant resource, so an operator and the reconciler can see "this node id was claimed from two overlapping transmissions" or "this claimed track is physically impossible."
- It never triggers an automated punitive transmit action. The SDR never transmits (transmit interlock), and the mesh does not auto-retaliate, auto-jam, or auto-deauthenticate a suspected Sybil. A confirmed detection at most lowers trust in that node's claims for automation purposes (for example, its unauthenticated position stops being allowed to drive a closed-loop decision) and raises a condition for human review through the normal Porch propose/approve lifecycle.
- This keeps the failure mode safe: a false positive costs a spurious status condition and a human glance, never an emission or an automated denial. Given the false-positive risks above (RSSI noise, multipath, fingerprint drift), that asymmetry is the whole point.

The design rule is the same one the closed-loop section of the threat model already commits to: the detector may inform a reviewed proposal, it may never key a radio or take an unattended punitive action.

## Roadmap mapping

This doc is the research base for the backlog item already recorded in [`../roadmap.md`](../roadmap.md): "the receive-only SDR as an out-of-band claim vs air trust anchor (duplicate-node-id, impossible-mobility, Sybil detection), plus a receive-only LoRa decoder alongside the energy sweep" (research-informed backlog, near-term, sim-first, item 4 in the ranked action list of [`resilient-comms-landscape.md`](resilient-comms-landscape.md)).

Dependencies and neighbors on the roadmap:

- It pairs with, and does not replace, the application-layer authentication and freshness envelope (the in-band complement, a separate near-term item). Build the envelope as the primary control; build this as the corroborating out-of-band channel.
- It builds on the existing sensing pipeline (`internal/airtime`, `internal/metrics`, the sweep tooling) rather than a new subsystem, and it stays inside the receive-only and transmit-interlock guarantees the security gates already enforce.
- The decoder is a natural input to the future MCP/observability surface (expose "frames actually seen on air" alongside occupancy), but that is downstream of the reliable core, not near-term.

## Open questions

- What time-synchronization precision does the claim-vs-air reconciler actually need between the mesh-telemetry timestamp and the SDR decode timestamp, and can a single co-located host provide it without extra hardware?
- Can the duplicate-id simultaneity check be made robust to legitimate relay/repeater behavior, where one id genuinely appears via multiple paths, without weakening it to uselessness?
- Is single-preset decode enough in practice, given a site may run more than one channel/preset, and what is the real CPU cost of scanning the presets that matter?
- How much of the RF-fingerprinting signal survives once the receiver is a cheap, drifting RTL-SDR/HackRF rather than a lab instrument, and is CFO-only fingerprinting worth anything as a weak corroborator?
- Is there a defensible minimal DF story (a single moving receiver, or two cheap stations) short of a full KrakenSDR array, or is direction finding simply out of scope for a commodity, $0 posture?
- What is the right confidence model for combining a strong deterministic signal (overlapping airtime) with weak probabilistic ones (RSSI, CFO) so the safety kernel gets calibrated evidence rather than a brittle boolean?

## Sources

LoRa decode on SDR: [gr-lora_sdr (Tapparel/EPFL)](https://github.com/tapparelj/gr-lora_sdr) · [SDRangel ChirpChat demodulator](https://github.com/f4exb/sdrangel/blob/master/plugins/channelrx/demodchirpchat/readme.md) · [gr-lora (rpp0, older, reliability gaps)](https://github.com/rpp0/gr-lora) · [soapy_power](https://github.com/xmikos/soapy_power) · [hackrf tools](https://hackrf.readthedocs.io/en/latest/installing_hackrf_software.html)

RF fingerprinting: [radio fingerprinting for LoRa (WiSec 2017)](https://dl.acm.org/doi/10.1145/3098243.3098267) · [carrier frequency offset](https://en.wikipedia.org/wiki/Carrier_frequency_offset)

Direction finding / TDOA: [KrakenSDR](https://www.krakenrf.com/) · [KrakenSDR DoA software](https://github.com/krakenrf/krakensdr_doa)

Sybil detection: [Douceur, The Sybil Attack (2002)](https://www.microsoft.com/en-us/research/publication/the-sybil-attack/) · [Newsome, Shi, Song, Perrig, The Sybil Attack in Sensor Networks (IPSN 2004)](https://dl.acm.org/doi/10.1145/984622.984660)

Meshtastic and project context: [Meshtastic encryption](https://meshtastic.org/docs/overview/encryption/) · [`sdr-spectrum-sensing.md`](sdr-spectrum-sensing.md) · [`resilient-comms-landscape.md`](resilient-comms-landscape.md) · [`../security/threat-model.md`](../security/threat-model.md) · [`../roadmap.md`](../roadmap.md)
