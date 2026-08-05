# Security Policy

NephMesh is pre-alpha experimental software. Do not deploy it anywhere that matters yet. It ships with a published [threat model](docs/security/threat-model.md) that openly lists unmitigated risks; read it before relying on anything here.

## Reporting

Report vulnerabilities privately via GitHub security advisories on this repository rather than public issues. Expect an acknowledgment within a week.

## Severity guidance

- **Highest severity: anything that could cause an unintended radio transmission.** The project is receive-only by default and transmit paths do not exist in the code. A code path that could transmit as a side effect, be induced by attacker-supplied intent, or be enabled by an automated loop is treated as critical even if it works as designed. See the transmit interlock principle in the threat model.
- **High: secret exposure.** Channel PSKs, MQTT credentials, and admin-channel keys must never appear in Git, custom resources, packages, or logs. Exported device config contains PSKs in cleartext; any path that logs or stores an unredacted export is a vulnerability.
- **High: exposure of an unauthenticated control surface.** The Meshtastic device API (TCP 4403) has no authentication. A manifest or default that exposes it, or an MQTT broker, outside the cluster is a vulnerability.

## Hardening expectations for real use

The demo defaults are for a local, cluster-internal lab, not production. Before using any of this beyond a laptop:

- Put management transports (device API, broker) on a segmented network; the device firmware offers no authentication of its own.
- Set MQTT credentials and TLS; never reuse the Meshtastic default broker credentials or the default channel PSK.
- Protect intent integrity with branch protection, signed commits, and least-privilege RBAC; whoever can change intent can reconfigure radios.
- Pin images by digest and mirror them if you do not trust upstream registries.
