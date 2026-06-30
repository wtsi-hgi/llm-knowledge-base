# Phase 5: Aggregate And Status Tools

Ref: [spec.md](spec.md) sections B1, B2, B3

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer`
skills.

## Items

### Batch 1 (parallel)

#### Item 5.1: B1 - Study overview [parallel with B2, B3]

spec.md section: B1

Register `mlwh_study_overview` in `internal/mlwh/tools_overview.go`,
call `(*wa.RemoteClient).StudyOverview`, and use Registry/OpenAPI
description and schema, covering all 3 acceptance tests from B1.
Depends on phase 4.

- [x] implemented
- [x] reviewed

#### Item 5.2: B2 - Study status breakdown [parallel with B1, B3]

spec.md section: B2

Register `mlwh_study_status_breakdown` in
`internal/mlwh/tools_overview.go`, call `StatusBreakdown`, and preserve
platform, QC, timeline, and cache fields, covering all 3 acceptance
tests from B2. Depends on phase 4.

- [x] implemented
- [x] reviewed

#### Item 5.3: B3 - Run overview, run status, and sample progress

Parallel with B1, B2.

spec.md section: B3

Register `mlwh_run_overview`, `mlwh_run_status`, and
`mlwh_sample_progress` in `internal/mlwh/tools_overview.go`, calling
`RunOverview`, `RunStatus`, and `SampleProgress`, covering all 5
acceptance tests from B3. Depends on phase 4.

- [x] implemented
- [x] reviewed

For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill
(review all items in the batch together in a single review
pass).
