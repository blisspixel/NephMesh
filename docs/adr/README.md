# Architecture Decision Records

An ADR captures one significant, hard-to-reverse decision: the context that forced
it, the choice made, and the consequences accepted. They exist so a later reader (or
a later maintainer, or an AI agent) can see why the project is shaped the way it is
without reverse-engineering it from the code.

Format: each record is numbered, has a status (Proposed, Accepted, Superseded), and
follows Context / Decision / Consequences. Records are append-only: a decision that
changes gets a new ADR that supersedes the old one, rather than an edit that erases
the history.

| ADR | Title | Status |
|---|---|---|
| [0001](0001-intent-as-an-outcome-envelope.md) | Intent is an outcome envelope; `MeshtasticNode` is a compiled artifact | Proposed (report-only compiler shipped; Accepted waits on `ChangePlan` and `objectives`) |
| [0002](0002-signed-autonomy-and-rejoin-before-closed-loop.md) | Define signed autonomy and rejoin semantics before the Phase 6 closed loop | Proposed |

0001's first report-only slice shipped; it stays Proposed until the compiler
expresses outcomes, not only renderability. 0002 stays Proposed until the safety
kernel exists. The full reasoning lives in [the design doctrine](../design/doctrine.md).
