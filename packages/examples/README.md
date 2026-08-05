# Example specialization resources

These are not kpt packages (no Kptfile), so the render gate skips them. They are Nephio custom resources applied to a management cluster running Porch, showing how the `mesh-gateway` blueprint in this repo is cloned and specialized per site. See [docs/guides/porch-registration.md](../../docs/guides/porch-registration.md) for the full flow.

- `packagevariant-site1.yaml`: clones `mesh-gateway` into a single site's deployment repo and namespace.
- `packagevariantset-edges.yaml`: fans the blueprint out to every `WorkloadCluster` labelled as a mesh edge, one specialized package per cluster.

Both are illustrative. They reference repository and cluster names that you replace with your own; nothing here is applied by the render gate or the demo.
