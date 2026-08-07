# Regulatory matrix (informal)

This is informal, non-lawyer research to make the shape of the constraints visible,
not legal advice and not authoritative. Numbers change, vary by exact sub-band and
hardware, and are frequently misstated on the internet (including here). Verify every
value against the primary sources before you transmit anything: your national
regulator, the relevant standard, and the Meshtastic LoRa region reference, which
tracks the per-region frequency plans the firmware actually uses. Read the repository
[DISCLAIMER](../../DISCLAIMER.md) first: legality where you are is your
responsibility.

Why this table exists here, and not only as a link: the single sharpest difference
between regions, the duty cycle, is a hard airtime constraint, and the
[airtime-as-a-commons doctrine](../design/doctrine.md) treats it as exactly that. A
`ChannelBudget` scoped to an interference domain has to know the region's ceiling
before it can protect an emergency reserve. So the regulatory facts are an input to
the design, not just paperwork.

## The dimensions that matter

For any region, the reconciler and the operator care about six things:

1. The license-free band(s) the mesh transmits on.
2. The governing regulation.
3. Duty cycle: is transmit time capped as a fraction of the hour, or not? This is the
   biggest operational lever and the one an airtime budget must respect.
4. Power limit (EIRP or ERP), which bounds range and therefore density.
5. Listen-before-talk or dwell-time requirements.
6. Encryption legality, which depends less on the band than on whether you transmit
   under a license-free allocation or under an amateur license.

## Regions

The values below are widely documented but must be confirmed against the primary
sources. Where a figure is commonly cited but sub-band-dependent, it is marked as
such rather than stated as fact.

| Region (firmware id) | Band | Regulation | Duty cycle | Power (commonly cited) | LBT / dwell | Notes |
|---|---|---|---|---|---|---|
| US (US) | 902 to 928 MHz | FCC Part 15.247 | None | up to ~30 dBm EIRP with antenna limits; devices run far below | frequency-hopping / dwell rules apply | No duty cycle, so airtime pressure is self-imposed, which is precisely where a budget earns its keep |
| EU 868 (EU_868) | 863 to 870 MHz | ETSI EN 300 220 | Yes, sub-band-dependent (commonly 1% or 0.1%) | commonly 25 mW ERP (14 dBm) on the main sub-band | some sub-bands allow LBT+AFA in place of duty cycle | The duty cycle is a hard airtime ceiling the budget must model |
| EU 433 (EU_433) | 433 MHz | ETSI EN 300 220 | Yes, sub-band-dependent | commonly 10 mW ERP | | Lower power and band, shorter range |
| ANZ (ANZ) | 915 to 928 MHz | ACMA class licence | None (LIPD class licence conditions apply) | class-licence limited | | Similar operational profile to US |
| Japan (JP) | 920 to 925 MHz | ARIB STD-T108 | conditions apply | limited | LBT required | Listen-before-talk is mandatory, which interacts with any change-rendezvous timing |
| China (CN) | 470 to 510 MHz | SRRC | conditions apply | limited | | Different band entirely; verify carefully |
| India (IN) | 865 to 867 MHz | WPC | conditions apply | limited | | Narrow allocation |
| Korea (KR) | 920 to 923 MHz | conditions apply | conditions apply | limited | | |

The firmware ids are the values the Meshtastic region setting uses; the operator
reconciles that field. Regions not listed here (and the exact current numbers for the
ones that are) live in the Meshtastic region reference, which is the source this table
defers to.

## Encryption

A point that is easy to get wrong: whether encryption is legal usually depends on the
allocation you transmit under, not on the band's physics.

- On license-free ISM allocations (the default Meshtastic case, for example US 915 or
  EU 868), encryption is generally permitted. NephMesh treats a non-default channel key
  as first-class for exactly this case.
- Under an amateur-radio licence on amateur frequencies, encryption that obscures the
  meaning of a message is generally prohibited (in the US, FCC Part 97). Meshtastic can
  be run on amateur bands in some regions and at higher power under a licence, and that
  configuration typically forfeits the ability to use encrypted channels.

The project's [threat model](../security/threat-model.md) is separately honest about
what Meshtastic encryption does and does not provide (confidentiality with a shared
key, but no per-sender authentication), which is a different question from whether
encryption is legal for you to use.

## How this feeds the design

- Duty cycle is a per-region hard invariant. In doctrine terms it belongs to
  constitutional memory: never traded away by any autonomous behavior, and an input to
  the `ChannelBudget`'s safe ceiling.
- The SDR side stays receive-only regardless of region, so its regulatory surface is
  much smaller; transmit from the mesh node is the regulated act, and the operator
  configures it.
- The transmit interlocks the CI already enforces (receive-only SDR, no default keys)
  are the mechanical expression of the two facts most likely to cause trouble:
  transmitting where you should not, and shipping a shared key everyone already knows.

This document should be kept current as a living reference, alongside the research
docs, rather than treated as settled.
