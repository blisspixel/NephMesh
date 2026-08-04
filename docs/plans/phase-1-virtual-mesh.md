# Phase 1 implementation plan: virtual mesh on a single-node cluster

Release gate: 0.1 (a virtual mesh node deployed, configured, and torn down declaratively). Cost: $0. No radios, no Nephio machinery yet. Sources: `docs/roadmap.md` (Phase 1), `docs/research/meshtastic.md`, `docs/research/nephio-codebase.md`. Facts not backed by those docs are flagged inline as VERIFY.

## 1. Deliverables (mapped to roadmap checkboxes)

| Roadmap checkbox | Deliverable in this plan |
|---|---|
| Deployment for meshtasticd in simulation mode | `meshnode-sim` Deployment (section 3.1) |
| PVC for `.portduino`, stable hardware ID via `-h` | `meshnode-portduino` PVC plus `-h` arg (3.2) |
| Declarative node config applied by Job or init container | ConfigMap plus `meshnode-configure` Job (3.4, 3.5) |
| In-cluster Mosquitto broker, MQTT module enabled | `mosquitto` Deployment, Service, config (3.6) |
| Demo: apply, sendtext, observe on MQTT, teardown | Demo script (section 4) |
| Stretch: Meshtasticator multi-node | Validation item V3 (section 5), not a gate |

## 2. Where this lands in the repo

Decision: plain manifests in `demo/phase1/`, no kpt packaging yet.

```
demo/phase1/
  README.md              How to run the demo on k3s or k3d/kind
  manifests/
    namespace.yaml       Namespace nephmesh
    meshtasticd.yaml     Deployment + PVC + Service
    node-config.yaml     ConfigMap with the desired-state node YAML
    configure-job.yaml   The config-apply Job
    mosquitto.yaml       Broker Deployment + ConfigMap + Service
  scripts/
    demo.sh              Apply, wait, sendtext, mosquitto_sub, assert
    teardown.sh          Delete and verify clean
```

Justification: AGENTS.md says planned layout dirs are not created until their roadmap phase starts. `packages/` (kpt) is Phase 3 work; the roadmap explicitly converts "the Phase 1 and 2 workloads" into kpt packages there. Starting with plain KRM YAML keeps Phase 1 honest about what it is (the smallest end-to-end pipeline) and costs nothing later, because kpt packages are plain KRM YAML mutated by Kptfile pipelines: the Phase 3 conversion is moving these files under `packages/mesh-gateway/` and `packages/mqtt-bridge/` and adding Kptfile, package-context.yaml, and a placeholder WorkloadCluster. Manifests are written with that split in mind (gateway resources and broker resources in separate files, no cross-file templating).

## 3. Kubernetes resources

Namespace `nephmesh` for everything. All images below publish arm64 variants (or must be verified to, per resource) so the same manifests run on the Orange Pi 5 and the PC.

### 3.1 meshtasticd Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata: {name: meshnode-sim, namespace: nephmesh}
spec:
  replicas: 1
  strategy: {type: Recreate}        # single RWO PVC; never two pods on it
  selector: {matchLabels: {app: meshnode-sim}}
  template:
    metadata: {labels: {app: meshnode-sim}}
    spec:
      containers:
      - name: meshtasticd
        image: meshtastic/meshtasticd:beta-debian
        args: ["-s", "-h", "dc:2c:6e:00:00:01", "-p", "4403"]
        ports: [{name: device-api, containerPort: 4403}]
        readinessProbe: {tcpSocket: {port: 4403}, initialDelaySeconds: 5}
        volumeMounts:
        - {name: portduino, mountPath: /var/lib/meshtasticd/.portduino}
      volumes:
      - name: portduino
        persistentVolumeClaim: {claimName: meshnode-portduino}
