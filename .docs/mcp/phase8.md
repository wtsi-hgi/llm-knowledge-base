# Phase 8: Seam proof + full registration assertions

Ref: [spec.md](spec.md) sections I1, I2, Implementation Order item 8

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phases 5-7: it asserts the complete MLWH tool/resource
surface (built across phases 5-7) registers, and proves the provider seam
with a second, test-only fake provider. Tests stay hermetic against the
stub harness and the in-memory transport.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)
- Shared conventions: `go-conventions`
  (/home/ubuntu/.claude/skills/go-conventions/SKILL.md)
- Shared testing: `testing-principles`
  (/home/ubuntu/.claude/skills/testing-principles/SKILL.md)

## Items

### Item 8.1: I1 - MLWH provider registers its full tool/resource surface

spec.md section: I1

Complete and assert the MLWH provider's `Register(ctx, r)` surface: every
tool from stories A-E and the workflow resource (G1) register via the
`Registrar`, and the shipped binary registers only this provider. Covers all
3 I1 acceptance tests (a tools listing over the in-memory client includes
`mlwh_search_samples`, `mlwh_count_samples`, `mlwh_search_studies`,
`mlwh_resolve_sample`, `mlwh_find_samples`, `mlwh_sample_detail`,
`mlwh_freshness`, `mlwh_call_endpoint`; a resources listing includes
`mlwh://workflow` and `mcp-server://version`; `Name()` returns "mlwh").
This also realises the end-to-end form of H1.1 (a connected client lists the
MLWH tools over `mcp.NewInMemoryTransports()`). Test file
`internal/mlwh/provider_test.go`.

- [ ] implemented
- [ ] reviewed

### Item 8.2: I2 - Multi-service seam proof (test-only fake provider)

spec.md section: I2

Add a `fakeProvider` defined ONLY in `internal/core/provider_seam_test.go`
that implements `core.Provider`, registering one trivial tool (e.g.
`fake_ping`) and one resource. A test builds a core server with BOTH the
MLWH provider and the fake provider and asserts both surfaces appear; no
production core file is modified to add the fake. Covers all 3 I2 acceptance
tests (a tools listing contains both `fake_ping` and `mlwh_search_samples`;
calling `fake_ping` returns its trivial result with `IsError=false`,
proving the same handler path; the fake is defined only in a `_test.go`
file with no production core reference).

- [ ] implemented
- [ ] reviewed
