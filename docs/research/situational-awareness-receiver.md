# Research: A multi-band, receive-only situational-awareness receiver

Status: research direction, not shipped. This explores one containerized, receive-only
SDR receiver that watches more than the mesh band to build a "what is on the air around
me" picture. Everything here is receive-only by policy and largely $0 (it can be
prototyped against recorded IQ before any hardware is attached). The legality notes are
informal, non-lawyer, US-scoped research gathered at one point in time, not legal advice
and not a statement of what the law is. Read the repository
[DISCLAIMER](../../DISCLAIMER.md) and defer to the
[regulatory matrix](../reference/regulatory-matrix.md) and the primary sources cited
below. Nothing in this document advises decoding private, scrambled, or encrypted
communications.

## 1. Why this matters for the resilience mission

NephMesh treats software defined radio as a co-equal pillar, not a Meshtastic accessory
(see `AGENTS.md`). Spectrum sensing today watches the mesh band to protect the mesh. A
situational-awareness receiver is a different and separable use of the same hardware: it
does not care about the mesh at all. It asks what else is on the air, and in a disaster
that question has real answers that matter.

- Aircraft overhead (search-and-rescue, medevac, relief flights) show up on ADS-B.
- Vessels nearby (coast guard, ferries, relief shipping) show up on AIS.
- Weather-satellite imagery on the 137 MHz band gives a local picture when internet
  weather feeds are gone.
- Voice and data activity on amateur, GMRS, FRS, and CB bands is a proxy for who is
  transmitting and roughly where activity is concentrated.

None of this requires the mesh, and none of it requires transmitting. A receiver that
turns ambient RF into a structured local picture is valuable on its own, and it happens
to compose neatly with the mesh: the same fleet-managed sensing model that watches
915 MHz for interference can watch 1090 MHz for aircraft. The mission is resilient
comms; knowing the RF environment around you is part of being resilient, and it degrades
gracefully because listening never depends on any infrastructure staying up.

This is deliberately scoped as observation, never action. The receiver reports; it does
not transmit, jam, or respond. That keeps the regulatory surface small (see section 4)
and matches the project's receive-only-by-default posture.

## 2. The catalog of receive-only bands and services

Commodity SDR has a mature ecosystem of receive-only decoders, most of which run on a
sub-$40 RTL-SDR. Each yields a distinct slice of situational awareness.

