# Engineering conventions and repo scaffolding plan

Status: Phase 0 deliverable, drafted 2026-08-04. Primary source: `docs/research/nephio-codebase.md`.
Goal: keep NephMesh upstream-compatible with the Nephio ecosystem and provider-neutral, so any
component is a header-swap and a module-path change away from fitting the Nephio house style.

## 0. Language mix and why the repo is shell-heavy today

Nephio's codebase is roughly 90% Go, with small amounts of Makefile, Python, shell, and Dockerfile.
NephMesh should converge toward that same profile, because the substance of the project, the
Meshtastic operator and the specializer functions, is Go (Phases 4 and 5). Until then the repo is
deliberately shell and YAML: what exists so far is CI gates (`hack/*.sh`), demo glue
(`demo/phase1/scripts/*.sh`), and Kubernetes manifests, which is exactly the category Nephio also
implements in shell and Makefile. This is not drift; it is the pre-Go phase of the roadmap.

The load-bearing rule that keeps future integration easy: no domain logic lives in shell. The shell
here is orchestration and enforcement only (run a demo, fail a build). The moment reconciliation
logic appears, it is Go, structured per the module layout below, so that a future contribution to
Nephio is a module-path and header change, not a rewrite. When the operator lands, Go becomes the
dominant language by volume, matching the upstream profile.

## 1. Target repo layout (final state)

Single repo. Nephio splits across `nephio`, `api`, `porch`, and `catalog` repos because it has many
vendors and independent release cadences; NephMesh has one maintainer and seven phases, so a
multi-module monorepo gives the same module boundaries without cross-repo version juggling. If the
`api` module ever needs independent consumption (the Nephio reason for a separate api repo), it can
be split out later because it is already its own Go module with no inward dependencies.

```
AGENTS.md, CLAUDE.md, LICENSE, README.md          (Phase 0, exists)
CONTRIBUTING.md, SECURITY.md                      (Phase 0)  Nephio root-file convention; no OWNERS file until there are owners
Makefile                                          (Phase 0)  discovers modules dynamically (find go.mod / Dockerfile), as upstream
default-*.mk                                      (Phase 0)  shared make fragments, see section 6
hack/boilerplate.go.txt, hack/check-headers.sh    (Phase 0)  license header source and checker
packages/                                         (Phase 3)  kpt packages, catalog pattern: Kptfile pipeline, package-context.yaml, placeholder WorkloadCluster
  mesh-gateway/                                   (Phase 3)  blueprint, pkg-example-*-bp pattern
  spectrum-sensor/                                (Phase 3)  blueprint
  mqtt-bridge/                                    (Phase 3)  broker plus bridge config
  meshtastic-operator/                            (Phase 4)  operator deployed per cluster, free5gc-operator / oai-operator pattern
api/                                              (Phase 4)  types-only Go module, Nephio api-repo conventions: one dir per group
  mesh/v1alpha1/                                  (Phase 4)  group mesh.nephmesh.io: MeshtasticNode
  sense/v1alpha1/                                 (Phase 6)  group sense.nephmesh.io: SpectrumScan, policy CR
krm-functions/                                    (Phase 5)  one Go module per function plus shared lib, condkptsdk pattern
  lib/                                            (Phase 5)  only if we need helpers beyond upstream condkptsdk; prefer importing nephio's lib
  <name>-fn/                                      (Phase 5)  3-line main.go, fn/function.go, fn/testdata/ golden tests
operators/                                        (Phase 4)  deployable binaries
  meshtastic-operator/                            (Phase 4)  controller manager; reconcilers self-register via init() into a registry
exporters/                                        (Phase 2)  small Go modules for metrics glue
  sweep-exporter/                                 (Phase 2)  rtl_power-format sweep CSV to per-band Prometheus gauges
demo/                                             (Phase 1)  per-phase reproducible environments (demo/phase1/, demo/phase3/, ...)
docs/                                             (exists)   architecture, roadmap, research, plans
distros/                                          (only if ever needed) cloud or distro-specific material, mirroring the Nephio catalog
```

