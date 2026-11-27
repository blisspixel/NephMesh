# Operating the MeshtasticNode operator

A practical runbook for running the operator: install, declare a node, watch it,
diagnose it when something is wrong, and the day-2 operations (key rotation,
decommission, upgrade). It is honest about what is implemented today and what is
still design.

The operator reconciles one `MeshtasticNode` custom resource against one device:
it reads the device, computes the drift against the declared intent, applies only
what changed, reboots the radio (any config write reboots it), and re-verifies. It
is a management layer, not a runtime dependency: once a device is configured it
keeps carrying traffic if the operator, or the whole cluster, is gone.

## Install

From a checkout of the release, copy and render the operator package. It contains both CRDs, the service account and RBAC, an egress policy and the Deployment. The Deployment pins a signed Linux amd64/arm64 image by digest. For the Nephio package lifecycle, see the [Porch registration guide](porch-registration.md).

```sh
pkg=$(mktemp -d)
cp -R packages/meshtastic-operator "$pkg/operator"
kpt fn render "$pkg/operator"
ref=$(awk '$1 == "image:" {print $2}' "$pkg/operator/deployment.yaml")
image=${ref%@*}
identity=https://github.com/blisspixel/NephMesh/.github/workflows/operator-image.yaml@refs/heads/main
issuer=https://token.actions.githubusercontent.com
```

### Pull from GHCR

The index is available for anonymous pulls. Verify the index signature on a connected machine. On a cluster with access to GHCR, the kubelet pulls the pinned image when you apply the Deployment.

```sh
cosign verify --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" "$ref"
```

