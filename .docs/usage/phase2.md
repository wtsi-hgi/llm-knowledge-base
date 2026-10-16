# Phase 2: Client-level identity

Ref: [spec.md](spec.md) sections A2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 1 is reviewed. This phase creates
`internal/mlwh/identity.go`, which phase 3 extends.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Item 2.1: A2 - Client-level identity in New

spec.md section: A2

Create `internal/mlwh/identity.go` with `identityRemoteConfig` and
`localUsername` as defined in the spec's Architecture section "Identity
functions". In `internal/mlwh/provider.go`, reorder `New` to validate
`BaseURL`, build the provider with defaults, apply options, then call
`wa.NewRemoteClient(identityRemoteConfig(cfg, p.serverVersion,
p.transport))`, and update the `WithServerInfo` doc comment to the spec's
text. Write the tests in `internal/mlwh/identity_test.go`, using
`runMLWHServerWithClientOptions` with a synced `/freshness` stub for the
harness tests. Covers all 7 acceptance tests from A2.

- [x] implemented
- [x] reviewed
