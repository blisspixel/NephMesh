# MeshToad V3 as a Linux-native meshtasticd gateway (bench plan)

Status: executed 2026-08-13. Observe, region, two-node RF text, MeshToad TCP
channels, MeshToad role ROUTER then CLIENT, and T-Deck serial channels all
passed and were restored. A replay script lives at
`demo/meshtoad-gateway/`. What remains outside this bench: in-cluster USB
device-plugin, a Porch/Git wrap, and publishing the operator image.

## Captured run (2026-08-13)

Linux USB host plus a Windows (or other) dev PC. Unrelated services on the
USB host were left running. HackRF stayed attached.

| Step | Result |
|---|---|
| USB | MeshToad class `1a86:5512`, no tty. EEPROM product string `MESHTOAD` is visible to `meshtasticd`; USB `iProduct` may be empty. |
| Bring-up | Docker `meshtastic/meshtasticd:beta-debian`, API on **127.0.0.1:4403 only**. Official `lora-usb-meshtoad-e22.yaml`, `SX126X_MAX_POWER: 10`, a stable `General.MACAddress`. `sx1262 init success`. |
| Observe | `reconcile-demo -host` through a tunnel, Ready. First-boot region UNSET. |
| Region | `reconcile-demo -region US` only. Reboot, Ready. Owner and preset unchanged. Firmware then sends NodeInfo. |
| Two-node RF | Handheld on USB serial, same firmware line, US / LONG_FAST / default primary. Each node listed the other at hopsAway 0, SNR about 6 dB. |
| Text both ways | Application texts tagged `TEXT_MESSAGE_APP` / `TRANSPORT_LORA`. Implicit ACK on send. |
| Channel apply | `reconcile-demo` plus `-channel-name` through `mesh-apply.py` on the gateway (TCP) and the handheld (serial). Key hash matched. USB drop on handheld reboot treated as transient. Restored with `--ch-del`. |
| Role | `reconcile-demo -role ROUTER` then `-role CLIENT` on the gateway, both Ready. |

Packets that closed the demo were tagged `TRANSPORT_LORA`. Do not treat a
later TCP session to a Wi-Fi handheld as an RF proof.

Not closed by this run: in-cluster `MeshtasticNode` against the sensor host;
device-plugin USB; a Porch wrap of `demo/meshtoad-gateway/`.

This is the current real-RF gateway path. It does not replace the ESP32/WiFi
board path in `phase-2-hardware-sensing.md`. It adds a CH341 USB-SPI radio that
`meshtasticd` drives natively, on the sensor host that already holds the HackRF.

## Why this, and why now

The operator's in-cluster transport is TCP 4403 only. Serial exists on
`reconcile-demo` for the T-Deck; the deployed operator reports serial
unsupported (`docs/guides/operations.md`). A NULLHOP MeshToad V3 is the radio
the original Phase 2 plan called optional (a CH341 USB stick plus `meshtasticd`
with real RF). The T-Deck stays on the Windows PC so the first two-node RF
path has geometry: one handheld, one Linux gateway, SDR on the gateway host.

What this session is for:

1. Bring `meshtasticd` up against the USB-SPI radio, API on localhost 4403.
2. Observe-only reconcile from the dev PC through an SSH tunnel.
3. One destructive channel apply over TCP (the distinct helper path; sim is
   weaker evidence). Restore.
4. One short text across real RF: handheld serial to the gateway.

What this session is not for: publishing the operator image, Porch, the USB
device plugin, `CommunicationIntent` actuation, autonomy, MeshCore, or moving
the T-Deck.

## Lab geometry

| Piece | Where | Transport |
|---|---|---|
| MeshToad-class USB-SPI radio | Linux USB host | `meshtasticd`, TCP 4403 on localhost |
| ESP32-S3 handheld | Dev PC USB CDC | `reconcile-demo -serial` |
| HackRF Pro `1d50:6089` | Same Linux host | receive-only witness |
| Operator / CLI | Dev PC | SSH tunnel to the host's localhost 4403 |

Facts that matter for any such host:

- Prefer Docker `meshtastic/meshtasticd:beta-debian` over an apt install if
  `sudo` is gated. Do not prune unrelated images (the host may hold LLM
  stacks).
