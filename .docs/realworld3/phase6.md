# Phase 6: Workflow, public docs, and hardening

Ref: [spec.md](spec.md) sections F1, F2, F3

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Start only after phases 3, 4, and 5 are reviewed. The workflow, README, and
hardening tracks operate on separate primary files and can proceed in
parallel, followed by one review of their combined public and operational
contract.

### Skills

The orchestrator and its subagents must read and follow these skills:

- Implementor: `go-implementor`
  (/home/ubuntu/.agents/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.agents/skills/go-reviewer/SKILL.md)

## Items

### Batch 1 (parallel)

#### Item 6.1: F1 - Curated workflow routing [parallel with F2, F3]

spec.md section: F1

Rewrite the curated prefix in `internal/mlwh/workflow.go`, retain the live
Registry catalogue suffix, and route product, file, CRAM, search, recency,
run, aggregate, programme, sponsor, and study-user questions accurately.
Cover all 6 acceptance tests from F1.

- [ ] implemented
- [ ] reviewed

#### Item 6.2: F2 - README for API 1.8.0 [parallel with F1, F3]

spec.md section: F2

Update `README.md` and `cmd/mlwh-mcp-server/readme_test.go` for API 1.8.0,
the complete curated tool catalogue, export and page continuation, and all
domain caveats, while retaining shared-HTTP regression assertions. Cover all
7 acceptance tests from F2.

- [ ] implemented
- [ ] reviewed

#### Item 6.3: F3 - Bounded and cancellable behavior [parallel with F1, F2]

spec.md section: F3

Harden `internal/mlwh/provider.go`, `errmap.go`, availability tools, and the
core result guard. Verify pagination boundaries, cancellation, sentinel error
precedence, accurate continuation guidance, samples-with-data date forwarding,
and hermetic HTTP stubs, covering all 6 acceptance tests from F3.

- [ ] implemented
- [ ] reviewed

For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill
(review all items in the batch together in a single review pass).
