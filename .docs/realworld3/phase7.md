# Phase 7: Full verification

Ref: [spec.md](spec.md) sections A1-F3, Implementation Order item 7

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 6 and all prior reviews. This phase changes code only
when the required formatting, lint, or full-suite verification exposes a
defect in the completed implementation.

### Skills

The orchestrator and its subagents must read and follow these skills:

- Implementor: `go-implementor`
  (/home/ubuntu/.agents/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.agents/skills/go-reviewer/SKILL.md)

## Items

### Item 7.1: A1-F3 - Run full verification

spec.md section: Implementation Order item 7

Run `golangci-lint run --fix`, then run
`CGO_ENABLED=1 go test -tags netgo --count 1 ./...` without live services.
Resolve any failures within the owning story's specified files and tests, then
rerun both commands cleanly. This verifies all 95 acceptance tests across
A1-F3 and introduces no additional feature behavior.

- [ ] implemented
- [ ] reviewed
