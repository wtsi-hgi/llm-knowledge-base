# Phase 5: Programme, users, CRAMs, status

Ref: [spec.md](spec.md) sections E1, E2, E3, E4

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Start after phase 2 is reviewed. Run independent production-file tracks in
parallel. The second batch continues the people and overview files from the
first batch without colliding with those edits. This phase may overlap phases
3 and 4.

### Skills

The orchestrator and its subagents must read and follow these skills:

- Implementor: `go-implementor`
  (/home/ubuntu/.agents/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.agents/skills/go-reviewer/SKILL.md)

## Items

### Batch 1 (parallel)

#### Item 5.1: E1 - Add programme discovery and study listing [parallel with E3]

spec.md section: E1

Add programme list/count/vocabulary tools in
`internal/mlwh/tools_people.go`, expose the upstream `programme` field through
`tools_overview.go` and OpenAPI-backed schemas, and add focused people and
overview tests. Cover all 4 acceptance tests from E1.

- [ ] implemented
- [ ] reviewed

#### Item 5.2: E3 - Add merged-aware sample CRAM tools [parallel with E1]

spec.md section: E3

Add the sample-CRAM page and count tools in
`internal/mlwh/tools_availability.go` with exact `wa.SampleCRAM` rows under
`sample_crams`, semantic page metadata, provider schemas, and freshness
guidance. Cover all 5 acceptance tests from E3.

- [ ] implemented
- [ ] reviewed

### Batch 2 (parallel, after batch 1 is reviewed)

#### Item 5.3: E2 - Add inverse study-user tools [parallel with E4]

spec.md section: E2

Extend `internal/mlwh/tools_people.go` and `schema.go` with study-user list and
count tools, optional exact case-insensitive role sets, semantic user pages,
and the distinction from faculty sponsorship and person-to-study defaults.
Cover all 5 acceptance tests from E2.

- [ ] implemented
- [ ] reviewed

#### Item 5.4: E4 - Preserve empty-study status arrays [parallel with E2]

spec.md section: E4

Normalize nil `PerPlatform` values in
`internal/mlwh/tools_overview.go` so structured and text JSON always expose
`per_platform: []` while populated values and other status fields remain
unchanged. Cover both acceptance tests from E4.

- [ ] implemented
- [ ] reviewed

For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill
(review all items in each batch together in a single review pass).