```

Decisions:

- Image tag `beta-debian`, not alpine. The research doc confirms the tag matrix (`beta|alpha|daily` x `debian|alpine`, amd64/arm64/arm-v7/riscv64). Debian is chosen because the official Docker docs and most community usage exercise the debian variant, so glibc-linked Portduino behavior is the well-trodden path; alpine (musl) saves image size we do not care about in Phase 1. `beta` because it tracks released firmware rather than daily builds. VERIFY: exact composed tag name on Docker Hub (`beta-debian` vs `beta` defaulting to debian) at implementation time, then pin by digest in the manifest.
- `-s` simulated radio: official image, real firmware, no radio, standard device API on TCP 4403 (research-confirmed, ideal Phase 1 target). A lone `-s` node is a mesh of one, which is sufficient for the gate.
- `-h <MAC>` stable hardware ID for containers (research-confirmed flag). Value is arbitrary but fixed in the manifest so identity is deterministic even before the PVC question is settled. VERIFY: exact accepted format of the `-h` argument.
- VERIFY: whether the image entrypoint passes `args` through to meshtasticd or needs a full `command`.

### 3.2 PVC

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: meshnode-portduino, namespace: nephmesh}
spec:
  accessModes: [ReadWriteOnce]
  resources: {requests: {storage: 128Mi}}
```

No `storageClassName`: uses the cluster default (local-path on k3s and k3d, standard on kind), keeping the manifest provider-neutral. Prefs live at `/var/lib/meshtasticd/.portduino/default/prefs/`; without the PVC, node identity resets on pod restart (research-confirmed). Mounting the parent `.portduino` directory captures everything Portduino persists.

### 3.3 Service

```yaml
apiVersion: v1
kind: Service
metadata: {name: meshnode-sim, namespace: nephmesh}
spec:
  selector: {app: meshnode-sim}
  ports: [{name: device-api, port: 4403, targetPort: 4403}]
```

ClusterIP. The demo reaches it via `kubectl port-forward`; the config Job reaches it by DNS name.

### 3.4 ConfigMap: desired-state node YAML

The desired state is one YAML document in the Meshtastic CLI `--export-config` / `--configure` format (owner, channels including PSKs, config, module config; research-confirmed as the round-trip format). Implementation step one is empirical: bring up a bare `-s` node, run `--export-config`, and use that captured output as the canonical schema rather than hand-guessing field names. Sketch of intent (field names to be corrected against the export):

```yaml
apiVersion: v1
kind: ConfigMap
metadata: {name: meshnode-desired-state, namespace: nephmesh}
data:
  node.yaml: |
    owner: NephMesh Sim 01
    config:
      lora: {region: US}
    module_config:
      mqtt:
        enabled: true
        address: mosquitto.nephmesh.svc.cluster.local
        encryption_enabled: true
        json_enabled: true      # lossy convenience topics for eyeball inspection
```

Protobuf topics (`msh/REGION/2/e/...`) are canonical; JSON is enabled only for demo readability and is fine here because meshtasticd supports it (the nRF52 limitation does not apply). Per-channel `uplink_enabled` must be true for the primary channel. VERIFY: whether channel uplink flags round-trip through `--export-config`/`--configure`; if not, the Job adds an explicit `--ch-set uplink_enabled true --ch-index 0`. Demo uses the default primary channel; do not reuse the default PSK for anything beyond this demo.

### 3.5 Config-apply Job

Decision: a Job, not an init container. An init container runs before the meshtasticd container in the same pod, so nothing is listening on 4403 yet. A native sidecar could work but a Job is simpler, retryable, and is exactly the artifact the Phase 4 operator replaces.

```yaml
apiVersion: batch/v1
kind: Job
metadata: {name: meshnode-configure, namespace: nephmesh}
spec:
  backoffLimit: 4
  template:
    spec:
      restartPolicy: OnFailure
      containers:
      - name: configure
        image: python:3.13-slim
        command: ["/bin/sh", "-c"]
        args:
        - |
          pip install --quiet "meshtastic[cli]"
          H=meshnode-sim.nephmesh.svc.cluster.local
          until meshtastic --host $H --info >/dev/null 2>&1; do sleep 3; done
          meshtastic --host $H --export-config > /tmp/live.yaml
          if python /config/compare.py /tmp/live.yaml /config/node.yaml; then
            echo "already converged"; exit 0
          fi
          meshtastic --host $H --configure /config/node.yaml
        volumeMounts: [{name: desired, mountPath: /config}]
      volumes:
      - name: desired
        configMap: {name: meshnode-desired-state}
```

Idempotency: `--configure` is not a true diff, and each applied section reboots the node (research-confirmed), so the Job exports first and exits early when live state already matches desired state (a small compare script normalizing the export, shipped in the ConfigMap). Re-running the Job on a converged node must be a no-op with zero reboots: that is the demo's idempotency check and the seed of the Phase 4 export-diff-apply loop. During apply, per-section reboots will drop the TCP connection; `backoffLimit` retries plus the converged-early-exit make the Job self-healing across reboot cycles. `pip install` at Job start needs internet access; acceptable for Phase 1, with a prebuilt CLI image as the offline fallback (no official Meshtastic CLI container image was found in research; building one is deferred).

