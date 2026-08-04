# Contributing to NephMesh

Thank you for considering a contribution. This is an experimental project; questions and skepticism are contributions too.

## Ground rules

- Read `AGENTS.md` first. Its conventions (writing style, provider neutrality, receive-only defaults, upstream-compatible engineering) apply to human and AI contributors alike.
- Check `docs/roadmap.md` before proposing work. Directories are created only when their phase starts.
- Everything must work hardware-free. If your change needs a radio to test, it also needs a simulated-radio test path.

## Developer Certificate of Origin

Every commit must be signed off (`git commit -s`), certifying the [Developer Certificate of Origin](https://developercertificate.org/). No CLA.

## Style

- No emojis, no em dashes, no AI attribution anywhere (commit messages, code, docs). CI enforces this.
- Source files carry the Apache-2.0 header (`hack/boilerplate.go.txt` is the canonical text). CI enforces this too.
- Go code follows the conventions in `docs/plans/engineering-conventions.md` (Go 1.25.x, testify plus golden tests, mockery, golangci-lint).

## Workflow

Fork, branch, PR against `main`. Run `make check` before pushing. Keep PRs scoped to one concern; reference the roadmap phase they serve.

## Security

See `SECURITY.md` for reporting anything sensitive, especially anything touching radio transmission paths.
