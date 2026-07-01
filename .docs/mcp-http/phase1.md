# Phase 1: Core HTTP foundation

Ref: [spec.md](spec.md) sections B1, B2, E1

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

This is the first phase; it is sequential and has no predecessors. It adds
the reusable core HTTP foundation and the direct dependency needed by later
shutdown, command, MLWH E2E, and documentation work.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.agents/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.agents/skills/go-reviewer/SKILL.md)

## Items

### Item 1.1: E1 - Dependency and test gates

spec.md section: E1

Add `github.com/wtsi-hgi/go-authserver v1.6.0` as a direct dependency in
`go.mod` and record the resolved checksums in `go.sum`. This covers E1
acceptance test 1 only; the remaining E1 gate checks are completed in
phase 5.

- [x] implemented
- [x] reviewed

### Item 1.2: B1 - Shared streamable HTTP handler

spec.md section: B1

Add the core HTTP serving surface in `internal/core/http.go` and reuse
provider registration from `internal/core/transport.go`. Define
`HTTPOptions`, `(*Server).RunHTTP`, the streamable HTTP handler factory,
test injection seams, and service-agnostic import boundaries, covering all
7 acceptance tests from B1.

- [x] implemented
- [x] reviewed

### Item 1.3: B2 - go-authserver foundation and plain routes

spec.md section: B2

Mount HTTP mode on `go-authserver` in `internal/core/http.go`: construct
`gas.New`, register `/mcp` `GET`/`POST`/`DELETE` through `gin.WrapH`, add
the unauthenticated `/health` route, and keep auth/TLS/token paths unused
but locally reachable through the adapter. Cover all 6 acceptance tests
from B2.

- [x] implemented
- [x] reviewed