### 3.6 Mosquitto

```yaml
apiVersion: v1
kind: ConfigMap
metadata: {name: mosquitto-config, namespace: nephmesh}
data:
  mosquitto.conf: |
    listener 1883
    allow_anonymous true      # demo only; broker is cluster-internal
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: mosquitto, namespace: nephmesh}
spec:
  replicas: 1
  selector: {matchLabels: {app: mosquitto}}
  template:
    metadata: {labels: {app: mosquitto}}
    spec:
      containers:
      - name: mosquitto
        image: eclipse-mosquitto:2
        ports: [{containerPort: 1883}]
        volumeMounts: [{name: conf, mountPath: /mosquitto/config}]
      volumes: [{name: conf, configMap: {name: mosquitto-config}}]
---
apiVersion: v1
kind: Service
metadata: {name: mosquitto, namespace: nephmesh}
spec:
  selector: {app: mosquitto}
  ports: [{port: 1883}]
```

Private broker, per the research finding that the public `mqtt.meshtastic.org` broker is heavily restricted. No persistence (retained state is worthless in a demo). VERIFY: `eclipse-mosquitto` arm64 availability (expected yes for the official image, confirm before pinning a digest).

## 4. Demo script (the 0.1 gate)

`demo/phase1/scripts/demo.sh`, runnable verbatim from the README:

```sh
kubectl apply -f demo/phase1/manifests/
kubectl -n nephmesh wait --for=condition=Available deploy/mosquitto deploy/meshnode-sim --timeout=180s
kubectl -n nephmesh wait --for=condition=Complete job/meshnode-configure --timeout=300s

# Watch the canonical protobuf topics (and JSON for readability) from inside the cluster
kubectl -n nephmesh run mqtt-watch --image=eclipse-mosquitto:2 --restart=Never -- \
  mosquitto_sub -h mosquitto -t 'msh/#' -v &

kubectl -n nephmesh port-forward svc/meshnode-sim 4403:4403 &
pip install "meshtastic[cli]"
meshtastic --host 127.0.0.1 --info                       # shows configured region, MQTT module on
meshtastic --host 127.0.0.1 --sendtext "hello nephmesh"
kubectl -n nephmesh logs -f pod/mqtt-watch               # expect the message on msh/US/2/...

# Teardown
kubectl delete -f demo/phase1/manifests/                  # removes everything including the PVC
```

Passing means all of: (1) apply-to-ready with no manual intervention; (2) the configure Job completes and a second run of it exits "already converged" with no reboot; (3) the sendtext appears on a `msh/US/2/...` topic within 60 seconds of sending; (4) teardown leaves no resources in the namespace. VERIFY (assumption): that a lone node uplinks its own transmitted text to MQTT. If it does not, the fallback observable is the node's periodic nodeinfo/telemetry packets on the same topics, and the gate wording adjusts to "node traffic appears on MQTT".

## 5. Validation items (from "What research cannot answer")

- V1, PVC identity across restarts: after the demo passes, record the node ID from `--info`, `kubectl delete pod` the meshtasticd pod, wait for readiness, confirm the same node ID and intact channel config. Negative control: delete the PVC too and confirm identity does reset, proving the PVC is what carries it. Also test whether `-h` alone (fresh PVC, same `-h` value) reproduces the same node ID, which determines how much identity weight the flag versus the volume carries. Result recorded in `docs/research/meshtastic.md`.
- V2, reconnect behavior: kill the pod mid `mosquitto_sub` session and confirm the MQTT uplink resumes after restart without reconfiguration.
- V3, Meshtasticator viability (stretch, not a gate): run Meshtasticator in a container per its docs and observe whether the known Docker reconnect issue bites. Outcome (works, works with workaround, or fails) goes into the research doc; failure means Phase 1 CI stays single-node simulation, the fallback the roadmap already names.

## 6. Cluster bootstrap

Both paths are supported and the manifests are identical:

- Orange Pi 5, k3s (arm64): `curl -sfL https://get.k3s.io | sh -`, default local-path storage class. This is the realistic edge target.
- PC, k3d or kind (amd64): `k3d cluster create nephmesh` or `kind create cluster`. This is the CI and newcomer path.