| Service | Frequency | What it yields | Typical tool | Notes |
|---|---|---|---|---|
| ADS-B | 1090 MHz | Aircraft position, altitude, callsign, velocity | [dump1090](https://github.com/flightaware/dump1090) | Mode S / Mode 3A/3C decoder; supports RTL-SDR, HackRF, LimeSDR, SoapySDR |
| UAT (US) | 978 MHz | General-aviation aircraft, plus FIS-B weather/NOTAM uplink | [dump978](https://github.com/flightaware/dump978) | US-only lower tier of ADS-B; carries weather text and graphics |
| AIS | 161.975 and 162.025 MHz | Vessel position, identity (MMSI), course, speed | [rtl-ais](https://github.com/dgiardini/rtl-ais) | Dual-channel FM demod; emits NMEA AIVDM/AIVDO sentences |
| Weather satellite (APT) | 137 MHz band | Low-resolution local weather imagery | WXtoImg / [SatDump](https://github.com/SatDump/SatDump) | See the honesty note below on NOAA APT status |
| ISM telemetry | 315 / 433.92 / 868 / 915 MHz | Sensors, weather stations, TPMS, remotes | [rtl_433](https://github.com/merbanan/rtl_433) | Emits JSON, native MQTT/InfluxDB output |
| Amateur / GMRS / FRS / CB voice | HF/VHF/UHF (see below) | Analog voice activity, band occupancy | rtl_fm, gqrx, SDR++ | Receive-only monitoring of unscrambled voice |

Frequency detail for the voice services, receive-only:

- CB: roughly 26.965 to 27.405 MHz (27 MHz), 40 AM/SSB channels.
- FRS and GMRS: 462 and 467 MHz UHF channels (shared plan).
- Amateur: the popular monitoring segments are 2 m (144 to 148 MHz) and 70 cm (420 to
  450 MHz), plus HF below the RTL-SDR's lower limit unless upconverted or using a
  direct-sampling dongle.

Honesty note on NOAA APT: the classic analog APT satellites (NOAA 15, 18, 19, on
137.6200, 137.9125, and 137.1000 MHz) were decommissioned in 2025, per the
[rtl-sdr.com NOAA tutorial](https://www.rtl-sdr.com/rtl-sdr-tutorial-receiving-noaa-weather-satellite-images/).
The 137 MHz band is still worth watching: the Russian Meteor-M series transmits digital
LRPT imagery in the same band, and modern decoders such as
[SatDump](https://github.com/SatDump/SatDump) handle it. So "weather-satellite imagery
on a $30 dongle" remains real, but the specific NOAA APT signals most tutorials describe
are now historical, and any NephMesh work here should target current satellites rather
than assume NOAA APT is live.

Deliberately out of scope: pager (POCSAG) traffic and trunked or encrypted voice
systems. Even where a decoder exists, monitoring pager messages and similar
person-to-person traffic raises exactly the interception questions section 4 flags, so
this research does not pursue them.

The underlying ecosystems that make all of the above hardware-agnostic are
[SoapySDR](https://github.com/pothosware/SoapySDR) (the vendor-neutral device API
NephMesh standardizes on) and [GNU Radio](https://www.gnuradio.org/) (the DSP framework
that many of these decoders build on or can be rebuilt in).

## 3. Legality of reception (honest, non-lawyer, US-scoped)

Reception is broadly permissive in many jurisdictions but not universal, and the details
matter. This section is informal research, not legal advice; verify your own
jurisdiction and defer to the [regulatory matrix](../reference/regulatory-matrix.md) and
the DISCLAIMER.

- In the US, the act that requires a license is transmission, not reception
  (47 U.S.C. 301, per the reading in `docs/research/sdr-spectrum-sensing.md`). Listening
  to a spectrum, decoding an unencrypted broadcast, and building a waterfall are not
  transmission.
- The federal Wiretap Act / ECPA
  ([18 U.S.C. 2511](https://www.law.cornell.edu/uscode/text/18/2511)) prohibits
  intentionally intercepting electronic communications, but it carves out radio
  communications "readily accessible to the general public." Section 2511(2)(g)(ii)
  specifically lists communications relating to ships, aircraft, vehicles, or persons in
  distress, and amateur, citizens band, and general mobile radio services, and marine
  and aeronautical systems. ADS-B, AIS, marine and aeronautical voice, amateur, GMRS,
  FRS, and CB fall on the permissive side of that line.
- The same statute puts encrypted or scrambled communications outside those exceptions.
  That is the bright line this project respects: NephMesh does not decode, deobfuscate,
  or attempt to decrypt scrambled or encrypted transmissions. Some US states also
  restrict scanner use in a moving vehicle, a separate wrinkle from the federal rule.
- Reception rules vary by country. Some jurisdictions restrict monitoring of certain
  services (for example, some countries treat listening to anything other than
  broadcast and amateur as an offense). This document is US-scoped; the regulatory
  matrix is where per-region facts belong, and a situational-awareness receiver deployed
  elsewhere is the operator's responsibility.

The practical upshot: the catalog in section 2 is deliberately limited to services that
are broadcast-by-design and unencrypted, which is both the technically easy path and the
legally conservative one. If a signal is encrypted, NephMesh treats it as occupancy
(energy present in a band) and never as content.

## 4. Hardware and antenna reality

The uncomfortable fact is that one antenna is not good across 137 MHz to 1090 MHz. An
antenna is resonant near a design frequency; a whip cut for 137 MHz is a poor match at
1090 MHz and vice versa. So the "one receiver, many bands" idea runs into a real
antenna-and-frontend tradeoff.

- ADS-B at 1090 MHz benefits from a band-specific antenna and, in practice, an
  [ADS-B LNA and 1090 MHz SAW filter](https://www.rtl-sdr.com/product-tag/ads-b/) to
  reject strong out-of-band signals (cellular, FM broadcast) that would otherwise
  desensitize an 8-bit dongle. This is the single biggest quality lever for ADS-B.
- AIS at 162 MHz and weather satellites at 137 MHz want VHF antennas; APT and LRPT
  reception specifically wants a right-hand circularly polarized antenna (a QFH,
  turnstile, or V-dipole) for a satellite pass, which is a different physical antenna
  again.
- A single wideband discone or a compromise vertical will hear many of these badly. It
  is fine for a first look and useless for a good ADS-B or satellite result.

What a single HackRF buys versus a cheap dedicated RTL-SDR per band:

- One HackRF Pro (1 MHz to 6 GHz, 20 MHz bandwidth) can tune any of these bands, but
  only one at a time, and it still needs the right antenna and filter per band. It is
  the wrong tool for watching ADS-B and AIS simultaneously.
- Several RTL-SDR dongles (about $30 to $40 each, receive-only) are the standard answer:
  one per band, each with its own antenna and filter, each running its own decoder
  container, all watched at once. This is how ADS-B/AIS/rtl_433 sites are actually
  built. It is cheaper than one HackRF and genuinely parallel.
- The HackRF earns its place for wideband sweeps and for bands above 1.7 GHz that the
  RTL-SDR cannot reach, not for multi-band simultaneous decode. The
  RTL-SDR-versus-HackRF tradeoff table in
  [`sdr-spectrum-sensing.md`](sdr-spectrum-sensing.md) covers this in more detail.

For NephMesh the honest position is: the situational-awareness receiver is most
naturally a small fleet of cheap receivers, one per band, unified in software, rather
than one heroic radio. SoapySDR makes that fleet uniform in code; the antennas and
filters are the part software cannot abstract away.

## 5. Architecture

The design follows the existing NephMesh sensing model: containerized, SoapySDR-
abstracted, $0 and simulation-first, and receive-only with no transmit path anywhere in
the container.

- Hardware-agnostic capture. Each decoder runs in its own container against a SoapySDR
  driver string (`driver=rtlsdr`, `driver=hackrf`), so the same image runs on whatever
  receiver is attached. SoapyRemote lets the radio sit on a small host near the antenna
  while decode runs in the cluster, which fits the "control plane is not in the field"
  convention. Device access reuses the `squat/generic-device-plugin` pattern already
  chosen for sensing (see `sdr-spectrum-sensing.md`), so no privileged pods.
- One decoder per band, per the hardware reality in section 4: a dump1090 container for
  1090 MHz, an rtl-ais container for 162 MHz, an rtl_433 container for ISM, and so on.
  Each is a separate workload manifest (one workload per file, per the code-quality
  bar), scheduled onto the node where its dongle lives.
- Outputs surface as metrics and events, never as raw content and never as an action.
  Decoders emit structured data (dump1090 has a JSON and Beast feed, rtl-ais emits NMEA,
  rtl_433 emits JSON with native MQTT). NephMesh would translate these into low-
  cardinality Prometheus gauges and counters plus MQTT events, for example:
  - `aircraft_visible` (count), nearest-aircraft range, message rate.
  - `vessels_visible` (count), nearest-vessel range.
  - per-band `occupancy_ratio` and `max_dbfs` for the voice and ISM bands.
  - a satellite-pass event when a 137 MHz decode succeeds.
  The cardinality caution from `sdr-spectrum-sensing.md` applies: aggregate per band and
  per feed, do not emit a time series per aircraft or per MMSI. Full per-target detail,
  if ever wanted, belongs on an MQTT topic or a local store, not in Prometheus labels.
- Simulation-first and $0. None of this needs hardware to prototype. dump1090 and
  rtl-ais both accept recorded input, rtl_433 ships sample captures, and recorded IQ can
  be replayed through the decoders. Standardizing recordings as
  [SigMF](https://github.com/sigmf/SigMF) (a data file plus a JSON metadata file
  describing sample rate and center frequency) lets the same fixtures drive CI and lets
  [IQEngine](https://github.com/IQEngine/IQEngine) inspect them. So the exporter,
  the metric mapping, and the container plumbing can all be built and tested against
  fixtures before a dongle is plugged in, exactly as the $0-first convention requires.
- Fleet-managed. In the intent model this is just more declared sensing: a policy says
  "watch these bands at this site," and the reconciler schedules the right decoder
  containers onto nodes with the right receivers. The receiver never transmits, so there
  is no ChannelBudget or duty-cycle interaction; the regulatory surface is reception
  only, which the regulatory matrix already notes is much smaller.

## 6. What to try first

A minimal, honest first step that stays $0 and receive-only:

1. Pick one band with a clean decoder and public recordings: ADS-B (dump1090) is the
   best-documented and has abundant sample data.
2. Run dump1090 in a container against a recorded input (or a live RTL-SDR on a Linux
   host with a 1090 MHz antenna and filter), and confirm decoded aircraft.
3. Write a small exporter that turns the dump1090 JSON feed into a few low-cardinality
   Prometheus metrics (aircraft count, nearest range, message rate). This is the novel
   glue; the decoders already exist.
4. Capture a short IQ sample as SigMF so CI has a hardware-free fixture, and wire that
   fixture into a test that asserts the exporter produces the expected metrics.
5. Only then add a second band (AIS via rtl-ais, or ISM via rtl_433, whose native
   MQTT/JSON output and existing
   [rtl_433_prometheus](https://github.com/mhansen/rtl_433_prometheus) exporter make it
   a fast second win).

The goal of the first step is not coverage; it is proving the containerized, SoapySDR-
abstracted, metrics-out, receive-only pattern end to end against fixtures, so adding
bands later is mechanical.

## 7. How this maps to the roadmap

This document is the research backing for two existing roadmap entries.

- The In-consideration SDR frontier item "A multi-band situational-awareness receiver
  (receive-only)" in `docs/roadmap.md` links here directly. Its stated honest constraint
  (legality of reception varies by band and country; each band wants its own antenna)
  and its unlock (receive-only, $0, SoapySDR keeps it hardware-agnostic) are exactly
  what sections 3, 4, and 5 work through.
- The backlog item "Multi-technology expansion beyond LoRa: receive-only monitoring of
  CB (27 MHz), GMRS, and amateur bands" is a subset of this receiver. The roadmap is
  careful that this is monitoring only; managing additional radio services is a separate
  question that needs a digital control surface, and analog CB is never a transport.
  This research keeps that line: the situational-awareness receiver observes those bands
  and never acts on them.
- The related backlog note "SigMF IQ capture plus IQEngine for post-hoc analysis" is the
  simulation-first substrate this receiver would reuse (section 5).

It sits behind the spectrum-sensing phases in dependency order: it reuses the same
container, device-plugin, SoapySDR, and exporter plumbing those phases establish, and
adds decoders and a metric mapping on top. It is not on the near-term critical path; it
is a credible, low-risk extension once sensing is solid.

## 8. Open questions

- Metric schema: what is the right low-cardinality set of gauges and events for each
  band that is useful in a disaster without exploding Prometheus cardinality? Nearest-
  range and counts seem right; the exact set needs validation against real feeds.
- Where do full per-target feeds live (per-aircraft, per-vessel)? MQTT topic, local
  store, or not retained at all? This is a privacy and storage question as much as a
  technical one.
- Antenna and filter guidance: how much of the per-band antenna and filter reality can
  be captured as documentation and a recommended parts list, given the project cannot
  abstract antennas in software?
- Which weather satellites to target now that NOAA APT is decommissioned, and whether
  Meteor LRPT decode via SatDump is worth containerizing or is too pass-scheduling-heavy
  for a first version.
- Non-US reception legality: the regulatory matrix is transmit-focused today; a
  receive-side per-region note (which services are legal to monitor where) would need
  adding before recommending this receiver outside the US.
- How, if ever, situational-awareness events should reach mesh operators (a decoded
  "aircraft overhead" or "weather pass" event surfaced as a mesh message is a transmit
  action and out of scope for the receiver itself; the boundary needs to stay clean).

## 9. Sources

- [dump1090 (FlightAware)](https://github.com/flightaware/dump1090)
- [dump978 (FlightAware)](https://github.com/flightaware/dump978)
- [rtl-ais](https://github.com/dgiardini/rtl-ais)
- [rtl_433](https://github.com/merbanan/rtl_433)
- [rtl_433_prometheus exporter](https://github.com/mhansen/rtl_433_prometheus)
- [SatDump](https://github.com/SatDump/SatDump)
- [rtl-sdr.com NOAA weather satellite tutorial](https://www.rtl-sdr.com/rtl-sdr-tutorial-receiving-noaa-weather-satellite-images/)
- [rtl-sdr.com ADS-B LNA and filter products](https://www.rtl-sdr.com/product-tag/ads-b/)
- [SoapySDR](https://github.com/pothosware/SoapySDR)
- [GNU Radio](https://www.gnuradio.org/)
- [SigMF (Signal Metadata Format)](https://github.com/sigmf/SigMF)
- [IQEngine](https://github.com/IQEngine/IQEngine)
- [18 U.S.C. 2511 (Wiretap Act / ECPA)](https://www.law.cornell.edu/uscode/text/18/2511)
- [Electrosense](https://electrosense.org)
- Internal: [regulatory matrix](../reference/regulatory-matrix.md),
  [SDR spectrum sensing research](sdr-spectrum-sensing.md), repository
  [DISCLAIMER](../../DISCLAIMER.md)
