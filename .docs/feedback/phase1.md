# Phase 1: Dependency

Ref: [spec.md](spec.md) sections A1

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

This is the first phase and has no predecessors. Every later phase depends
on it.

### Prerequisite

wa `v0.10.0` (`wa.APIVersion` `1.9.0`) must be tagged and released from
`/home/ubuntu/wa` before this phase starts. That release implements the wa
companion spec at `/home/ubuntu/wa/.docs/feedback/spec.md`, which owns the
feedback wire contract. Confirm the tag resolves with
`timeout 60 go list -m github.com/wtsi-hgi/wa@v0.10.0`. If it does not
resolve, stop and report the blocker. Do not add a replace directive, a
pseudo-version, or a local path to work around a missing release.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Item 1.1: A1 - Bump wa to v0.10.0

spec.md section: A1

Require `github.com/wtsi-hgi/wa v0.10.0` in `go.mod` and `go.sum` with no
replace directive. Rename `TestWAAPI18Contract` to `TestWAAPI19Contract` in
`internal/mlwh/provider_test.go` and update every `1.8.1` pin in that test,
`cmd/mlwh-mcp-server/main_test.go`, `cmd/mlwh-mcp-server/readme_test.go`,
and `README.md` (prose and `--version` example) to `1.9.0`. Confirm the
Registry has no `/feedback` entry and the `mlwh_call_endpoint` parity tests
pass unchanged. Covers all 4 acceptance tests from A1.

- [x] implemented
- [x] reviewed