Deliberate differences from upstream, and why:

- Single repo, not four. Scale does not justify the coordination cost; boundaries live at the Go
  module level instead.
- No Prow, no OWNERS. GitHub Actions is the only CI (section 4). OWNERS lands when a second
  maintainer exists.
- `controllers/pkg` vs `operators/`: Nephio splits reconciler logic (one shared module) from thin
  operator binaries. NephMesh starts with one operator, so reconciler packages live inside
  `operators/meshtastic-operator/` initially, using the same init()-registry pattern so a later
  split into a shared controllers module is mechanical.
- Directories are created only when their phase starts (per AGENTS.md). This file records the end
  state, not a license to scaffold early.

## 2. Go conventions

- Go 1.25.x (match the nephio repo; porch is on 1.26 but we consume porch, not build it).
- `sigs.k8s.io/controller-runtime v0.22.x`, `k8s.io/* v0.34.x`.
- kpt function SDK: `github.com/kptdev/krm-functions-sdk/go/fn` (kptdev org, not
  GoogleContainerTools). Kptfile types from `github.com/kptdev/kpt/pkg/api/kptfile/v1`.
- Specializer functions use condkptsdk (For/Owns/Watch, `specializer.nephio.org/*` annotations,
  pure and idempotent populate functions, WorkloadCluster read via Watch callback).
- Tests: testify plus golden tests (`RunGoldenTests` style with `testdata/<case>/` and
  `_expected.yaml`, `WRITE_GOLDEN_OUTPUT=1` regeneration). Not ginkgo, not envtest, matching the
  dominant upstream style.
- Mocks: mockery v3 with per-module `.mockery.yaml`. External services get narrow interfaces first
  (a `meshtasticclient.Client` over TCP 4403 is the flagship example, mirroring
  `giteaclient.GiteaClient`).
- Lint and security: golangci-lint v2.8 config shared at repo root, gosec per module.
- Typed YAML access via `kubeobject` generics; never marshal Go structs straight to YAML (upstream
  house style, preserves comments).

Module boundaries and replace directives:

- No root go.mod. Modules: `api/`, `operators/meshtastic-operator/`, `exporters/sweep-exporter/`,
  each `krm-functions/<x>-fn/`, and `krm-functions/lib/` if created.
- Decision: relative `replace` directives in go.mod, exactly as upstream, and no committed
  go.work. Replace directives are what upstream CI and Dockerfiles assume (repo-root build context
  exists so they resolve), and committing go.work would diverge from that. Developers may keep a
  local go.work; add `go.work` and `go.work.sum` to `.gitignore`.
- Module paths: `github.com/blisspixel/nephmesh/<dir>` (adjust to the real GitHub owner at scaffold
  time). Consumers import the api module as
  `meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"`.
- api module conventions per group: `<kind>_types.go`, `<kind>_interfaces.go` (Validate(),
  builders), `groupversion_info.go`, `condition.go`, generated `zz_generated.deepcopy.go`,
  exported `<Kind>Kind` string constants. controller-gen v0.20.0; `make generate` and
  `make manifests` targets with CRDs into `config/crd/bases/` named `<group>_<plural>.yaml`.

## 3. Licensing and provenance

- Apache-2.0, root LICENSE (exists). Every source file (Go, shell, Make, Dockerfile, YAML where
  comments are legal) carries:

  ```
  Copyright <year> The NephMesh Authors.

  Licensed under the Apache License, Version 2.0 (the "License");
  ... standard boilerplate ...
  ```

  Same shape as upstream's `The Nephio Authors.` header, so upstreaming a file is a one-line swap.
  The canonical text lives in `hack/boilerplate.go.txt` and controller-gen reuses it for generated
  files.
