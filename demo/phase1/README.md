# Phase 1 demo: virtual mesh, declaratively configured

The 0.1 gate from the [roadmap](../../docs/roadmap.md): a simulated Meshtastic node deployed, configured, and torn down declaratively, with its traffic observable on a private MQTT broker. No radios, no Nephio machinery, no cloud. Plan and rationale: [docs/plans/phase-1-virtual-mesh.md](../../docs/plans/phase-1-virtual-mesh.md).

## Prerequisites

- A local cluster: kind, k3d, or k3s. On Windows, Docker Desktop plus kind or k3d works; run the scripts under WSL2 or Git Bash.
- kubectl pointed at it. Nothing else: the CLI and the MQTT watcher run in-cluster.
- Internet egress from pods (the applier Job pip-installs the Meshtastic CLI).

## Run it

```sh
make demo-phase1          # or: sh demo/phase1/scripts/demo.sh
make demo-phase1-down     # or: sh demo/phase1/scripts/teardown.sh
```

The demo passes when all of: workloads become Ready with no manual steps; the config Job converges; a re-run of the Job reports "already converged" with zero device reboots; a sent text appears on the `msh/#` MQTT topics within 60 seconds; teardown leaves nothing behind.

## What is validated here (Docker rehearsal, 2026-08-04, firmware 2.7.26)

These manifests encode findings from running the real `meshtastic/meshtasticd:beta-debian` image end to end:

- The image has no entrypoint (default cmd is a shell wrapper), so the Deployment sets `command: ["meshtasticd"]` explicitly.
- Persistence root is `--fsdir`; prefs land at `<fsdir>/prefs/`. The PVC mounts at `/var/lib/meshtasticd`.
- `--hwid=dc2c6e000001` deterministically yields MAC `dc:2c:6e:00:00:01` and node ID `!6e000001`.
- Applying config makes the process exit with code 1: that is the device "reboot". Kubernetes restarts the container; the applier tolerates the dropped session and verifies after.
- A lone simulated node does uplink its own sent text to MQTT (both the protobuf and JSON topics), so the gate observable is real.
- Observed topic shape on this firmware: `msh/2/e/LongFast/!6e000001` and `msh/2/json/...` (no region segment, unlike some documentation).
- The device's MQTT client did not connect when `mqtt.address` was a hostname but connected immediately with an IP; the applier therefore resolves the broker Service to its ClusterIP at apply time.
- Multiple copies of the same packet appear on the broker (mesh rebroadcast behavior); consumers dedupe on packet `id`.

## Gate result

The gate passed on a kind cluster (Windows host, Docker Desktop) on 2026-08-04. Trimmed transcript:

```
== 3/6 wait for declarative config to converge (device reboots are expected)
job.batch/meshnode-configure condition met
applying configuration
enabling MQTT uplink on the primary channel
rebooting device to activate module config
configuration verified after apply

== 4/6 idempotency: re-running the applier must be a no-op
job.batch/meshnode-configure condition met
idempotency: OK

== 6/6 verify the message reached MQTT
message observed on MQTT topics:

PHASE 1 GATE: PASS
```

Persistence (validation item V1) also passed: after `kubectl delete pod` on the node, the replacement pod loaded its prefs from the PVC and reconnected to MQTT on its own:

```
INFO  | 21:23:40 0 Loaded /prefs/nodes.proto successfully
INFO  | 21:23:40 0 [mqtt] Connecting directly to MQTT server 10.96.215.62, port: 1883
INFO  | 21:23:40 0 [mqtt] MQTT connected
```

Two Kubernetes-specific findings from the gate run, both encoded in the manifests:

- A TCP readiness probe on 4403 is harmful: the device API is single-client and each probe connection force-closes the active CLI session, interrupting applies. The Deployment has no probe; the applier does its own reachability waits.
- The MQTT client thread starts only at device boot; an in-place config write does not start it. The applier issues an explicit `--reboot` after applying, making the outcome deterministic.
- The device API on 4403 is single-client, and its Portduino implementation wedges (stops accepting connections) under rapid reconnection. The demo opens one connection per step and sends exactly once; this is a real constraint the future operator must respect (serialize access, do not poll tightly).

## Troubleshooting

- `meshnode-sim` restarting once or twice during configuration is expected (see above), not a crash loop.
- If the configure Job exhausts its backoff, check `kubectl -n nephmesh logs job/meshnode-configure`; the usual cause is no pod egress for pip.
- Windows: if you shell into containers manually with `docker exec` and paths get mangled, prefix commands with `MSYS_NO_PATHCONV=1` (Git Bash path conversion).
