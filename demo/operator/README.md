# Operator demo: declare intent, converge a radio, including a secure channel

This is the reproducible, hardware-free, $0 demonstration of the flagship: the
operator taking a declared intent and making a Meshtastic radio match it, then
keeping it matched. It runs the operator's real reconcile loop (the same
`Converge` state machine and CLI-backed device client the controller uses)
against a simulated radio running real Meshtastic firmware, no RF and no board.

## What it shows

- The operator reads the device, computes the drift between the declared intent
  and the live config, applies only what changed, reboots the radio, and
  re-verifies to `Ready`.
- The declared intent includes a **secure private channel**: its pre-shared key
  is applied through a distinct path, kept off the command line, and confirmed by
  comparing key hashes, never the key itself.
- Running it a second time is a no-op: the device is already in the declared
  state, so nothing is written. That idempotence is the whole point of a
  reconcile loop, not a one-shot push.

## Prerequisites

- `docker`
- `go` (1.25.x)
- the Meshtastic CLI and library: `pip install "meshtastic[cli]"`

If your CLI and python are not `meshtastic` and `python` on your PATH, set
`MESH_BIN` and `NEPHMESH_PY`.

## Run it

From the operator module directory:

```sh
cd operators/meshtastic-operator
sh ../../demo/operator/run.sh
```

## Expected output

Captured from an actual run against `meshtasticd --sim`:

```
== bringing up a simulated Meshtastic radio (real firmware, no RF) ==
waiting for the device API ready

== reconcile declared intent onto the device (region, preset, owner, and a secure channel) ==
nephmesh reconcile-demo: driving Meshtastic device at 127.0.0.1:14403
desired intent: map[config:map[lora:map[modemPreset:MEDIUM_SLOW region:US]] owner:NephMesh Field 01 owner_short:NF01]
desired channel: index=1 name="relief" key=set (from -channel-key, not shown)
  step 1  reachable=true  inSync=false rebootPending=true  ready=false  <- applied drift, device rebooting
  step 2  reachable=true  inSync=true  rebootPending=true  ready=false
  step 3  reachable=true  inSync=true  rebootPending=false ready=true   <- converged
converged: node !1520da6c, config in sync, Ready=true
secure channel provisioned: index=1 name="relief", key hash matches the device

== run it again: the device is already in the declared state, so nothing is written ==
nephmesh reconcile-demo: driving Meshtastic device at 127.0.0.1:14403
desired intent: map[config:map[lora:map[modemPreset:MEDIUM_SLOW region:US]] owner:NephMesh Field 01 owner_short:NF01]
desired channel: index=1 name="relief" key=set (from -channel-key, not shown)
  step 1  reachable=true  inSync=true  rebootPending=false ready=true   <- converged
converged: node !1520da6c, config in sync, Ready=true
secure channel provisioned: index=1 name="relief", key hash matches the device

== done. The operator declared intent, converged the radio (including a secure channel), and was idempotent on re-run. ==
```

The node id (`!1520da6c` above) is whatever the sim generates on your machine.

## Honest scope

This demonstrates the operator's reconcile engine against real firmware, which is
the load-bearing part. It is not the full Kubernetes path (a `MeshtasticNode`
custom resource reconciled by the deployed operator in a cluster); that is
exercised separately by the envtest controller tier and the CI integration test.
It also does not measure over-the-air message delivery. That bench is
[demo/meshtoad-gateway](../meshtoad-gateway/): T-Deck on serial plus MeshToad
on `meshtasticd` TCP, texts tagged `TRANSPORT_LORA`. The
control-plane-independence resilience test that measures delivery without
radios is `demo/resilience/`.

The same `reconcile-demo` tool points at a real board over USB with `-serial`
(plus `-exporter`), and its read-only `-observe` mode reads a device without ever
modifying it.
