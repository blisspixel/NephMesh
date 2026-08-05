# CRD API design: MeshtasticNode and SpectrumScan

Status: draft for review. Targets Phase 4 (MeshtasticNode) and Phase 6 (SpectrumScan, whose
declarative API lands when the policy loop needs it; Phase 2's sensing work uses a plain config
file and informs the CR design). Written now so package and operator work shares one vocabulary. Mirrors the conventions of
`github.com/nephio-project/api` as documented in `docs/research/nephio-codebase.md` so the types
could integrate into the Nephio ecosystem later with minimal rework. Nothing here presumes
upstreaming.

## 1. API group strategy

Decision: two groups, one per functional area, mirroring Nephio's per-area groups
(`infra.nephio.org`, `req.nephio.org`, `workload.nephio.org`):

- `mesh.nephmesh.io`: mesh radio intent (MeshtasticNode, later MeshTopology).
- `sense.nephmesh.io`: spectrum sensing intent (SpectrumScan, later a policy CR for Phase 6).

Rationale: a single `nephmesh.io` group would work today, but the two areas mature independently
(mesh reconciliation is Phase 4, closed-loop sensing policy is Phase 6), have different security
postures (mesh CRs reference PSK Secrets, sensing CRs do not), and per-area groups are the pattern
a Nephio reviewer would expect. Splitting later is a breaking change; splitting now is free.

The `nephmesh.io` domain is a provisional placeholder. An API group is a stable DNS-style string, not a claim that the domain is registered; for a pre-alpha experiment with no external consumers this is fine, and renaming the group while at `v1alpha1` is cheap. Revisit before any public or 1.0 release, when a controlled domain (or a deliberately chosen stable group) starts to matter.

Version: `v1alpha1` for both. No conversion webhooks until a v1beta1 exists.

Module layout (types-only `api/` module, exactly the api-repo shape):

```
api/
  go.mod                          module github.com/blisspixel/nephmesh/api (owner TBD, engineering-conventions open decision 1)
  mesh/v1alpha1/
    meshtasticnode_types.go       Spec/Status structs + kubebuilder markers
    meshtasticnode_interfaces.go  Validate(), condition helpers, builders
    groupversion_info.go          GroupVersion, SchemeBuilder, AddToScheme
    condition.go                  condition type/reason constants
    zz_generated.deepcopy.go      controller-gen output
  sense/v1alpha1/
    spectrumscan_types.go
    spectrumscan_interfaces.go
    groupversion_info.go
    condition.go
    zz_generated.deepcopy.go
```

Conventions carried over from the api repo: exported Kind constants
(`const MeshtasticNodeKind = "MeshtasticNode"`, `const SpectrumScanKind = "SpectrumScan"`) so KRM
functions build ObjectReferences without reflection; controller-gen (pin whatever version the
pinned Nephio release uses, v0.20.0 today) with `make generate` (deepcopy, boilerplate header) and
`make manifests` (CRDs into `config/crd/bases/` named `<group>_<plural>.yaml`); consumers import as
`meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"`. Apache-2.0 headers
(`The NephMesh Authors`) on every file.

## 2. MeshtasticNode (mesh.nephmesh.io/v1alpha1)

Field surface follows `docs/research/meshtastic.md` faithfully: everything below maps to a real
protobuf pref reachable via the TCP 4403 client API or Python CLI (`--set`, `--ch-set`,
`--export-config`/`--configure`). No invented device fields.

