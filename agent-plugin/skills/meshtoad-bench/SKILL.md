---
name: meshtoad-bench
description: Run or interpret the MeshToad plus handheld LoRa bench. Use when the user has a CH341 meshtasticd gateway on a Linux USB host and a USB handheld, or asks about the two-radio RF proof.
license: Apache-2.0
---

# MeshToad plus handheld bench

## Lab rules (do not invent addresses)

- MeshToad-class radio is USB `1a86:5512` on the Linux USB host, not a COM port.
- `meshtasticd` API on **127.0.0.1:4403 only**.
- From the dev PC: `ssh -N -L 14403:127.0.0.1:4403 "$SENSOR_SSH"`
- Handheld serial is `$COM_PORT`. Never commit hostnames, IPs, or node ids.
- The USB host may run a local LLM. Do not reboot it, stop that service, prune Docker, or bind the LLM port.

## Replay

```sh
export SENSOR_SSH="user@linux-usb-host"
export COM_PORT="COMn"
export EXPORTER="python operators/meshtastic-operator/hack/mesh-export.py"
export RECONCILE_DEMO="reconcile-demo"
sh demo/meshtoad-gateway/run.sh
```

`DO_CHANNELS=1` is optional and destructive on the gateway only.

## Writes

Any `-role`, `-region`, or channel apply reboots the radio. Pass `-region US` with sparse flags so the canned MEDIUM_SLOW demo intent is not used. Restore after a destructive check.

See `demo/meshtoad-gateway/README.md` and `docs/plans/meshtoad-gateway-bench.md`.
