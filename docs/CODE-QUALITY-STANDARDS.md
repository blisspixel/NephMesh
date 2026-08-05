# Code quality standards

The bar for this project: code good enough that a critical-infrastructure or mission user could trust it. That is aspirational, and stating it does not make it true, so this document is concrete about what "exceptional" means here, what is already enforced, and what is honestly still a gap. It is derived from August-2026 research into Go, shell, Docker, and Kubernetes-operator reliability and into assume-breach security testing; sources are in the research notes.

Two principles run through everything:

- **A wired gate beats an instruction.** If a rule matters, it is a CI check that fails the build, not a sentence someone must remember. The gaps below become gates, not guidelines.
- **Assume breach.** Tests start from a compromised position (a hostile custom resource, a spoofed device, a tampered intent, a pod that is already owned) and prove a control holds, rather than assuming it.

## Already enforced (the current floor)

- Go: `go build`, `go vet`, `golangci-lint` (v2.12.2, standard set), unit tests, the race detector, and an 80% meaningful-coverage gate (generated and cmd/main excluded), plus a real integration test that drives the reconcile loop against a live `meshtasticd --sim`. Native fuzzing of the untrusted-input parsers, and `govulncheck` reachability scanning on every module.
- Repo gates: license headers, writing style (no emojis, em dashes, or attribution), manifest control-surface exposure, the transmit interlock, SHA-pinned GitHub Actions (`hack/check-actions.sh`), ShellCheck on the gate scripts, and kpt package render. DCO sign-off. One command runs them all: `sh hack/check-all.sh`.
- Containers: multi-stage, `CGO_ENABLED=0`, non-root. Manifests: non-root, dropped capabilities, no privilege escalation, seccomp, NetworkPolicy isolation.

## The bar, by area

### Go reliability

- **Tests-first, table-driven**, testify plus golden tests, no coverage regression. The pure logic (config diff, state machine) stays at or near 100%.
- **The race detector** (`go test -race`) runs in CI. An operator is concurrent by construction; races are the highest-severity, least-reproducible bug class.
- **`govulncheck`** runs in CI on every module. It uses reachability analysis, so it stays low-noise against the large Kubernetes dependency tree, and it catches standard-library vulnerabilities a Go upgrade would fix.
- **Fuzzing** covers the parsers that consume untrusted data (the device CLI output and the config diff): the property is that parsing never panics and always terminates, so malformed or hostile device output cannot crash the operator. Crashers become permanent regression tests in `testdata/fuzz`. Seed corpora and a short per-PR run gate; longer runs are scheduled.
- **Goroutine-leak detection** (`goleak`) on packages that start goroutines or subprocesses (the device exec path, the controller).
- **Error conventions:** wrap with `%w` only when callers are meant to `errors.Is`/`As` through it, else `%v`; never branch on `err.Error()`; reconcilers branch on typed API errors (`apierrors.IsNotFound`, `IsConflict`). `errorlint` enforces this.
- **Context propagation:** the reconcile context threads through every downstream call, including the CLI exec, never `context.Background()` inside `Reconcile`.

### Kubernetes operator correctness

- **Status is `metav1.Condition` with `observedGeneration`**, positive-polarity types, machine-readable reasons, so convergence is observable and stale reconciles are not acted on.
- **Server-side apply for status and owned objects** rather than read-modify-write update, to avoid lost updates and field clobbering (a tracked upgrade from the current `Status().Update`).
- **Idempotency is tested:** a second reconcile of an unchanged object returns an empty result, no error, and mutates nothing. The bounded apply-and-reboot loop is the kind of retry logic this test protects.
- **Validation lives in the CRD** via CEL (`x-kubernetes-validations`) and OpenAPI markers, fail-closed in the API server, no admission webhook (a down webhook is a self-inflicted outage). Every string and list field carries a length or item bound.
- **envtest** is the primary controller-test tier (a real API server and etcd), because the fake client does not reproduce CRD validation, optimistic-concurrency conflicts, status subresource semantics, or watch timing. The fake client stays for pure branch-logic tests.
- The manager keeps leader election, a signal handler, health probes, and secured metrics; the logger runs in production mode in the released binary (not development mode).

### Assume-breach security testing

Every threat-model boundary should have a test that proves the control, not assumes it. In priority order:

