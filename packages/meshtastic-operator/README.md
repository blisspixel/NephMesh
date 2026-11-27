# meshtastic-operator package

The NephMesh MeshtasticNode operator as a kpt package, deployed once per workload cluster. It contains the CRDs, least-privilege RBAC, a ServiceAccount, and the operator Deployment. This is the operator half of the free5gc-operator / oai-operator pattern: a `PackageVariantSet` fans this package out to workload clusters, and the `mesh-gateway` package ships the per-site `MeshtasticNode` instances the operator reconciles.

The operator serves two CRDs: `MeshtasticNode` (the compiled, per-device desired state it reconciles against a radio) and `CommunicationIntent` (the outcome-level intent it compiles, report-only, into proposed `MeshtasticNode` specs on status). The intent path never actuates: the RBAC grants no create or write on `MeshtasticNode` from an intent (ADR 0001).

The operator source is `operators/meshtastic-operator`; its behavior and tests are described there and in `docs/plans/phase-4-operator.md`.

Notes:

- The namespace function specializes namespaced resources and RBAC subject references. It skips the single-version CRDs so they retain their generated conversion settings.
- The Deployment pins the signed Linux amd64/arm64 index by digest. The [operations guide](../../docs/guides/operations.md#install) covers installation using the release archive or a registry.
- `crd.yaml` (MeshtasticNode) and `crd-communicationintent.yaml` (CommunicationIntent) are generated from the api module (`make -C api manifests`); do not edit them by hand.
