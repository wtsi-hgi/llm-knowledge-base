# Phase 7: People, Study Lookup, And Count Coverage

Ref: [spec.md](spec.md) sections D1, D2, D3, D4

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer`
skills.

## Items

### Batch 1 (parallel)

#### Item 7.1: D1 - Study-name lookup through search [parallel with D2, D3, D4]

spec.md section: D1

Update `mlwh_search_studies` in `internal/mlwh/tools_search.go` so
study-name and id lookup questions use bounded search results with
disambiguating fields, covering all 2 acceptance tests from D1.
Depends on phase 4.

- [x] implemented
- [x] reviewed

#### Item 7.2: D2 - Faculty sponsor, user, and person tools

Parallel with D1, D3, D4.

spec.md section: D2

Register sponsor, user, resolve-person, and related count tools in
`internal/mlwh/tools_people.go`, preserving sponsor versus
`study_users` semantics, covering all 7 acceptance tests from D2.
Depends on phase 4.

- [x] implemented
- [x] reviewed

#### Item 7.3: D3 - Surface every missing upstream count

Parallel with D1, D2, D4.

spec.md section: D3

Add missing Registry count tools in `internal/mlwh/tools_detail.go` for
run, study, library, lane, and library-type sample counts, covering all
8 acceptance tests from D3. Depends on phase 4.

- [x] implemented
- [x] reviewed

#### Item 7.4: D4 - Unified exact sample-finder count [parallel with D1, D2, D3]

spec.md section: D4

Add `mlwh_count_find_samples` in `internal/mlwh/tools_resolve.go`,
sharing the `mlwh_find_samples` field enum and dispatching to the
matching `CountFindSamplesBy*` method, covering all 6 acceptance tests
from D4. Depends on phase 4.

- [x] implemented
- [x] reviewed

For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill
(review all items in the batch together in a single review
pass).
