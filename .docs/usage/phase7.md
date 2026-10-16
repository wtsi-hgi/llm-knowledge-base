# Phase 7: Verification

Ref: [spec.md](spec.md) sections Implementation Order

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 6 is reviewed. This is the last phase. It ends
with a pull request against `develop`; merging it and tagging a release
are left to the user.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Item 7.1: Verification - Lint, tests, drives, and PR

spec.md section: Implementation Order, step 7

Run `golangci-lint run --fix`, then
`CGO_ENABLED=1 go test -tags netgo --count 1 ./...` with no live services.
Fix any lint finding or failure at its root cause. Confirm `go.mod` still
requires `github.com/wtsi-hgi/wa v0.11.0` with no replace directive, and
that `internal/core/`, `cmd/mlwh-mcp-server/main.go`, every
`internal/mlwh/tools_*.go` handler, and `internal/mlwh/workflow.go` have
no changes on this branch. Launch a verify-mlwh-mcp stack against a wa
`v0.11.0` checkout, pass Doctor, and run both `drive-usage.sh` and
`drive-feedback.sh` to a clean exit. Then push the branch and open a pull
request against `develop`. Do not merge the PR and do not create a tag;
report the PR URL to the user.

- [x] implemented
- [x] reviewed
