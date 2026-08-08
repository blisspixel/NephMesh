# Examples

Starting-point `MeshtasticNode` resources for the operator. They assume the CRD
and operator are installed (see the [operator package](../packages/meshtastic-operator/)
and the [Porch registration guide](../docs/guides/porch-registration.md)), and a
reachable device or a `meshtasticd --sim` at the address in each file.

| File | What it shows |
|---|---|
| [basic-node.yaml](basic-node.yaml) | The minimal declaration: region, modem preset, role, owner, over the TCP API. No secrets. |
| [secure-channel-node.yaml](secure-channel-node.yaml) | A private, encrypted channel whose pre-shared key comes from a Kubernetes Secret, never inlined or committed. |

To try the operator without a resource file or any hardware, the scripted
[operator demo](../demo/operator/) stands up a simulated radio and reconciles it,
including a secure channel, end to end.

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

## Secrets, said once more

Channel keys and the MQTT broker password are referenced from Secrets and never
belong in a `MeshtasticNode` or in Git. The example key in
`secure-channel-node.yaml` is a placeholder; generate your own, and never reuse
the public default channel key for anything private (the operator refuses an empty
key rather than silently falling back to it). The [threat model](../docs/security/threat-model.md)
is precise about what a shared channel key does and does not protect.