```go
type MeshtasticNodeSpec struct {
    // Connection selects exactly one transport. Validated in Validate().
    Connection ConnectionSpec `json:"connection"`

    // maps to lora.region (US, EU_868, ...). Required. Phase 4.
    Region string `json:"region"`
    // maps to lora.modem_preset (LONG_FAST, MEDIUM_SLOW, ...). Phase 4.
    ModemPreset string `json:"modemPreset,omitempty"`
    // maps to device.role (CLIENT, ROUTER, REPEATER, ...). Phase 4.
    Role string `json:"role,omitempty"`

    // Owner maps to the owner short/long name set via the client API. Phase 4.
    Owner *OwnerSpec `json:"owner,omitempty"`

    // Channels is the full desired channel set, by index. Phase 4.
    Channels []ChannelSpec `json:"channels,omitempty"`

    // MQTT maps to the MQTT module config. Phase 4.
    MQTT *MQTTSpec `json:"mqtt,omitempty"`

    // DeletionPolicy: what CR deletion means for the physical radio.
    // Retain (default): stop managing, leave the radio running with its
    // last-applied config. Wipe: factory-reset during finalization (later
    // phase; needs careful ordering, see section 5). An enum rather than a
    // boolean leaves room for a future third mode.
    // +kubebuilder:validation:Enum=Retain;Wipe
    DeletionPolicy string `json:"deletionPolicy,omitempty"`
}

type ConnectionSpec struct {
    TCP        *TCPConnection        `json:"tcp,omitempty"`        // Phase 4
    Serial     *SerialConnection     `json:"serial,omitempty"`     // Phase 4
    ViaGateway *ViaGatewayConnection `json:"viaGateway,omitempty"` // Phase 4 (remote admin)
}

type TCPConnection struct {
    Host string `json:"host"`
    Port int32  `json:"port,omitempty"` // default 4403
}

type SerialConnection struct {
    Device string `json:"device"` // e.g. /dev/ttyUSB0, exposed via device plugin
}

// Remote admin through a managed gateway: CLI --dest '!nodeid' over the admin channel.
type ViaGatewayConnection struct {
    GatewayRef corev1.LocalObjectReference `json:"gatewayRef"` // another MeshtasticNode
    Dest       string                      `json:"dest"`       // "!nodeid" of the target radio
}

type OwnerSpec struct {
    ShortName string `json:"shortName,omitempty"` // max 4 chars on device
    LongName  string `json:"longName,omitempty"`
}

type ChannelSpec struct {
    Index int32  `json:"index"` // 0 is primary
    Name  string `json:"name,omitempty"`
    // PSK is never inline. See section 6. Omit for the default PSK (discouraged).
    PSKSecretRef *corev1.SecretKeySelector `json:"pskSecretRef,omitempty"`
    // Per-channel MQTT flags (real device prefs).
    UplinkEnabled   bool `json:"uplinkEnabled,omitempty"`
    DownlinkEnabled bool `json:"downlinkEnabled,omitempty"`
}

// Mirrors the MQTT module config surface exactly (address, username, password,
// encryption_enabled, json_enabled, tls_enabled, root).
type MQTTSpec struct {
    Enabled           bool                      `json:"enabled"`
    Address           string                    `json:"address,omitempty"`
    Username          string                    `json:"username,omitempty"`
    PasswordSecretRef *corev1.SecretKeySelector `json:"passwordSecretRef,omitempty"`
    EncryptionEnabled bool                      `json:"encryptionEnabled,omitempty"`
    JSONEnabled       bool                      `json:"jsonEnabled,omitempty"` // lossy; unsupported on nRF52
    TLSEnabled        bool                      `json:"tlsEnabled,omitempty"`
    Root              string                    `json:"root,omitempty"`
}

type MeshtasticNodeStatus struct {
    Conditions      []metav1.Condition `json:"conditions,omitempty"`
    NodeID          string             `json:"nodeID,omitempty"` // "!hexid" from device
    FirmwareVersion string             `json:"firmwareVersion,omitempty"`
    NeighborCount   int32              `json:"neighborCount,omitempty"`
    LastHeard       *metav1.Time       `json:"lastHeard,omitempty"`
}
```

Reachability and sync state live in `Conditions` (section 5) rather than duplicate booleans;
`kubectl get` columns are derived from conditions via printcolumn markers. Region, modemPreset, and
role are string enums validated by kubebuilder markers against the protobuf enum names; keeping
them strings (not a Go enum type) avoids api-module churn when firmware adds values. Speculative:
whether `--seturl` style whole-channel-set replacement needs first-class support; deferred, the
per-channel list plus minimal diff covers it.

## 3. SpectrumScan (sense.nephmesh.io/v1alpha1)

