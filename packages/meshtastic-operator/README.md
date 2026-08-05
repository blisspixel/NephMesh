# meshtastic-operator package

The NephMesh MeshtasticNode operator as a kpt package, deployed once per workload cluster. It contains the CRD, least-privilege RBAC, a ServiceAccount, and the operator Deployment. This is the operator half of the free5gc-operator / oai-operator pattern: a `PackageVariantSet` fans this package out to workload clusters, and the `mesh-gateway` package ships the per-site `MeshtasticNode` instances the operator reconciles.

The operator source is `operators/meshtastic-operator`; its behavior and tests are described there and in `docs/plans/phase-4-operator.md`.

Notes:

- The CRD and ClusterRole are cluster-scoped; `set-namespace` places only the ServiceAccount and Deployment into the per-clone namespace.
- The Deployment image tag is a placeholder until a release publishes the image to a registry with a digest. The image bundles the pinned Meshtastic CLI the operator execs.
- `crd.yaml` is generated from the api module (`make -C api manifests`); do not edit it by hand.
