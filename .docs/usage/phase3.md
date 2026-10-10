# Phase 3: Per-call identity

Ref: [spec.md](spec.md) sections B1, B2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 2 is reviewed. The items share
`internal/mlwh/identity.go` and `internal/mlwh/identity_test.go`, so they
run sequentially in order.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Item 3.1: B1 - Identity helpers

spec.md section: B1

In `internal/mlwh/identity.go`, add `processClientID`,
`sessionClientID`, `agentIdentity`, and `callIdentity` as defined in the
spec's Architecture section "Identity functions". `callIdentity` must not
panic on nil `Session`, `Params`, or `Extra`. Covers all 5 acceptance
tests from B1.

- [ ] implemented
- [ ] reviewed

### Item 3.2: B2 - tools/call middleware

spec.md section: B2

Add `clientIdentityMiddleware()` to `internal/mlwh/identity.go`, wrapping
the context with `wa.WithClientIdentity(ctx, callIdentity(req))` for
`"tools/call"` requests only. In `internal/mlwh/provider.go`, make
`Register` call
`r.Server().AddReceivingMiddleware(clientIdentityMiddleware())` before the
existing `register*` calls. No tool handler changes. Write the harness
tests with `runMLWHServerWithClientOptions` or
`runMLWHServerWithClientImplementation` and `WithServerInfo("0.4.0",
"stdio")`. Covers all 8 acceptance tests from B2. Depends on item 3.1.

- [ ] implemented
- [ ] reviewed
