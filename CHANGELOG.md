# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow the roadmap's phase-gated version path (`docs/roadmap.md`).

## [Unreleased]

## [0.1.0] - 2026-08-04

The Phase 1 gate: a virtual Meshtastic mesh node deployed, declaratively configured, observed on MQTT, and torn down, on a plain Kubernetes cluster. Gate transcript and findings: `demo/phase1/README.md`.

### Added

- Project foundation: research base, dependency-ordered roadmap with version path, implementation plans (Phase 1, Phase 2, CRD API design, engineering conventions), architecture, FAQ, agent conventions.
- Phase 0 scaffolding: contributing and security policies, license header and style checks, CI workflow, Makefile.
- Phase 1 virtual mesh demo (`demo/phase1/`): simulated Meshtastic node (`meshtasticd --sim`, firmware 2.7.26), idempotent declarative config Job (export, subset-compare, apply on drift, explicit reboot, verify), private Mosquitto broker, demo and teardown scripts. Gate passed on kind; config persistence across pod restarts validated.
