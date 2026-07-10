# Phase 2: Schema/page helpers and export

Ref: [spec.md](spec.md) sections B1, B2, B3

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 1. The three items share export production and test
files, so complete and review them in order. Later curated-tool phases depend
on the schemas and page helpers established here.

### Skills

The orchestrator and its subagents must read and follow these skills:

- Implementor: `go-implementor`
  (/home/ubuntu/.agents/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.agents/skills/go-reviewer/SKILL.md)

## Items

### Item 2.1: B1 - Register an exact generated export input

spec.md section: B1

Add `exportInput` and `mlwh_export` in
`internal/mlwh/tools_export.go`, register it from `provider.go`, and generate
its MCP schema from `wa.ExportRelationshipDescriptions()` and
`wa.ExportColumnVocabularies()` in `schema.go`. Map fields directly to
`wa.ExportRelationship` and `wa.ExportOptions`, covering all 5 acceptance
tests from B1.

- [ ] implemented
- [ ] reviewed

### Item 2.2: B2 - Return bounded matrices and materialize streams

spec.md section: B2

Implement bounded `wa.ExportResult` matrix returns and `all=true` iterator
materialization in `internal/mlwh/tools_export.go`, including context
cancellation, relationship-specific continuation, and integration with the
core result-size guard. Extend export and core server tests to cover all 8
acceptance tests from B2. This item depends on item 2.1 being reviewed.

- [ ] implemented
- [ ] reviewed

### Item 2.3: B3 - Preserve product and shared-filter semantics

spec.md section: B3

Complete the thin export adapter in `internal/mlwh/tools_export.go` so product
grain, optional iRODS attachment, tri-state deliverability, canonical columns,
and relationship-specific shared filters remain entirely upstream-defined.
Cover all 7 acceptance tests from B3. This item depends on item 2.2 being
reviewed.

- [ ] implemented
- [ ] reviewed
