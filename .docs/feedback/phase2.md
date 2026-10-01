# Phase 2: Provider option and harness

Ref: [spec.md](spec.md) sections A2, B2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 1 is reviewed. This phase adds the provider option
and the harness recording that the feedback tool tests in phase 3 rely on.
The two items touch separate files and run in parallel.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Batch 1 (parallel)

#### Item 2.1: A2 - Server info option [parallel with 2.2]

spec.md section: A2

In `internal/mlwh/provider.go`, add `Option`, `WithServerInfo(serverVersion,
transport string) Option`, and the `serverVersion` and `transport` provider
fields, defaulting to `core.ServerVersion` and `"stdio"`. Change `New` to
`New(cfg wa.RemoteConfig, opts ...Option)` so existing `New(cfg)` call sites
compile unchanged. A2 acceptance tests 1 and 2 need the tool to submit, so
they are written in item 3.2. Test 3 (main wiring) is phase 4.

- [ ] implemented
- [ ] reviewed

#### Item 2.2: B2 - Harness request recording [parallel with 2.1]

spec.md section: B2

Extend `recordedRequest` in `internal/mlwh/harness_test.go` with
`Method string`, `Header http.Header`, and `Body []byte`, captured for every
request the stub receives. Existing tests must pass unchanged. The B2
acceptance tests that use these fields are written in item 3.2.

- [ ] implemented
- [ ] reviewed

For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill
(review all items in the batch together in a single review
pass).
