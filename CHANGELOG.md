# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow the roadmap's phase-gated version path (`docs/roadmap.md`).

## [Unreleased]

### Added

- Phase 4 operator plan (`docs/plans/phase-4-operator.md`) from August-2026 research: the CLI-via-sidecar device path with mandatory read-back verification, a non-blocking reboot-aware reconcile state machine, single-client serialization via the workqueue, server-side-apply status with `metav1.Condition`, CEL validation over webhooks, open-string enums for firmware-churn tolerance, External Secrets Operator for the GitOps secrets flow, and testcontainers plus `meshtasticd --sim` for hardware-free testing.
- Expert-panel review outcomes folded into the docs: the control-plane-is-not-in-the-field clarification and mesh autonomy (README and FAQ), PACE and DIL and EMCON framing, a defined resilience metric (message delivery ratio, time to failover, control-plane-independence) with matching validations, and threat-model additions (induced-reconfiguration via the closed loop, Sybil and default-PSK mesh join, emission and direction-finding OPSEC).
- Scope broadened to radio systems generally with a radio-driver abstraction (Meshtastic as the first driver, SDR as a co-equal pillar, room for wider LoRa and multi-transport backhaul); north-star vision of a self-adapting, multi-transport, cockroach-resilient fabric documented honestly as the destination, not a current claim.
- Standalone Porch install path researched for the 0.3 gate (v1.6.2 via kpt package on one kind cluster, about 1 vCPU, no Config Sync required, WSL2 and local-gitea notes) and folded into the roadmap.
- Phase 3 kpt packages: `mesh-gateway` and `mqtt-bridge` blueprints following the Nephio catalog pattern (Kptfile pipeline, package-context, placeholder WorkloadCluster), namespace-neutral with short service names. A render gate (`hack/check-packages.sh`, `make check-packages`) renders every package and fails on pipeline errors, wired into CI; a pinned kpt-runner image gives local Windows and macOS the same toolchain. PackageVariant and PackageVariantSet examples plus a Porch registration and propose/approve guide.
- Security-first threat model (`docs/security/threat-model.md`) written from an adversary's standpoint, honest about unmitigated risks, with a transmit-interlock principle now enforced by a deterministic gate.
- `DISCLAIMER.md`: research-project framing and strong user-responsibility terms; the project makes no claim to know the laws that apply to any user.
- Deterministic security gates wired into CI and `make check`: `hack/check-manifests.sh` (fails on any manifest exposing the control surface, tolerant of quoting and flow style, scanning all tracked and untracked YAML) and `hack/check-transmit.sh` (fails on unmarked SDR-transmit entry points). Both proven to catch planted violations.
- Phase 1 demo hardening, validated by the passing gate: non-root pods with dropped capabilities, no privilege escalation, RuntimeDefault seccomp, no service-account tokens; a read-only root filesystem on the broker; default-deny NetworkPolicies allowing only DNS and intra-namespace traffic (enforced by kindnet on kind v0.30+, verified empirically); and a locally built, version-pinned CLI image that removes all runtime PyPI dependence.

### Changed

- Legality notes across the docs are now uniformly hedged, US-scoped, marked as informal non-lawyer research, and linked to the disclaimer. No document asserts legality as settled fact.
- Phase 0 close-out: the open decisions in the plan docs are resolved with evidence (GitHub owner, `deletionPolicy` enum over a boolean, secrets story, SPDX and DCO and lib direction); explicit code-quality bars and cross-platform rules added to `AGENTS.md`. The `nephmesh.io` API group is a provisional placeholder (no domain owned; renaming stays cheap at v1alpha1) and revisited only before a public or 1.0 release.
- Recorded the Meshtasticator attempt: multi-node simulation is not practical headless in containers today, so CI stays single-node; revisit paths noted.

## [0.1.0] - 2026-08-04

The Phase 1 gate: a virtual Meshtastic mesh node deployed, declaratively configured, observed on MQTT, and torn down, on a plain Kubernetes cluster. Gate transcript and findings: `demo/phase1/README.md`.

### Added

- Project foundation: research base, dependency-ordered roadmap with version path, implementation plans (Phase 1, Phase 2, CRD API design, engineering conventions), architecture, FAQ, agent conventions.
- Phase 0 scaffolding: contributing and security policies, license header and style checks, CI workflow, Makefile.
- Phase 1 virtual mesh demo (`demo/phase1/`): simulated Meshtastic node (`meshtasticd --sim`, firmware 2.7.26), idempotent declarative config Job (export, subset-compare, apply on drift, explicit reboot, verify), private Mosquitto broker, demo and teardown scripts. Gate passed on kind; config persistence across pod restarts validated.
