# MeshToad gateway bench

A scripted replay of a two-radio bench: the operator observes a Linux-native
`meshtasticd` gateway (CH341 USB-SPI) and a USB handheld, then sends one
application text each way over LoRa. Plan:
[`docs/plans/meshtoad-gateway-bench.md`](../../docs/plans/meshtoad-gateway-bench.md).

## What it is, and what it is not

It **is** a hand-run check that the gateway and the handheld still hear each
other on US LongFast, and that `reconcile-demo` can still read both.

It **is not** a cluster install, a Porch apply, or autonomy. The gateway API
stays on the sensor host's localhost 4403; this script talks to it through an
SSH tunnel. The SDR is not required.

## Preconditions

Set these. Do not commit their values.

```sh
export SENSOR_SSH="user@linux-usb-host"
export COM_PORT="COMn"   # or /dev/ttyACM0
export MESH_HOST="127.0.0.1:14403"
export EXPORTER="python operators/meshtastic-operator/hack/mesh-export.py"
export RECONCILE_DEMO="reconcile-demo"
```

- Sensor host is running `meshtasticd` as in the bench plan. Do not reboot
  that host or disturb unrelated services (including a local LLM).
- Tunnel: `ssh -N -L 14403:127.0.0.1:4403 "$SENSOR_SSH"`
- Handheld on `COM_PORT`
- `meshtastic` on PATH

```sh
sh demo/meshtoad-gateway/run.sh
```

Optional: `DO_CHANNELS=1` plus `APPLIER` set to `mesh-apply.py` applies a
secondary channel on the gateway and deletes it. Off by default.

## What a passing run shows

- Both radios Ready under `reconcile-demo -observe`.
- One text each way, receiver log contains the payload (LoRa, not an IP
  shortcut).
- Destructive channel apply is optional and must restore.