- The USB-SPI node is often `root:root` 0664 until a udev rule matches
  `1a86:5512` to `plugdev`. Docker as root can still pass `--device`.
- If the host also runs a local LLM, do not reboot it, stop that service,
  or bind its port.

## Device class (do not treat this as a COM port)

The MeshToad is a USB-C CH341 plus an EBYTE E22P-915M30S (SX1262). There is no
Meshtastic MCU on the stick. Linux-native `meshtasticd` is the node.

| Mode | VID:PID | What the OS sees | Role |
|---|---|---|---|
| CH341 USB-SPI (MeshToad, MeshStick) | `1a86:5512` | USB device only; no `/dev/ttyUSB*` | Correct. Use this. |
| CH341 USB-UART | `1a86:7523` | `/dev/ttyUSB*` | Wrong for this board. |
| ESP32-S3 CDC (handheld) | `303a:1001` | `COMn` / `ttyACM*` | Stays on the dev PC. |

Windows may show `1A86:5512` in Error with no COM port. That is expected: it
is not a serial device. Official `meshtasticd` Docker docs use exactly that
ID ([usage, CH341 USB](https://meshtastic.org/docs/meshtasticd/usage/)).

`meshtasticd` 2.6.5+ can autoconfig from EEPROM `PRODUCT=MESHTOAD`. This
unit's USB `iProduct` string was empty and a config-less start failed with
"Blank MAC Address not allowed" (the container has no host NIC for
`MACAddressSource`). Host `config.yaml` is radio/host plumbing only: a
`General.MACAddress` plus `config.d/lora-usb-meshtoad-e22.yaml`. Region,
role, channels, MQTT live in protobuf prefs on TCP 4403
(`docs/research/meshtastic.md`).

Peak TX is 1 W / 30 dBm and about 900 mA, above the USB 2.0 budget
([MtnMe.sh](https://mtnme.sh/devices/MeshToad/)). Antenna on before power.
Drop `lora.tx_power` by hand on the CLI before any `--sendtext`. The operator
must not write `tx_power` (`hack/check-transmit.sh`).

## Coexistence with an LLM on the USB host

The USB host may be shared. A local LLM is a first-class tenant. `meshtasticd` is a
small CPU process and does not need the GPU. The bench must not evict or
restart the model.

Forbidden on that host:

- Reboot the host or change its power or clock profile.
- Stop, restart, or reconfigure the unrelated LLM service or its listen port.
- `docker prune`, `docker system prune`, image deletes, or any cleanup of
  images that service did not create.
- `apt upgrade` / dist-upgrade, CUDA or compiler installs, model pulls.
- Binding, proxying, or fire-walling the LLM listen port.
- Long-running `hackrf_sweep` loops that contend for USB or CPU while a model
  is mid-request. Short, bounded sweeps are fine.
- Passing the HackRF USB node into the `meshtasticd` container.

Allowed: `lsusb` / `dmesg`, a small `meshtasticd` container on `1a86:5512`
only, publish 4403 on `127.0.0.1` only, SSH from Windows, `hackrf_info` and
short receive-only sweeps.

If a step needs `sudo` (udev rule so `1a86:5512` is `plugdev`, matching the
HackRF), stop and ask. Do not invent a password.

## Bring-up (docker, not apt)

If `sudo` is password-gated and the user can run Docker, prefer the official
`meshtastic/meshtasticd:beta-debian` image (arm64, same line Phase 1 already
trusts at 2.7.26) over an apt install. Need version 2.6.5 or newer for EEPROM
autoconf. Do not copy a `config.d` LoRa fragment until autoconf is shown to
fail.

After the stick is on a **hub port** (not the same USB node as the HackRF):

```sh
# on the USB host, observe only
lsusb -d 1a86:5512
lsusb -d 1d50:6089
# MeshToad must not replace the HackRF line.
```

Record the MeshToad `Bus` / `Device`. Pass only that node:

```sh
# XXX/YYY is the MeshToad bus/dev, never the HackRF node
docker pull meshtastic/meshtasticd:beta-debian
docker run -d --name meshtasticd-toad \
  --restart unless-stopped \
  --device=/dev/bus/usb/XXX/YYY \
  -p 127.0.0.1:4403:4403 \
  -v meshtasticd-toad-data:/var/lib/meshtasticd \
  meshtastic/meshtasticd:beta-debian
```

`--restart unless-stopped` is required: a config apply makes `meshtasticd`
exit (firmware reboot). The named volume keeps node identity across that
exit. Do not publish `0.0.0.0:4403`. Do not install the Avahi snippet from
upstream usage docs.

If the container cannot open the USB node, the likely cause is a 0600
device file. That is a one-line udev rule for `1a86:5512` to `plugdev`,
same shape as the existing HackRF rule. Ask before applying it.

## Observe, then one apply, then RF

The device API has no auth and is single-client. A second client kicks the
first. Drive it from Windows through a tunnel. Do not leave a CLI `--info`
session up when `reconcile-demo` runs.

```sh
# Dev PC (keep this tunnel up). SENSOR_SSH is user@host, never committed.
ssh -N -L 14403:127.0.0.1:4403 "$SENSOR_SSH"
```

1. `meshtastic --host 127.0.0.1:14403 --info`
   Record node id, region (often UNSET), preset, role, reported `tx_power`.
2. `python operators/meshtastic-operator/hack/mesh-export.py --host 127.0.0.1:14403`
3. `go run ./cmd/reconcile-demo -host 127.0.0.1:14403 -observe`
   Empty intent, no write.
4. Drop TX power on the CLI by hand. Antenna must already be on.
5. Channel apply through the bundled helper, then restore:

   ```sh
   cd operators/meshtastic-operator
   go run ./cmd/reconcile-demo -host 127.0.0.1:14403 \
     -exporter "python hack/mesh-export.py" \
     -applier "python hack/mesh-apply.py" \
     -channel-name relief -channel-key "demo-relief-key!" -channel-index 1
   ```

   Role on this radio is applied with `-role` (ROUTER then CLIENT both
   Ready on 2026-08-13). Pass `-region US` with it so the canned demo
   intent is not used.
6. Same region and primary channel on the handheld and the gateway. Leave
   handheld Wi-Fi unused for the proof. Short `--sendtext` from the serial
   port, hear it on the tunneled host. Reverse only after TX power is known-low.
7. Optional: one short `hack/spectrum-sweep.sh` over existing `SENSOR_SSH`
   while the burst is on the air. Receive-only.

Pass: observe printed a node id; channel apply reached Ready and restored;
a text crossed RF with no IP mesh between the two radios.

## What this closes, and what it does not

Closes, if the sequence succeeds:

- The stale "no Linux real-RF gateway" assumption.
- Correct USB class (`1a86:5512`, not `7523`).
- Operator-shaped TCP reconcile against a live radio, including the
  distinct channel path.
- Two-node over-the-air delivery (T-Deck serial, MeshToad TCP).

Does not close:

- In-cluster serial, or a `MeshtasticNode` with `connection.serial`.
- In-cluster USB device plugin for the MeshToad or the HackRF.
- Publishing the operator image (hardware-free, next sitting after this).
- Porch 0.3, two-cluster control-plane independence, autonomy.

Role apply through `reconcile-demo` on the gateway closed on 2026-08-13 (ROUTER then CLIENT, both Ready, restored).

## Non-goals

Carried from Phase 2 and the host constraint:

- No HackRF transmit. MeshToad TX is application-layer Meshtastic on the
  license-free US ISM band from an owned node. That is not an SDR transmit
  path. Legality is US-scoped, non-lawyer research; see
  `docs/research/terminology-and-legality.md` and `DISCLAIMER.md`.
- No closed-loop actuation. `demo/closed-loop` stays a hand-run proof.
- No new CRDs, packages, or Porch changes in this bring-up.
- No meshtasticd-with-real-RF on the Pis. The gateway radio is this stick
  on the USB host.
- No NodePort, LoadBalancer, hostPort, or LAN bind of 4403 in repo
  manifests (`hack/check-manifests.sh`).
- Node config is not written into `meshtasticd` `config.yaml`.