- DCO, not CLA: every commit `Signed-off-by:` via `git commit -s`. Documented in CONTRIBUTING.md.
- No AI attribution anywhere (per AGENTS.md): no generated-with lines, no AI co-author trailers,
  no tool names in headers or commit messages. This applies to commits, file headers, and docs.
- Enforcement without Prow: `hack/check-headers.sh`, a short grep-based script that fails CI if
  any tracked source file lacks the header or contains a forbidden attribution string. GitHub's
  DCO app (or a one-line Actions check for `Signed-off-by:`) enforces sign-off on PRs. Upstream's
  SPDX allowlist scan is deferred until there are enough dependencies to matter (open decision,
  section 9).

## 4. CI plan (GitHub Actions, no Prow)

CI grows with the phases and never requires hardware: every job runs against simulated radios
(`meshtasticd -s`, Meshtasticator) or pure rendering. That is a project rule, not an aspiration.

- Phase 0 (now): markdown lint (also enforcing AGENTS.md style where lintable), license header
  check, DCO check.
- Phase 1 to 2: add a smoke job that stands up kind and applies the demo manifests with
  `meshtasticd -s` (amd64 runner; the official image is multi-arch).
- Phase 3: `kpt fn render` on every package plus golden output comparison (rendered output checked
  against committed expected output), and `kpt fn eval` validators (kubeval-style schema checks).
  This is the package analog of function golden tests.
- Phase 4 onward: go unit tests, golangci-lint, gosec, mockery drift check, and image builds.
  Images build multi-arch (linux/amd64 plus linux/arm64) with buildx and QEMU, because the edge
  nodes (Pi, Orange Pi 5) are arm64; arm64 is a first-class target from the first image, not a
  port. CodeQL is cheap to enable at this point too.
- Jobs are path-filtered per module (the Actions analog of upstream's changed-component build
  validation) so docs changes do not build images.

## 5. Provider neutrality rules

- Forbidden in core (api/, operators/, krm-functions/, packages/ except distros): imports of cloud
  SDKs or cloud-specific K8s libraries. Concretely, module graphs must not contain
  `cloud.google.com/go`, `github.com/aws/aws-sdk-go*`, `github.com/Azure/azure-sdk-for-go`, or
  provider-specific CAPI providers. Enforced by a grep over go.mod files in CI once Go code exists
  (add to `hack/check-headers.sh`'s sibling, `hack/check-neutrality.sh`).
- No package, CRD default, or operator behavior may assume a cloud load balancer, cloud storage
  class, cloud DNS, or managed-Kubernetes annotation. Defaults must work on kind and k3s.
- Local-first targets, in priority order: kind and k3d on the dev PC, k3s on arm64 SBCs. CI depends
  only on these.
- If distro-specific material ever exists (a GKE-tuned variant, an OpenShift SCC patch), it lives
  under `distros/<name>/`, mirroring the Nephio catalog where GCP is one distro among several,
  never a dependency of core.
- SDR code targets SoapySDR; driver strings are configuration, not code (existing project rule,
  restated here because it is the radio-side neutrality analog).

## 6. Makefile strategy

Adopt upstream's fragment pattern: a root Makefile that discovers modules dynamically, plus small
per-module Makefiles (about 15 lines) that set a few variables and include shared fragments.

Fragments to write (Phase 0 stubs, filled in as their consumers land):

- `default-go.mk`: fmt, vet, test (with coverage), generate.
- `default-go-lint.mk`: golangci-lint with the shared root config.
- `default-gosec.mk`: gosec scan.
- `default-docker.mk`: buildx multi-arch build and push, repo-root context, tag and label
  conventions from section 7.
- `default-kpt.mk`: NephMesh addition with no upstream equivalent: render, validate, and golden
  targets for package directories.
- `detect-container-runtime.mk`: podman/docker autodetect; Go targets run inside a golang
  container when one is available, as upstream does, so contributor toolchains stay uniform.

## 7. Image conventions

- Two-stage builds: digest-pinned `golang:1.25.x-alpine` build stage, `CGO_ENABLED=0`, final stage
  `gcr.io/distroless/static:nonroot`, also digest-pinned. Non-Go images (the spectrum sensor needs
  SoapySDR native libs) use a minimal Debian-slim or distroless-base final stage, still two-stage
  and digest-pinned; static distroless is the default, not a straitjacket.
- Build context is always the repo root so relative replace directives resolve, matching upstream
  Dockerfiles.
- Multi-arch linux/amd64 and linux/arm64 via buildx from the first published image.
- Registry: `ghcr.io/<owner>/nephmesh-<name>` (for example `ghcr.io/blisspixel/nephmesh-operator`).
  ghcr.io over Docker Hub because: pushes authenticate with the built-in `GITHUB_TOKEN` (no
  long-lived secrets), no anonymous-pull rate limits that would break newcomer demos, images sit
  next to the source with linked provenance, and public storage is free. Docker Hub adds
  discoverability we do not need yet; a mirror can be added later without breaking anything
  because packages reference images by full registry path.

## 8. Versioning and release mechanics

Versions follow the roadmap's version path: each phase demo gates a `v0.x.0` git tag (Phase 1 demo
tags v0.1.0, and so on). Patch releases (v0.x.y) are allowed for fixes between gates. Tags are
annotated and signed-off like commits.

