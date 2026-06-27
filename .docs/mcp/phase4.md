# Phase 4: MLWH provider config + registration shell

Ref: [spec.md](spec.md) sections H2, I1 (scaffolding), Implementation
Order item 4

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phases 2 and 3 (needs the core seam and the schema/error
foundations). Phases 5 and 7 both build on this phase.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)
- Shared conventions: `go-conventions`
  (/home/ubuntu/.claude/skills/go-conventions/SKILL.md)
- Shared testing: `testing-principles`
  (/home/ubuntu/.claude/skills/testing-principles/SKILL.md)

## Items

### Item 4.1: H2 - MLWH provider configuration

spec.md section: H2

In `internal/mlwh/provider.go`, read provider config from flags/env:
`MLWH_BASE_URL`/`--mlwh-base-url` (required),
`MLWH_CA_CERT`/`--mlwh-ca-cert` (optional),
`MLWH_TIMEOUT`/`--mlwh-timeout` (optional duration). Populate
`mlwh.RemoteConfig{BaseURL, CACert, Timeout}`; do NOT wire `CacheTTL`
(inert for the remote client); do not expose `Token`. A missing base URL is
a clear startup error. Covers all 3 H2 acceptance tests (base URL from env;
missing base URL errors mentioning it is required; `MLWH_TIMEOUT=5s` yields
5s with `CacheTTL` left zero).

- [x] implemented
- [x] reviewed

### Item 4.2: I1 scaffolding - New, Register shell, RemoteClient construction

spec.md section: I1 (scaffolding), Implementation Order item 4

Implement `mlwh.New(cfg) (core.Provider, error)` to build the provider from
config, construct the `*mlwh.RemoteClient` via
`mlwh.NewRemoteClient(...)`, and provide a `Register(ctx, r core.Registrar)`
method that is initially empty (a shell; tools and resources are added in
phases 5-7). `Name()` returns "mlwh". Wire the provider's targeted API
version (`mlwh.APIVersion`) so the core can assemble `VersionInfo`.

This item delivers the registration shell only; the full I1 surface
assertion (all expected tools/resources present) is verified in phase 8.
The single assertion verifiable now is that `Name()` returns "mlwh" (the
`Name()` part of I1.3).

- [x] implemented
- [x] reviewed
