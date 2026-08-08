# meshtastic-operator package

The NephMesh MeshtasticNode operator as a kpt package, deployed once per workload cluster. It contains the CRDs, least-privilege RBAC, a ServiceAccount, and the operator Deployment. This is the operator half of the free5gc-operator / oai-operator pattern: a `PackageVariantSet` fans this package out to workload clusters, and the `mesh-gateway` package ships the per-site `MeshtasticNode` instances the operator reconciles.

The operator serves two CRDs: `MeshtasticNode` (the compiled, per-device desired state it reconciles against a radio) and `CommunicationIntent` (the outcome-level intent it compiles, report-only, into proposed `MeshtasticNode` specs on status). The intent path never actuates: the RBAC grants no create or write on `MeshtasticNode` from an intent (ADR 0001).

The operator source is `operators/meshtastic-operator`; its behavior and tests are described there and in `docs/plans/phase-4-operator.md`.

Notes:

- The CRDs and ClusterRole are cluster-scoped; `set-namespace` places only the ServiceAccount and Deployment into the per-clone namespace.
- The Deployment image tag is a placeholder until a release publishes the image to a registry with a digest. The image bundles the pinned Meshtastic CLI the operator execs.
- `crd.yaml` (MeshtasticNode) and `crd-communicationintent.yaml` (CommunicationIntent) are generated from the api module (`make -C api manifests`); do not edit them by hand.
