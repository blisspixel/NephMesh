# NephMesh Agent Plugin

An [Agent Plugins 1.0.0](https://agent-plugins.org/specification) package around
the binaries this repo already ships. It is a directory layout plus manifests,
not a new control loop.

```text
agent-plugin/
├── plugin.json
├── mcp.json
└── skills/
    ├── plan-intent/SKILL.md
    ├── operator-demo/SKILL.md
    └── meshtoad-bench/SKILL.md
```

`mcp.json` launches `nephmesh-mcp` from PATH (stdio). Build it from
`operators/meshtastic-operator/cmd/nephmesh-mcp`. The tools stay report-only:
`plan_intent` and `sense_spectrum` do not write to a radio.

Point a compatible client at this directory. Client-specific install, trust,
and UX stay with the client. The agent is never the live reconcile loop
(ADR 0002).

Skills follow the [Agent Skills](https://agentskills.io/specification) `SKILL.md`
format. Do not add a parallel `.claude/skills/` tree.
