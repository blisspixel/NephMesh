# Example specialization resources

These resources run on a management cluster with Porch. Select a published upstream workspace and register the target deployment repositories before applying them. The [Porch guide](../../docs/guides/porch-registration.md#specialization-examples) explains how to find those names.

- `packagevariant-site1.yaml` creates a draft whose downstream name sets the site namespace.
- `packagevariantset-edges.yaml` selects labelled WorkloadClusters, creates per-site variants, and injects site facts into the local WorkloadCluster marker.

The packages CI job submits both manifests to a Kubernetes API server with the released Porch CRDs and strict field validation.
