# Phase 4: Main wiring

Ref: [spec.md](spec.md) sections A2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 3 is reviewed. It may run in parallel with phase 5,
which touches different files.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Item 4.1: A2 - Server info wiring in main

spec.md section: A2

In `cmd/mlwh-mcp-server/main.go`, construct the provider with
`mlwh.New(remoteCfg, mlwh.WithServerInfo(core.ServerVersion,
string(cfg.Transport)))`. In `cmd/mlwh-mcp-server/main_test.go`, run
`serve` with and without `--http 127.0.0.1:0` against a stub
`MLWH_BASE_URL`, capture the provider through `newCoreServer`, run it in a
real `core.New`, call the tool over in-memory transports, and assert
`transport` and `mcp_server_version` in the stub body. Covers A2 acceptance
test 3; tests 1 and 2 are covered in item 3.2.

- [ ] implemented
- [ ] reviewed
