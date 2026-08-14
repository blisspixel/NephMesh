# Examples

Starting-point resources for the operator. They assume the CRDs and operator are
installed (see the [operator package](../packages/meshtastic-operator/) and the
[Porch registration guide](../docs/guides/porch-registration.md)), and a reachable
device or a `meshtasticd --sim` at the address in each file.

| File | What it shows |
|---|---|
| [basic-node.yaml](basic-node.yaml) | The minimal declaration: region, modem preset, role, owner, over the TCP API. No secrets. |
| [secure-channel-node.yaml](secure-channel-node.yaml) | A private, encrypted channel whose pre-shared key comes from a Kubernetes Secret, never inlined or committed. |
| [regional-intent.yaml](regional-intent.yaml) | A `CommunicationIntent`: the outcome-level input the operator compiles, report-only, into proposed `MeshtasticNode` specs on status. It never actuates. |

To try the operator without a resource file or any hardware, the scripted
[operator demo](../demo/operator/) stands up a simulated radio and reconciles it,
including a secure channel, end to end. The MeshToad plus T-Deck LoRa bench is
[demo/meshtoad-gateway](../demo/meshtoad-gateway/), not a `MeshtasticNode` that
points at a LAN 4403 (the device API stays on localhost).

## Watching a node

The `MeshtasticNode` printer columns surface the important state at a glance:

```console
$ kubectl get meshtasticnode
NAME       READY   REGION   PRESET        NODEID       LASTHEARD   AGE
field-01   True    US       MEDIUM_SLOW   !6e000001    12s         2m
```

`kubectl describe meshtasticnode field-01` shows the full condition set (Reachable,
ConfigInSync, RebootPending, Ready, Degraded, plus AirtimeHealthy, ChannelsInSync,
and AirtimeBudget where they apply) and the Events (config applied, degraded, a
missing Secret), so you can see both the current state and what the operator did.

## Reading an intent's plan

A `CommunicationIntent` is compiled, not actuated. Applying `regional-intent.yaml`
creates no `MeshtasticNode` and touches no radio; the operator records what it
would render on the intent's status:

```console
$ kubectl get communicationintent
NAME              REGION   PRESET        NODES   FEASIBLE   AGE
regional-relief   US       MEDIUM_SLOW   2       True       5s
```

`FEASIBLE=True` means the approved set held a preset the airtime model recognizes
(here `MEDIUM_SLOW`, the first approved entry); an empty approved set, no target
nodes, or an all-unknown set reports `False` with the reason on the `Feasible`
condition. The proposed per-device specs are on `.status.proposedNodes`:

```console
$ kubectl get communicationintent regional-relief -o jsonpath='{.status.proposedNodes[*].name}'
relief-01 relief-02
```

You apply those `MeshtasticNode` specs yourself: the report-only boundary (ADR
0001) is enforced by RBAC, not merely intended. Autonomous actuation is gated
behind the signed-autonomy and safety-kernel work (ADR 0002).

If the intent declares `expectedTraffic`, the operator also reports whether the
whole fleet fits the channel's airtime budget on the `AirtimeWithinBudget`
condition and `.status.predictedChannelUtilizationPercent`. This is the
fleet-wide airtime check only the intent layer can make; the per-node reconcile
sees one radio, not the shared channel. The estimate is a conservative floor
(each message counted once, mesh rebroadcast ignored), so an over-budget verdict
is certain while within-budget is advisory, and the device's measured
`AirtimeHealthy` condition stays the ground truth.

## Secrets, said once more

Channel keys and the MQTT broker password are referenced from Secrets and never
belong in a `MeshtasticNode` or in Git. The example key in
`secure-channel-node.yaml` is a placeholder; generate your own, and never reuse
the public default channel key for anything private (the operator refuses an empty
key rather than silently falling back to it). The [threat model](../docs/security/threat-model.md)
is precise about what a shared channel key does and does not protect.
