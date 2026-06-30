# Phase 2: Guard over-budget tool results

Ref: [spec.md](spec.md) sections A2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer`
skills.

## Items

### Item 2.1: A2 - Guard over-budget tool results

spec.md section: A2

Add `ResultSizeGuard`, `ToolResultSizeError`, and `MaxToolResultBytes`
options in `internal/core/`, plus MLWH config flag/env parsing and
`cmd/mlwh-mcp-server` wiring to the guard. Include dynamic
`mlwh_call_endpoint` coverage, covering all 5 acceptance tests from A2.
Depends on phase 1.

- [ ] implemented
- [ ] reviewed
