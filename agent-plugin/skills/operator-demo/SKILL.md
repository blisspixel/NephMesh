---
name: operator-demo
description: Run the hardware-free MeshtasticNode operator demo against meshtasticd --sim. Use when the user wants to see reconcile without radios or a cluster.
license: Apache-2.0
---

# Operator demo (sim, no RF)

Runs the real `Converge` loop against a simulated radio.

```sh
cd operators/meshtastic-operator
sh ../../demo/operator/run.sh
```

Needs Docker, Go 1.25.x, and `meshtastic` on PATH.

This is not over-the-air delivery. For the MeshToad plus T-Deck RF bench, use the `meshtoad-bench` skill.

See `demo/operator/README.md`.
