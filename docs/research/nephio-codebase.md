# Research: Nephio codebase conventions (what NephMesh must mirror to integrate later)

Researched 2026-08-04 by cloning `nephio-project/nephio`, `nephio-project/api`, and `porch` at HEAD of main. Purpose: if NephMesh mirrors these languages, layouts, and patterns, its code could integrate or upstream later with minimal rework.

## Languages and toolchain

- **Go 1.25.x** everywhere in the nephio repo (go.mod `go 1.25.6`; porch is on 1.26). One operator (`o2ims-operator`) is Python, so a Python component is not disqualifying, but Go is the house language.
- Key dependencies: `sigs.k8s.io/controller-runtime v0.22.x`, `k8s.io/*` at v0.34.x, kpt function SDK at **`github.com/kptdev/krm-functions-sdk/go/fn`** (org is kptdev now, not GoogleContainerTools), Kptfile types from `github.com/kptdev/kpt/pkg/api/kptfile/v1`. Porch's module moved to `github.com/kptdev/porch` but the API group remains `porch.kpt.dev`.
- Tests: **testify + golden tests, not ginkgo** (ginkgo/envtest appear only in the kubebuilder-scaffolded focom-operator and in porch). Mocks via mockery v3 with per-module `.mockery.yaml`.
- Lint and security: golangci-lint v2.8, gosec. Spellcheck via pyspelling (root `tox.ini`).
- Docker: two-stage builds, digest-pinned `golang:1.25.x-alpine` build stage with `CGO_ENABLED=0`, **`gcr.io/distroless/static:nonroot` final stage**, build context is the repo root so local `replace` directives resolve.
- CI: GitHub Actions for CodeQL and changed-component build validation; **Prow is the real gate** (`.prow.yaml` in-repo): unit, lint, gosec, license-header check, SPDX license scans against an `allowlist.json` (Apache-2.0, MIT, BSD-3-Clause, CC-BY-4.0).

## Repo structure (multi-module monorepo)

No root go.mod; ~18 modules linked by relative `replace` directives. Top level:

```
krm-functions/           one dir per function, each its own Go module
  <name>-fn/             main.go (3 lines) + fn/function.go + fn/testdata/ golden tests
  lib/                   shared SDK module (condkptsdk, kubeobject, kptfile, test)
  pipeline-tests/        golden tests for function *sequences*
controllers/pkg/         ONE module, one package per reconciler + shared clients
  reconcilers/<name>/    self-registers via init() into a registry
  giteaclient/, porch/, resource/, mocks/
operators/               thin deployable binaries
  nephio-controller-manager/   just main.go importing reconcilers for side effect
  focom-operator/              full kubebuilder scaffold + its own kpt-package/
default-*.mk             shared make fragments included by every module's ~15-line Makefile
Makefile                 discovers modules dynamically (find go.mod / Dockerfile)
OWNERS, .prow.yaml, allowlist.json, CONTRIBUTING.md
```

Root Makefile fragments worth copying: `default-go.mk`, `default-go-lint.mk`, `default-gosec.mk`, `default-docker.mk`, `detect-container-runtime.mk` (podman/docker autodetect, targets run in a golang container when available).

## The api repo (CRD types live separately)

`github.com/nephio-project/api` is a single types-only module (no controllers, no CI workflows), one directory per API group with `v1alpha1/` inside:

- Groups: `infra.nephio.org` (WorkloadCluster, ClusterContext), `req.nephio.org` (Interface, DataNetwork, Capacity; directory is `nf_requirements/`, so dir name and group name may differ), `workload.nephio.org` (NFDeployment).
- File conventions per group: `<kind>_types.go` (spec/status plus kubebuilder markers), `<kind>_interfaces.go` (helper methods: `Validate()`, builders), `groupversion_info.go` (Group/Version consts, SchemeBuilder), `condition.go`, `zz_generated.deepcopy.go`.
- Exported `<Kind>Kind` string constants so KRM functions can build ObjectReferences without reflection.
- Generation: controller-gen v0.20.0; `make generate` (deepcopy with `hack/boilerplate.go.txt` header) and `make manifests` (CRDs into `config/crd/bases/` named `<group>_<plural>.yaml`).
- Consumers import as `infrav1alpha1 "github.com/nephio-project/api/infra/v1alpha1"`.

NephMesh mirror: an `api/` module (or repo) with groups like `mesh.nephmesh.io` and `sense.nephmesh.io`, same file layout, same generation targets.

## Key programming patterns

### condkptsdk (the core abstraction for specializer functions)

`github.com/nephio-project/nephio/krm-functions/lib/condkptsdk`. Functions do not implement raw `fn.Runner`; they use `fn.AsMain(fn.ResourceListProcessorFunc(Run))` and construct the SDK with:

