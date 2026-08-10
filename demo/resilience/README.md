# Resilience harness: measure that the mesh keeps delivering

The README stakes NephMesh on a claim it never actually measured: that the mesh
routes around loss and keeps carrying traffic even when the management plane, and
the site running it, are gone. This harness turns that adjective into a number. It
stands up a multi-node Meshtastic mesh with no radios, originates traffic, and
measures delivery ratio and latency, so a perturbation (a killed node, a killed
management plane, a congested channel) can be shown to change the number or not.

Everything here is hardware-free and receive-only in spirit: the nodes are
`meshtasticd` in simulated-radio mode, and they mesh over the firmware's real UDP
multicast transport (group 239.0.0.69, port 4403), a genuine product feature of
the Linux build. No RF is emitted.

## Why UDP multicast, and why that matters

`meshtasticd --sim` alone does not mesh: its simulated radio is a single-node
loopback to one connected API client, so two sim daemons never see each other. The
Linux build ships a real inter-node transport, UDP multicast, that carries encrypted
`MeshPacket`s between daemons on a shared network independent of the API. That gives
the harness the exact separation NephMesh claims in the field:

- **Data plane:** the nodes mesh over UDP multicast. This is the stand-in for the
  LoRa RF link.
- **Management plane:** each node's TCP API (4403) stays free for the operator to
  configure it.

Because the data plane does not depend on the management plane, the
control-plane-independence test is real rather than circular: configure the fleet
through the operator, then remove the operator (and the cluster, and the host), and
the UDP mesh keeps delivering.

## Run it

```sh
sh demo/resilience/up.sh 3          # three sim nodes on a Docker network, meshing
MSYS_NO_PATHCONV=1 docker exec meshcli python /probe.py \
    --sender sim1 --receivers sim2,sim3 --count 20 --interval 3
sh demo/resilience/down.sh          # tear it all down
```

The probe originates sequenced broadcasts from `sim1` and counts how many arrive at
`sim2` and `sim3` over their APIs, then prints a JSONL event log and a summary:

```
SUMMARY {"sender": "sim1", "receivers": ["sim2", "sim3"], "sent": 20,
         "expected": 40, "delivered": 40, "delivery_ratio": 1.0,
         "latency_ms_p50": 267.3, "latency_ms_max": 552.7}
```

Requires Docker (Docker Desktop on Windows works). No radio, no cluster, no cloud.

## What is validated here (2026-08-10)

Run end to end on a Windows host with Docker Desktop:

- Multiple `meshtasticd -s` nodes mesh over UDP multicast on a plain Docker bridge,
  no radio, no special CNI, and no privileged container. A text sent on `sim1` is
  carried to `sim2` and `sim3`.
- At a sustainable cadence (3 s between messages at the default preset), delivery is
  100% to every receiver, with a median latency around 270 ms.
- Drive the send interval below the channel's airtime ceiling and delivery falls,
  and it falls **at the sender** (every receiver misses the same messages), which is
  the airtime-as-a-commons wall the project names, made directly visible. This is a
  measurement the harness is meant to make, not a defect.

Two gotchas the scripts encode, both learned the hard way:

- A Meshtastic config change reboots the device by exiting the process, so each node
  runs with a Docker restart policy; without it the container stays down after the
  first `--set`.
- On Git Bash, `MSYS_NO_PATHCONV=1` is required so an in-container path argument
  (`-d /var/lib/meshtasticd`) is not rewritten into a Windows path.

## What this is for, and what is next

This is brick one of the mission-1.0 direction: a measurement instrument, so
"resilient" stops being asserted and starts being shown. The scenarios it unlocks,
in order:

1. **Control-plane independence.** Configure the fleet through the operator, begin
   measuring, then kill the management plane (the operator, then the whole cluster)
   and show the delivery ratio is unchanged. This is the README's load-bearing claim,
   currently asserted, not demonstrated.
2. **Survival under degradation.** Congest or jam the channel, sense it through the
   SDR pillar, adapt the mesh (relocate the preset), and show delivery recover, all
   measured.

## Honest scope

- The mesh here is flat (every node hears every other in one UDP hop), so this is not
  a multi-hop routing study; it measures keep-alive and delivery under perturbation,
  which is what the thesis needs. Multi-hop topology simulation is a separate tool
  (Meshtasticator) and a later concern.
- UDP multicast stands in for the LoRa RF link. It is a faithful stand-in for
  delivery and airtime behavior, not for RF propagation, range, or interference
  physics; those enter only with a real radio.
- Nothing here transmits over the air. The simulated radios emit no RF, and the SDR
  side of the project stays receive-only.
