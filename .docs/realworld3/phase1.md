# Phase 1: Version and removal

Ref: [spec.md](spec.md) sections A1, A2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

This is the sequential foundation for every later phase. Complete A1 before
A2 so Registry and OpenAPI parity are restored only after the dependency and
obsolete manifest surface have been updated.

### Skills

The orchestrator and its subagents must read and follow these skills:

- Implementor: `go-implementor`
  (/home/ubuntu/.agents/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.agents/skills/go-reviewer/SKILL.md)

## Items

### Item 1.1: A1 - Upgrade the contract and remove manifests

spec.md section: A1

Update `go.mod` and `go.sum` to `github.com/wtsi-hgi/wa v0.8.0`, retain
`wa.APIVersion` as the provider version source, and remove the manifest tools,
types, schemas, registration, tests, workflow text, and downstream API
references. Update `internal/mlwh/provider.go`, availability code, and command
tests, covering all 4 acceptance tests from A1.

- [x] implemented
- [x] reviewed

### Item 1.2: A2 - Preserve complete Registry and OpenAPI parity

spec.md section: A2

Update `internal/mlwh/tools_call.go`, `schema.go`, and `workflow.go` so
`mlwh_call_endpoint` remains driven by all 90 `wa.Registry` methods, including
`Export`, while schemas come from `wa.OpenAPIDocument()` and the catalogue
comes from `wa.EndpointReference()`. Cover all 4 acceptance tests from A2.
This item depends on item 1.1 being reviewed.

- [x] implemented
- [x] reviewed
