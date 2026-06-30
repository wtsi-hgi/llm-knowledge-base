# Phase 3: Command config

Ref: [spec.md](spec.md) sections A1, A2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 1. This phase wires command configuration and mode
selection. Phase 5 depends on the command examples from this phase; phase 4
may already have started after phase 1 and can run in parallel with phase 5
after this phase is reviewed.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.agents/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.agents/skills/go-reviewer/SKILL.md)

## Items

### Item 3.1: A1 - Stdio remains default

spec.md section: A1

Update `cmd/mlwh-mcp-server/main.go` and
`cmd/mlwh-mcp-server/main_test.go` so no `--http` flag and no
`MLWH_HTTP_ADDR` resolves to stdio, builds the MLWH provider, calls
`Run(ctx, &mcp.StdioTransport{})`, preserves config validation ordering,
and keeps `--version` short-circuit behaviour. Cover all 4 acceptance tests
from A1.

- [ ] implemented
- [ ] reviewed

### Item 3.2: A2 - HTTP config selects HTTP

spec.md section: A2

Add `const envHTTPAddr = "MLWH_HTTP_ADDR"`, parse `--http <addr>`, apply
the precedence rule that a present flag wins over env even when empty, and
call `RunHTTP` with `HTTPOptions{Addr, MCPPath:"/mcp", HealthPath:"/health"}`
for non-empty resolved addresses. Cover all 4 acceptance tests from A2.

- [ ] implemented
- [ ] reviewed
