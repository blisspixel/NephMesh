# Phase 2 implementation plan: real radios and spectrum sensing

Target: the 0.2 release gate. Intent drives physical Meshtastic boards, and the mesh is visible in sensed spectrum from a receive-only HackRF Pro. Assumes the Phase 1 pipeline (meshtasticd -s, YAML config Job, Mosquitto) works.

Lab hardware in scope: two or more ESP32-class Meshtastic boards, one HackRF Pro, a Raspberry Pi, an Orange Pi 5 (single-node k3s edge cluster), the dev PC. Everything here is provider-neutral and receive-only. No transmit paths of any kind.

## 1. Deliverables mapped to roadmap checkboxes

| Roadmap item (Phase 2) | Deliverable |
|---|---|
| Attach owned boards, extend config pipeline to physical nodes | Board bootstrap doc plus the Phase 1 config-applier Job retargeted at physical nodes over TCP (section 2) |
| USB device access via generic-device-plugin, documented host prep | DaemonSet manifest, udev rules doc, validation checklist, fallback chain doc (section 3) |
| Spectrum sensor container, receive-only, 902 to 928 MHz | Multi-arch sensor image plus sweep configuration (section 4) |
| Exporter: sweep CSV to per-band aggregate gauges | Go exporter module, band config format, container image (section 5) |
| Demo: intent change propagates, message crosses RF, mesh visible in metrics | Scripted demo with verification steps (section 7), gated by section 6 metrics stack |

## 2. Physical-node config pipeline

Phase 1 establishes desired state as the Meshtastic CLI YAML format (`--export-config` / `--configure`) applied by a Job over TCP 4403. Phase 2 keeps that format and that applier unchanged; only the target address changes.

Two transports exist for physical boards:

- **USB serial:** `meshtastic --port /dev/ttyUSB0 ...`. Requires the board to be plugged into a cluster node and the serial device passed into the pod.
- **WiFi/TCP:** ESP32 boards join WiFi and expose the standard client API on TCP 4403. `meshtastic --host <board-ip> ...`, identical to how Phase 1 talks to meshtasticd -s.

**Decision: prefer WiFi/TCP as the managed path; serial is bootstrap-only.** Rationale:

- It is literally the Phase 1 code path. The config-applier Job needs zero changes beyond the host address, and it is the same interface the Phase 4 operator will use, so nothing built here is throwaway.
- No USB plumbing for the mesh path. Device-plugin risk (the main Phase 2 unknown) stays isolated to the SDR.
- Boards can be physically placed for RF geometry (antenna separation, different rooms) instead of tethered to the Pi. Two boards touching the cluster node is a poor mesh demo.
- Caveat to verify: the Meshtastic TCP API serves one client at a time, and enabling WiFi disables Bluetooth on ESP32. Both are acceptable here; flagged as assumptions to confirm on real boards.

Serial remains necessary exactly once per board, because a factory-fresh board has no WiFi credentials (chicken and egg). Bootstrap is a documented host-side step on the dev PC or Pi, not cluster-managed:

```
meshtastic --port /dev/ttyUSB0 \
  --set network.wifi_enabled true \
  --set network.wifi_ssid "<lab-ssid>" --set network.wifi_psk "<psk>" \
  --set lora.region US
```

After bootstrap, each board's IP (static DHCP lease recommended) goes into the per-node config as data. The config-applier runs as a Job on the k3s cluster, as in Phase 1: no USB device access needed, and applies remain observable via kubectl. A host-side applier is rejected because it breaks the "everything through the cluster" story that Phase 3 packaging depends on. Serial-attached-to-cluster (Job with a serial device resource) is the documented fallback if a board's WiFi proves unreliable, reusing the device-plugin machinery from section 3.

Minimal-diff discipline applies even in this pre-operator phase: the Job should export, diff, and apply only changed sections, since each applied section reboots the board.

## 3. USB device access

Target: HackRF Pro attached to the Orange Pi 5 (the k3s node). Advertised to pods by `squat/generic-device-plugin`, no privileged pods.

