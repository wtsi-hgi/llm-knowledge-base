# Phase 5: Search/count + resolve/find/expand tools

Ref: [spec.md](spec.md) sections A1, A2, A3, A4, B1, B2, B3,
Implementation Order item 5

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 4 (adds tools to the provider built there, using the
schema wrappers and error mapping from phase 3). This phase also stands up
the hermetic stub MLWH server test harness (an `httptest.Server` serving
canned JSON for exact `wa mlwh` paths, plus a shared helper that builds a
core server with the MLWH provider and an `mcp.NewInMemoryTransports()`
client/server pair), per the spec's Testing strategy; later phases reuse
it. Tests are hermetic - never a live warehouse.

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

### Item 5.1: A1 - Search samples by word prefix

spec.md section: A1

Add `mlwh_search_samples` (file `internal/mlwh/tools_search.go`) calling
`SearchSamples(ctx, term, limit, offset)`; input `Term` (min 3), `Limit`
(default 100, max 1000), `Offset` (default 0); output `[]mlwh.Sample`
wrapped per F2. Cheap guards reject short terms and over-max limits before
the call. Covers all 7 A1 acceptance tests (structured + text results;
min-3 and max-1000 guards with no HTTP request; limit/offset passthrough;
defaults 100/0; tool name and description containing "word-prefix",
"minimum" 3, and "1000"). Also satisfies the end-to-end F2.1 assertion.

- [ ] implemented
- [ ] reviewed

### Item 5.2: A2 - Count samples matching a word prefix

spec.md section: A2

Add `mlwh_count_samples` (file `internal/mlwh/tools_search.go`) calling
`CountSampleSearch(ctx, term)`; input `Term`; output `mlwh.Count`.
Description states the count is exact up to 10000 and that exactly 10000
means "at least 10000". Covers all 4 A2 acceptance tests (count 42; count
10000 floor; min-3 guard with no request; tool name and description
containing "10000" and "at least").

- [ ] implemented
- [ ] reviewed

### Item 5.3: A3 - Search studies by substring

spec.md section: A3

Add `mlwh_search_studies` (file `internal/mlwh/tools_search.go`) calling
`SearchStudies(ctx, term, limit, offset)`; input as A1; output
`[]mlwh.Study`. Description states case-insensitive substring over
name/study_title/programme/faculty_sponsor; min term 3; default 100, max
1000. Covers all 4 A3 acceptance tests (three studies; min-3 guard; max-1000
guard; tool name and description containing "substring").

- [ ] implemented
- [ ] reviewed

### Item 5.4: A4 - Count studies (search + all)

spec.md section: A4

Add `mlwh_count_studies_search` (`CountStudySearch(ctx, term)`, input
`Term` min 3) and `mlwh_count_studies` (`CountStudies(ctx)`, no input,
input schema `{"type":"object"}`); both output `mlwh.Count`. File
`internal/mlwh/tools_search.go`. Covers all 3 A4 acceptance tests (search
count 7; search min guard; whole-set count 1234 from `{}`).

- [ ] implemented
- [ ] reviewed

### Item 5.5: B1 - Resolve and classify identifiers

spec.md section: B1

Add the seven resolve/classify tools (file
`internal/mlwh/tools_resolve.go`), each input `Identifier string`, output
`mlwh.Match`: `mlwh_classify_identifier`, `mlwh_resolve_sample`,
`mlwh_resolve_sample_name`, `mlwh_resolve_study`, `mlwh_resolve_run`,
`mlwh_resolve_library`, `mlwh_resolve_library_identifier`. Descriptions
derive from the matching Registry entry's Summary/Description. Covers all 5
B1 acceptance tests (sample Match with kind; 404 not-found error; 409
ambiguous error suggesting disambiguation; 422 unsupported error; all seven
names registered). Also realises F1.2's tool-level assertion (the
`mlwh_resolve_sample` output schema is non-nil, object-typed, and carries
the `Match` field descriptions).

- [ ] implemented
- [ ] reviewed

### Item 5.6: B2 - Find samples by exact field (unified enum)

spec.md section: B2

Add the single `mlwh_find_samples` tool (file
`internal/mlwh/tools_resolve.go`) unifying the five `FindSamplesBy*`
endpoints behind a `Field` enum (`sanger_id`, `lims_id`, `accession`,
`supplier_name`, `library_type`, in Registry order) plus `Value string`;
output `[]mlwh.Sample`. The enum and the field<->Method mapping come from
the code-sourced table (phase 3). Covers all 4 B2 acceptance tests
(accession lookup; sanger_id routes to `/find/sample/sanger-id/S1`;
invalid enum rejected at schema with no request; the exact 5-value enum in
Registry order).

- [ ] implemented
- [ ] reviewed

### Item 5.7: B3 - Expand identifiers

spec.md section: B3

Add the expand tools (file `internal/mlwh/tools_resolve.go`):
`mlwh_expand_identifier` (`ExpandIdentifier` -> `[]mlwh.TaggedID`),
`mlwh_expand_search_values` (`ExpandSearchValues` -> `mlwh.SearchValues`),
`mlwh_expand_sample_search_values` (`ExpandSampleSearchValues` ->
`[]string`). Input `Kind string` (enum from `mlwh.IdentifierKinds()`),
`Canonical string`. Covers all 3 B3 acceptance tests (two TaggedIDs; invalid
`kind` rejected with no request; `kind` enum equals `IdentifierKinds()`'s
15 values in order, first `sample_uuid`, last `id_library_lims`).

- [ ] implemented
- [ ] reviewed