```go
type SpectrumScanSpec struct {
    // SoapySDR driver string, e.g. "driver=hackrf" or "driver=rtlsdr". Config, not code.
    Driver string `json:"driver"`

    Bands []BandSpec `json:"bands"`

    BinWidthHz      int64  `json:"binWidthHz,omitempty"`
    IntervalSeconds int32  `json:"intervalSeconds,omitempty"` // sweep period
    Gain            string `json:"gain,omitempty"`            // SoapySDR gain expression

    Output OutputSpec `json:"output"`
}

type BandSpec struct {
    Name    string `json:"name"` // e.g. "ism-915"
    StartHz int64  `json:"startHz"`
    StopHz  int64  `json:"stopHz"`
}

type OutputSpec struct {
    // Per-band aggregate gauges (occupancy percent, max/mean dB), never per-bin series.
    Prometheus *PrometheusOutput `json:"prometheus,omitempty"`
    // Full spectra published to MQTT for consumers that want raw sweeps.
    MQTT *MQTTOutput `json:"mqtt,omitempty"`
}

type PrometheusOutput struct {
    Enabled bool  `json:"enabled"`
    Port    int32 `json:"port,omitempty"`
}

type MQTTOutput struct {
    Enabled bool   `json:"enabled"`
    Broker  string `json:"broker"`
    Topic   string `json:"topic,omitempty"`
}

type SpectrumScanStatus struct {
    Conditions       []metav1.Condition `json:"conditions,omitempty"`
    DeviceAttached   bool               `json:"deviceAttached,omitempty"`
    LastSweepTime    *metav1.Time       `json:"lastSweepTime,omitempty"`
    SamplesPerSecond int64              `json:"samplesPerSecond,omitempty"`
}
```

Receive-only by construction: the Spec has no transmit surface at all, matching project policy.

## 4. Interaction with Nephio types (Phase 3+)

- Packages: the `mesh-gateway` and `spectrum-sensor` blueprints carry a placeholder
  `infra.nephio.org/v1alpha1 WorkloadCluster` for PackageVariant injection, copying the
  `pkg-example-*-bp` catalog pattern. The CRs above ride inside those packages as plain KRM.
- `req.nephio.org` requirement types (Interface, DataNetwork, Capacity): recommendation is to stay
  independent now. Those types model IP interface attachment and data-network requirements for NF
  deployments; a LoRa mesh channel is not an IP attachment, and forcing the fit would import Nephio
  API dependencies into the core for no mechanical benefit. Document the conceptual mapping
  instead: MeshtasticNode is intent at roughly the `workload.nephio.org/NFDeployment` altitude; a
  channel plus MQTT uplink is the moral equivalent of a DataNetwork requirement. Revisit only if a
  specializer ever needs to consume requirement CRs.
- Future `MeshTopology` CR (deferred, sketch only, speculative): a mesh-wide intent
  (shared channel set, region, PSK rotation policy) that a controller fans out into per-node
  MeshtasticNode CRs, one per radio, gateway nodes managing radio-only nodes via `viaGateway`.
  Not designed further until Phase 5/6 experience shows what it needs.

## 5. Reconciliation semantics

MeshtasticNode reconcile loop (Phase 4): connect over the selected transport, export live config
(the CLI `--export-config` YAML round-trip format, or the Python library equivalents), diff against
Spec section by section, apply only drifted sections. Each applied section reboots the node, so the
loop must batch chained settings into one apply where possible, and must expect the connection to
drop and re-verify after reboot. `--configure` is not a diff; the operator never blindly re-applies
full config.

Condition types (constants in `condition.go`), following `metav1.Condition`:

| Type          | True when                          | Reasons (enumerated)                                         |
|---------------|------------------------------------|--------------------------------------------------------------|
| `Reachable`   | device API session established     | `Connected`, `ConnectFailed`, `GatewayUnreachable`           |
| `ConfigInSync`| exported config matches Spec       | `InSync`, `DriftDetected`, `ApplyFailed`, `RebootPending`, `SecretMissing` |
| `Ready`       | Reachable and ConfigInSync         | `Ready`, `NotReady`                                          |

SpectrumScan conditions: `DeviceReady` (`DeviceAttached`, `DeviceMissing`, `DriverError`) and
`Sweeping` (`SweepOK`, `SweepFailed`).

Finalizers, following the `"<group>/finalizer"` pattern observed in Nephio controllers:
`mesh.nephmesh.io/finalizer` and `sense.nephmesh.io/finalizer`.

