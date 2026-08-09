# Edge AI advisor: a local model reasoning over sensed spectrum

This demo puts a language model in the sensing loop: a model reads the sensed
spectrum (occupancy plus classified emissions) and the current radio config, and
proposes a mesh action with a rationale. It runs against a local Ollama server, so
it works with no cloud, which is the point for comms that must survive a carrier
outage.

## What it is, and what it is not

It **is** an advisor. The model proposes; it never actuates. The recommendation
is report-only, for a human (or, later, the safety kernel of ADR 0002) to
approve. This keeps the project's separation intact: the AI proposes, the
reconcile loop enforces, a human approves. The model is not the control loop.

The output is guarded, not trusted. A recommendation to change to a preset
outside the approved set is downgraded to "investigate"; an unrecognized action
becomes "hold"; a missing confidence becomes "low". A weak or hallucinating model
therefore cannot propose anything unsafe. The whole sensing path is receive-only.

## The loop

1. **Sense** with the SDR (`hackrf_sweep`, receive-only).
2. **Reduce and classify** with `nephmeshctl`: per-band occupancy, peak, noise
   floor, and the classified emissions (packet-shaped LoRa versus continuous or
   wideband interference).
3. **Advise**: hand that situation to a local model and parse a structured,
   validated recommendation.

```sh
nephmeshctl advise -f sweep.csv -model qwen2.5:14b -ollama-url http://localhost:11434
```

`-num-gpu 0` forces CPU inference so a capable model runs in system RAM on a
memory-tight edge host (a Jetson shares RAM between CPU and GPU, so a model that
will not fit the GPU alongside the SDR still fits on the CPU).

## Captured runs

Both are real, on the bench (2026-08-09), reasoning over a live HackRF Pro sweep
of the 902-928 MHz band (~33% occupancy, a wideband emission detected).

Capable model (qwen2.5:14b on a nearby GPU):

```
recommendation: hold (confidence medium)
  rationale: Occupancy is slightly above recommended ceiling but not significantly
  so. The wideband emission could be a false positive or transient.
```

That is the correct, honest call for a single ambiguous reading: it noted the
occupancy over the ceiling, referenced the classified wideband emission, treated
it skeptically, and held.

Edge-resident model (tinyllama on the Jetson itself, the only model that fits
alongside the SDR in 7.4 GB of shared memory):

```
recommendation: hold (confidence low)
  rationale: model returned an unrecognized action; defaulting to hold. ...
```

The tiny model's structured output was malformed, and the guardrail safely
downgraded it to hold rather than trusting it. That is the safety design working
on real hardware: a bad model cannot cause a bad proposal.

## Honest limits

- Model quality and memory trade off on the edge. A capable model reasons well
  but needs real memory; the model that fits a constrained sensor host is weak
  enough that the guardrails, not the model, carry the safety. That is by design,
  but it means the interesting reasoning currently wants a capable model nearby
  rather than on the smallest node.
- This is advisory research, not autonomy. Wiring a recommendation to actuation
  stays gated behind the safety kernel (ADR 0002). The value here is that the
  proposal is grounded in real sensed spectrum and cannot escape the approved,
  report-only envelope.
