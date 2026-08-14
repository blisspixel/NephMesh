# Research: Meshtastic automation & containerization surface

Researched 2026-08-04. Bottom line: **everything needed for declarative fleet management exists - official OCI images, a TCP device API, YAML config export/import, a radio-free simulation mode, documented MQTT topics - but the Kubernetes packaging (chart/operator, declarative reconciliation, device passthrough policy) is unbuilt. Greenfield.**

## meshtasticd (Linux-native daemon)

The official firmware compiled for Linux via Portduino; intended for always-on base stations and gateways. [Docs](https://meshtastic.org/docs/hardware/devices/linux-native-hardware/)

- **Hardware:** tested SPI LoRa HATs (MeshAdv-Pi, Adafruit RFM9x, Elecrow RFM95, RAK6421) and CH341-based USB radios (MeshStick, Pinedio, MeshToad V3) - "any Linux device with a USB port." CH341 USB-SPI is `1a86:5512` (no tty); CH341 UART sticks are `1a86:7523`. A MeshToad V3 was brought up 2026-08-13 (`docs/plans/meshtoad-gateway-bench.md`). Not supported: UART HATs, SX1302/03 LoRaWAN concentrators, the Waveshare SX1262 **LoRaWAN** HAT. [Supported hardware](https://meshtastic.org/docs/meshtasticd/hardware/)
- **Config:** `/etc/meshtasticd/config.yaml` covers *host/radio* config (driver, pins, webserver); pinout fragments ship in `available.d/`. **Node-level config (region, channels, MQTT) is NOT in config.yaml** - it lives in protobuf prefs and is set via the client API. [Usage](https://meshtastic.org/docs/meshtasticd/usage/)
- **Persistence:** prefs at `/var/lib/meshtasticd/.portduino/default/prefs/` - must be a PVC in K8s or node identity resets on pod restart. Useful flags: `-h MAC` (stable hardware ID for containers), `-p TCP_PORT`, `-s` (simulated radio).
- **Device API:** standard Meshtastic client protobuf API on **TCP 4403** - all clients (Python CLI, apps, web) work against it.
- **Official images:** [`meshtastic/meshtasticd`](https://hub.docker.com/r/meshtastic/meshtasticd) - `beta|alpha|daily` × `debian|alpine` tags; amd64/arm64/arm-v7/riscv64. USB passthrough via `--device=/dev/bus/usb/BUS/DEV`; SPI HATs need `/dev/spidev*` + `/dev/gpiochip*`. [Docker docs](https://meshtastic.org/docs/meshtasticd/installation/docker/)

## Configuration automation (the declarative primitive)

Fully scriptable via the [Python CLI](https://meshtastic.org/docs/software/python/cli/) (`pip install "meshtastic[cli]"`, connect with `--host <ip>:4403`):

- Settings: `--set lora.region US --set device.role ROUTER --set mqtt.enabled true` (chainable, one reboot cycle).
- Channels: `--ch-add`, `--ch-set psk ... --ch-index N`, `--seturl <url>` (replaces the whole channel set from one shareable URL - good for fleet-wide channel config).
- **YAML round-trip:** `--export-config > node.yaml` / `--configure node.yaml` - owner, channels (incl. PSKs), config + module-config. This is the desired-state format for an operator. Caveat: applying config reboots the node per changed section, and `--configure` is not a true diff - **an operator should export, diff, and apply only drift**.
- Remote admin of other mesh nodes via `--dest '!nodeid'` (admin channel/keys) - one managed gateway can reconcile radio-only nodes across the mesh.
- [Python library](https://python.meshtastic.org/): `TCPInterface`, `localNode.localConfig`/`writeConfig()`, pubsub events (`meshtastic.receive`, connection lost/established) - enough to build an operator without shelling out.

## MQTT bridging

[MQTT integration](https://meshtastic.org/docs/software/integrations/mqtt/) · [module config](https://meshtastic.org/docs/configuration/module/mqtt/)

- Any internet-connected node with the MQTT module enabled is a gateway. Multiple gateways duplicate traffic - **consumers dedupe on packet `id`**.
- Settings (all CLI/YAML-settable): `mqtt.address/username/password`, `encryption_enabled` (payloads stay channel-AES-encrypted over MQTT), `json_enabled`, `tls_enabled`, `root`; per-channel `uplink_enabled`/`downlink_enabled`.
- **Protobuf topics (canonical, full fidelity):** `msh/REGION/2/e/CHANNEL/!nodeid` - binary `ServiceEnvelope` wrapping a `MeshPacket`; decode with [official protobufs](https://github.com/meshtastic/protobufs).
- **JSON topics (convenient, lossy):** `msh/REGION/2/json/CHANNEL/!nodeid` - decoded text/position/telemetry/nodeinfo etc. **Not supported on nRF52 devices** (e.g. RAK4631); meshtasticd and ESP32 support it. JSON downlink supports only text/position via a channel literally named `mqtt`.
- The public `mqtt.meshtastic.org` broker is heavily restricted (zero-hop, filtered, positions truncated) - **run a private broker** for a real bridge, and don't reuse the default PSK.
- Bridge design implication: prefer protobuf topics on a private broker, or skip MQTT and consume TCP 4403 directly per gateway (richer, no dedupe problem).

## Cheap hardware & zero-hardware simulation

- **~$10:** Seeed XIAO ESP32S3 + Wio-SX1262 kit - cheapest official-firmware node, WiFi-capable.
- **~$18–27:** Heltec WiFi LoRa 32 V3 (ESP32-S3 + SX1262 + OLED) - best beginner value. (V4 with 28 dBm now shipping.)
- **~$30–35:** RAK4631 WisBlock - lowest power (solar), but no WiFi and no MQTT-JSON (nRF52).
- **~$40+:** LilyGO T-Beam (adds GPS).
- **Pi route:** Pi Zero 2 W (~$15) / Pi 4 + supported SPI HAT (MeshAdv-Pi ~$20–35, Adafruit RFM9x ~$20) or a CH341 USB stick (~$25–35).
- **Simulation (confirmed, two levels):**
  1. `meshtasticd -s` - no radio at all; fully functional API endpoint on 4403 (configure it, enable MQTT). **Ideal CI / Phase 1 target.** A lone `-s` node is a mesh of one.
  2. [Meshtasticator](https://github.com/GUVWAF/Meshtasticator) - spawns multiple meshtasticd instances whose simulated LoRa chips exchange packets over local TCP with RF propagation modeling - multi-hop mesh behavior with zero radios.

  Attempted 2026-08-04 (containerized, headless): inside the meshtasticd debian image with Tk and Xvfb installed, `interactiveSim.py --help` runs, but script mode (`-s 2 -p /usr/bin`) requires gnome-terminal or xterm to spawn nodes; with xterm under Xvfb the nodes boot but the simulator fails to connect to them (Errno 111 connection refused). Verdict: not practical headless in containers today without deeper surgery (possible future paths: its Docker mode against a real Docker socket, or upstream headless support). Phase 1 CI therefore stays single-node simulation, the fallback the roadmap names.

## Prior art in Kubernetes

Thin as of 2026-08-13. No official meshtasticd Helm chart, and no CRD operator that continuously observes, diffs, and reconciles node config. Closest: [MeshMonitor](https://meshmonitor.org/) - a live dashboard with a maintained Helm chart, remote admin, automation, and MeshCore alongside Meshtastic. That is imperative fleet UI. Nobody has published Git-declared observe-diff-reconcile of Meshtastic node state; the Phase-4 operator is that attempt.
