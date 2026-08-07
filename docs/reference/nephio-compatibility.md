# Nephio compatibility posture

NephMesh is an independent project that consumes Nephio as a third party, the same
way other projects ship their own Nephio-ready packages. It is not affiliated with
the Nephio project or the Linux Foundation. This doc records, honestly and with a
check date, where the code deliberately stays compatible with upstream Nephio and
where it diverges on purpose, so a later decision to contribute upstream (or a
decision not to) is made with eyes open. It complements the deeper conventions
research in [nephio-codebase.md](../research/nephio-codebase.md).

Compatibility here means "could integrate with minimal rework," not "depends on."
The load-bearing rule from AGENTS.md still holds: nothing in NephMesh may take a
runtime dependency on Nephio or on a control plane in the field.

## What was checked, and when

Verified 2026-08-07 against the upstream repositories:

- [nephio-project/api](https://github.com/nephio-project/api): the Nephio Go API
  objects, CRDs, and KRM-manipulation libraries.
- [nephio-project/nephio](https://github.com/nephio-project/nephio): the main
  project (operators, functions, packaging).

Findings from `nephio-project/api`:

- API groups follow `X.nephio.org` at `v1alpha1` (for example `workload.nephio.org`,
  `infra.nephio.org`, `nf_requirements`, `nf_topology`, `references`), each in its own
  directory, with generated CRDs under `config/crd/bases`. Apache-2.0.
- The `api` module's `go.mod` at the time of the check: `go 1.25.6`,
  `k8s.io/api` and `k8s.io/apimachinery` at `v0.30.1`, `sigs.k8s.io/controller-runtime`
  at `v0.18.5`. These are worth re-checking before any direct-import work, since they
  move.

## Where NephMesh already aligns

- **A separate `api` Go module.** NephMesh keeps its CRDs and types in a standalone
  `api/` module (`mesh.nephmesh.io/v1alpha1`), mirroring Nephio's split of the API
  objects into their own module so they can be imported and manipulated by KRM
  functions without pulling in operator code.
- **Domain-suffixed API groups at `v1alpha1`.** Same shape as `X.nephio.org/v1alpha1`,
  with generated CRDs and kubebuilder markers.
- **controller-runtime operators.** The `MeshtasticNode` operator is a standard
  controller-runtime manager with envtest coverage, matching how Nephio builds its
  controllers.
- **Configuration-as-Data and kpt packaging.** Packages are plain KRM YAML mutated by
  Kptfile pipelines (no Helm-style templating), specialized per site with
  `PackageVariant` and `PackageVariantSet` at `config.porch.kpt.dev` (`v1alpha1` and
  `v1alpha2`), and driven through Porch's propose/approve lifecycle. These porch and
  kpt API versions are independent of the Go library versions below.
- **The intent direction.** Making `MeshtasticNode` the compiled output of a
  higher-level `CommunicationIntent` (see [ADR 0001](../adr/0001-intent-as-an-outcome-envelope.md))
  is the Nephio move (intent expressed as KRM data, reconciled continuously) applied one
  layer lower, to the radio substrate. It is meant to be expressible as KRM so it fits
  the same Porch and specializer machinery.
- **Engineering conventions.** Apache-2.0 file headers, DCO sign-off, golden tests with
  testify, and Go 1.25.x, per AGENTS.md and the codebase research.

## Where NephMesh deliberately diverges, and why

- **Group domain `mesh.nephmesh.io`, not `*.nephio.org`.** NephMesh is not affiliated
  with Nephio, so it does not put resources in the `nephio.org` group. The `.io` group
  is provisional and kept at `v1alpha1` so a later rename stays cheap.
- **Newer Kubernetes libraries.** NephMesh tracks the latest GA line
  (`k8s.io v0.34.x`, `controller-runtime v0.22.x`, `go 1.25.12`), which is ahead of the
  upstream `api` module's `v0.30.1` / `v0.18.5` at the check date. This is a forward
  skew, not a lag. It is fine because NephMesh interops at the KRM and package boundary,
  not by importing Nephio's Go types.

## The one thing to watch: the interop boundary

The compatibility is at the **data and packaging boundary** (KRM resources, kpt
packages, Porch), which does not care about Go library versions. NephMesh does not
import `github.com/nephio-project/api` Go types today, so the version skew above costs
nothing.

If that ever changes, that is, if NephMesh wants to import Nephio's Go API types or
reuse a Nephio specializer library directly, the k8s and controller-runtime versions
would have to be reconciled first (either NephMesh pinning back, or upstream moving
forward). Until then, the seam to keep clean is:

- resources stay KRM-shaped and Porch-consumable,
- any specializer functions are written against the kpt function SDK and the
  condition-based pattern Nephio uses (`condkptsdk`), not ad hoc,
- packages stay provider-neutral and local-first,
- and the management plane stays out of the data plane at runtime.

Keeping that seam clean is what makes "tie back to Nephio later" a real option rather
than a slogan.

## Re-verify before relying on this

Upstream moves. Before any direct-integration work, re-check the two repositories above
for current API groups, the porch and PackageVariant served versions, and the `api`
module's k8s and controller-runtime versions, and update this doc with a fresh check
date. Treat this as a living reference, like the [regulatory matrix](regulatory-matrix.md).
