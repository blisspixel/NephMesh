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
| [0001](0001-intent-as-an-outcome-envelope.md) | Intent is an outcome envelope; `MeshtasticNode` is a compiled artifact | Proposed |
| [0002](0002-signed-autonomy-and-rejoin-before-closed-loop.md) | Define signed autonomy and rejoin semantics before the Phase 6 closed loop | Proposed |

Both are Proposed, not Accepted: they set a direction for work that is mostly not
built yet. They become Accepted when the first slice that depends on them ships. The
full reasoning behind both lives in [the design doctrine](../design/doctrine.md).
