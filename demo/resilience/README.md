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

## Control-plane independence, demonstrated

The README's load-bearing claim is that NephMesh is a management layer, not a
runtime dependency: the mesh keeps carrying traffic even when the cluster and its
site are gone. `independence.sh` shows it with a number rather than asserting it.

```sh
sh demo/resilience/independence.sh
```

It brings up the mesh, has the operator's real reconcile loop (the same `Converge`
state machine the controller runs) configure a node over its API, measures delivery
while destroying the management plane mid-run, and reduces the log to a before/after
verdict with `nephmeshctl resilience`.

Captured run (2026-08-10):

```
3/5 Operator configures sim1 (the management plane manages the mesh)
  step 1  reachable=true  inSync=false rebootPending=true  ready=false  <- applied drift, device rebooting
  step 2  reachable=true  inSync=true  rebootPending=false ready=true   <- converged
converged: node !6e000001, config in sync, Ready=true

4/5 Measure delivery, then destroy the management plane mid-run
  perturbation at t=...: destroying the management plane (operator + its host)

5/5 Verdict
control-plane-independence report (receivers: sim2, sim3)
  before delivery 100.0% (24/24), latency p50 277 ms, max 439 ms
  after  delivery 100.0% (24/24), latency p50 341 ms, max 538 ms
  verdict: mesh kept delivering across the perturbation (management plane gone, traffic unaffected)
```

The operator genuinely managed the node (it applied drift, the device rebooted, and
it re-verified to `Ready`), and the mesh delivered 100% both before and after the
management plane was destroyed. Honest reading: because the data plane (UDP
multicast) never depended on the operator, this operationalizes the claim, turning
it from asserted into shown, rather than uncovering a hidden dependency. That is
exactly what the roadmap asks for: until it runs, "resilient" is asserted, not
shown.

The verdict logic is pure and unit-tested (`internal/resilience`, exposed as
`nephmeshctl resilience -at <perturbation-time> -f <probe-log>`): it splits the
probe's event log at the perturbation and reports delivery ratio and latency before
and after, so any run, not just this one, is judged the same way.

## Survival under congestion, demonstrated (the airtime commons, measured)

`survival.sh` shows, with numbers, that the shared channel has a finite airtime
budget: offering traffic beyond it collapses delivery, and admission control
(pacing offered load back within the budget) restores it. This grounds the
project's airtime-as-a-commons claim, and the intent layer's fleet airtime budget,
in measured delivery.

```sh
sh demo/resilience/survival.sh
```

One continuous probe run over three offered-load phases, reduced to a per-phase
verdict by `nephmeshctl resilience -phases`:

Captured run (2026-08-10):

```
airtime-commons survival report (receivers: sim2, sim3)
  baseline  delivery 100.0% (20/20), latency p50 305 ms
  degraded  delivery  50.0% (20/40), latency p50 186 ms  (down 50.0 pts vs baseline)
  adapted   delivery  90.0% (18/20), latency p50 248 ms  (recovered)
  verdict: delivery fell under load and recovered after the adaptation
```

Baseline offers 1 msg/3s (within budget), degraded offers 1 msg/1s (over budget),
adapted paces back to 1 msg/3s. The airtime model predicts exactly this: at
LONG_FAST (~559 ms time-on-air) 1 msg/3s is ~19% channel utilization (under the
25% ceiling) while 1 msg/1s is ~56% (over it). The measured collapse confirms the
model's over-budget verdict is authoritative, which is precisely how the airtime
doctrine already describes it (a conservative floor whose over-budget verdict is
certain). The verdict logic is the pure, unit-tested `internal/resilience`
(`ReducePhases`/`PhaseReport`), so any run is judged the same way.

An honest finding from building this: the recovery lever is **admission control**
(pacing offered load to the budget), not a modem-preset change. We measured that a
faster preset does not recover delivery in this sim, delivery is capped at a fixed
per-node broadcast cadence (a firmware timing, not the PHY airtime), so a
lower-time-on-air preset does not raise the per-node send rate. Pacing the offered
load is therefore the honest recovery here; on real RF a preset change also
relocates the channel frequency (a different lever the closed-loop hardware PoC
shows), which this UDP substrate cannot model.

Scope, restated: the mesh is flat, UDP multicast stands in for the LoRa RF link
(the sim firmware models time-on-air, which is why offered load maps to real
delivery), the sense in sim is the app-layer delivery/airtime signal while on
hardware the SDR senses the RF cause, and nothing transmits over the air.

## What is next

The safety-kernel slice (ADR 0002) that lets an adaptation like this run with no
human inside a signed envelope, the self-adapting fabric earned honestly. Until
then, every adaptation here is report-only and human-gated.

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
