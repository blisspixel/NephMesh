# Phase 4 implementation plan: the MeshtasticNode operator

Target: the 0.4 gate, a Go operator that reconciles a `MeshtasticNode` custom resource against a real (or simulated) device, correcting drift and surviving the device's reboot-on-write. This is the project's flagship deliverable: no Meshtastic Kubernetes operator exists today. Sources: `docs/research/nephio-codebase.md`, `docs/plans/crd-api-design.md`, and the August-2026 research summarized inline below with the caveats it carried. Where research flagged uncertainty, this plan says so; nothing here is claimed to work until it is executed.

## Toolchain baseline (verify against go.mod at build time)

- Go 1.25.x; `sigs.k8s.io/controller-runtime` v0.22.x (pins k8s.io/* v0.34.x); Kubebuilder 4.9 line, `go/v4` plugin; controller-tools v0.21.0. These match the Nephio codebase conventions and were current as of August 2026.
- testify plus golden tests for pure logic; envtest for controller-against-apiserver; testcontainers-go with `meshtastic/meshtasticd:*` in `--sim` mode for device integration. Not ginkgo for the pure layer.

## 1. Talking to the device: decision

The device speaks a binary protobuf stream on TCP 4403 (4-byte framing, `AdminMessage` protobufs for config and reboot), it is single-client, and it reboots on write. Three options were evaluated:

- (a) A Go protobuf client. No stable, feature-complete Go client exists as of August 2026: `lmatte7/gomesh` has the widest AdminMessage surface but is self-described as under development; `meshnet-gophers/meshtastic-go` has clean transport but explicitly lacks admin/config-set/reboot. Both warn contracts will change.
- (b) Exec the Python `meshtastic` CLI (the `--export-config` / `--configure` YAML round-trip Phase 1 already uses). Known reliability bug (meshtastic/python #891, Jan 2026): channels, keys, and favorites can silently fail to restore and `--configure` can report false success. Batching with `--begin-edit`/`--commit-edit` collapses to one reboot.
- (c) The CLI as a sidecar or per-device agent, isolating the flaky link and the CPython runtime from the controller.

Shipped (option b, refined): the operator execs the CLI for scalar config (`--configure` from a file so the broker password never reaches argv), but reads config through a bundled helper (`hack/mesh-export.py`, which streams the config on connect rather than re-requesting it, since `--export-config` hangs over serial) and applies channels through a second helper (`hack/mesh-apply.py`) using the device library's per-channel `writeChannel`, not `--configure`. That sidesteps the #891 channel-restore bug entirely, keeps channel keys in a file rather than on the command line, and lets channel drift be detected by key hash (the helper emits the channel set the CLI's `--export-config` omits). The channel apply is validated against `meshtasticd --sim`.

**Decision: (c) for now, with (b)'s round-trip inside it, and a migration path to (a) later.** The controller stays a pure Go controller-runtime binary and calls a CLI sidecar. This reuses the validated Phase 1 apply logic, isolates flakiness and reboot-driven disconnects, and avoids embedding CPython in the controller. **Read-back verification after every apply is mandatory, not optional**, because `--configure` can falsely report success. The medium-term path is a small in-house Go client on the meshtastic protobufs that sends `AdminMessage{begin_edit, set_config..., commit_edit, reboot_seconds}` directly, removing the CPython dependency at the cost of owning protocol drift. Send reboot over the 4403 path, never the WiFi HTTP endpoint (firmware #9873, that path is broken).

## 2. Reconciling a device that reboots on write (do not block the worker)

The reconcile is a non-blocking state machine driven by `RequeueAfter`, never a `sleep` in the worker goroutine:

1. Connect, export live config, compute the minimal diff against `.spec`. Empty diff: set `Ready=True`, `Configured=True`, return (optionally `RequeueAfter` a drift-check interval).
2. Non-empty diff: open a batched edit, apply only the changed sections, commit (or apply then explicit reboot). Set `RebootPending=True` with reason `ConfigApplied`, stamp `status.observedGeneration`, and `return ctrl.Result{RequeueAfter: ~20 to 45s}`. Exit.
3. Next pass: try to reconnect. If refused (mid-reboot), `return RequeueAfter` with bounded backoff. Because 4403 is single-client and fragile under rapid reconnection, pace reconnects with `RequeueAfter` (which bypasses the built-in rate limiter via `AddAfter`); return an `error` only for genuinely unexpected failures (that path does go through the exponential limiter).
4. Reachable again: re-export, verify the diff is now empty (read-back). Converged: clear `RebootPending`, set `Ready=True`. Not converged after a retry budget: `Degraded=True`.

Serialization: the controller-runtime workqueue already guarantees one in-flight reconcile per object key, so one `MeshtasticNode` maps to one device session with no concurrent reconciles. To also serialize across different CRs pointing at the same physical device, either model one CR per device (preferred) or add a per-device keyed lock on the reconciler struct (which is shared across goroutines, so any shared state must be goroutine-safe). Keep `MaxConcurrentReconciles` modest; concurrency helps across devices, not for one device.

## 3. Status and validation conventions (2026)

- Status via server-side apply (`r.Status().Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner("nephmesh-meshtasticnode"))`), not `Status().Update()`, so multiple condition writers do not clobber each other. Be consistent: do not mix Update and Apply on the same fields.
- Conditions are `[]metav1.Condition` with `+listType=map` / `+listMapKey=type`, positive-polarity CamelCase types (`Reachable`, `Configured`, `RebootPending`, `Ready`, optional `Degraded`), tri-state status, required CamelCase `Reason`, and per-condition `observedGeneration` so GitOps agents can tell a spec has been reconciled. Manage with `meta.SetStatusCondition`; guard writes with `equality.Semantic.DeepEqual` to avoid reconcile loops.
- Validation in CEL (`x-kubernetes-validations`, GA since 1.29), not a webhook, so the operator ships with no TLS certs or extra pod. The "exactly one of tcp/serial/viaGateway" rule lives at the `connection` parent so `self` sees all three: `[has(self.tcp), has(self.serial), has(self.viaGateway)].filter(x, x).size() == 1`. Use transition rules (`self == oldSelf`) to make identity fields immutable. CRD Validation Ratcheting (GA 1.33) lets unrelated updates through when the failing field is unchanged, which pairs well with a churning upstream firmware schema.
- Enum-like fields that mirror firmware (`region`, `modemPreset`, `role`) are open strings with a shape `Pattern`, plus Go constants and a graceful default branch, not closed `Enum` markers. A closed enum would reject any new firmware value until the CRD is re-released. This is the 2026 forward-compatibility recommendation.
- Finalizer (`mesh.nephmesh.io/finalizer`) quiesces the device link on delete, best-effort with a bounded timeout so a dead device never wedges deletion. `deletionPolicy: Retain` (default) stops managing; `Wipe` factory-resets (later, careful ordering).

## 4. Secrets (channel PSKs, MQTT credentials)

- CRDs reference secrets by `secretKeyRef`, never inline. The controller reads the Secret via the cached client at reconcile time and watches it (index + `EnqueueRequestsFromMapFunc`) so rotation re-triggers reconcile. Values stay in local variables only.
- Recommended provisioning path: External Secrets Operator. In a PackageVariant fan-out, the cloned package carries only an `ExternalSecret` reference, so one template fans out to N sites unchanged; Sealed Secrets and SOPS bind ciphertext to a specific key and force per-site re-encryption, fighting the clone-one-to-many model. (This is a reasoned inference from how PackageVariant injection works, not an official Nephio prescription; Sealed Secrets and SOPS remain compatible, never dependencies.)
- Leak prevention: never put PSKs in `.Status`, events, or logs; wrap secret values in a redacting type whose `String()`/`MarshalJSON` emit a placeholder with an explicit reveal accessor used only where the device config is written; treat the exported config (which contains cleartext PSKs) as sensitive output, never logged. The threat model requires this.

## 5. Testing without hardware

- Golden tests (testify, `testdata/`) cover pure transforms: the diff/apply-plan generator and any condkptsdk specializer functions. Hermetic, run on every PR.
- envtest covers controller-against-apiserver: CRD install, CEL accept/reject, SSA status writes, condition transitions, finalizer add/remove. The device client sits behind an interface with an in-memory fake that simulates "reboot then brief unreachable window then new state" to drive the state machine deterministically.
- Integration behind a CI tag: testcontainers-go runs `meshtastic/meshtasticd:latest` with `-s` (sim, no hardware), exposing 4403; a `MeshtasticNode` CR points at it and asserts convergence and reboot handling. This is the same image the Phase 1 gate validated.

## 6. Packaging (the free5gc-operator / oai-operator pattern)

Ship the operator as a kpt package (`operators/meshtastic-operator` plus a deployment package): CRD, RBAC (including the `.../status` verb for SSA), and the controller Deployment, kustomize-style with a `namePrefix`. Fan the operator out to workload clusters with a `PackageVariantSet` over `WorkloadCluster` labels; instantiate per-site `MeshtasticNode` CRs via `PackageVariant`. Mirrors how OAI and free5GC ship their operators.

## 7. Hard preconditions and open items

- The `nephmesh.io` API group is a provisional placeholder, not a domain that has to be owned. An API group is a stable DNS-style string; for a private, pre-alpha experiment with no external consumers, using it does not require registering the domain, and this does not block Phase 4. Keep the CRDs at `v1alpha1` so a group rename stays cheap, and revisit before any public or 1.0 release (buying the domain then, or picking a group that is actually controlled, becomes worthwhile once other people depend on the API).
- No stable Go Meshtastic client exists; the sidecar-CLI path is the safe start, verified read-back is mandatory.
- Confirm the exact SSA-for-status call against the controller-runtime v0.22 GoDoc for the pinned patch (research flagged it as first-class but under-documented).
- kro and Crossplane were assessed and set aside for the device loop: they orchestrate declarative resource composition well but are weak at stateful reconciliation against a flaky single-client endpoint, which is exactly what a hand-written controller is for. They remain optional as a higher-level per-site composition layer.
