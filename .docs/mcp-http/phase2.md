# Phase 2: Shutdown and logging

Ref: [spec.md](spec.md) sections B3, C2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 1. This phase completes ctx-aware plain HTTP serving,
graceful shutdown, and the HTTP startup log attributes. It may be worked
independently of phase 3 once phase 1 is reviewed.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.agents/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.agents/skills/go-reviewer/SKILL.md)

## Items

### Item 2.1: B3 - Graceful shutdown

spec.md section: B3

Add ctx-aware plain HTTP serving in `internal/core/http.go` using a local
listener, `http.Server`, default `ShutdownTimeout` of 5 seconds, graceful
shutdown on context cancellation, `http.ErrServerClosed` handling, and
`authServer.Stop()` on exit. Add the command signal seam in
`cmd/mlwh-mcp-server/main.go` as needed for the shutdown test, covering all
5 acceptance tests from B3.

- [x] implemented
- [x] reviewed

### Item 2.2: C2 - Version surfaces still work over HTTP

spec.md section: C2

Share startup logging in `internal/core/server.go` and `internal/core/http.go`
so HTTP mode logs `server_version`, `api_versions`, `transport=http`,
`addr`, `mcp_path`, and `health_path` while preserving existing stdio log
fields. This covers C2 acceptance test 4; the remaining C2 HTTP client
version checks are completed in phase 4.

- [x] implemented
- [x] reviewed