For a fresh kind cluster, run `kind create cluster --name nephmesh-demo` and continue at [Apply and verify](#apply-and-verify).

### Load the release archive on kind

The release archive works on a local kind cluster without a registry account or a local operator build. On a connected machine, download the archive and signature, verify the workflow identity with Cosign, and check the index digest against the package.

```sh
release=https://github.com/blisspixel/NephMesh/releases/download/v0.2.7
curl -fL "$release/operator.tar" -o "$pkg/operator.tar"
curl -fL "$release/operator.sigstore.json" -o "$pkg/operator.sigstore.json"
cosign verify-blob --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" \
  --bundle "$pkg/operator.sigstore.json" "$pkg/operator.tar"
test "$(tar -xOf "$pkg/operator.tar" index.json | jq -er '.manifests | select(length == 1) | .[0].digest')" = "${ref#*@}"
```

Once both checks succeed, create a disposable cluster with kind's Docker provider. Import the verified archive under the same repository name and digest on every node. For an offline site, transfer the verified archive and rendered package and pre-stage the kind node and simulator images as needed.

```sh
kind create cluster --name nephmesh-demo
for node in $(kind get nodes --name nephmesh-demo); do
  docker exec -i "$node" ctr -n k8s.io images import --local --base-name "$image" --digests - < "$pkg/operator.tar" || exit 1
  docker exec "$node" crictl inspecti "$ref" > /dev/null || exit 1
done
```

For clusters that pull from an internal registry, [mirror the verified archive](operator-image.md) and use the mirror repository name with the same digest.

### Apply and verify

Check that kubectl is targeting the intended cluster. These commands use the package's default namespace.

```sh
kubectl config current-context
kubectl create namespace meshtastic-operator --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f "$pkg/operator/crd.yaml" -f "$pkg/operator/crd-communicationintent.yaml"
kubectl wait --for=condition=Established crd/meshtasticnodes.mesh.nephmesh.io crd/communicationintents.intent.nephmesh.io
kubectl apply -f "$pkg/operator/rbac.yaml" -f "$pkg/operator/networkpolicy.yaml" -f "$pkg/operator/deployment.yaml"
kubectl -n meshtastic-operator rollout status deployment/meshtastic-operator --timeout=3m
```

On the disposable cluster, use the Phase 1 simulator to exercise a real config write, reboot and subsequent read over the TCP API.

```sh
kubectl apply -f demo/phase1/manifests/namespace.yaml -f demo/phase1/manifests/meshtasticd.yaml
sed 's/host: 10\.0\.0\.51/host: meshnode-sim.nephmesh.svc.cluster.local/' examples/basic-node.yaml | kubectl -n meshtastic-operator apply -f -
kubectl -n meshtastic-operator wait --for=condition=Ready meshtasticnode/field-01 --timeout=5m
kubectl -n meshtastic-operator get meshtasticnode field-01
```

Captured on 2026-11-27 with the 0.2.7 index pulled anonymously by a clean kind node running Kubernetes 1.37.0:

```console
$ kubectl -n meshtastic-operator get meshtasticnode field-01 -o 'custom-columns=NAME:.metadata.name,READY:.status.conditions[?(@.type=="Ready")].status,SYNC:.status.conditions[?(@.type=="ConfigInSync")].status'
NAME       READY   SYNC
field-01   True    True
```

To tear down this disposable installation, run `kind delete cluster --name nephmesh-demo`.

To try the reconcile engine with no cluster and no hardware at all, the scripted
[operator demo](../../demo/operator/) stands up a simulated radio and converges
it, including a secure channel, end to end. With a `meshtasticd` USB gateway
and a serial handheld, [demo/meshtoad-gateway](../../demo/meshtoad-gateway/)
observes both and sends one LoRa text each way. `reconcile-demo` accepts
`-role` as well as `-region`, `-preset`, owner, and channels.

## Declare a node

Start from the [examples](../../examples/): a basic node, or a node with a private
channel whose key comes from a Secret. Apply and watch:

```console
$ kubectl apply -f examples/basic-node.yaml
$ kubectl get meshtasticnode -w
NAME       READY   REGION   PRESET        NODEID       LASTHEARD   AGE
field-01   True    US       MEDIUM_SLOW   !6e000001    8s          40s
```

## Watch and observe

The operator surfaces its state three ways, so you can see both the current state
and what it did.

**Printer columns** (`kubectl get meshtasticnode`): Ready, Region, Preset, NodeID,
LastHeard, Age.

**Conditions** (`kubectl describe meshtasticnode <name>`):

| Condition | Meaning |
|---|---|
| `Reachable` | the device answered this reconcile |
| `ConfigInSync` | the scalar config matches the declared intent (the message names any drift) |
| `RebootPending` | a config change was applied and the device is rebooting |
| `Ready` | reachable and in sync |
| `Degraded` | stopped converging after the apply bound; the message names the fields that never converged |
| `AirtimeHealthy` | the radio's measured channel utilization and transmit airtime are within the recommended ceilings |
| `ChannelsInSync` | the declared channels match the device (present only when channels are declared) |
| `AirtimeBudget` | a declared modem-preset change is predicted to stay within the channel ceiling (present only when such a change is pending) |

**Events** (in `kubectl describe`, and via the API): `ConfigApplied` (Normal, a
change was applied and the device is rebooting), `ApplyFailed` (Warning, went
Degraded, with the drifted fields), `SecretMissing` (Warning, a referenced Secret
could not be resolved), `ConnectFailed` (Warning, the device became unreachable).

**Metrics** (Prometheus, on the manager's `/metrics`, prefix
`nephmesh_meshtasticnode_`): `ready`, `reachable`, `degraded`, `config_in_sync`,
`apply_attempts`, `channel_utilization_percent`, `air_util_tx_percent`. `reachable`
and `degraded` are separate from `ready` on purpose, so you can tell an offline
device (`reachable=0`) from a reachable-but-drifted one, and alert on `degraded`
directly.

## Troubleshoot

Map the symptom to the signal:

- **Node stuck not Ready, `Degraded=True`.** The config never converged after the
  apply bound. Read the Degraded condition message or the `ApplyFailed` Event: it
  names the fields still drifted (for example `channel[1].name`, `lora.modemPreset`).
  A common cause is a value the device silently rejects or truncates. Channel names
  are bounded to the device's ~12-byte limit and owner short names to a few
  characters, so an over-long value is rejected at admission rather than looping;
  if you hit Degraded, the named field is where to look.
- **`Ready=False`, Reason `SecretMissing`.** A referenced Secret (broker password
  or a channel PSK) is missing, has the wrong key, or is empty. The condition
  message and the Warning Event name the secret and key (never the value). An
  empty PSK Secret is refused deliberately, rather than silently falling back to
  the public default key.
- **`reachable=0` / `Reachable=False`.** The device did not answer. Check power,
  the network path, and that the address and port in `spec.connection.tcp` are
  right (the port is honored). During a normal reboot after an apply the device is
  briefly unreachable, that is expected and the loop requeues.
- **`AirtimeHealthy=False`.** The radio reports channel utilization or transmit
  airtime over the recommended ceilings. The channel is saturating; consider a
  faster preset or less telemetry. Airtime, not node count, is the LoRa scaling
  wall.
- **`AirtimeBudget=False`.** A declared preset change is predicted to push the
  channel over the utilization ceiling. It is advisory (the operator still applies
  the declared intent), so treat it as a warning that the change will load the
  channel.
- **`ChannelsInSync=False`.** A declared channel does not match the device; the
  message names the drifted field. If it never clears, check the channel name
  length and that the PSK Secret holds the raw key bytes.

## Day-2 operations

- **Rotate a channel key (single node, today).** Update the channel's Secret with
  the new key. The operator resolves it, sees the key-hash drift, applies the new
  key, reboots, and re-verifies. Coordinated make-before-break rotation across a
  whole fleet is a design, not built yet: see
  [key-rotation-and-epochs](../plans/key-rotation-and-epochs.md).
- **Rotate an MQTT broker password (single node, today).** Update the password
  Secret. The device never echoes the password, so the operator compares a stored
  hash of the last-applied value (on status, not the password itself) and applies
  when it changes. A node and its Secrets must live in the operator's namespace:
  Secret access is a namespaced get-only Role, not a cluster-wide grant.
- **Turn a feature off.** Owned fields (MQTT enabled, encryption, JSON) reconcile
  both ways: set `mqtt.enabled: false` (or `encryptionEnabled: false`) and the
  operator turns it off on the device, it does not only ever turn things on.
- **Decommission a node.** Delete the `MeshtasticNode`. The default deletion policy
  is Retain: the operator stops managing the device and removes its finalizer, and
  the device keeps running with its last-applied config. A Wipe policy that clears
  the device is a later-phase feature and is not performed yet.
- **Upgrade the operator.** Verify the new release signature and digest, load the new image on each offline node, then render and reapply the CRDs, RBAC and Deployment from that release. Wait for rollout and recheck node readiness. The `MeshtasticNode` API is `v1alpha1` and may change before 1.0.

## What this operator does not do (yet)

Stated plainly so nothing here overpromises: serial and viaGateway transports are
reported as unsupported by the deployed operator (TCP is the wired path today; the
reconcile engine drives serial via the `reconcile-demo` tool and a real T-Deck,
and TCP against `meshtasticd` plus a MeshToad-class USB radio is recorded in
`docs/plans/meshtoad-gateway-bench.md`);
the Wipe deletion policy is not implemented; and coordinated fleet key rotation,
the airtime-budget admission gate, and the closed loop are designs on the roadmap,
not shipped behavior.
