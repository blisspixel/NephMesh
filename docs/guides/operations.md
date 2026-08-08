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

The operator ships as a kpt package at `packages/meshtastic-operator/` (CRD,
least-privilege RBAC, ServiceAccount, Deployment, NetworkPolicy). Two honest notes
before you start:

- The operator image is not published to a public registry yet (that is a pending
  release step, see the roadmap). Until then you build it from the repo and load
  it into your cluster (for kind: `docker build` then `kind load docker-image`).
  The `Dockerfile` bundles the pinned Meshtastic CLI and the helper scripts.
- For the full Nephio/Porch path (register the repo, propose/approve/apply), see
  the [Porch registration guide](porch-registration.md). For a plain cluster,
  `kubectl apply` the rendered package.

To try the reconcile engine with no cluster and no hardware at all, the scripted
[operator demo](../../demo/operator/) stands up a simulated radio and converges
it, including a secure channel, end to end.

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
- **Turn a feature off.** Owned fields (MQTT enabled, encryption, JSON) reconcile
  both ways: set `mqtt.enabled: false` (or `encryptionEnabled: false`) and the
  operator turns it off on the device, it does not only ever turn things on.
- **Decommission a node.** Delete the `MeshtasticNode`. The default deletion policy
  is Retain: the operator stops managing the device and removes its finalizer, and
  the device keeps running with its last-applied config. A Wipe policy that clears
  the device is a later-phase feature and is not performed yet.
- **Upgrade the operator.** Redeploy the new operator image; the reconcile loop is
  idempotent, so a rolling replace does not disturb configured devices. The
  `MeshtasticNode` API is `v1alpha1` and may still change before 1.0.

## What this operator does not do (yet)

Stated plainly so nothing here overpromises: serial and viaGateway transports are
reported as unsupported by the deployed operator (TCP is the wired path today; the
reconcile engine drives serial via the `reconcile-demo` tool and a real board);
the Wipe deletion policy is not implemented; and coordinated fleet key rotation,
the airtime-budget admission gate, and the closed loop are designs on the roadmap,
not shipped behavior.
