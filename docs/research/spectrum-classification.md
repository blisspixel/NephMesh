# Research: signal classification and interference ground-truth on receive-only commodity SDR

Status: research and design direction, not shipped. Everything here is receive-only by policy (the SDR never transmits, per the transmit interlock referenced in [`sdr-trust-anchor.md`](sdr-trust-anchor.md) and [`../security/threat-model.md`](../security/threat-model.md)). It is meant to be prototyped $0/simulation-first against recorded or synthetic IQ before any hardware is plugged in. Unlike SDR transmit, this frontier is largely unlocked already: it is receive-only, legal in our US-scoped reading of reception, and cheap. What is missing is the work, not a permission. This doc is the research base for the "signal classification and interference ground-truth" item under the SDR pillar in [`../roadmap.md`](../roadmap.md).

## The gap: "busy" needs a cause

Today the SDR does one thing: a coarse energy sweep that reports how busy a band is (occupancy percent per band via `hackrf_sweep` or `soapy_power`, see [`sdr-spectrum-sensing.md`](sdr-spectrum-sensing.md)). That is a real and useful signal, but it is causeless. "The 915 MHz band is 40 percent busy" does not say whether that 40 percent is my own Meshtastic mesh doing its job, a neighbor's unrelated LoRa network, a wireless doorbell or weather station chattering in the ISM band, or a jammer parked on the channel.

Those four causes call for opposite responses, and that is the whole point:

- My own mesh busy is the system working. The correct response is nothing, or at most an airtime-budget note.
- Another LoRa network or unrelated ISM traffic is legitimate congestion by a third party. The correct response is to coexist: back off, re-time, or (in a future closed loop) propose a channel or preset change, never to treat it as an attack.
- A jammer is hostile denial. The correct response is to raise an alarm and a status condition, preserve emergency airtime reserve, and inform a human, never to auto-retaliate (the SDR cannot transmit anyway).

The closed-loop spectrum policy (Phase 6) and the independent Simplex-style safety kernel (roadmap items 6 and 13) are, in the roadmap's own words, only as good as this ground truth. A loop that reacts to occupancy alone will treat its own healthy mesh, a polite neighbor, and a jammer identically, which is exactly the behavior a governed airtime commons must avoid. Moving from "how busy" to "busy with what, and is it mine" is the prerequisite for any of that automation being trustworthy. It is also strictly an evidence-gathering upgrade: nothing here ever keys a radio.

## Techniques, cheapest and most feasible first

The ordering is deliberate and mirrors [`sdr-trust-anchor.md`](sdr-trust-anchor.md): lead with cheap deterministic methods over data we can already get, and treat learned RF classification as a corroborator whose lab numbers are contested on cheap hardware, not as a solved oracle.

### 1. Energy detection plus time-frequency heuristics (cheapest, already partly built)

What it does: extend the existing sweep from a single occupancy scalar to simple, explainable features of the energy over time and frequency. Duty cycle (fraction of time above threshold), bandwidth of active emissions, burst duration and repetition, and whether energy is a narrow channel or smeared wideband. These features alone separate several cases without any decoding:

- A jammer that sits on a barrage or sweep pattern shows sustained high duty cycle and/or wide instantaneous bandwidth with no gaps, unlike bursty packet traffic. Legitimate congestion shows many discrete, short, gap-separated bursts (packets contending for airtime); a continuous carrier or a fast wideband sweep with near-100 percent duty is not what a polite CSMA-style network looks like.
- LoRa emissions have a distinctive chirp time-frequency signature (a linear frequency ramp across the channel bandwidth) that is visible in a spectrogram and separable from the flat-carrier or FSK/OOK signatures of many ISM devices and from the wideband FM blob of an analog video sender.

