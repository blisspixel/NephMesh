# Research: Nephio building blocks & the third-party package pattern

Researched 2026-08-04. Latest release at time of writing: **R6** (April 2026 - Porch v1.5.6, Nephio v6.0.0, K8s v1.26–v1.32). R6 was a stability/security release; **R7 is slated to focus on "modularization and easier onboarding for new use cases"** - directly relevant to NephMesh. R5 (July 2025) added ArgoCD/FluxCD as reconciliation-agent options alongside Config Sync.

Sources: [R6 announcement](https://nephio.org/nephio-release-6-strengthening-stability-and-security/) · [R6 notes](https://docs.nephio.org/docs/release-notes/r6/) · [R5 notes](https://docs.nephio.org/docs/release-notes/r5/)

## Core components

- **kpt / Configuration-as-Data** - all artifacts are kpt packages: raw KRM YAML (no templating) mutated by KRM functions declared in a `Kptfile` pipeline. kpt is now a CNCF project ([kpt.dev](https://kpt.dev)).
- **Porch** ([nephio-project/porch](https://github.com/nephio-project/porch)) - aggregated API server exposing Git repos of kpt packages as `Repository`/`PackageRevision` resources with a Draft → Proposed → Published lifecycle. CLI: `porchctl`. [Docs](https://docs.nephio.org/docs/porch/)
- **Key CRDs** ([nephio-project/api](https://github.com/nephio-project/api)):
  - `PackageVariant` / `PackageVariantSet` (`config.porch.kpt.dev`) - upstream→downstream cloning with mutations; PVS fans out via label selectors. [Docs](https://docs.nephio.org/docs/porch/package-variant/)
  - `WorkloadCluster` (`infra.nephio.org/v1alpha1`) - per-site facts on the management cluster; the anchor for injection.
  - NF requirements: `Interface`, `DataNetwork`, `Capacity` (`req.nephio.org/v1alpha1`); workload intent: `NFDeployment`.
- **Specializer KRM functions** (docker.io/nephio/*: `interface-fn`, `dnn-fn`, `nad-fn`, `nfdeploy-fn`, …) - each reads the package's singleton `WorkloadCluster` and emits derived resources (NADs, IPClaims, VLANClaims). They run repeatedly in the pipeline because specialization converges iteratively.
- **Controllers** ([nephio-project/nephio](https://github.com/nephio-project/nephio)) - `nephio-controller-manager` (PV approval/bootstrap, IPAM/VLAN specializers), resource-backend, network-config-operator, plus O-RAN focom/o2ims operators.
- **Catalog** ([nephio-project/catalog](https://github.com/nephio-project/catalog)) - the Git repo *is* the catalog; Porch registers it directly. Layout: `distros/` (sandbox/gcp/openshift), `infra/` (CAPI cluster packages incl. `nephio-workload-cluster`), `nephio/core|optional/`, `workloads/` (free5gc, oai, ric, ueransim).

## How a third party ships packages (the NephMesh path)

There is no marketplace - just Git + Porch:

```bash
porchctl repo register --namespace default \
  [--deployment=true|false] https://github.com/<you>/<catalog>.git
```

Blueprint repos hold upstream packages; deployment repos receive published, deployment-ready packages that Config Sync/Argo/Flux pull onto workload clusters. Your repo becomes a peer of `nephio-project/catalog`. **Precedent: OpenAirInterface maintains [oai-packages](https://github.com/OPENAIRINTERFACE/oai-packages) and [oai-operators](https://github.com/OPENAIRINTERFACE/oai-operators) in its own org**, mirrored into the Nephio catalog.

### Blueprint anatomy (from `catalog/workloads/free5gc/pkg-example-upf-bp`)

Files: `Kptfile` (function pipeline), `package-context.yaml` (a ConfigMap named `kptfile.kpt.dev`; Porch rewrites `data.name` to the deployment package name on clone - that plus `set-namespace` gives per-instance namespacing with zero templating), placeholder `workload-cluster.yaml` (replaced by injection), intent CRs (`upfdeployment.yaml`, `capacity.yaml`), requirement CRs (`interface-*.yaml`, `dnn.yaml`), `apply-replacements-*.yaml` configs.

### Per-site specialization

A `PackageVariant` names upstream/downstream plus mutations; `spec.injectors` points at a `WorkloadCluster` by name, and the PV controller injects that live resource into the clone so specializers see real cluster facts. `PackageVariantSet` fans out with `targets.objectSelector` over `WorkloadCluster` labels and `template.downstream.nameExpr: target.name`.

### The repeatable integration recipe (what free5GC/OAI/RIC all did)

1. Define a CRD for your NF's intent (or reuse `NFDeployment`).
2. Write an operator reconciling it, shipped as an "operator" kpt package deployed to every workload cluster via PVS.
3. Publish `pkg-example-*-bp` blueprints: intent CR + requirement CRs + placeholder `WorkloadCluster` + specializer pipeline.
4. Ship `package-variants/` examples selecting clusters by label.
5. Host it all in your own Porch-registered Git repo.

Caveat: the specializer-CRD layer is evolving (nephio-project/nephio#933 proposes moving interface-fn/nad-fn to Kubenet/Kuid CRs) - **pin to a release branch**.

## Minimal single-machine setup

The sandbox installer ([nephio-project/test-infra](https://github.com/nephio-project/test-infra) `e2e/provision/init.sh`) Ansible-installs everything on one VM:

- **Requirements:** Ubuntu 22.04 or Fedora 34; **16 vCPU / 32 GB RAM minimum, 200 GB disk** recommended. [Single-VM guide](https://docs.nephio.org/docs/guides/install-guides/install-on-single-vm/)
- **You get:** a kind-based management cluster with porch, nephio-controllers, configsync, cert-manager, CAPI, gitea, metallb, webui. Workload clusters are additional kind clusters created via CAPI on the same VM.
- **Slim path:** Porch alone runs on any vanilla kind cluster - enough for early package-lifecycle work on a laptop.
- Known wart: VPN IP clashes with kind's 172.18.x.x network.

## Trademark / naming

- Nephio is an LF project; no Nephio-specific trademark policy is published - the general [LF Trademark Usage policy](https://www.linuxfoundation.org/legal/trademark-usage) applies. "Nephio" did not surface on the LF registered-marks list, but the safe assumption is that the name and logo are LF-claimed marks.
- Fair use of the word mark for true factual statements ("NephMesh works with Nephio") is allowed; implying endorsement/affiliation, using the mark in your own name, or using the logo is not.
- **"NephMesh" does not contain the "Nephio" mark** (shared "Neph-" prefix only, from Greek *nephos*, cloud) - materially lower risk than "NephioMesh"-style names. We carry a prominent README disclaimer; questions go to brand@linuxfoundation.org. (Lay assessment, not legal advice.)
