# Phase 1: Dependency

Ref: [spec.md](spec.md) sections A1

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

This is the first phase and has no predecessors. Every later phase depends
on it.

### Prerequisite

wa `v0.11.0` (`wa.APIVersion` `1.10.0`) must be tagged and released from
`/home/ubuntu/wa` before this phase starts. That release implements the wa
companion spec at `/home/ubuntu/wa/.docs/usage/spec.md`, which owns the
identification header wire contract. Confirm the tag resolves with
`timeout 60 go list -m github.com/wtsi-hgi/wa@v0.11.0`. If it does not
resolve, stop and report that wa `v0.11.0` is not tagged. Do not add a
replace directive, a pseudo-version, or a local path to work around a
missing release.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Item 1.1: A1 - Bump wa to v0.11.0

spec.md section: A1

Require `github.com/wtsi-hgi/wa v0.11.0` in `go.mod` and `go.sum` with no
replace directive. Rename `TestWAAPI19Contract` to `TestWAAPI110Contract`
in `internal/mlwh/provider_test.go` (A1.1 title `1.10.0`) and update the
current-version pins in `TestRunVersionFlag`
(`cmd/mlwh-mcp-server/main_test.go`), `TestREADMEPublicAPIDocs`
(`cmd/mlwh-mcp-server/readme_test.go`), `README.md` (prose and
`--version` example), and
`.agents/skills/verify-mlwh-mcp/features/feedback-tool.md` (stored
`wa_api_version`) to `1.10.0`. Change the Requirements line in
`.agents/skills/verify-mlwh-mcp/SKILL.md` to a wa checkout at `v0.11.0`+
(agent feedback and usage metrics). Also update the error message in
`.agents/skills/verify-mlwh-mcp/scripts/launch.sh` (about line 25, "wa
v0.10.0+ checkout") to say `v0.11.0`+, consistent with SKILL.md. Leave the
history statements the spec lists unchanged. Covers all 4 acceptance tests
from A1.

- [x] implemented
- [x] reviewed
