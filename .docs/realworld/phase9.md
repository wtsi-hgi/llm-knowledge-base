# Phase 9: Workflow, Error, And Registration Consistency

Ref: [spec.md](spec.md) sections E3, F1, F2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer`
skills.

## Items

### Batch 1 (parallel)

#### Item 9.1: E3 - Workflow guidance chooses cheap tools first

Parallel with F1.

spec.md section: E3

Update `workflowGuidance` in `internal/mlwh/workflow.go` so
`mlwh://workflow` prefers overview, count, status, manifest, person, and
freshness tools before expensive detail or list calls, covering all 6
acceptance tests from E3. Depends on phase 8.

- [ ] implemented
- [ ] reviewed

#### Item 9.2: F1 - Preserve actionable upstream errors [parallel with E3]

spec.md section: F1

Ensure every new handler uses `mapToolError` in
`internal/mlwh/errmap.go` for bad request, not found, ambiguity,
unsupported identifier, never-synced, and impaired upstream failures,
covering all 8 acceptance tests from F1. Depends on phase 8.

- [ ] implemented
- [ ] reviewed

### Item 9.3: F2 - Register the full MLWH surface

spec.md section: F2

Extend `internal/mlwh/provider.go` and
`internal/mlwh/provider_test.go` registration, schema, and description
assertions for every new or updated MLWH tool, covering all 4
acceptance tests from F2. Depends on batch 1 and all prior phases.

- [ ] implemented
- [ ] reviewed

For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill
(review all items in the batch together in a single review
pass).