- CHANGELOG.md at root, Keep-a-Changelog format, updated in the release commit. No auto-generation
  tooling until volume justifies it.
- Release artifacts grow per phase:
  - v0.1.0 to v0.2.0: a manifests tarball (the demo YAML for that phase) attached to the GitHub
    release, plus the changelog.
  - v0.3.0: kpt package tags. Packages are consumed from Git, so the release mechanics are git
    tags per package revision in the form Porch expects (`<package-path>/vN` tags alongside the
    repo-wide `v0.3.0`); this repo registers with Porch as a catalog at this point.
  - v0.4.0 onward: container images pushed to ghcr.io tagged with the release version and
    immutable digests referenced from the released packages; CRD YAML bundle attached to the
    release.
- The pinned Nephio release (R6 now) is recorded in the README and in each release's notes; a
  release never floats against Nephio main.

## 9. Open decisions (resolutions annotated 2026-08-04; rationale in the changelog history)

1. GitHub owner and module path: DECIDED. The repo's actual remote is `github.com/blisspixel/NephMesh`,
   so module paths are `github.com/blisspixel/nephmesh/<dir>` (lowercase, per Go module convention;
   GitHub resolves repo names case-insensitively) and images are `ghcr.io/blisspixel/nephmesh-<name>`.
   Reversible until the first go.mod ships.
2. Domain for API groups: DECIDED (provisional). Use `nephmesh.io` (`mesh.nephmesh.io`,
   `sense.nephmesh.io`) as a placeholder. An API group is just a stable DNS-style string; it does not
   require owning the domain to use in a private, experimental cluster, and there are no external
   consumers. It is explicitly provisional: while the CRDs are `v1alpha1` and pre-alpha, renaming the
   group is cheap, so this does not block Phase 4. Revisit before any public or 1.0 release, at which
   point group stability (and ideally a domain that is actually controlled) matters; the CRD
   versioning and conversion plan makes that rename orderly rather than breaking.
3. SPDX license scanning: DECIDED. Adopt at Phase 4 when the Go dependency tree becomes real.
4. DCO enforcement: DECIDED and implemented. The in-repo Actions check in `.github/workflows/ci.yaml`
   verifies Signed-off-by on every PR commit; no third-party app dependency.
5. `krm-functions/lib/`: DECIDED direction, final call at Phase 5. Import upstream's lib first;
   write our own helpers only where friction is demonstrated.
6. Brand email to brand@linuxfoundation.org: OPEN. External communication is the maintainer's call;
   risk remains low either way (the name does not contain the Nephio mark).
