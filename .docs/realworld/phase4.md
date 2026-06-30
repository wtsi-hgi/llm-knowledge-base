# Phase 4: Bound and annotate every paged typed tool

Ref: [spec.md](spec.md) sections A3

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer`
skills.

## Items

### Item 4.1: A3 - Bound and annotate every paged typed tool

spec.md section: A3

Add paged result wrappers in `internal/mlwh/schema.go` and switch
existing typed search and fan-out tools to header-aware bounded pages
with default `limit=100`, maximum `1000`, and `offset=0`, covering all
5 acceptance tests from A3. Depends on phase 3.

- [x] implemented
- [x] reviewed