Deletion semantics for a physical radio: default is `deletionPolicy: Retain` (stop managing).
Deleting the CR removes the operator's claim on the device and nothing else; the radio keeps
running with its last-applied config. Rationale: radios are often shared or community
infrastructure, a factory reset is destructive and unrecoverable over the air, and wiping a
remote node can sever the admin-channel path used to manage it. `deletionPolicy: Wipe` opts into
a factory reset during finalization for lab teardown, and is a later-phase feature because it
needs careful ordering (wipe before the gateway that carries the admin channel is itself
deleted).

## 6. Secrets handling (channel PSKs, MQTT passwords)

- PSKs and broker passwords are never inline in CRs, never in Git, never in kpt packages. CRs carry
  only `corev1.SecretKeySelector` references; the controller reads the Secret at reconcile time and
  requeues with `ConfigInSync=False/SecretMissing` if absent.
- Channel share URLs (`--seturl`) encode PSKs; they are treated as secrets too and never appear in
  package YAML or logs. Exported config from the device includes PSKs, so the operator must redact
  exports before any logging or status reporting.
- kpt packages ship the Secret reference (name/key) as ordinary package data, with the name
  settable via the Kptfile pipeline (apply-replacements) per site. Secret material is provisioned
  out of band on each workload cluster; SealedSecrets or External Secrets Operator are compatible
  choices but not dependencies (open decision, section 8).
- Do not reuse the Meshtastic default PSK for anything managed.

## 7. condkptsdk fit (future mesh-gateway specializer function)

Speculative until Phase 3/4 lands, but the shape is clear: a `mesh-gateway-fn` KRM function built
on condkptsdk would specialize the mesh-gateway package, with MeshtasticNode as its For resource
and the workload plumbing as Owns children:

| Role  | Resource                                   | Kind (condkptsdk)      | Notes                                          |
|-------|--------------------------------------------|------------------------|------------------------------------------------|
| For   | `mesh.nephmesh.io/v1alpha1 MeshtasticNode` |                        | the intent being specialized                   |
| Owns  | `apps/v1 Deployment`                       | ChildRemote            | meshtasticd (simulated or hardware variant)    |
| Owns  | `v1 Service`                               | ChildRemote            | TCP 4403 device API                            |
| Owns  | `v1 PersistentVolumeClaim`                 | ChildRemote            | `/var/lib/meshtasticd/.portduino` identity     |
| Owns  | `v1 ConfigMap`                             | ChildRemote            | host-side `/etc/meshtasticd/config.yaml`       |
| Watch | `infra.nephio.org/v1alpha1 WorkloadCluster`| callback               | site context (cluster name, labels), errors if missing |

PopulateOwnResourcesFn stays pure and idempotent; ownership uses the SDK's
`specializer.nephio.org/owner` annotations unchanged. A `viaGateway` MeshtasticNode emits no
workload children (the radio is not a pod), so the populate function returns only what the
transport implies. A parallel `spectrum-sensor-fn` would use SpectrumScan as For with the scanner
Deployment, exporter ConfigMap, and device-plugin resource requests as Owns.

## 8. Open decisions (resolutions annotated 2026-08-04)

1. Module placement for `api/`: DECIDED. In-repo module until an external consumer exists; already
   its own Go module, so a later split to a separate repo is mechanical.
2. Secret provisioning: DECIDED. Document plain out-of-band Kubernetes Secrets as the recommended
   path; SealedSecrets and External Secrets Operator remain compatible options, never dependencies.
3. `viaGateway.dest`: DECIDED for v1alpha1. Node IDs are stable, human-provided Spec fields; no
   discovery. Whether admin-channel key material needs its own secretKeyRef is settled at
   implementation time against real firmware (Phase 4).
4. Enum shape: DECIDED. Strings with kubebuilder validation markers, for firmware-churn tolerance.
5. SpectrumScan gain: OPEN pending Phase 2 findings with the HackRF Pro (plain SoapySDR gain string
   proposed).
6. Deletion shape: DECIDED. `deletionPolicy: Retain|Wipe` enum (Retain default), replacing the
   earlier `wipeOnDelete` boolean; reflected in section 2 and section 5.
