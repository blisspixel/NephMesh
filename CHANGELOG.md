# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow the roadmap's phase-gated version path (`docs/roadmap.md`).

## [Unreleased]

### Added

- Project foundation: research base, dependency-ordered roadmap with version path, implementation plans (Phase 1, Phase 2, CRD API design, engineering conventions), architecture, FAQ, agent conventions.
- Phase 0 scaffolding: contributing and security policies, license header and style checks, CI workflow, Makefile.
- Phase 1 virtual mesh demo (`demo/phase1/`): simulated Meshtastic node, declarative config Job, private MQTT broker, demo and teardown scripts. Validated end to end against the real `meshtastic/meshtasticd:beta-debian` image (firmware 2.7.26).
