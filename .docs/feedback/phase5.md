# Phase 5: Guidance and docs

Ref: [spec.md](spec.md) sections C1, C2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 3 is reviewed. It may run in parallel with phase 4,
which touches different files.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Batch 1 (parallel)

#### Item 5.1: C1 - Workflow resource guidance [parallel with 5.2]

spec.md section: C1

In `internal/mlwh/workflow.go`, replace the opening sentence with the
spec's text and add a `Feedback:` paragraph to `workflowGuidance` before the
`mlwh_call_endpoint` paragraph. Add assertions to
`internal/mlwh/workflow_test.go`. Covers all 4 acceptance tests from C1.

- [ ] implemented
- [ ] reviewed

#### Item 5.2: C2 - README [parallel with 5.1]

spec.md section: C2

In `README.md`, add the **Feedback** bullet to "What the server exposes" and
update the intro before `## Requirements` to name `mlwh_provide_feedback` as
the one tool that sends data upstream. Add assertions to
`cmd/mlwh-mcp-server/readme_test.go`. Covers all 3 acceptance tests from
C2.

- [ ] implemented
- [ ] reviewed

For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill
(review all items in the batch together in a single review
pass).
