# Threat model

Security-first analysis of NephMesh, written from the standpoint of an adversary trying to break it. Pre-alpha: this document is honest about what is not yet mitigated. It is maintained per phase, not written once.

Scope today is Phase 1 (the virtual mesh demo). Later sections mark risks as they enter scope. Where a risk is real but unaddressed, it says so rather than implying coverage.

## Method

We enumerate assets, then trust boundaries, then adversaries and their capabilities, then concrete attacks per boundary, each with a status: MITIGATED (with the control), ACCEPTED (with why it is tolerable at this phase), or OPEN (a real gap, tracked). We favor deterministic controls (a wired check, a dropped capability) over documentation promises.

## Assets

- Channel PSKs and any MQTT or admin credentials. Compromise breaks message confidentiality and lets an attacker inject or manage traffic.
- The integrity of desired-state intent (Git, packages, CRs). If an attacker edits intent, they reconfigure the fleet.
- Radio configuration and, in later phases, the ability to cause transmission. The highest-severity asset: an unintended or attacker-induced transmit is both a safety and a legal event.
- Availability of the mesh and of the sensing pipeline.
- The software supply chain (container images, Go modules, kpt functions).

## Trust boundaries

1. Git and the package repository to the cluster (intent flows across here).
2. The cluster API and GitOps agent to the workload (who may apply manifests).
3. The pod to the device API (TCP 4403) and to the MQTT broker.
4. The device to the RF medium (the mesh itself, shared and unauthenticated at the PHY).
5. The host kernel to the container (USB device access, later phases).
6. The build and release pipeline to consumers (supply chain).

## Adversaries

- Network-adjacent attacker on the cluster network or the same LAN as a gateway. Can reach a Service, the broker, or a board's IP.
- RF-adjacent attacker within radio range. Can receive all traffic, transmit on the same band, jam, and replay. This adversary is assumed present and capable; LoRa PHY is open.
- Malicious or compromised dependency (image, module, function).
- Insider or compromised credential with cluster or Git access.

## Boundary analysis

### 3. Pod to device API and broker (Phase 1, in scope now)

- **Unauthenticated device API on 4403.** The Meshtastic client API has no auth; anyone who can open a TCP connection can read and rewrite node config. Additionally observed during the Phase 1 gate: the API is single-client and a new connection force-closes the current one, so an attacker who can reach the port can also deny service to the legitimate applier by connection-flooding. STATUS: ACCEPTED for Phase 1 with controls: the Service is ClusterIP (not exposed outside the cluster), and the port is never made a NodePort or LoadBalancer in any manifest. OPEN beyond Phase 1: when boards are reached over WiFi/TCP (Phase 2), the device API is exposed on the LAN with no auth; the mitigation there is network segmentation (a dedicated management VLAN), documented as a requirement, because the device firmware offers no auth to add.
- **Default MQTT credentials.** Meshtastic ships a well-known default broker username and password (`meshdev` / `large4cats`), and the Phase 1 broker allows anonymous connections. STATUS: ACCEPTED for Phase 1 because the broker is ClusterIP-only and carries a demo channel with no sensitive traffic; the manifest comment says so. OPEN for any real deployment: documented in SECURITY.md that a private broker must set credentials and TLS, and that the default PSK must never be reused. This becomes a wired requirement when the mqtt-bridge package ships (Phase 3).
- **PSK and credential exposure.** Exported device config includes PSKs in cleartext. An applier or operator that logs an export leaks them. STATUS: MITIGATED in design (CRD design mandates redaction and secretKeyRef, never inline), and the Phase 1 demo carries no real PSK. Becomes testable code at Phase 4; until then it is a design control, stated as such.

### 4. Device to RF medium (present at every phase with real radios)

The LoRa PHY is shared, unauthenticated, and receivable by anyone in range. This is a property of the medium, not a bug to fix.

