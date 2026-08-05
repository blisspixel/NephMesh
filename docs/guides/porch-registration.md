# Registering NephMesh packages with Nephio Porch

How this repository's kpt packages become deployments through Nephio, and where a human stays in the loop. This is the Phase 3 flow; it targets a management cluster that already runs Porch. NephMesh consumes Nephio as a third party, exactly as OpenAirInterface ships its own packages, so nothing here modifies Nephio itself.

Status note: the packages and specialization resources are validated by rendering (`make check-packages`) and by the Phase 1 demo they are derived from. A full end-to-end Porch deployment needs a running Porch install; the 0.3 release gate in the [roadmap](../roadmap.md) is that end-to-end run, and it is not claimed here.

## The pieces

- This repo is a blueprint catalog: `packages/mesh-gateway` and `packages/mqtt-bridge`.
- A management cluster runs Porch, which exposes Git repositories as `Repository` and `PackageRevision` resources.
- A `PackageVariant` (or `PackageVariantSet`) clones a blueprint into a per-site deployment repo and specializes it. Examples: [packages/examples](../../packages/examples).
- A GitOps agent (Config Sync, Argo CD, or Flux) syncs each site's deployment repo onto that site's cluster.

## Register the catalog

Register this repository as a blueprint source. `--deployment=false` (the default) marks it as blueprints rather than deployment-ready packages:

```sh
porchctl repo register \
  --namespace default \
  --repo-basic-username <user> --repo-basic-password <token> \
  https://github.com/blisspixel/NephMesh.git
```

Confirm it, and that each site's deployment repository is registered too:

```sh
porchctl repo get -A
```

## Specialize per site

Apply a `PackageVariant` for a single site, or a `PackageVariantSet` to fan out across every matching `WorkloadCluster`:

```sh
kubectl apply -f packages/examples/packagevariant-site1.yaml
# or, for fan-out over labelled clusters:
kubectl apply -f packages/examples/packagevariantset-edges.yaml
```

Porch creates a **draft** `PackageRevision` in each downstream repo. It is not deployed yet.

## The human-approved step (this matters for radios)

Changing a radio's configuration is consequential, so the propose/approve lifecycle is where a person reviews before anything reaches hardware. Inspect the draft, then propose and approve it:

```sh
porchctl rpkg get                              # list package revisions
porchctl rpkg pull <package-revision> ./review # inspect the specialized output
porchctl rpkg propose <package-revision>       # draft -> proposed
porchctl rpkg approve <package-revision>       # proposed -> published
```

What should always stay human-approved, not automated: region and modem-preset changes (they affect legality and interoperability), channel and PSK changes (they affect security and who can join), and anything that would enable transmission or raise power. The closed loop in a later phase may propose such changes, but a person approves them here. This is the boundary the [threat model](../security/threat-model.md) calls the transmit interlock, expressed as process.

## After approval

The published `PackageRevision` lands in the site's deployment repo. The GitOps agent syncs it onto the site cluster, where the gateway comes up, the config Job converges the node, and the node uplinks to its site broker. Deleting the `PackageVariant` removes the site's package again.

## What Nephio specialization adds over plain manifests

The same blueprint produces every site with per-site namespaces and, once specializer functions exist (Phase 5), per-site configuration injected from each `WorkloadCluster`. The placeholder `WorkloadCluster` in `mesh-gateway` marks where that injection happens. Until then, the value is the reviewable, versioned, one-blueprint-many-sites flow itself.
