# Phase 3: Extend the hermetic MLWH harness

Ref: [spec.md](spec.md) sections F3

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer`
skills.

## Items

### Item 3.1: F3 - Extend the hermetic MLWH harness

spec.md section: F3

Extend `internal/mlwh/harness_test.go` with HTTP headers on
`stubResponse` and `respondJSONWithHeaders`, while preserving the
wa-shaped unmatched-route 404 behavior, covering all 2 acceptance tests
from F3. Depends on phase 2.

- [ ] implemented
- [ ] reviewed
