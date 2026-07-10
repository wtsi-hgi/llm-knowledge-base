# Phase 3: Search and data-object reads

Ref: [spec.md](spec.md) sections C1, C2, C3

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Start after phase 2 is reviewed. The search and availability tracks can begin
in parallel. C3 follows the C2 availability work. This phase may run in
parallel with phases 4 and 5 once their shared phase 2 dependency is met.

### Skills

The orchestrator and its subagents must read and follow these skills:

- Implementor: `go-implementor`
  (/home/ubuntu/.agents/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.agents/skills/go-reviewer/SKILL.md)

## Items

### Batch 1 (parallel)

#### Item 3.1: C1 - Literal-prefix sample search [parallel with C2]

spec.md section: C1

Extend `internal/mlwh/tools_search.go` and its schemas with the `words`, exact
filter, deliverability, and paging inputs. Use one option-preserving
`CallWithHeaders("SearchSamples", ...)` request for optioned lists and retain
the exact-count floor guidance, covering all 7 acceptance tests from C1.

- [ ] implemented
- [ ] reviewed

#### Item 3.2: C2 - Preserve optioned iRODS pages and fields [parallel with C1]

spec.md section: C2

Update the sample, study, and run iRODS list/count tools in
`internal/mlwh/tools_availability.go` and `schema.go`. Implement one-request
header-aware list pages, direct optioned counts, exact `wa.IRODSPath` rows,
and upstream error behavior, covering all 7 acceptance tests from C2.

- [ ] implemented
- [ ] reviewed

For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill
(review all items in the batch together in a single review pass).

### Item 3.3: C3 - Add latest-data study and sponsor tools

spec.md section: C3

After batch 1 is reviewed, add the four latest-data list/count tools to
`internal/mlwh/tools_availability.go` and their schemas to `schema.go`. Call
the upstream study and faculty-sponsor page/count methods directly with the
10-row list default, covering all 5 acceptance tests from C3.

- [ ] implemented
- [ ] reviewed
