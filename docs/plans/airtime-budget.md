# Airtime budget enforcement (plan)

Airtime, not node count, is the wall a LoRa mesh actually hits: the founding scaling study (Bor et al., MSWiM 2016) and the 2026 Meshtastic preset link-budget work both show a channel collapses from airtime and collisions, and the longest-range presets have the longest time-on-air and collapse a dense channel fastest. This was the single highest-confidence finding of the August 2026 research sweep (`docs/research/resilient-comms-landscape.md`), and governing airtime as a declared, enforced budget is the clearest guarantee a declarative intent system can offer that hand-tuning a device cannot. This plan turns that into a concrete design.

## What exists

`internal/airtime` (landed) computes LoRa time-on-air per Meshtastic modem preset (the Semtech formula), unit-tested against a reference value, plus a duty-cycle helper. That is the *predictive* half: the cost of a frame at a given preset.

A finding from the first real-hardware session sharpens the *observed* half: a Meshtastic device already reports its own airtime telemetry. The T-Deck's `--info` carried `deviceMetrics.airUtilTx` and `channelUtilization` (both 0.0 on an idle bench node). So the operator does not have to model the whole fleet's traffic to know the channel is saturating; it can read what the radio measured.

## The gap

The `MeshtasticNode` CRD models one node's desired config, not the shared channel's aggregate airtime. A true fleet-level invariant ("do not let this channel exceed budget") needs cross-node awareness, which is Phase 5 (multi-site fan-out) territory. But two useful, grounded controls land well before that.

## Design: predict, observe, enforce

**1. Observe (near-term, per node).** Parse `airUtilTx` and `channelUtilization` from the device and surface them as `MeshtasticNode` status (a new `status.airUtilTxPercent` and `status.channelUtilizationPercent`, populated from `--info`). Add an `AirtimeHealthy` condition that goes false when either exceeds a threshold. This is honest telemetry from the radio itself, needs no traffic model, and is the prerequisite for the resilience metrics the roadmap already committed to. Cost: a `parseInfo` extension and two status fields (a CRD regen).

**2. Predict (near-term, pre-flight).** Before applying a config change, use `internal/airtime` to compute the delta in per-frame time-on-air the change implies (for example moving from `LONG_FAST` to `LONG_SLOW` multiplies airtime severalfold). Log it, and refuse a change that would raise per-frame airtime past a bound the node's own measured `airUtilTx` says it cannot afford. This is the operator saying "this preset would push you over" using the device's real utilization, not an assumed message rate.

**Landed (advisory).** The operator surfaces this as the `AirtimeBudget` status condition: when a node declares a modem-preset change, it predicts the post-change channel utilization (`airtime.PredictedChannelUtilizationPercent`, scaling the radio's measured utilization by the presets' time-on-air ratio) and reports False, with the before/after numbers, when the change would exceed the recommended ceiling. It is deliberately advisory, the declared preset is still applied, because hard refusal of an over-budget change is a fleet decision that belongs at control 3 (the Porch validator, which sees the whole channel), not the per-node reconcile, which cannot see the neighbors that share the channel. The measured `AirtimeHealthy` condition remains the ground-truth check on the prediction.

**3. Enforce a fleet budget (Phase 5).** Introduce a channel-scoped budget (a `spec.airtimeBudgetPercent` on a channel intent, or a dedicated `ChannelBudget` CR) and a Porch pre-merge KRM validator that sums the predicted airtime contribution of every node targeted at a channel and fails the PR if the sum oversubscribes the budget. This is the "reject a fleet change that oversubscribes the channel, the way a Kubernetes resource quota rejects an over-commit" idea from the research, and it belongs with `PackageVariantSet` fan-out where the fleet is actually modeled. CEL cannot compute time-on-air, so the check is a KRM validator (or the controller), not an admission expression.

## Honest limits

Airtime is regional and traffic-dependent; the EU enforces a 10% duty cycle in firmware while the US uses dwell-time rules, so the budget default must be region-aware (the operator already reconciles region). Predicted airtime is per-frame; actual channel load depends on the whole neighborhood, including nodes NephMesh does not manage, which is exactly why the observed `airUtilTx` half matters as the ground-truth check on the prediction. And a budget can only shape what this control plane provisions; it cannot quiet a neighbor's mesh.

## Recommended first step

Land control 1 (observe): parse `airUtilTx`/`channelUtilization` into status with an `AirtimeHealthy` condition. It is small, grounded in real device telemetry, hardware-free to develop against the sim, and unblocks both the predictive refusal (control 2) and the committed resilience metrics. Control 3 is scheduled with Phase 5, when a fleet exists to budget.
