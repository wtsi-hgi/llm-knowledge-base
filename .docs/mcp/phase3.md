# Phase 3: Schema + error foundations

Ref: [spec.md](spec.md) sections F1, F2, J1, Implementation Order item 3

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Per the spec's Implementation Order, this phase has no network dependency
and underpins every tool. It is parallelisable with phase 2 once the module
builds (it needs only phase 1's bootstrapped module, not phase 2's core).
It must be reviewed before phase 4.

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

### Item 3.1: F1 - Output schemas preserve doc-tag field descriptions

spec.md section: F1

Implement `outputSchemaFor(componentName string) (map[string]any, error)`
in `internal/mlwh/schema.go`, sourcing each typed tool's output schema from
`mlwh.OpenAPIDocument()`'s `components.schemas` (which carry the `doc:`
field descriptions the SDK reflection would otherwise drop). Inline/resolve
`$ref`s so the schema is self-contained, and build it as a
`map[string]any` of type "object" suitable for pre-setting on
`Tool.OutputSchema`. Covers all 3 F1 acceptance tests
(`outputSchemaFor("Sample")` carries the `supplier_name` description;
schema marshals to a valid JSON object with no unresolved `$ref`; the
`Match` schema description preserved - the tool-level assertion in F1.2 is
realised once resolve tools exist in phase 5, but the schema source is
verified here).

- [x] implemented
- [x] reviewed

### Item 3.2: F2 - Slice results wrapped as an object

spec.md section: F2

Add the per-element-type wrapper structs (one slice field each) in
`internal/mlwh/schema.go` (wrappers may live beside their tools), with
consistent JSON field names per element type (samples, studies, runs,
lanes, irods_paths, libraries, tagged_ids, values). These let
list-returning tools produce object-typed `StructuredContent` and
object-typed output schemas as MCP requires. Covers F2's 2 acceptance tests
at the schema/wrapper level (object wrapper shape; output schema top-level
type "object" with an array property); the end-to-end assertions via
`mlwh_search_samples` are exercised when that tool lands in phase 5 (A1.1).

- [x] implemented
- [x] reviewed

### Item 3.3: J1 - Map wa/mlwh sentinels to clear MCP tool errors

spec.md section: J1

Implement `mapToolError(err error) error` in `internal/mlwh/errmap.go`,
preferring `errors.Is` against the `mlwh.Err*` sentinels over HTTP status,
and checking `ErrCacheNeverSynced` BEFORE `ErrNotFound` (slice endpoints
return `errors.Join(ErrCacheNeverSynced, ErrNotFound)`). Cover all six
documented codes (400/404/409/422/502/503), preserving the upstream message
and appending a short actionable hint. Covers all 6 J1 acceptance tests,
including the joined never-synced case mapping to cache-not-synced (not
"not found") and a 400 "term too short" preserving its message; `nil` maps
to `nil`. Also derive the enum/field tables the spec attributes to
`schema.go` (the `find_samples` field <-> Registry Method table from the
`FindSamplesBy` prefix, and the `kind` enum from `mlwh.IdentifierKinds()`)
so phase 5's tools consume them; their full enum assertions land with the
tools (B2.4, B3.3).

- [x] implemented
- [x] reviewed
