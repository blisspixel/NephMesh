# Registering NephMesh packages with Porch

This guide uses the stock Porch v1.6.2 package on one disposable kind cluster. Use a public read-only NephMesh catalog and a separate writable Git repository for deployment revisions. Run the commands from the release checkout in a POSIX shell (WSL2 on Windows) with Docker, kind v0.33.0, kubectl, kpt v1.0.0-beta.67, Git, curl, shasum and porchctl v1.6.2. The workload uses simulated firmware and a broker inside the cluster.

## Install Porch

Use a dedicated kubeconfig so commands target the disposable cluster. Verify the upstream package against its release checksum before extracting it.

```sh
set -eu
work=$(mktemp -d)
export KUBECONFIG="$work/kubeconfig"
kind create cluster --name nephmesh-porch --kubeconfig "$KUBECONFIG"
mkdir "$work/porch"
curl -fsSL https://github.com/kptdev/porch/releases/download/v1.6.2/porch-kpt-package.tar.gz -o "$work/porch.tar.gz"
(cd "$work" && printf '%s  %s\n' 94925edebef145bb45dca5be361adf4eb219c4dcd22725922072a6ed20bf17cd porch.tar.gz | shasum -a 256 -c -)
tar -xzf "$work/porch.tar.gz" -C "$work/porch"
(cd "$work/porch" && kpt live init && kpt live apply)
kubectl wait --for=condition=Available apiservice/v1alpha1.porch.kpt.dev --timeout=3m
```

## Register repositories

For the disposable demo, start a cluster-local Git service with a private, initialized repository. The helper requires OpenSSL. It creates the credential in the disposable cluster.

```sh
sh demo/porch/create-site-repo.sh
SITE_REPO=http://git.porch-demo.svc.cluster.local:3000/porchdemo/site.git
```

Alternatively, set `SITE_REPO` to your own writable Git repository initialized with a `main` branch. Its URL must be reachable from the Porch server pod. Supply its credentials as a Kubernetes basic-auth Secret named `porch-site-git` in `default`. The public blueprint catalog requires no credentials.

```sh
porchctl repo register -n default --name nephmesh-catalog --directory packages https://github.com/blisspixel/NephMesh.git
kubectl apply -f - <<YAML
apiVersion: config.porch.kpt.dev/v1alpha1
kind: Repository
metadata:
  name: site1-deployments
  namespace: default
spec:
  type: git
  deployment: true
  git:
    repo: ${SITE_REPO:?set SITE_REPO to the URL of the writable deployment repository}
    branch: main
    secretRef:
      name: porch-site-git
YAML
kubectl wait --for=condition=Ready repositories.config.porch.kpt.dev/nephmesh-catalog repositories.config.porch.kpt.dev/site1-deployments --timeout=3m
porchctl rpkg get -n default
```

All four blueprints appear with workspace `main` and revision `-1` when the catalog contains only repository release tags. Porch rejects a direct clone of the `.main` placeholder revision. Clone the package directory at an immutable Git commit to create a reviewable downstream draft.

## Prepare the site and review the draft

From the matching NephMesh checkout, build the configure Job image and load it into the disposable cluster. The simulated gateway resolves `mosquitto` in its own namespace, so install the broker in the same namespace as the gateway.

```sh
docker build -t nephmesh/meshtastic-cli:2.7.11 demo/phase1/images/meshtastic-cli
kind load docker-image nephmesh/meshtastic-cli:2.7.11 --name nephmesh-porch
kubectl create namespace site1-mesh
cp -R packages/mqtt-bridge "$work/broker"
sed 's/^  name: mqtt-bridge$/  name: site1-mesh/' packages/mqtt-bridge/package-context.yaml > "$work/broker/package-context.yaml"
kpt fn render "$work/broker"
kubectl apply -f "$work/broker/mosquitto.yaml" -f "$work/broker/networkpolicy.yaml"
kubectl -n site1-mesh rollout status deploy/mosquitto --timeout=3m
source_commit=$(git rev-parse HEAD)
porchctl rpkg clone https://github.com/blisspixel/NephMesh.git site1-mesh -n default \
  --ref "$source_commit" --directory packages/mesh-gateway --repository site1-deployments
revision=site1-deployments.site1-mesh.v1
porchctl rpkg pull "$revision" "$work/review" -n default
porchctl rpkg propose "$revision" -n default
```

