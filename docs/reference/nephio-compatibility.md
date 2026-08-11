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

## Re-verified 2026-08-10

A fresh check against upstream (by fetching raw files and repository pages; treat
release-date specifics as lower confidence). The conclusion is that the interop
boundary is stable and a future integration would work, with one served-version fix
already applied and a few tag-hygiene notes.

- Boundary stable. API groups are still `X.nephio.org` at `v1alpha1` only (cfg, common,
  infra, nf_requirements, nf_topology, references, workload); no new groups and no
  version bumps. `condkptsdk` is still the specializer SDK pattern
  (`nephio-project/nephio/krm-functions/lib/condkptsdk`).
- The `nephio-project/api` go.mod has not moved since 2026-08-07: still `go 1.25.6`,
  `k8s.io v0.30.1`, `controller-runtime v0.18.5`. NephMesh's forward skew to
  `k8s v0.34.10` / `controller-runtime v0.22.5` (go.mod directive `go 1.25.0`) is
  therefore unchanged and, at the data boundary, still costs nothing.
- Porch has moved to the kptdev org. Its canonical Go module is now
  `github.com/kptdev/porch` (the `nephio-project/porch` path still resolves).
  `PackageVariant` and `PackageVariantSet` are still served under `config.porch.kpt.dev`,
  with `PackageVariant` at `v1alpha1` and `PackageVariantSet` at `v1alpha2`. One concrete
  fix from this check: the `packages/examples/packagevariant-site1.yaml` example declared
  `PackageVariant` at `v1alpha2` by mistake and is corrected to `v1alpha1` so it would
  apply to a real Porch. The exact served versions should still be pinned against the
  specific Porch release targeted when the Porch gate (Phase 3) is closed.
- The KRM function catalog registry path (`ghcr.io/kptdev/krm-functions-catalog/<fn>`) is
  unchanged, but the tags have advanced: `set-namespace` is at `v0.4.6` and `set-labels`
  at `v0.2.5` upstream, while the Kptfiles here pin the older `v0.4.1` and `v0.2.0`.
  Pinned older tags are immutable and still render, so this is hygiene, not a break; bump
  and re-render (`make check-packages`) when convenient.
- Current release baseline is Nephio R6 (v6.0.0), a stability and security maintenance
  release with no change to the KRM/kpt/Porch data-and-packaging boundary. Its supported
  Kubernetes range is min v1.26, max v1.32.0, and it pins Porch v1.5.6. NephMesh's k8s
  v0.34 client is ahead of that max server, which is fine for data-boundary interop
  (NephMesh does not run inside a Nephio cluster and imports no Nephio Go types) and would
  need reconciling only if the operator were ever run directly against a Nephio R6 API
  server.

Net: nothing at the interop boundary breaks a future integration. The one concrete fix
was the `PackageVariant` served version; the rest is version-tag hygiene and a forward
library skew that only matters if NephMesh ever imports Nephio Go types.

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
  (`k8s.io v0.34.x`, `controller-runtime v0.22.x`, `go 1.25.x`), which is ahead of the
  upstream `api` module's `v0.30.1` / `v0.18.5` and of Nephio R6's supported Kubernetes
  ceiling (max v1.32.0). This is a forward
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
