# Phase 5: Guidance and docs

Ref: [spec.md](spec.md) sections C1, C2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 3 is reviewed. It may run in parallel with phase 4,
which touches different files. The two items touch separate files and run
in parallel.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Batch 1 (parallel)

#### Item 5.1: C1 - Agent-facing text unchanged [parallel with 5.2]

spec.md section: C1

Add a guard test to `internal/mlwh/workflow_test.go` asserting that
`workflowGuidance`, the server `Instructions` from the client's
`InitializeResult()`, and the `mlwh_provide_feedback` tool description
contain neither `X-WA-` nor, case-insensitively, `usage`. Do not check the
appended upstream `wa.EndpointReference()` text. No production code
changes. Covers the 1 acceptance test from C1.

- [ ] implemented
- [ ] reviewed

#### Item 5.2: C2 - README [parallel with 5.1]

spec.md section: C2

In `README.md`, add the `## Usage metrics` section before
`## Development` with the content the spec lists, reword the intro
sentence to "sends a report upstream" with a link to `#usage-metrics`, and
add the stdio OS username note to the **Feedback** bullet. Add assertions
to `cmd/mlwh-mcp-server/readme_test.go` using the `readmeSection` and
`singleSpaced` reads the spec names. `TestREADMEFeedbackDocs` and
`TestREADMEHTTPDocs` must pass unchanged. Covers all 5 acceptance tests
from C2.

- [ ] implemented
- [ ] reviewed

For parallel batch items, use separate subagents per item under the
`subagents` skill's shared concurrency limits.
Launch review subagents using the `go-reviewer` skill
(review all items in the batch together in a single review
pass).
