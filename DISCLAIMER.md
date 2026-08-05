# Disclaimer

NephMesh is an experimental research project for radio-adjacent systems. Please read this before using any of it.

## This is a research project, and lawful use is entirely up to you

NephMesh exists to explore ideas. It is not a product, not a service, and not advice of any kind. The authors make no claim to know the laws, regulations, or rules that apply to you.

Radio regulation differs by country, region, band, power level, device certification status, time, and sometimes by who you are (licensed amateur vs not), and it changes. **No one associated with this project can know, and none of them claims to know, the laws that apply in your location. You, and only you, are fully and solely responsible for determining what is legal where you are and for ensuring that anything you do with this code, and with any radio hardware it touches, complies with all applicable laws and regulations.** If you are unsure, consult a qualified professional and the relevant authority in your jurisdiction before transmitting anything.

By using this project you accept that all responsibility, risk, and consequence of that use is yours alone.

What the project does to help, and its limits:

- Defaults are receive-only. Software defined radio use in this project senses spectrum; it does not transmit, and transmit paths do not exist in the code today.
- Where the docs discuss legality (for example [docs/research/terminology-and-legality.md](docs/research/terminology-and-legality.md)), the analysis is US-scoped, cites its sources, and is informal research by non-lawyers gathered at a point in time. It is not legal advice, it is not a statement of what the law is, it may be incomplete, wrong, or out of date, and it says nothing about the rest of the world. Treat it as a starting point for your own verification, not a conclusion.
- Meshtastic hardware transmits on license-free bands under rules that vary by region (power, duty cycle, allowed frequencies). Configuring a radio for the wrong region, exceeding power limits, or operating non-certified transmit hardware may be illegal where you are. The project's region defaults are demo conveniences, not compliance guidance.
- Encryption of radio traffic may be permitted in some contexts and restricted in others (for example, it is commonly restricted on amateur bands, and some jurisdictions restrict it broadly). We do not assert which applies to you. Verify your own situation.

## No warranty, no endorsement

This software is provided under the [Apache License 2.0](LICENSE), which includes its disclaimer of warranty and limitation of liability. NephMesh is an independent experiment; it is not affiliated with or endorsed by the Nephio project, the Linux Foundation, Meshtastic LLC, Great Scott Gadgets, or any hardware vendor mentioned in the docs.

## Security is a work in progress

This is pre-alpha software with a published [threat model](docs/security/threat-model.md) that openly lists unmitigated risks. Do not deploy it anywhere that matters. If you find a problem, see [SECURITY.md](SECURITY.md).