**Product ID caveat:** HackRF One is USB `1d50:6089`. The HackRF Pro is new hardware and its product ID is not verified; it may share 6089 or use a new ID. First task on hardware arrival: `lsusb` on the Pi with the Pro attached and record the `1d50:xxxx` pair. The manifest below uses 6089 as a placeholder and must be corrected from the lsusb output. Serial adapters vary too (CH341 sticks are `1a86:7523`; ESP32 boards commonly carry CP210x `10c4:ea60`); identify each with lsusb before writing rules.

DaemonSet sketch (final YAML lands with the manifests):

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: generic-device-plugin
  namespace: kube-system
spec:
  template:
    spec:
      containers:
        - name: generic-device-plugin
          image: ghcr.io/squat/generic-device-plugin
          args:
            - --domain=nephmesh.io
            # verify Pro product ID with lsusb; 6089 is the One's, placeholder here
            - --device={"name":"hackrf","groups":[{"usb":[{"vendor":"1d50","product":"6089"}]}]}
            - --device={"name":"serial","groups":[{"paths":[{"path":"/dev/serial/by-id/*"}]}]}
          volumeMounts:
            - name: dev
              mountPath: /dev
            - name: device-plugin
              mountPath: /var/lib/kubelet/device-plugins
      volumes:
        - name: dev
          hostPath: {path: /dev}
        - name: device-plugin
          hostPath: {path: /var/lib/kubelet/device-plugins}
```

Pods then request `nephmesh.io/hackrf: 1` in `resources.limits`. The `serial` group (by-id paths are stable across replug, unlike ttyUSBn) covers the serial-applier fallback and future CH341 radios.

Host prep, documented in the same doc:

- Install the upstream `53-hackrf.rules` udev rules (mode 0660, plugdev or a dedicated group) so the device node is accessible without root.
- No kernel module conflicts exist for HackRF (unlike RTL-SDR's `dvb_usb_rtl28xxu` blacklist; note it anyway for future RTL-SDR sites).
- Validation checklist: `lsusb` shows the device; `hackrf_info` works on the host; the node reports the `nephmesh.io/hackrf` allocatable resource; a busybox pod requesting it sees the device node.

**Fallback chain if the plugin fails on arm64 k3s** (open upstream issue about devices not mounting in some environments):

1. **Akri** udev discovery handler: heavier, but purpose-built for this and follows the device across nodes.
2. **Host-level SoapySDRServer:** run SoapySDR's server on the Orange Pi host (systemd unit), sensor pods connect over TCP with `driver=remote`. Sidesteps device plumbing entirely; works because the sensor is written against SoapySDR (section 4).
3. **Privileged pod** with `/dev/bus/usb` hostPath: last resort, explicitly labeled as a deviation to be removed.

Whichever lands is recorded in the doc with the reason, so Phase 3 packages encode reality.

## 4. Spectrum sensor container

Base image: `debian:stable-slim` plus `hackrf` (libhackrf and tools including `hackrf_sweep` and `hackrf_info`), `soapysdr-tools`, `soapysdr-module-hackrf`, and `soapy_power` (pip). Multi-arch build (arm64 for the Pis, amd64 for the PC). This mirrors normal practice; no suitable official image exists.

**Sweep tool decision: `soapy_power` is the primary scanner; `hackrf_sweep` ships in the image as a diagnostic and fallback.** Rationale: the project convention is hardware-agnostic SDR code where the SoapySDR driver string is configuration, so the same container serves a future RTL-SDR site by changing `-d driver=rtlsdr`. `hackrf_sweep` is faster at wideband sweeps but HackRF-only, and 26 MHz is not wide enough for that to matter. Both emit rtl_power-format CSV, so the exporter is agnostic.

Sweep configuration for the North American ISM band (exact flags validated on hardware; treat as the starting point):

```
soapy_power -d driver=hackrf -f 902M:928M -B 100k -T 10 -c \
  -O /data/sweep.csv --even --output-format rtl_power
