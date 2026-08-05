# Research: What the industry calls this, and the legality of encrypted off-carrier networks

Researched 2026-08-04.

> This is informal research by non-lawyers, US-scoped, gathered at one point in time. It is not legal advice and not a statement of what the law is. It may be incomplete, wrong, or out of date, and it says nothing about jurisdictions outside the United States. Regulations change. You are solely responsible for verifying what applies to you. See the repository [DISCLAIMER](../../DISCLAIMER.md).

## Terminology - "INaC" is our coinage, not an industry term

"Intent Network as Code" / "INaC" has no established usage (and collides visually with "IaC"). The established vocabulary, from broadest to most specific:

| Term | Who uses it | Notes |
|---|---|---|
| **Intent-Based Networking (IBN)** | Gartner (coined Feb 2017, Lerner/Skorupa), Cisco ("The Network. Intuitive.", 2017), Apstra | The umbrella term. Canonical definitions: **[RFC 9315](https://www.rfc-editor.org/info/rfc9315/)** (IRTF informational, Oct 2022 - intent vs. policy, intent lifecycle, fulfillment/assurance) |
| **Intent-driven management** | Telecom standards bodies | **3GPP TS 28.312** (intent-driven management services; intent expectations/targets/contexts), **TM Forum TMF921** Intent Management API (Autonomous Networks program; IG1253 intent framework), **ETSI GR ZSM 011** (intent-driven autonomous networks). The three are deliberately aligned with each other and RFC 9315 |
| **Intent-driven automation / Configuration as Data** | **Nephio's own language** - "the user specifies what they want, the system figures out how"; intent as machine-manageable *data* (KRM), GitOps-managed | What NephMesh directly inherits |
| **Network as Code** | Two conflicting meanings | **Nokia's** Network as Code (2023) is a network **API monetization platform** (CAMARA/Open Gateway) - nothing to do with declarative config. **Cisco's** [NaC](https://netascode.cisco.com/docs/start/what_is_network_as_code/) is the IaC/GitOps-for-networking lineage (also "NetDevOps", ipSpace's "Network Infrastructure as Code"). Avoid unadorned "Network as Code" - the Nokia collision |

**How NephMesh should describe itself** (in order of legibility to industry/research readers):

1. **"Intent-driven network automation"** - safest; matches Nephio, 3GPP, and ETSI usage.
2. **"Intent-based networking meets GitOps"** - good tagline; both halves established.
3. **"Configuration as Data for LoRa mesh and SDR"** - signals the Nephio lineage precisely.

Fine to use "INaC" as an introduced coinage - just never present it as an existing industry term.

### Relation to AI agents (Claude Code + MCP, 2026)

Both are "intent → action" systems, but they're complementary, not the same thing: an LLM agent translates *natural-language* intent probabilistically and stops; Nephio-style automation continuously *reconciles* structured, versioned intent deterministically across sites. NephMesh's position (see roadmap, "Agent-native"): **the LLM proposes (NL → draft PackageRevision), a human approves via Porch's lifecycle, the reconciliation loop enforces.** The agent is never the control loop.

## Legality - dynamically created, encrypted, off-carrier networks (US)

**Short version of what we found for the US: this appears to be legal and ordinary, on the same regulatory basis as a Wi-Fi router.** This is our reading of the sources below, not a legal conclusion you should rely on; US-scoped, and for example EU 868 MHz adds duty-cycle limits. Verify your own situation.

- **Encryption on license-free ISM bands appears permitted under our reading of Part 15.** Meshtastic operates at 902–928 MHz under [47 CFR § 15.247](https://www.ecfr.gov/current/title-47/chapter-I/subchapter-A/part-15/subpart-C/subject-group-ECFR2f2e5828339709e/section-15.247), the same rule section as Wi-Fi and Bluetooth. Part 15 limits power/bandwidth/emissions and says nothing against encrypting content. Meshtastic's default AES-256-CTR (+ X25519 for DMs) appears consistent with this, and we found no license requirement. Constraints that do matter: 1 W conducted power cap (stock hardware is far under), and use FCC-certified radio modules operated within their grants - which commercial Meshtastic boards are. No duty-cycle limit in the US 915 MHz band.
- **The encryption prohibition people half-remember is a ham-band rule.** [47 CFR § 97.113(a)(4)](https://www.ecfr.gov/current/title-47/chapter-I/subchapter-D/part-97/subpart-B/section-97.113) bars amateur stations from obscuring message meaning - which is exactly why Meshtastic's opt-in "Licensed (Ham) Mode" disables encryption and broadcasts a callsign in exchange for higher power. Stay in ISM mode, keep encryption.
- **Every WPA3 Wi-Fi network is precedent:** an encrypted, dynamically formed, unlicensed network under the same § 15.247. Billions of devices, zero controversy.
- **Dynamic/adaptive spectrum behavior is designed into the band.** § 15.247 explicitly provides for frequency hopping; Wi-Fi auto-channel selection and Bluetooth adaptive hopping do what NephMesh's intent-driven channel management would do. Caveat: orchestration must never push a certified radio outside its FCC grant (band edges, power, modulation).
- **Where real legal risk lives:** (a) transmitting outside authorized bands; (b) transmitting with non-certified equipment - a HackRF is a test instrument with no Part 15 TX certification, so meaningful-power transmission outside controlled/licensed contexts risks equipment-authorization and interference violations ([47 CFR § 2.944](https://www.ecfr.gov/current/title-47/chapter-I/subchapter-A/part-2/subpart-J/subject-group-ECFRd5ad3b739dbf27a/section-2.944) adds SDR-specific certification rules). Receive-only sensing, NephMesh's default, **appears to need no authorization** in the US per 47 U.S.C. § 301's transmission-only scope; note that intentionally intercepting certain private or encrypted communications is separately restricted.
- **The FCC itself endorses dynamic private networks:** CBRS (47 CFR Part 96, 3.55–3.7 GHz) lets anyone run private LTE/5G with no spectrum license under the GAA tier - a sensor-coordinated shared-spectrum regime that is the licensed-lite cousin of NephMesh's idea, with 420k+ radios deployed.

**FAQ one-liner (carries its own hedge, since quotes travel without their context):** *"In our US-scoped reading, encrypted mesh on license-free bands appears to sit on the same FCC rule section as password-protected Wi-Fi, with encryption restrictions applying to amateur bands that NephMesh does not use unless you opt in. This is informal non-lawyer research, not legal advice; verify your own jurisdiction. See DISCLAIMER."*