arm64 image audit for every image in this phase: `meshtastic/meshtasticd` publishes arm64 (research-confirmed); `eclipse-mosquitto` and `python` official images are expected multi-arch (VERIFY and pin digests per-arch or use the multi-arch manifest list). No other images are used.

## 7. Risks and fallbacks

- Per-section reboots make `--configure` racy (CLI loses the TCP session mid-apply). Fallback: Job retries plus converged-early-exit; worst case, split the apply into ordered `--set` batches.
- The export/desired-state compare is fragile if `--export-config` output includes volatile fields (timestamps, node numbers). Fallback: compare only the fields the desired state declares.
- Image entrypoint or tag assumptions wrong. Fallback: set explicit `command`, adjust tag, pin digest; one-line fixes.
- Meshtasticator fails in containers (V3). Fallback: single-node simulation only, already acceptable per the roadmap.
- `pip install` in the Job requires egress. Fallback: prebuild a small CLI image and reference it.

## 8. Non-goals for Phase 1

No real radios, no USB or SPI passthrough, no SDR, no Nephio components (Porch, PackageVariant, Config Sync), no kpt packaging, no CRDs or operator, no reconciliation loop beyond the Job's converged check, no multi-site, no auth on the demo broker, no cloud services of any kind.

## 9. Open decisions needing a human call

- OD1: DECIDED (by the passing gate). `kubectl apply` alone is the 0.1 gate; GitOps is optional polish.
- OD2: DECIDED. Region US (validated; note the observed topic prefix is `msh/2/...` regardless, see section 10).
- OD3: DECIDED. Deferred until the image/registry pipeline exists (Phase 4 per the conventions plan); the pip-at-runtime path passed the gate.
- OD4: DECIDED. Attempt-and-record, not gate; outcome recorded in `docs/research/meshtastic.md` and the roadmap.

## 10. Validation results (Docker rehearsal, 2026-08-04)

The full pipeline (simulated node, YAML configure, channel uplink, sendtext, MQTT observation) was rehearsed end to end in plain Docker against `meshtastic/meshtasticd:beta-debian` (firmware 2.7.26) before the manifests were written. Where findings differ from the sections above, the manifests in `demo/phase1/manifests/` are authoritative. Resolved VERIFY items:

- Image tag: `beta-debian` exists on Docker Hub, multi-arch (amd64, arm64, arm, riscv64). Resolved.
- Entrypoint: none; default cmd is `sh -cx "meshtasticd --fsdir=/var/lib/meshtasticd"`. The Deployment sets an explicit `command`. Resolved (section 3.1 assumption corrected).
- Persistence: `--fsdir` is the state root and prefs land at `<fsdir>/prefs/`; the PVC mounts at `/var/lib/meshtasticd`, not `.portduino`. Section 3.2 corrected by the manifests.
- `-h` format: `--hwid=dc2c6e000001` (12 hex digits, no colons) produced MAC `dc:2c:6e:00:00:01` and node ID `!6e000001`. Deterministic identity confirmed.
- Reboot behavior: applying config makes the process exit with code 1. In Kubernetes the Deployment restart covers it; the applier waits and re-verifies.
- Lone-node uplink: confirmed yes. A single simulated node publishes its own sent text to both protobuf and JSON topics. The gate observable stands as written.
- Topic shape: observed `msh/2/e/CHANNEL/!id`, with no region segment (docs elsewhere describe `msh/REGION/2/e/...`). Demo assertions subscribe to `msh/#`.
- MQTT address: hostname did not connect (no error logged, just "MQTT not connected" retries); the broker IP connected instantly. The applier resolves the Service DNS name to its ClusterIP and configures that. Whether in-cluster DNS behaves differently remains open; the IP path works regardless.
- `--configure` round trip: partial YAML (only the desired subset) applies cleanly; export output is prefixed with a comment marker line the applier strips.
- Mosquitto: `eclipse-mosquitto:2` ships a built-in `/mosquitto-no-auth.conf`, removing the need for a custom broker ConfigMap. Section 3.6 simplified accordingly.
- Duplicates: the same packet id appears multiple times on the broker (rebroadcast); consumers dedupe on `id`, as the research predicted.

Still open from section 5: V1 (PVC identity across pod restarts) and V3 (Meshtasticator) need a Kubernetes run, not a Docker one.
