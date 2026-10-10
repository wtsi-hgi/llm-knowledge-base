# Phase 6: Verification

Ref: [spec.md](spec.md) sections Implementation Order

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phases 4 and 5 are both reviewed.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Item 6.1: Verification - Lint and full test run

spec.md section: Implementation Order, step 6

Run `golangci-lint run --fix`, then
`CGO_ENABLED=1 go test -tags netgo --count 1 ./...` with no live services.
Fix any lint finding or failure at its root cause. Confirm `go.mod` still
requires `github.com/wtsi-hgi/wa v0.10.0` with no replace directive and
that `internal/core/` has no changes on this branch.

- [x] implemented
- [x] reviewed
