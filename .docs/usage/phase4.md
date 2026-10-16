# Phase 4: HTTP identity and main wiring

Ref: [spec.md](spec.md) sections B3, A3

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 3 is reviewed. It may run in parallel with phase 5,
which touches different files. The two items touch separate files and run
in parallel.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Batch 1 (parallel)

#### Item 4.1: B3 - HTTP mode identity [parallel with 4.2]

spec.md section: B3

In `internal/mlwh/http_test.go`, give `runMLWHHTTPServer` a variadic
`providerOpts ...Option` passed to `New`, so existing callers compile
unchanged. Write the tests in `internal/mlwh/identity_test.go` with
`WithServerInfo("0.5.0", "http")` and a synced `/freshness` stub,
connecting through
`connectMLWHHTTPClientWith` (`userAgentRoundTripper` for a custom
User-Agent) or `connectMLWHHTTPClient`. `TestMLWHHTTPFeedbackUserAgent`
must pass unchanged. Covers all 5 acceptance tests from B3.

- [x] implemented
- [x] reviewed

#### Item 4.2: A3 - Main wiring [parallel with 4.1]

spec.md section: A3

`cmd/mlwh-mcp-server/main.go` is unchanged. Extend
`TestServeWiresServerInfo` in `cmd/mlwh-mcp-server/main_test.go` so its
stub captures the `/feedback` request headers, and assert `User-Agent`,
`X-WA-MCP-Transport`, `X-WA-MCP-Tool`, and the absence of
`X-WA-Client-User` in HTTP mode. Covers both acceptance tests from A3.

- [x] implemented
- [x] reviewed

For parallel batch items, use separate subagents per item under the
`subagents` skill's shared concurrency limits.
Launch review subagents using the `go-reviewer` skill
(review all items in the batch together in a single review
pass).