Review the upstream commit in the pulled Kptfile, the rendered namespace, simulator arguments, desired region and channel settings, and the configure Job target. A person approves region and modem-preset changes, channel and PSK changes, and changes enabling transmission or increasing power.

## Publish and install the approved revision

After human approval, publish the revision, pull it again, and compare the workload files with the reviewed draft. Porch updates package lifecycle metadata when it publishes. Apply the reviewed workload files explicitly so the local package context and WorkloadCluster injection marker stay off the cluster.

```sh
porchctl rpkg approve "$revision" -n default
porchctl rpkg get "$revision" -n default
porchctl rpkg pull "$revision" "$work/published" -n default
for file in configure-job.yaml meshtasticd.yaml networkpolicy.yaml node-config.yaml; do
  cmp "$work/review/$file" "$work/published/$file" || exit 1
done
kubectl apply -f "$work/published/meshtasticd.yaml" -f "$work/published/node-config.yaml" \
  -f "$work/published/configure-job.yaml" -f "$work/published/networkpolicy.yaml"
kubectl -n site1-mesh wait --for=condition=Complete job/meshnode-configure --timeout=15m
kubectl -n site1-mesh logs job/meshnode-configure
```

Read the settings back from the running simulator in a separate, read-only client session.

```sh
kubectl -n site1-mesh run porch-readback --image=nephmesh/meshtastic-cli:2.7.11 --restart=Never \
  --overrides='{"spec":{"automountServiceAccountToken":false,"securityContext":{"runAsNonRoot":true,"runAsUser":65532,"seccompProfile":{"type":"RuntimeDefault"}}}}' \
  --command -- python -c "$(cat demo/porch/verify-site.py)"
kubectl -n site1-mesh wait --for=jsonpath='{.status.phase}'=Succeeded pod/porch-readback --timeout=90s
kubectl -n site1-mesh logs porch-readback
```

### Captured result

On 2026-11-27, stock Porch v1.6.2 installed on Kubernetes 1.37.0 and discovered four catalog packages. The gateway blueprint came from commit `ed6239e`. The downstream Git tag `site1-mesh/v1` and its deployment branch pointed to the same commit. All eight published workload resources matched the reviewed draft.

```console
$ kubectl -n site1-mesh logs job/meshnode-configure
applying configuration
enabling MQTT uplink on the primary channel
rebooting device to activate module config
configuration verified after apply
$ kubectl -n site1-mesh logs porch-readback
region=US mqtt_enabled=True site_broker=True primary_uplink=True
```

Teardown for the disposable demo is `kind delete cluster --name nephmesh-porch`. The local Git service and its credential are removed with the cluster. A separately hosted site repository retains its approved package history.

## Specialization examples

The examples select a published package by `upstream.workspaceName: v1`. Set the upstream repository, package and workspace to one listed by `porchctl rpkg get`. In the disposable demo, `site1-deployments / site1-mesh / v1` is available after approval. Porch v1.6.2 can select the non-clonable `main` placeholder if only a numeric revision is supplied. The downstream package name determines the namespace through the Kptfile pipeline. Porch reserves the `name` key in package context. The set example also requires the WorkloadCluster API and matching labelled objects. CI submits both examples to the released Porch CRDs with strict server-side validation. To repeat that check on this cluster, run the following command.

```sh
kubectl apply --dry-run=server --validate=strict -f packages/examples/
```

With a published input, the single-site example produced a rendered draft in namespace `site1-mesh-gateway`. In a separate controller check, one labelled WorkloadCluster generated a site-specific draft and supplied its cluster name to an optional local injection point. These drafts still require human review before deployment.

For cross-cluster GitOps, configure a sync agent on the site cluster to consume the approved package from its deployment repository. The single-cluster demonstration uses an explicit pull and apply.

[Upstream Porch v1.6.2 release and CLI downloads](https://github.com/kptdev/porch/releases/tag/v1.6.2)