```go
condkptsdk.Config{
    For:   corev1.ObjectReference{...},        // the CR this function specializes
    Owns:  map[corev1.ObjectReference]ResourceKind{...},  // children it emits
    Watch: map[corev1.ObjectReference]WatchCallbackFn{...}, // context, canonically WorkloadCluster
    PopulateOwnResourcesFn: ...,               // pure function: for-object + context -> children
    UpdateResourceFn: ...,
}
```

- ResourceKind values: `ChildRemoteCondition`, `ChildRemote`, `ChildLocal`, `ChildInitial`.
- Ownership annotations (constants in the SDK): `specializer.nephio.org/owner` on every child, `specializer.nephio.org/delete` marks children for deletion. The SDK diffs desired vs existing children, so populate functions must be pure and idempotent (functions run repeatedly until conditions converge).
- WorkloadCluster is read via a Watch callback using `kubeobject.KubeObjectToStruct[infrav1alpha1.WorkloadCluster](o)` and errors if missing.
- Typed YAML access: `krm-functions/lib/kubeobject` generics (`KubeObjectExt[T]`, `NewFromKubeObject[T]`, `SetStatus` preserving comments). House style: never marshal Go structs straight to YAML.

### Conditions and readiness

Condition types encode an ObjectReference as `"<group>/<version>.<Kind>.<name>"` (`krm-functions/lib/kptfile/v1`). The SDK maintains a package-level specialize condition and installs readiness gates in the Kptfile when the function is the root. Controller side bridges Kptfile conditions to PackageRevision conditions (`controllers/pkg/porch/condition`); the approval controller gates on them.

### Controller-manager registry

Reconcilers self-register in `init()`:

```go
func init() { reconcilerinterface.Register("repositories", &reconciler{}) }
```

The interface is `reconcile.Reconciler` plus `SetupWithManager(ctx, mgr, cfg interface{}) (map[schema.GroupVersionKind]chan event.GenericEvent, error)`. The manager binary is a thin main.go importing reconciler packages for side effect, with runtime enablement via `--reconcilers=name1,name2` or `ENABLE_<NAME>=true` env vars. Reconcilers embed a shared `resource.APIPatchingApplicator`, use finalizers like `"infra.nephio.org/finalizer"`, and carry kubebuilder RBAC markers.

External services are wrapped in narrow interfaces (`giteaclient.GiteaClient`, porch client helpers) and mocked with mockery. NephMesh equivalent: a `meshtasticclient.Client` interface over the TCP 4403 API, mockable the same way.

### Golden tests

`krm-functions/lib/test` `RunGoldenTests(t, "testdata", processor)`: each `testdata/<case>/` holds the input package (Kptfile plus YAMLs), optional `_fnconfig.yaml`, and `_expected.yaml` (exact output) or `_expected_error.txt`. `WRITE_GOLDEN_OUTPUT=1` regenerates. `RunGoldenTestForPipeline` chains functions. This is the primary test style for anything package-shaped.

## Cloud and provider neutrality (observed, and NephMesh policy)

Nephio's catalog isolates provider-specific material under `distros/` (`sandbox/`, `gcp/`, `openshift/`) and `infra/` (CAPI variants: kind, docker, metal3 baremetal, gcp). The core (`nephio/core`, workloads) is provider-neutral; GCP appears only as one distro among several, reflecting Google's role in the project's history. NephMesh policy: same shape. Local-first (kind, k3s, bare SBCs) is the default and the only thing CI depends on; any cloud-specific material lives in clearly separated distro directories; nothing in core packages, CRDs, or operators may import or assume a specific cloud provider.

## Contribution mechanics (adopt from day one to make upstreaming painless)

- **Apache-2.0** with per-file headers: `Copyright <year> The Nephio Authors.` plus the standard boilerplate; CI-enforced upstream. NephMesh should use the same format with its own name (`The NephMesh Authors`) so files are a header-swap away from upstream compliance.
- **DCO, not CLA**: every commit `Signed-off-by:` (`git commit -s`).
- Prow-style `OWNERS` file, `CONTRIBUTING.md`, `SECURITY.md` at root.
- The api repo takes no issues directly; issues go to the main repo prefixed `api: `.
- `nephio-experimental` is the org-level home for incubating components (formal proposal process not verifiable from the repos; likely wiki/governance).

## Summary: the NephMesh compatibility checklist

1. Go 1.25.x, controller-runtime v0.22.x, kptdev fn SDK, testify plus golden tests, mockery, golangci-lint, gosec.
2. Multi-module layout: `krm-functions/<x>-fn` (module each), `controllers/pkg` (one module, registry pattern), `operators/<manager>` (thin main), shared `default-*.mk` fragments, distroless images built from repo root.
3. CRD types in a dedicated `api` module: one dir per group, `_types.go` and `_interfaces.go` split, controller-gen generation, exported Kind constants.
4. Specializer functions built on condkptsdk with For/Owns/Watch and the `specializer.nephio.org/*` annotation scheme, watching `WorkloadCluster` for site context.
5. Apache-2.0 headers on every file, DCO sign-off on every commit, provider-neutral core with distro-isolated cloud material.
