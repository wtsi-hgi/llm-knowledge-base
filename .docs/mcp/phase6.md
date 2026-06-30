# Phase 6: Detail/fan-out + freshness + escape hatch

Ref: [spec.md](spec.md) sections C1, C2, D1, E1, Implementation Order
item 6

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 5 (reuses the stub MLWH test harness and the F2 slice
wrappers). Tests stay hermetic against the `httptest` stub.

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

### Item 6.1: C1 - Detail tools (sample/study/run/library)

spec.md section: C1

Add the grouped detail tools (file `internal/mlwh/tools_detail.go`):
`mlwh_sample_detail` (`SampleDetail`, input `SangerName`, output
`mlwh.SampleDetail`), `mlwh_study_detail` (`StudyDetail`, input
`StudyLimsID`, output `mlwh.StudyDetail`), `mlwh_run_detail` (`RunDetail`,
input `IDRun`, output `mlwh.RunDetail`), `mlwh_library_detail`
(`LibraryDetail`, input `PipelineIDLims` + `StudyLimsID`, output
`mlwh.LibraryDetail`). Covers all 4 C1 acceptance tests (SampleDetail
aggregate; library detail routes to `/library/P1/study/5901/detail`; 503
cache-never-synced error on study detail; all four names registered).

- [x] implemented
- [x] reviewed

### Item 6.2: C2 - Fan-out enumeration tools

spec.md section: C2

Add the fan-out tools (file `internal/mlwh/tools_detail.go`). Paginated
(input adds `Limit`, `Offset`; FETCH-ALL DEFAULT: omitted `limit` MUST send
`limit=1000000`, the upstream fetch-all sentinel, NOT 0): `mlwh_all_studies`
(`AllStudies`), `mlwh_samples_for_study` (`SamplesForStudy`),
`mlwh_samples_for_run` (`SamplesForRun`), `mlwh_libraries_for_study`
(`LibrariesForStudy`), `mlwh_runs_for_study` (`RunsForStudy`),
`mlwh_lanes_for_sample` (`LanesForSample`), `mlwh_irods_paths_for_sample`
(`IRODSPathsForSample`), `mlwh_irods_paths_for_study` (`IRODSPathsForStudy`).
Non-paginated: `mlwh_studies_for_sample` (`StudiesForSample`),
`mlwh_count_samples_for_study` (`CountSamplesForStudy` -> `mlwh.Count`).
Descriptions state the fetch-all default. Covers all 4 C2 acceptance tests
(fetch-all sends `limit=1000000`/`offset=0`, not `limit=0`; explicit
`limit=50`/`offset=50` passthrough; samples-for-study count 300; empty
studies-for-sample array with `IsError=false`).

- [x] implemented
- [x] reviewed

### Item 6.3: D1 - Report cache freshness

spec.md section: D1

Add `mlwh_freshness` (file `internal/mlwh/tools_freshness.go`) calling
`Freshness(ctx)` (no input); output `mlwh.Freshness`. Description states it
reports per-table high-water + last-run timestamps and ever_synced and
succeeds even on a never-synced cache. Covers all 3 D1 acceptance tests
(five synced tables; never-synced state returns `IsError=false`; tool name
and accepts empty input `{}`).

- [x] implemented
- [x] reviewed

### Item 6.4: E1 - Call any MLWH endpoint (escape hatch)

spec.md section: E1

Add the generic `mlwh_call_endpoint` (file `internal/mlwh/tools_call.go`).
Input `Method string`, `PathParams []string`, `QueryParams
map[string]string`; dispatch via `(*mlwh.RemoteClient).Call(ctx, method,
pathParams, query)` (which validates method and path-param arity itself).
Output is `any` (untyped passthrough, so the SDK omits the output schema).
Description explains how to find Method names (the workflow resource, G1)
and that it is an escape hatch. Covers all 5 E1 acceptance tests (decoded
Match via `ResolveStudy`; `limit`/`offset` passthrough for `AllStudies`;
unknown-method error naming the method; path-param arity error for
`SampleDetail`; no output schema because `Out` is `any`).

- [x] implemented
- [x] reviewed