Honest limits: energy detection has a fundamental floor. Under noise-power uncertainty there is an SNR wall below which no amount of integration lets energy detection reliably distinguish signal from noise ([Tandra and Sahai, SNR Walls for Signal Detection, IEEE JSTSP 2008](https://ieeexplore.ieee.org/document/4453852)). Cheap 8-bit front ends with drifting gain make the noise floor itself uncertain, so thresholds must be adaptive and the "jammer vs congestion" heuristic is advisory, not proof. A high-duty wideband emission is suspicious, not confirmed hostile: a nearby legitimate wideband device can look similar. This tier is best treated as a fast, always-on triage that flags "something here is not packet-shaped" and hands off to the more specific tiers.

Cost: near zero. It is DSP over the sweep and spectrogram the pipeline can already produce, and it fits $0/simulation-first cleanly (synthesize chirps, FSK bursts, and a continuous jammer, assert the features separate them).

### 2. Receive-only LoRa detect and decode for attribution (low cost, high value)

What it does: detect and, where a preset matches, demodulate LoRa frames from raw IQ, so traffic can be attributed. This is the single highest-value tier because it directly answers "is this mine": a decoded Meshtastic frame carries a node id, a channel hash, and physical measurements (timestamp, RSSI, CFO, spreading factor) that tie an on-air event to a known network.

What is genuinely possible: open-source receivers demodulate LoRa from IQ. The most complete is [`gr-lora_sdr`](https://github.com/tapparelj/gr-lora_sdr) (Tapparel et al., EPFL), a GNU Radio out-of-tree module with full synchronization, Gray mapping, deinterleaving, Hamming decode, CRC, spreading factors 5 to 12, and soft-decision decoding, published as an open LoRa PHY prototype ([Tapparel et al., SPAWC 2020](https://infoscience.epfl.ch/record/278605)). [SDRangel's ChirpChat](https://github.com/f4exb/sdrangel/blob/master/plugins/channelrx/demodchirpchat/readme.md) is an interactive alternative usable with HackRF and RTL-SDR. The older [`gr-lora`](https://github.com/rpp0/gr-lora) is widely cited but is a reverse-engineering effort with known reliability gaps; prefer `gr-lora_sdr`.

Attribution is not content, and this is a policy-relevant distinction. Decoding recovers the LoRa PHY frame and the Meshtastic packet structure, but the payload stays AES-CTR encrypted under the shared channel key. Decode does not require the key and does not reveal message content. What it provides is metadata: a frame was really on the air, at this time, at this RSSI, with this SF, and (for Meshtastic) with this channel hash and sender field. That is exactly enough to attribute airtime to my mesh versus a foreign LoRa network (different channel hash, or a preset we do not run) without ever reading anyone's traffic. This is the same posture as the trust anchor: attribution and physical ground truth, not payload semantics.

Honest limits (the one-preset-at-a-time problem): a decoder instance is configured for one spreading factor, bandwidth, and coding rate at a time. Meshtastic uses named modem presets (LongFast, MediumSlow, and so on), each a specific SF and bandwidth for a region ([Meshtastic radio settings](https://meshtastic.org/docs/configuration/radio/lora/)). You decode the preset you configured, or you run several decoder instances to scan presets, which multiplies CPU. LoRa channel bandwidths are 125, 250, and 500 kHz and the receivers oversample by a few times, so a single channel needs capture rates of a few hundred kHz to a few MHz. Both RTL-SDR (about 2.4 MHz usable) and HackRF (up to 20 MHz) clear that on bandwidth; the constraint is CPU and configuration, not the radio. Watching one active preset on a modern laptop-class CPU is tractable in real time; watching every preset and every channel at once, in a container, alongside the sweep, is the expensive part. A pragmatic design uses tier 1 to detect that a chirp is present and estimate its bandwidth, then points a matched decoder only at the presets that are plausible.

Cost: low for one known preset, rising with the number of presets scanned. Sim-first fit is strong: `gr-lora_sdr` ships transmitter and receiver flowgraphs, so synthetic LoRa IQ can be generated, decoded, and turned into attribution evidence entirely without a radio, and recorded IQ ([SigMF](https://github.com/sigmf/SigMF)) is the natural CI fixture.

### 3. Non-LoRa ISM discrimination by time-frequency signature (low to medium)

What it does: separate LoRa chirps from the rest of the ISM zoo so that "not mine" can be sub-typed into "another LoRa network," "an unrelated ISM device," or "something anomalous." Many 433 and 915 MHz consumer devices (sensors, tire-pressure monitors, remotes, weather stations) use simple OOK or FSK, and the mature decoder [`rtl_433`](https://github.com/merbanan/rtl_433) already identifies hundreds of such device protocols and emits JSON. Running `rtl_433` alongside the sweep is a cheap way to positively identify benign ISM chatter rather than guessing. Frequency-hopping devices (some cordless and industrial links) and analog video senders (a wide FM blob) have their own spectrogram signatures distinct from both LoRa chirps and packetized FSK.

Honest limits: this is a growing catalog of known signatures, not a universal classifier. It positively identifies what it knows and leaves the rest as "unknown," which is honest but means the residual "unknown wideband high-duty" bucket, the one that overlaps with jamming, is where the hard calls live. `rtl_433` covers OOK/FSK telemetry, not LoRa or arbitrary waveforms, so it complements tier 2 rather than replacing it.

Cost: low (reuse `rtl_433` containers) to medium (custom spectrogram signature work). Sim-first fit is good for the signatures we synthesize and partial for real-world device variety, which only a capture campaign exercises.

### 4. Learned modulation recognition (most speculative; corroborator only)

What it promises: a neural network that takes raw IQ and names the modulation (or the emitter type), including signals no hand-written decoder covers. This is the RF machine-learning line of work, seeded by the [RadioML datasets](https://www.deepsig.ai/datasets/) and [O'Shea, Roy, and Clancy, Over-the-Air Deep Learning Based Radio Signal Classification (IEEE JSTSP 2018)](https://arxiv.org/abs/1712.04578), which report a ResNet classifying 24 modulation types with high accuracy at strong SNR.

Why to be skeptical, plainly:

- The headline accuracy is a high-SNR number. In the same body of work, accuracy degrades steeply as SNR falls and approaches chance (roughly 1 in N classes) at the low SNR where a struggling mesh actually needs the answer. A jammer or a distant foreign node is often exactly the low-SNR case where these models are weakest.
- The canonical datasets are known to be limited. DeepSig, who published them, now warns they are early academic work with errata and "HIGHLY recommend" researchers build their own datasets or use real over-the-air data instead ([RadioML datasets page](https://www.deepsig.ai/datasets/)). A model that scores well on RML2016/2018 has not been shown to work on your band, your hardware, or your interference.
- Domain shift is the real killer on commodity SDR. A model trained on one receiver, one day, one SNR regime tends to degrade on a different cheap 8-bit dongle with its own gain drift, DC offset, and I/Q imbalance. The receiver's imperfections become part of what the model learned, so the sensor contaminates the label. The over-the-air study itself measures carrier frequency offset, symbol-rate, and multipath as impairments precisely because they move the answer.
- Class set mismatch. Off-the-shelf models classify textbook modulations (BPSK, QAM, and so on), not "is this my LongFast mesh versus a neighbor's LoRa versus a jammer," which is NephMesh's actual question. Retargeting means building a NephMesh-specific labeled set, which is a real effort.

Where it could earn a place: as a weak corroborator that raises or lowers confidence on the "unknown" residual from tiers 1 to 3, trained and validated on our own recorded and synthetic IQ, never as the primary or sole basis for a decision. Treat any published accuracy figure as an upper bound measured in favorable conditions, and require our own held-out captures before trusting it at all.

Cost: medium to high, and the honest cost is data and validation, not GPU time. Sim-first fit is partial: training on synthetic IQ is easy and misleading; a credible claim needs real multi-condition captures, which is a hardware-and-time campaign outside a $0 first pass.

## Feasibility and cost on commodity hardware

| Technique | HackRF (wideband, 8-bit, up to 20 Msps) | RTL-SDR (~2.4 MHz usable, 8-bit) | Offline against recorded/synthetic IQ | Honest reliability |
|---|---|---|---|---|
| Energy + time-frequency heuristics | Strong: wide sweeps see many channels; good for wideband/jammer duty features | Fine for one band at a time; enough for the mesh channel | Fully doable $0 in CI (synthesize chirps, FSK, continuous jammer) | Bounded by the energy-detection SNR wall and noise-floor drift; advisory, not proof |
| LoRa detect/decode (attribution) | Can watch several adjacent channels at CPU cost | Sufficient for a single 125 to 500 kHz channel at healthy oversampling | Strong: `gr-lora_sdr` TX+RX flowgraphs, SigMF captures, no radio needed | Solid for a matched preset; misses unmatched presets (missed frames, not false alarms) |
| Non-LoRa ISM signatures (`rtl_433`, spectrogram) | Good; can cover more of the band at once | Good; the standard `rtl_433` sensor | Good for known device types; real-world variety needs captures | Positive ID of known protocols only; unknowns remain unknown |
| Learned modulation recognition | Same 8-bit front end; no accuracy magic from wider band | Same limits; low SNR is where it fails | Training is easy and misleading; validation needs real captures | Contested on cheap, drifting hardware and at low SNR; corroborator only |

Two facts anchor the cost story. First, neither dongle's bandwidth is the bottleneck for the useful tiers; CPU and per-preset configuration are. Second, and most important for a $0 project, the entire classification stack up to and including tier 2 can be built and regression-tested offline against recorded or synthetic IQ, with zero hardware and zero transmit path. That matches the project rule that every feature works against simulation before any hardware variant, and it means classification can live in CI (generate labeled IQ, run the classifier, assert the label and confidence) exactly as the trust-anchor reconciler is meant to.

## Classification as evidence, never as an action

A classification is evidence with a confidence and an uncertainty, not a trigger. The output contract, consistent with [`sdr-trust-anchor.md`](sdr-trust-anchor.md), is:

- Every classification carries a label, a calibrated confidence, and an explicit uncertainty or "unknown" option. "60 percent likely a jammer, 30 percent unknown wideband, 10 percent congestion" is a valid and useful output; a bare "JAMMER" boolean is not. The deterministic tiers (decoded my-mesh frame, positively identified `rtl_433` device) can carry high confidence; the learned tier carries low, corroborating weight by construction.
- It surfaces to three consumers. (1) The airtime commons and airtime-budget model gets attribution, so occupancy can be split into mine versus foreign-LoRa versus other-ISM versus unknown, which is what lets a budget hold an emergency reserve against real contention rather than against my own healthy traffic. (2) The independent safety kernel gets it as an observation it can weigh when vetoing a proposed action, never as an instruction. (3) Status conditions on the relevant resource expose it to operators and the reconciler in the existing `metav1.Condition` style already used for `Reachable`, `ConfigInSync`, and the rest (see [`../roadmap.md`](../roadmap.md)), for example an `InterferenceSuspected` or `ForeignTrafficObserved` condition with the cause and confidence in the message.
- It never keys a radio and never takes an unattended punitive action. The SDR cannot transmit (transmit interlock), and a suspected jammer does not auto-trigger retaliation, a channel change, or a deauthentication. At most a high-confidence, human-reviewed classification informs a proposal through the normal Porch propose/approve lifecycle. A false positive costs a spurious status condition and a human glance, never an emission or an automated denial, which is the correct asymmetry given the reliability caveats above.

## What NephMesh should try first

Scoped to receive-only, commodity hardware, and $0/simulation-first:

1. Time-frequency features on the existing sweep. Extend the current occupancy pipeline to emit duty cycle, active bandwidth, and burst structure, and a first heuristic that separates "packet-shaped bursts" from "continuous or wideband high-duty" (the coarse congestion-vs-jammer triage). Prototype against synthetic chirps, FSK bursts, and a synthetic jammer. Cheapest, and it upgrades a component that already exists.
2. Single-preset LoRa attribution with `gr-lora_sdr`. Decode one active Meshtastic preset from synthetic and recorded IQ, extract node id, channel hash, RSSI, SF, and timestamp, and split observed airtime into "mine" versus "foreign LoRa." This is the highest-value tier and shares its decoder with the trust anchor, so the two efforts reinforce each other. Still $0: no radio needed to build and test it.
3. Fold `rtl_433` in as a benign-ISM identifier so the "not mine" bucket can be positively sub-typed rather than guessed.

Research-only for now (track, do not build yet): learned modulation recognition. CFO and simple features can be explored on recorded IQ, but a credible NephMesh-specific classifier needs a labeled multi-condition capture campaign, which is out of a $0 first pass. Keep it as a corroborator target, not a near-term deliverable.

## Roadmap mapping

This doc backs the "signal classification and interference ground-truth" bullet under the SDR pillar in the In-consideration frontier of [`../roadmap.md`](../roadmap.md), which already flags it as receive-only, mostly unlocked, and prototypable against recorded or synthetic IQ. Its neighbors and dependencies:

- It is the ground truth the closed-loop spectrum policy (Phase 6) and the independent Simplex-style safety kernel (roadmap items 6 and 13) explicitly depend on. Neither should move onto the core spine trusting occupancy alone; attribution and cause are the prerequisite.
- It feeds the airtime-commons and airtime-budget work (the mission traffic classes and reserve accounting items): attribution is what lets the budget distinguish self-inflicted busy from third-party contention from hostile denial.
- It shares its LoRa decoder and its recorded-IQ test harness with the [receive-only SDR trust anchor](sdr-trust-anchor.md), and it builds on the existing sensing pipeline (`internal/airtime`, `internal/metrics`, the sweep tooling) rather than a new subsystem. It sits beside, not on, the core spine until the core has earned the complexity.
- It is distinct from the multi-band situational-awareness receiver (a different In-consideration item about watching bands beyond the mesh) and from SDR transmit (gated by law and hardware). This item stays firmly inside receive-only and $0.

## Open questions

- What confidence model correctly fuses a strong deterministic signal (a decoded my-mesh frame) with weak probabilistic ones (a learned label, an energy heuristic) so the safety kernel and airtime budget get calibrated evidence rather than a brittle boolean?
- Where is the honest congestion-versus-jamming decision boundary on cheap 8-bit hardware with a drifting noise floor, and can it be made robust enough to raise a condition without crying wolf at a busy but benign band?
- Is single-preset decode plus tier-1 triage enough attribution in practice, or does a real site running multiple presets force a multi-decoder cost that changes the feasibility story?
- How much of any learned classifier's accuracy survives the move from synthetic or lab IQ to a specific cheap RTL-SDR/HackRF on our band, and is a NephMesh-specific labeled capture set worth building before this tier is trusted at all?
- What is the minimum viable labeled IQ corpus (synthetic plus recorded SigMF) to regression-test classification in CI, and how is it kept honest so a model that passes CI is not just overfit to synthetic artifacts?
- Can attribution stay strictly metadata-only (channel hash and physical parameters, never payload) in a way that is easy to audit, so the "attribution not content" guarantee is enforced, not merely intended?

## Sources

RF machine learning and its limits: [DeepSig RadioML datasets (with the authors' own caveats)](https://www.deepsig.ai/datasets/) · [O'Shea, Roy, Clancy, Over-the-Air Deep Learning Based Radio Signal Classification (IEEE JSTSP 2018)](https://arxiv.org/abs/1712.04578) · [O'Shea, West, Radio Machine Learning Dataset Generation with GNU Radio (GRCon 2016)](https://pubs.gnuradio.org/index.php/grcon/article/view/11)

Detection theory limits: [Tandra, Sahai, SNR Walls for Signal Detection (IEEE JSTSP 2008)](https://ieeexplore.ieee.org/document/4453852)

LoRa decode and attribution: [gr-lora_sdr (Tapparel/EPFL)](https://github.com/tapparelj/gr-lora_sdr) · [Tapparel et al., An Open-Source LoRa PHY Prototype on GNU Radio (SPAWC 2020)](https://infoscience.epfl.ch/record/278605) · [SDRangel ChirpChat demodulator](https://github.com/f4exb/sdrangel/blob/master/plugins/channelrx/demodchirpchat/readme.md) · [gr-lora (rpp0, older, reliability gaps)](https://github.com/rpp0/gr-lora) · [Meshtastic radio settings and modem presets](https://meshtastic.org/docs/configuration/radio/lora/)

ISM decoding and recorded IQ: [rtl_433 (ISM device decoder)](https://github.com/merbanan/rtl_433) · [SigMF (Signal Metadata Format)](https://github.com/sigmf/SigMF) · [soapy_power](https://github.com/xmikos/soapy_power) · [hackrf tools](https://hackrf.readthedocs.io/en/latest/installing_hackrf_software.html)

Project context: [`sdr-spectrum-sensing.md`](sdr-spectrum-sensing.md) · [`sdr-trust-anchor.md`](sdr-trust-anchor.md) · [`resilient-comms-landscape.md`](resilient-comms-landscape.md) · [`../roadmap.md`](../roadmap.md) · [`../security/threat-model.md`](../security/threat-model.md)
