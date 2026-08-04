# Security Policy

NephMesh is pre-alpha experimental software. Do not deploy it anywhere that matters yet.

## Reporting

Report vulnerabilities privately via GitHub security advisories on this repository rather than public issues. Expect an acknowledgment within a week.

## Scope notes

- Channel PSKs and MQTT credentials must never appear in Git, CRs, packages, or logs. A report that they can is a vulnerability.
- Anything that could cause an unintended radio transmission is treated as highest severity. The project is receive-only by default and transmit paths are explicit opt-in; a code path that violates that is a bug even if it works as designed.