- **Eavesdropping.** All frames are receivable. Channel encryption (AES-256) is a Meshtastic feature the project configures, not something it adds, and it protects confidentiality ONLY where a non-default PSK is set. The Phase 1 demo uses the default channel, whose PSK is a publicly known constant, so demo traffic is effectively cleartext to anyone in range: that is acceptable because the demo carries no sensitive content, but it is not confidentiality. STATUS: MITIGATED only with a non-default PSK; ACCEPTED-as-cleartext for the demo, stated plainly. Metadata (that transmission is occurring, from where, roughly when) is never protected and cannot be.
- **Jamming.** An RF-adjacent attacker can deny the channel. There is no PHY-layer defense. STATUS: ACCEPTED as inherent. The spectrum-sensing side of the project is, usefully, a detector for exactly this: sustained high occupancy on the active channel is what a jam looks like, which is one honest reason the closed loop (Phase 6) is interesting rather than just a demo.
- **Spoofing and replay.** Without application-layer authentication of message origin, frames can be forged or replayed within a channel's trust. STATUS: OPEN and partly out of the project's hands (it depends on Meshtastic's own PKI, which is evolving). The project must not weaken whatever authentication the firmware provides, and must never disable encryption except where a user explicitly opts into an unencrypted licensed mode.
- **Joining the mesh (Sybil, injection).** Meshtastic's default channel PSK is a publicly known constant, so anyone in range can join a default-configured mesh, inject traffic, or flood it with fabricated nodes (Sybil). STATUS: ACCEPTED for the demo (default channel, no sensitive traffic); a real deployment must use a non-default PSK, which is a documented requirement. The deeper limit (a shared symmetric channel key means any authorized member can impersonate any other) is inherent to the channel model and is stated, not hidden.
- **Emission and location (OPSEC).** A transmitting node is a beacon: its emissions can be direction-found and its position located, and the mere fact of transmission is detectable regardless of encryption. STATUS: inherent, ACCEPTED, and turned into a design stance. Receive-only is the default, which is emission control; enabling transmit is a deliberate operator choice with a locational cost. The project must never make a node transmit as a side effect (this is the transmit interlock), precisely because emission is a safety and OPSEC event, not just a functional one.

### 1 and 2. Intent integrity and who may apply

- **Tampered intent.** Anyone who can commit to the package repo or apply to the cluster can reconfigure radios, including, in later phases, changing region or power. STATUS: partially MITIGATED by the platform (Porch's propose/approve lifecycle keeps a human in the loop; Git history is auditable), OPEN where this project owns it (branch protection, signed commits, and RBAC are deployment concerns the docs must require, not defaults the code can set). The highest-consequence version of this attack, inducing transmission, is addressed by the transmit-interlock principle below.
- **Induced reconfiguration (herding via the closed loop).** The Phase 6 closed loop reacts to sensed spectrum: sustained high occupancy on the active channel triggers a channel or preset change. An RF-adjacent attacker can therefore jam the active channel to spike occupancy and drive the mesh onto a channel the attacker has chosen or is ready to jam next, herding the network. This is an attack the feedback design invites, and it is the same failure the control theory warns about: a loop reacting to a noisy, attacker-influenceable signal can be driven or made to oscillate (flap) between channels. STATUS: OPEN, in scope for Phase 6 design, with the mitigations stated up front so the loop is not built naively. Required controls: hysteresis and a minimum dwell time so brief or single-channel occupancy cannot cause a move; rate limiting on automated changes; treating a pattern of "every candidate channel is congested" as evidence of jamming rather than a reason to keep hopping; and, above all, that the loop only ever proposes an intent change through the human-approved Porch lifecycle. The loop never keys a transmitter and never raises power (the transmit interlock), so the worst an attacker can force is a reviewed channel-change proposal, not an emission.

### 6. Supply chain

- **Compromised image or dependency.** STATUS: partially MITIGATED. Images are pinned by tag today and the plan requires digest pinning at build time; CI forbids cloud SDKs in core; DCO and license checks gate contributions. OPEN: no SBOM or image signing yet (planned: SPDX scan at Phase 4). The Phase 1 demo pulls `meshtastic/meshtasticd`, `eclipse-mosquitto`, and `python` from public registries; a consumer who does not trust those should mirror and digest-pin them, documented in the demo README's spirit.

### 5. Host to container (Phase 2+, not in scope yet)

USB device access (the SDR, serial radios) crosses the kernel boundary. STATUS: OPEN, addressed when Phase 2 lands: prefer the device plugin (no privileged pods) over `/dev/bus/usb` hostPath; a privileged pod is explicitly labeled a deviation to be removed. Recorded here so it is not forgotten.

### 7. Workload to cluster API, and pod-to-pod (Phase 1, in scope now)

A compromised pod (for example via the config Job's dependency tree) could try to reach the Kubernetes API with its service-account token, or reach other pods laterally. STATUS: MITIGATED for the demo, and the mitigation is enforced, not just declared. Every pod spec sets `automountServiceAccountToken: false` (none of them talk to the API server), so there is no token to steal. The broker and config Job run with a hardened `securityContext` (`runAsNonRoot`, `allowPrivilegeEscalation: false`, all capabilities dropped, `seccompProfile: RuntimeDefault`, `readOnlyRootFilesystem` on the broker). The node runs non-root with the same profile. A `NetworkPolicy` set (`demo/phase1/manifests/networkpolicy.yaml`) denies all traffic by default, then allows DNS and intra-namespace traffic only, so no external ingress reaches the unauthenticated device API or broker and a compromised pod cannot exfiltrate externally (DNS aside). This is empirically enforced on the demo cluster: kind's kindnet (v0.30+) applies NetworkPolicy, verified by observing that removing the policies lets a cross-pod socket to 4403 succeed and re-applying them blocks all but intra-namespace access. Earlier notes in this repo claimed kindnet did not enforce; that was wrong and has been corrected.

### Container hardening (cross-cutting, Phase 1)

STATUS: MITIGATED and validated. The three demo workloads (node, broker, config Job) run as non-root with escalation disabled, capabilities dropped, and RuntimeDefault seccomp; the broker also has a read-only root filesystem, and no pod carries a service-account token. Supply chain: the Meshtastic CLI is baked into a locally built, version-pinned image (`demo/phase1/images/meshtastic-cli`), so no pod installs from PyPI at runtime, which removes the per-run supply-chain surface and the need for any external egress from the config Job. Remaining OPEN item: the CLI image's own transitive dependency tree is not hash-pinned, and the image is not yet digest-pinned or signed (tracked for Phase 4, which publishes it to a registry with a digest and provenance).

## The transmit interlock principle (highest severity)

An unintended transmission is the worst outcome this project could cause: it is a safety, legal, and security event at once. Precisely stated so the claim is true: **no code path in this project transmits with a software defined radio, and no reconcile loop or automated action can key a radio transmitter or raise transmit power.** The Phase 1 demo does send mesh text messages (`--sendtext`), which is application-layer messaging against a simulated (`--sim`) node with no radio; that is the purpose of a messaging network, not the RF-transmit hazard this principle governs. The hazard is SDR transmit and programmatic radio-power or region escalation.

Standing rules, now enforced by a deterministic gate (`hack/check-transmit.sh`, wired into CI), not by review alone:

- The gate fails the build on any SDR-transmit invocation (for example `hackrf_transfer -t`, a SoapySDR TX stream) or programmatic `txPower` change that is not accompanied by a reviewed `transmit-ok: <reason>` marker. It is proven to catch a planted `hackrf_transfer -t` and to pass only with the marker present.
- Any future transmit capability must be opt-in, off by default, gated behind explicit configuration that names the legal basis (for example a licensed mode with a callsign), and impossible to trigger as a side effect of a reconcile loop or attacker-supplied intent.
- The closed loop (Phase 6) may change channels and presets; it must never gain the ability to enable transmission or raise power as an automated action.

This principle is the single most important line in this document, and it is now wired, not just written.

## Deterministic gates in place

- `hack/check-manifests.sh` (wired into CI and `make check`): fails on any manifest that exposes the control surface (NodePort, LoadBalancer, externalIPs, hostPort, host namespaces, privileged, allowPrivilegeEscalation, hostPath, dangerous capabilities, or an Ingress/Gateway resource). Scans every tracked and not-yet-committed YAML, tolerant of quoting and flow style, so the tidy-form bypasses a first draft missed are closed.
- `hack/check-transmit.sh` (wired into CI): fails on any unmarked SDR-transmit or programmatic-power-escalation entry point (see the interlock section).
- `hack/check-style.sh`, `hack/check-headers.sh`: enforce the no-attribution writing rules and license headers.

## What a hostile reviewer should check next

- That the manifest gate stays ahead of new exposure vectors as Kubernetes adds them (it enumerates known ones; it cannot know future APIs).
- Imperative exposure: scripts using `kubectl expose`, `--type=NodePort`, or `port-forward` on 4403/1883 are not covered by the YAML gate (tracked follow-up: extend the gate to shell).
- That no code path logs an exported config containing a PSK (becomes a test at Phase 4).
- That the default PSK and default MQTT credentials never appear in any package meant for real use.

## Reporting

See [SECURITY.md](../../SECURITY.md). Anything that could cause an unintended transmission is treated as highest severity even if it works as designed.
