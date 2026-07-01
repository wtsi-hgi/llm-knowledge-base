# Phase 4: MLWH HTTP E2E

Ref: [spec.md](spec.md) sections C1, C2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

May start after phase 1 is reviewed. If phase 3 is already reviewed, this
phase may run in parallel with phase 5. It extends the MLWH stub harness and
HTTP client coverage without changing MLWH provider/tool/resource behaviour.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.agents/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.agents/skills/go-reviewer/SKILL.md)

## Items

### Item 4.1: C1 - HTTP exposes the same MLWH surface

spec.md section: C1

Extend `internal/mlwh/harness_test.go` and/or add
`internal/mlwh/http_test.go` to start the core over streamable HTTP against
the existing stub MLWH warehouse. Compare HTTP and stdio/in-memory clients
for tools, resources, workflow resource content, dynamic endpoint calls, and
stub query recording. Include the concurrency coverage called out in the
Implementation Order. Cover all 5 acceptance tests from C1.

- [x] implemented
- [x] reviewed

### Item 4.2: C2 - Version surfaces still work over HTTP

spec.md section: C2

Add HTTP client version coverage in `internal/mlwh/http_test.go`,
`internal/core/http_test.go`, and, if needed,
`cmd/mlwh-mcp-server/main_test.go`: initialization server info,
instructions, and `mcp-server://version` must expose `0.1.0` and
`wa.APIVersion`. Cover the remaining 3 acceptance tests from C2; C2
acceptance test 4 is completed in phase 2.

- [x] implemented
- [x] reviewed
