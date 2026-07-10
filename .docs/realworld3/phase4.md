# Phase 4: Runs and aggregates

Ref: [spec.md](spec.md) sections D1, D2, D3

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Start after phase 2 is reviewed and run in parallel with phase 3 when useful.
The sample-detail and global-run tracks are independent. D3 follows D2 because
they share `tools_runs.go`, its filter helpers, and focused tests.

### Skills

The orchestrator and its subagents must read and follow these skills:

- Implementor: `go-implementor`
  (/home/ubuntu/.agents/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.agents/skills/go-reviewer/SKILL.md)

## Items

### Batch 1 (parallel)

#### Item 4.1: D1 - Add sample run and sample study counts [parallel with D2]

spec.md section: D1

Add `mlwh_runs_for_sample`, `mlwh_count_runs_for_sample`, and
`mlwh_count_studies_for_sample` in `internal/mlwh/tools_detail.go`, retaining
`mlwh_studies_for_sample` and the normal semantic page wrapper. Update schemas
and focused tests, covering all 3 acceptance tests from D1.

- [ ] implemented
- [ ] reviewed

#### Item 4.2: D2 - Add global run listing and count [parallel with D1]

spec.md section: D2

Create `internal/mlwh/tools_runs.go` with `mlwh_runs` keyset pagination and
`mlwh_count_runs`, add their schemas and provider registration, and preserve
exact `wa.RunListingRow` fields and run-date caveats. Cover all 5 acceptance
tests from D2.

- [ ] implemented
- [ ] reviewed

For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill
(review all items in the batch together in a single review pass).

### Item 4.3: D3 - Add monthly and general aggregates

spec.md section: D3

After batch 1 is reviewed, extend `internal/mlwh/tools_runs.go` and its schemas
with `mlwh_monthly_run_counts` and `mlwh_sequencing_aggregate`. Forward the
required grouping, unit, date, and repeated platform values in exactly one
upstream call, covering all 5 acceptance tests from D3.

- [ ] implemented
- [ ] reviewed