```

That is: 902 to 928 MHz, 100 kHz bins (260 bins, comfortably resolving 250 kHz LoRa channels), a sweep roughly every 10 seconds, continuous, appending rtl_power-format CSV. soapy_power hops automatically since 26 MHz exceeds the HackRF's 20 MHz instantaneous bandwidth. Gain settings start moderate (for example LNA 16, VGA 20 equivalents via `-g`) and are tuned against the observed noise floor. Fallback invocation: `hackrf_sweep -f 902:928 -w 100000` piped to the same CSV path.

Output lands on a shared `emptyDir` volume read by the exporter sidecar in the same pod (simplest contract; a pipe is an alternative if file tailing is awkward).

**HackRF Pro compatibility validation (do this first, on the host, before any container work):** the Pro is newer than most published tooling. Steps: run `hackrf_info` from the distro's hackrf package; if the Pro is not recognized, build hackrf tools from a release that lists Pro support (expected in 2025-era releases; verify release notes, this is an unverified assumption) and rebuild SoapyHackRF against that libhackrf. Record working versions and pin them in the Dockerfile. If Debian stable's packages are too old for the Pro, the Dockerfile builds both from pinned source tags.

## 5. Exporter: sweep CSV to Prometheus gauges

No existing exporter parses rtl_power-format sweep data; this is the phase's novel glue. Language: **Go**, per the upstream-compatibility conventions (Go 1.25.x, testify golden-style tests, Apache-2.0 headers, DCO), so it can later sit beside the operator code without rework.

Input: rtl_power-format CSV lines (`date, time, hz_low, hz_high, hz_step, samples, dB, dB, ...`), tailed from the shared volume. Each completed sweep is folded into per-band aggregates.

Band definitions are configuration (a mounted YAML file, later the `SpectrumScan` CR):

```yaml
noise_floor_db: -75        # static for Phase 2; auto-estimation is an open decision
bands:
  - name: meshtastic-us-longfast
    low_hz: 906000000
    high_hz: 907000000
  - name: ism-902-928-full
    low_hz: 902000000
    high_hz: 928000000
```

Metrics, all labeled only by `band` (and instance labels Prometheus adds):

- `nephmesh_band_occupancy_percent`: percent of bins in the band above the noise floor in the latest sweep
- `nephmesh_band_power_max_db`, `nephmesh_band_power_mean_db`
- `nephmesh_sweep_timestamp_seconds`, `nephmesh_sweeps_total`: freshness and liveness

**Why not per-bin metrics:** 260 bins at 100 kHz times three statistics is near a thousand series per sensor, growing linearly with resolution and sites, all to answer questions Prometheus is bad at (spectral shape). Per-band aggregates keep cardinality at a handful of series and match what the Phase 6 policy loop actually consumes (occupancy on the active channel). Full spectra, if ever needed, go to MQTT or SigMF captures, not Prometheus.

Package layout (own Go module, mirroring the multi-module convention):

```
exporters/sweep-exporter/
  main.go                      thin entrypoint
  internal/parser/             rtl_power CSV line parsing (golden tests on captured fixtures)
  internal/bands/              band config load and validation
  internal/collector/          aggregate computation, prometheus.Collector impl
  testdata/                    captured real sweep CSVs as fixtures
