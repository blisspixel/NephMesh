# Research: Containerized SDR, Kubernetes device access, spectrum sensing

Researched 2026-08-04. Bottom line: **containerized SDR is well-trodden at the Docker level, thin at the Kubernetes level, and no project combines declarative K8s CRDs + SDR device scheduling + spectrum sweeps - open ground.**

## HackRF (and RTL-SDR) in a container

- Host stack: `apt install hackrf` → libhackrf + `hackrf_info`, `hackrf_transfer`, `hackrf_sweep`. [Docs](https://hackrf.readthedocs.io/en/latest/installing_hackrf_software.html)
- udev rules live **on the host** ([53-hackrf.rules](https://github.com/greatscottgadgets/hackrf/blob/master/host/libhackrf/53-hackrf.rules)): HackRF One is USB `1d50:6089`; RTL-SDR typically `0bda:2838`.
- Docker patterns, most→least privileged: `--privileged -v /dev/bus/usb:/dev/bus/usb` (what most SDR images document); `--device /dev/bus/usb/BUS/DEV` (least privilege, but bus/dev renumbers on replug - pair with udev serial-keyed symlinks for stability).
- **RTL-SDR-specific:** the host kernel's `dvb_usb_rtl28xxu` driver claims the dongle - [blacklist it on the host](https://sdr-enthusiasts.gitbook.io/ads-b/setting-up-rtl-sdrs/blacklist-kernel-modules). HackRF has no competing kernel driver.
- Community images: [igorfreire/gnuradio-oot-dev](https://github.com/igorauad/docker-gnuradio-oot-dev) (GNU Radio dev/CI), [olijf/docker-gnuradio-soapy](https://github.com/olijf/docker-gnuradio-soapy), [hbaier/soapysdr](https://hub.docker.com/r/hbaier/soapysdr) (multi-arch SoapySDRServer). No "official" GNU Radio+HackRF image; a thin `debian + hackrf + soapysdr-module-hackrf` image is normal practice.

## SoapySDR abstraction (key architectural choice)

[SoapySDR](https://github.com/pothosware/SoapySDR) is the vendor-neutral SDR API with per-device plugin modules ([SoapyHackRF](https://github.com/pothosware/SoapyHackRF), [SoapyRTLSDR](https://github.com/pothosware/SoapyRTLSDR)). Writing the scanner against SoapySDR makes the container hardware-agnostic - RTL-SDR ↔ HackRF ↔ Airspy ↔ LimeSDR is just a `driver=` string. SoapyRemote/SoapySDRServer additionally lets the radio live on a small host while compute runs in the cluster over TCP. Related: [sdr-server](https://github.com/dernasherbrezon/sdr-server) slices one dongle's bandwidth for multiple simultaneous clients.

## Kubernetes USB device access

**Recommendation: [`squat/generic-device-plugin`](https://github.com/squat/generic-device-plugin)** - single-binary DaemonSet advertising devices by path or USB VID:PID (e.g. `{"usb":[{"vendor":"1d50","product":"6089"}]}` → `squat.ai/hackrf`); pods consume via `resources.limits`, **no privileged mode**, exclusive allocation by default, widely used in k3s/homelab (Zigbee dongles). Known issue [#65](https://github.com/squat/generic-device-plugin/issues/65) (device not mounted on some environments) - test on the target distro.

Alternatives:
- **[Akri](https://docs.akri.sh/discovery-handlers/udev)** (CNCF Sandbox, active but small contributor base) - udev discovery handlers create an `Instance` CR per device and can auto-deploy a "broker" pod wherever the device appears, following the hardware across nodes. More moving parts; the better fit if dynamic SDR discovery/scheduling becomes a *feature*. No published Akri+SDR tutorial exists - we'd be first.
- **smarter-device-manager** (Arm) - effectively unmaintained since ~2021; avoid for new work.
- **Watch item:** Kubernetes DRA (Dynamic Resource Allocation) is the long-term successor; no USB/SDR DRA driver exists yet.
- Escape hatch: run SoapySDRServer/`rtl_tcp` on the host (or tiny privileged DaemonSet), pods connect over TCP - sidesteps device plumbing entirely.

## Spectrum sensing software (headless, structured output)

| Tool | Hardware | Output | Notes |
|---|---|---|---|
| [`soapy_power`](https://github.com/xmikos/soapy_power) | any SoapySDR | rtl_power-format CSV | **best single choice** - auto frequency-hopping for wide ranges; upstream quiet-but-stable |
| `hackrf_sweep` | HackRF only | CSV to stdout | very fast wideband sweeps; part of hackrf-tools |
| `rtl_power` | RTL-SDR only | CSV | the original; same format |
| GNU Radio flowgraphs | any | custom | for real DSP (energy detection w/ hysteresis, channelization) |
| [`rtl_433`](https://github.com/merbanan/rtl_433) | RTL-SDR | **JSON → MQTT/InfluxDB natively** | not a scanner - an ISM telemetry decoder; mature containers ([hertzg/rtl_433](https://hub.docker.com/r/hertzg/rtl_433)) |

**Gap found:** mature Prometheus exporters exist for *decoded* signals ([rtl_433_prometheus](https://github.com/mhansen/rtl_433_prometheus)), but **no established exporter for raw sweep/PSD data**. A NephMesh exporter parsing rtl_power-format CSV into per-band aggregate gauges (occupancy %, max/mean dB - not per-bin series, cardinality!) would be genuinely novel glue. MQTT is the better transport for full spectra.

Visualization: [OpenWebRX](https://github.com/jketterl/openwebrx) / [OpenWebRX+](https://fms.komkon.org/OWRX/) (official Docker images, web waterfall - the "look at the site's spectrum from a browser" option); offline heatmaps from sweep CSVs ([heatmap.py](https://github.com/keenerd/rtl-sdr-misc), [sdr-heatmap](https://hub.docker.com/r/j2ghz/sdr-heatmap)); [IQEngine](https://github.com/IQEngine/IQEngine) for SigMF IQ recordings. gqrx/QSpectrumAnalyzer are GUI-only - not container-suitable.

## RTL-SDR vs HackRF (start cheap)

| | RTL-SDR (~$30–40) | HackRF (~$300+; Pro $400) |
|---|---|---|
| Range | ~24 MHz–1.766 GHz | 1 MHz–6 GHz |
| Bandwidth | ~2.4 MHz usable | 20 MHz |
| TX | **no (receive-only)** | yes (half-duplex) |
| RX quality | competitive below 1.7 GHz (both 8-bit) | not automatically better |

The RTL-SDR covers everything early phases need (433/868/**915 MHz ISM** - you can watch your own Meshtastic mesh transmit) and is the standard Electrosense sensor. HackRF buys 1.7–6 GHz and fast wideband sweeps - and TX for closed-loop tests. **HackRF One was discontinued Sept 2025; HackRF Pro is $400** (supply-constrained); used Ones circulate ~$200–250. RTL-SDR Blog V4 is end-of-line (successor "V4L" pending); V3/clones remain in the same price band. Multi-dongle sites need `rtl_eeprom`-assigned unique serials + udev symlinks.

## Legal note (US, brief, non-alarmist)

> Informal non-lawyer research, US-scoped, gathered at one point in time. Not legal advice, not a statement of what the law is, and silent on jurisdictions outside the US. Verify your own situation. See the repository [DISCLAIMER](../../DISCLAIMER.md).

- **Receiving appears license-free** (47 U.S.C. § 301 licenses *transmission*, per our reading). Spectrum sweeps, waterfalls, decoding unencrypted broadcasts: no license found to be required. Do not intercept cellular traffic or attempt to decrypt encrypted comms; some states also restrict *mobile* scanner use.
- **Transmitting needs authorization** outside license-free bands; the entry-level FCC Technician license is a 35-question exam, roughly $35, 10-year term.
- **Meshtastic TX appears license-free** at 902–928 MHz under FCC Part 15, within device power/duty limits. (Meshtastic's optional ham mode: higher power, encryption disabled, callsign ID.)
- Suggested README framing (self-hedged): *"NephMesh spectrum sensing is receive-only; in our US-scoped reading that needs no license, but you are responsible for your own jurisdiction. Transmit features are opt-in and your responsibility; see DISCLAIMER."*

## Prior art

- **[Electrosense](https://electrosense.org)** - crowdsourced RTL-SDR + Pi spectrum monitoring with open API; the architecture papers ([arXiv:1703.09989](https://arxiv.org/pdf/1703.09989)) are directly relevant reading. Centralized cloud backend, not intent-driven, not co-managed with comms infra.
- **[KrakenSDR](https://www.krakenrf.com/)** - 5-channel coherent RTL-SDR for direction finding; natural future integration for *locating* detected interference.
- **SDR + K8s specifically:** IBM Edge Application Manager ships an SDR reference example; REDHAWK did pre-K8s SDR cluster management; homelab rtl_433-on-k3s writeups. **Nothing combining declarative CRDs + device scheduling + sweeps.**
