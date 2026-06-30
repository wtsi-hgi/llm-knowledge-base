# Phase 8: Existing Tool Updates And Dynamic Calls

Ref: [spec.md](spec.md) sections E1, E2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer`
skills.

## Items

### Item 8.1: E1 - Lean, paged study and run detail

spec.md section: E1

Update `mlwh_study_detail` and `mlwh_run_detail` in
`internal/mlwh/tools_detail.go` to accept `limit`, `offset`, and
`lean`, call `StudyDetailWithOptions` and `RunDetailWithOptions`, and
leave `mlwh_sample_detail` unchanged, covering all 3 acceptance tests
from E1. Depends on phase 7.

- [ ] implemented
- [ ] reviewed

### Item 8.2: E2 - Header-aware generic call endpoint

spec.md section: E2

Update `mlwh_call_endpoint` in `internal/mlwh/tools_call.go` to call
`CallWithHeaders`, wrap only responses with pagination headers, and
preserve dynamic error and freshness behavior, covering all 4
acceptance tests from E2. Depends on item 8.1.

- [ ] implemented
- [ ] reviewed