- **No secret in logs, status, or events:** inject a sentinel PSK through the reconcile and device-apply path with a captured logger and assert it never appears anywhere. The real surface is device output flowing into a wrapped error that gets logged; secret material is wrapped in a redacting type before it can reach any observable surface.
- **NetworkPolicy actually blocks** cross-namespace access to the device API, proven with a negative test (an attacker pod in another namespace cannot dial 4403) and a positive control (an in-namespace pod can), on a pinned CNI so the proof does not silently rot.
- **The transmit interlock cannot be bypassed:** the config builder is fuzzed and property-tested over arbitrary specs to prove it never emits a transmit-power key, and the gate is extended to imperative exposure in scripts.
- **RBAC stays least-privilege:** a golden test asserts the operator's effective permissions equal an expected allowlist (any new grant fails the test), plus a policy rule forbidding wildcards and secret-write.
- **Hostile custom resources are rejected at admission** (envtest): a leading-dash host, oversized fields, unicode tricks, an out-of-range port, or two transports set at once must all be refused by the pattern, length, and CEL validations.
- **No default credentials in any shipped package:** a policy test asserts the Meshtastic default channel PSK and the default broker credentials never appear in a package meant for real use.
- **Failure paths are tested deterministically** at the fake-device seam: unreachable mid-reconcile, reboot storms, a device that never converges (which must go Degraded, not loop forever, the anti-DoS property), and flapping config. Full cluster chaos frameworks are deliberately not used; deterministic fault injection is more reliable and less flaky.

### Supply chain and reproducibility

- **GitHub Actions pinned by commit SHA** (not mutable tag), enforced by `hack/check-actions.sh` and kept current by Dependabot. Least-privilege top-level `permissions` on each workflow is the remaining piece.
- **Network artifacts verified by checksum** before use (the kpt download is verified, not `curl | tar` with sudo).
- **Reproducible builds:** `-trimpath` and version stamping; committed `go.sum`; base images digest-pinned; a dependency-update tool keeps the pins fresh.
- **Image and dependency scanning** (trivy for the image, `govulncheck` for Go, `pip-audit` for the bundled CLI tree), gating on high and critical with a documented exceptions path.
- **When images are published:** an SBOM (syft) and keyless signing plus provenance (cosign, SLSA), verified in consumers. This lands with image publishing, not before.

### Shell and container gates

- **ShellCheck** gates `hack/*.sh`; the gate scripts are the safety mechanism, so linting them is cheap and high-value. Pipelines use `pipefail` where a masked producer failure would be dangerous.
- **hadolint** lints the Dockerfile. Note deliberately: the operator image cannot be distroless-static or scratch, because the operator execs the Meshtastic CLI in-process and therefore needs a Python runtime; the right move is a digest-pinned slim base, not a wrong "fix" to scratch.

### Process and documentation

- **Architecture Decision Records** in `docs/adr/` capture the why behind load-bearing choices (the CLI-in-image versus sidecar decision, requeue backoff, the transport oneof, the provisional API group), as durable evidence.
- **A real coordinated-disclosure process** (GitHub Private Vulnerability Reporting, supported versions, response SLA, embargo), not just an email.
- **Release discipline:** SemVer, signed tags, a changelog with a security category.

## What is deliberately out of scope (so nobody over-engineers)

Honesty about overkill is part of quality. The research explicitly de-scoped, and so does this project until a concrete need appears: SLSA L3 hermetic builds and custom builders; OSS-Fuzz onboarding (a nightly fuzz run suffices); admission webhooks (CEL covers the validation need); Ginkgo BDD and `gopter` (table tests, envtest, and `rapid` suffice); running both trivy and grype; a Dockerfile `HEALTHCHECK` (the kubelet ignores it; probes are in the Deployment); the deprecated `kube-rbac-proxy`; CLA infrastructure and release automation; and chasing low-weight scorecard checks. Cluster chaos frameworks are avoided in favor of deterministic fault injection.

## How this rolls out

These are tracked as prioritized roadmap items under Phase 4 hardening rather than done all at once, so each lands with its own validation. Already in: the race detector and `govulncheck` in CI, ShellCheck, SHA-pinned Actions with a gate and Dependabot, a checksum-verified kpt download, the fuzzers, and the fix for the production logger mode. The tier order for what remains, highest leverage first: envtest as the controller-test tier, reproducible-build flags and digest-pinned bases, least-privilege workflow `permissions`, then the assume-breach control-proving tests, then SBOM and signing when publishing begins.