```

Captured CSV fixtures from the real HackRF become the CI test corpus, keeping the $0 rule: CI never needs hardware.

## 6. Metrics stack for this phase

**Decision: deploy a minimal single-pod Prometheus; defer Grafana.** A bare `/metrics` endpoint scraped manually was considered and rejected: the demo's core claim is "mesh transmissions appear in occupancy metrics", which is a time-series claim. A curl shows one instant; the gate needs occupancy visibly rising during a message burst and falling after, which requires retention and a graph.

Minimal means: the vanilla `prom/prometheus` image, one replica, a static scrape config listing the exporter service, short retention (a few hours), emptyDir storage, no kube-prometheus stack, no Alertmanager, no ServiceMonitors or operator CRDs. Prometheus's built-in graph UI (port-forwarded) is sufficient for the demo. Grafana adds nothing at one metric family and is deferred to whenever dashboards earn their keep (Phase 5 or 6). Phase 6 needs Prometheus anyway, so this is not throwaway.

## 7. Demo script for the 0.2 gate

Preconditions: two bootstrapped boards on WiFi with known IPs, HackRF Pro attached to the Orange Pi 5, device access validated, sensor pod and Prometheus running, Phase 1 broker running.

1. **Intent propagates.** Commit a config change to the desired-state YAML for both boards: modem preset LongFast to MediumSlow. Apply (kubectl apply or GitOps sync) and watch the applier Job logs. Verify: `meshtastic --host <board-ip> --export-config` on each board shows the new preset; the boards rebooted only for the changed section.
2. **Message crosses real RF.** `meshtastic --host <board-A-ip> --sendtext "phase2-gate" --ack`. Verify: acknowledgment received; the message appears via board B (`--host <board-B-ip>` listen, or the MQTT topic if board B uplinks to the private broker). Confirm the path is RF, not IP: the boards share no mesh transport except LoRa.
3. **Mesh visible in spectrum.** Port-forward Prometheus. Establish a quiet baseline of `nephmesh_band_occupancy_percent{band="meshtastic-us-longfast"}` for a few minutes. Generate traffic (a scripted loop of sendtext, or range-test module). Verify: occupancy and max dB on the active-channel band rise during the burst and return to baseline after, while an unrelated control band stays flat. Screenshot the graph for the release notes.
4. **Teardown.** Delete the sensor and applier manifests; verify the device resource frees and metrics stop, boards keep their last applied config (expected: boards are not torn down, they are external state, which is exactly what Phase 4 exists to reconcile).

Pass criteria: all four steps succeed from repo manifests plus the documented bootstrap, with no manual container fiddling. Tag 0.2.

## 8. Risks, fallbacks, non-goals, open decisions

Risks and fallbacks:

- **generic-device-plugin fails on arm64 k3s** (known open issue): fallback chain in section 3 (Akri, host SoapySDRServer, privileged pod), in that order.
- **HackRF Pro unsupported by packaged tools** (newest hardware): build hackrf tools and SoapyHackRF from pinned source; validated first, on the host, before anything depends on it (section 4).
- **soapy_power unmaintained or misbehaving with the Pro:** upstream is quiet but stable; fallback is `hackrf_sweep` in the same container with the same CSV contract (accepting temporary loss of hardware-agnosticism).
- **Board WiFi flaky or TCP API contention:** fallback to the serial applier Job via the `serial` device group; worst case, host-side serial apply, documented as a deviation.
- **Mesh transmissions too brief for 10 second sweeps to catch reliably:** shorten sweep interval, narrow the swept range to the active channel, or drive sustained traffic with the range-test module during the demo window.

Explicit non-goals for Phase 2:

- No transmit of any kind, including for testing. The HackRF is receive-only here and always in this phase.
- No closed loop: metrics are observed by humans, nothing acts on them (Phase 6).
- No operator or CRDs (Phase 4), no kpt packaging or Porch (Phase 3): plain manifests and one-shot Jobs are correct for this phase.
- No Grafana, no per-bin metrics, no MQTT spectra, no IQ capture.
- No meshtasticd-with-real-RF on the Pis (no LoRa HAT owned); physical RF is the ESP32 boards only.

Open decisions needing a human call:

- HackRF Pro USB product ID: read from lsusb on arrival; correct the DaemonSet manifest.
- Sensor host: Orange Pi 5 (assumed, it is the k3s node) versus the Raspberry Pi as a second host; affects nothing until Phase 5.
- Noise floor: fixed configured dB versus percentile auto-estimation per sweep. Plan says fixed for Phase 2; revisit if the lab RF environment drifts.
- Whether board desired-state YAML (containing WiFi PSK and channel PSKs) needs Secret-backed handling already, or waits for Phase 4's Secrets design. Leaning: keep PSKs out of Git now via a locally mounted overlay, decide the real mechanism in Phase 4.
