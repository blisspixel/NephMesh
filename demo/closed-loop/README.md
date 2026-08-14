# Closed-loop proof of concept: sense, decide, actuate, verify

This demo closes the loop across real hardware for the first time: an SDR senses
the spectrum, a decision is made from the sensed value, the operator actuates the
mesh, and the SDR senses again to confirm the change took effect. It is the
physical proof of concept behind the Phase 6 vision, where sensed occupancy
proposes a configuration change (see [the roadmap](../../docs/roadmap.md) and
[the doctrine](../../docs/design/doctrine.md)).

## What it is, and what it is not

It **is** a demonstration that the two halves of NephMesh, the operator that
reconciles a real Meshtastic node and the SDR that senses the RF environment, can
be joined into a control loop on one bench.

It is **not** autonomous operation. The policy is one fixed threshold, it makes a
single decision, actuates once, verifies, and restores. Autonomous, standing
actuation is deliberately gated behind the signed-autonomy and safety-kernel work
(ADR 0002); nothing here self-drives, and the loop is hand-run. The SDR is
receive-only; the only transmit is application-layer Meshtastic text on a
license-free band, from the operator's own node.

## The loop

1. **Sense.** Run an integrated `hackrf_sweep` on the sensor host while the mesh
   is on the air, and reduce it with `nephmeshctl spectrum` to the band's peak
   power and peak frequency.
2. **Decide.** If the peak power is above a threshold, the mesh's channel is
   treated as active, and the policy is to relocate it (a preset change that
   moves the channel slot). Otherwise, hold.
3. **Actuate.** The operator's reconcile loop (`reconcile-demo`) applies the new
   modem preset to the physical node: apply the drift, wait out the reboot,
   re-verify to `Ready`.
4. **Verify.** Sense again; the mesh's transmission should now appear at a
   different frequency, confirming the actuation took effect and that the sensor
   sees it.
5. **Restore.** Return the node to its original preset.

## Running it

Needs, on this machine, the `nephmeshctl` and `reconcile-demo` binaries (built
from `operators/meshtastic-operator`), a Meshtastic CLI, and the config export
helper; and on the sensor host, `hackrf_sweep`. Configure with environment
variables (see the defaults at the top of `run.sh`) and run:

```sh
export SENSOR_SSH="user@linux-usb-host"
export COM_PORT="COMn"
export EXPORTER="python /path/to/operators/meshtastic-operator/hack/mesh-export.py"
# ... plus the binary paths; see run.sh
sh demo/closed-loop/run.sh
```

## Captured run

The following is a real run on the bench (2026-08-09), a HackRF Pro on the
Linux USB host sensing, and a Meshtastic handheld actuated by the operator over
USB serial:

```
=== SENSE: sweep 902-928 MHz with the mesh on the air ===
sensed ism-915-us: peak -17.2 dB at 906.500 MHz

=== DECIDE: is the channel active (peak > -35 dB)? ===
channel is active (peak -17.2 > -35 dB); ACTUATE: move the mesh to LONG_MODERATE.

=== ACTUATE: operator reconciles the T-Deck to LONG_MODERATE ===
  step 1  reachable=true  inSync=false rebootPending=true  ready=false  <- applied drift, device rebooting
  ...
  step 7  reachable=true  inSync=true  rebootPending=false ready=true   <- converged
converged: node <handheld-id>, config in sync, Ready=true

=== VERIFY: sense again; the mesh should have moved frequency ===
after actuation, ism-915-us peak is at 903.500 MHz (was 906.500 MHz)

=== RESTORE: return the node to LONG_FAST ===
  step 1  reachable=true  inSync=false rebootPending=true  ready=false  <- applied drift, device rebooting
  step 3  reachable=true  inSync=true  rebootPending=false ready=true   <- converged
converged: node <handheld-id>, config in sync, Ready=true
```

The decision was driven by the sensed peak, the actuation went through the
operator's real reconcile loop (validating modem-preset round-trip on a physical
board), and the verify step confirmed the mesh moved frequency, exactly the
behavior the closed loop needs, with the node's own telemetry and the external
sensor agreeing. The remaining distance to Phase 6 is not mechanism, which this
shows works, but the safety envelope (ADR 0002) that would let it run unattended.
