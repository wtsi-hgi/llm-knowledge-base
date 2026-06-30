# MLWH Real-World MCP Tools Specification

## Overview

Add MCP tools that answer common MLWH sequencing questions with cheap aggregate
calls, bounded list calls, count counterparts, and clear freshness/error
caveats. The MCP layer remains a thin wrapper over
`github.com/wtsi-hgi/wa/mlwh`; it must not reimplement MLWH joins, counts,
recency, QC, platform, or person routing logic.

The upstream `wa` code in `/home/ubuntu/wa/mlwh` is authoritative for endpoint
paths, query params, field names, result shapes, descriptions, and semantics.
This feature updates this repo from `wa v0.6.1` to the released tag or exact
commit containing `wa` API `1.7.0`, including header-aware page methods,
`CallWithHeaders`, `PagedStudyManifest`, paged detail, and all new result types.

Large responses must fail safely. All paginated typed tools default to one page
(`limit=100`, max `1000`) and return `total` plus `next_offset` from upstream
headers. A core result-size guard returns an actionable MCP tool error instead
of sending an over-budget result.

## Architecture

- `go.mod`, `go.sum`: require a `github.com/wtsi-hgi/wa` version whose
  `mlwh.APIVersion` is `1.7.0`.
- `internal/core/`: service-agnostic result-size guard and server options.
- `cmd/mlwh-mcp-server/main.go`: wire MLWH max-result flag/env to core.
- `internal/mlwh/provider.go`: register new tool groups.
- `internal/mlwh/tools_overview.go`: study/run overview, study status, run
  status, sample progress.
- `internal/mlwh/tools_availability.go`: samples-with/without-data, iRODS,
  manifest, and their counts.
- `internal/mlwh/tools_people.go`: faculty sponsor, user, and person tools.
- `internal/mlwh/tools_detail.go`: bounded paging, lean detail, count fan-outs.
- `internal/mlwh/tools_search.go`: header-aware search pages.
- `internal/mlwh/tools_resolve.go`: unified `mlwh_count_find_samples`.
- `internal/mlwh/tools_call.go`: `CallWithHeaders` dynamic calls.
- `internal/mlwh/schema.go`: OpenAPI-sourced schemas and paged wrappers.
- `internal/mlwh/workflow.go`: cheap-tool workflow guidance.
- `internal/mlwh/harness_test.go`: stub headers for page metadata.

New core public surface:

```go
package core

type Options struct {
    ServerVersion          string
    Logger                 *slog.Logger
    Providers              []Provider
    MaxToolResultBytes     int
    ToolResultSizeGuidance string
}

type ToolResultSizeError struct {
    Code        string `json:"code"`
    Message     string `json:"message"`
    LimitBytes  int    `json:"limit_bytes"`
    ActualBytes int    `json:"actual_bytes"`
    Guidance    string `json:"guidance"`
}

func ResultSizeGuard(maxBytes int, guidance string) mcp.Middleware
```

`MaxToolResultBytes <= 0` disables the guard. The MLWH binary passes a default
of `1048576` bytes and guidance naming overview/count/page workflows.

MLWH config additions preserve the existing remote config API:

```go
package mlwh

const DefaultMaxToolResultBytes = 1048576

type Config struct {
    BaseURL            string
    CACert             string
    Timeout            string
    MaxToolResultBytes string
}

func (c *Config) BindFlags(fs *flag.FlagSet)
func (c Config) Resolve(getenv func(string) string) (wa.RemoteConfig, error)
func (c Config) ResolveMaxToolResultBytes(
    getenv func(string) string,
) (int, error)
```

`BindFlags` adds `--mlwh-max-tool-result-bytes`; env fallback is
`MLWH_MAX_TOOL_RESULT_BYTES`.

Paged typed results keep semantic list fields:

```json
{"samples":[...],"total":250,"next_offset":100}
{"irods_paths":[...],"total":4,"next_offset":-1}
```

Envelope tools flatten metadata at the top level:

```json
{"id_study_lims":"S1","rows":[...],"cache_synced_at":"...","total":3,
 "next_offset":-1}
```

`next_offset` is `-1` when there is no next page. Count tools return only
`{"count":N}`.

Descriptions and workflow guidance must separate answer timestamps from cache
freshness. Responses carrying `cache_synced_at` use that field for the as-of
caveat. Bare lists, `Count`, `RunStatusTimeline`, and dynamic
`mlwh_call_endpoint` responses without `cache_synced_at` must direct agents to
`mlwh_freshness`.

## A. Core Hardening And Dependency

### A1: Target wa API 1.7.0

As an implementor, I want the MCP provider compiled against the current `wa`
MLWH API, so that the downstream tools use the upstream 1.7.0 contract.

Use the released tag or exact commit matching local `/home/ubuntu/wa/mlwh`.
Descriptions and output schemas continue to come from `wa.Registry` and
`wa.OpenAPIDocument()` wherever the existing pattern supports it.

**Package:** root module
**File:** `go.mod`
**Test file:** `internal/mlwh/provider_test.go`

**Acceptance tests:**

1. Given the updated module, when tests read `wa.APIVersion`, then it equals
   `"1.7.0"` and `mlwh.New(...).APIVersion()` reports `"1.7.0"`.
2. Given `wa.Registry`, when `registryEntryByMethod("StudyOverview")` and
   `registryEntryByMethod("ResolvePerson")` run, then both entries exist and
   have non-empty `Summary`, `Description`, and expected paths.
3. Given `wa.OpenAPIDocument()`, when `outputSchemaFor("StudyOverview")` and
   `outputSchemaFor("PersonCandidate")` run, then their JSON properties use the
   upstream `json` names and field descriptions.

### A2: Guard over-budget tool results

As an agent, I want oversized tool results to return a structured error, so
that I can switch to an overview, count, or smaller page.

Install `ResultSizeGuard` in `core.New` when `MaxToolResultBytes > 0`. It runs
after typed handlers populate `CallToolResult`. It measures
`json.Marshal(*mcp.CallToolResult)`; if that is not possible, it measures the
larger of marshaled `StructuredContent` and marshaled `Content`. Over-budget
results are replaced with `IsError=true`, structured content of
`ToolResultSizeError`, and one JSON text content block containing the same
object. The replacement error is not guarded again. The error `Code` is
`"tool_result_too_large"`.

**Package:** `internal/core/`
**File:** `internal/core/errs.go`
**Test file:** `internal/core/server_test.go`

**Acceptance tests:**

1. Given a fake provider tool returning `{"payload":"1234567890"}` and core
   `MaxToolResultBytes=20`, when the tool is called, then `IsError=true`,
   `structuredContent.code` is `"tool_result_too_large"`,
   `limit_bytes` is `20`, `actual_bytes` is greater than `20`, and `guidance`
   equals the configured guidance.
2. Given the same tool and `MaxToolResultBytes=0`, when called, then
   `IsError=false` and the original payload is returned.
3. Given `cmd/mlwh-mcp-server` args
   `--mlwh-max-tool-result-bytes=2048`, when config is resolved, then core
   receives `MaxToolResultBytes=2048`.
4. Given env `MLWH_MAX_TOOL_RESULT_BYTES=bad`, when config is resolved, then
   startup returns an error mentioning `MLWH_MAX_TOOL_RESULT_BYTES`.
5. Given `mlwh_call_endpoint` returns a dynamic payload larger than
   `MaxToolResultBytes`, when the tool is called, then the result is the same
   structured `tool_result_too_large` error and the dynamic payload is absent.

### A3: Bound and annotate every paged typed tool

As an agent, I want list tools to return one bounded page plus sizing hints, so
that counts and pagination do not exhaust context.

All typed paged tools use default `limit=100`, maximum `1000`, `offset=0`, and
header-aware `wa` methods. Reject `limit > 1000` before HTTP. Include top-level
`total` and `next_offset`. This applies to existing search/fan-out tools,
new lists, and paged detail/manifest envelopes. It does not apply to count
tools or non-paged `mlwh_studies_for_sample`.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/schema.go`
**Test file:** `internal/mlwh/tools_detail_test.go`

**Acceptance tests:**

1. Given `/studies` returns two rows with headers `X-Total-Count: 250` and
   `X-Next-Offset: 100`, when `mlwh_all_studies` is called with `{}`, then the
   stub receives `limit=100&offset=0` and the result has `studies` length 2,
   `total=250`, and `next_offset=100`.
2. Given any paged typed tool, when called with `limit=1001`, then it returns
   `IsError=true`, mentions the `1000` maximum, and no HTTP request is made.
3. Given `/study/S1/samples` returns no `X-Next-Offset`, when
   `mlwh_samples_for_study` is called, then `next_offset=-1`.
4. Given `mlwh_search_samples` receives headers `X-Total-Count: 7` and
   `X-Next-Offset: -1`, when called with `{"term":"mus"}`, then the result is
   `{"samples":[...],"total":7,"next_offset":-1}`.
5. Given the registered `mlwh_search_studies` tool, when its description is
   inspected, then it states the search covers `name`, `study_title`,
   `programme`, and `faculty_sponsor`.

## B. Aggregate And Status Tools

### B1: Study overview

As an agent, I want one small study overview call, so that I can answer
availability, recency, study metadata, and data-access-group questions without
fetching sample lists.

Register `mlwh_study_overview`. Input: `study_lims_id`. Call
`(*wa.RemoteClient).StudyOverview(ctx, studyLimsID)`. Output is
`wa.StudyOverview` unchanged. Use Registry/OpenAPI description and schema.
Preserve `cache_synced_at`; date fields mean iRODS `created`, not row changes.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_overview.go`
**Test file:** `internal/mlwh/tools_overview_test.go`

**Acceptance tests:**

1. Given `/study/S1/overview` returns `samples_total=5`,
   `samples_with_data=3`, `samples_without_data=2`,
   `samples_sequenced_no_data=1`, `data_access_group="dag1"`,
   `newest_data_added="2026-06-26T00:00:00Z"`, `added_last_7_days=2`, and
   `cache_synced_at="2026-06-30T09:00:00Z"`, when called, then those exact
   fields are present in `StructuredContent`.
2. Given the same call, then the stub path is `/study/S1/overview` and no list
   endpoint is requested.
3. Given a 404 `not_found`, when called, then `IsError=true` and the message
   tells the caller to check the identifier.

### B2: Study status breakdown

As an agent, I want one study status breakdown call, so that I can answer
received, sequenced, not sequenced, and manual-QC count questions cheaply.

Register `mlwh_study_status_breakdown`. Input: `study_lims_id`. Call
`StatusBreakdown`. Output is `wa.StatusBreakdown`. Preserve platform ladders,
QC split, `with_detailed_timeline`, and `cache_synced_at`. Do not collapse ONT
or registered-only samples to "no data"; pass through upstream buckets.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_overview.go`
**Test file:** `internal/mlwh/tools_overview_test.go`

**Acceptance tests:**

1. Given `/study/S1/status-breakdown` returns `distinct` `{with_data:3,
   sequenced_no_data:1, registered:1}`, `qc` `{qc_pass:2,qc_fail:1,
   qc_pending:1}`, one `per_platform` row for `"ONT"` with `registered=1`,
   and `cache_synced_at`, when called, then all values are returned unchanged.
2. Given the call, then the stub path is `/study/S1/status-breakdown`.
3. Given a never-synced joined upstream error, when called, then the mapped tool
   error mentions cache sync before not-found wording.

### B3: Run overview, run status, and sample progress

As an agent, I want small run and sample progress calls, so that "what is
happening?" questions avoid detail fan-out.

Register:

- `mlwh_run_overview`: input `id_run`, call `RunOverview`, output
  `wa.RunOverview`.
- `mlwh_run_status`: input `id_run`, call `RunStatus`, output
  `wa.RunStatusTimeline`.
- `mlwh_sample_progress`: input `sanger_name`, call `SampleProgress`, output
  `wa.SampleProgress`.

`mlwh_run_status` has no `cache_synced_at`; its description must tell agents to
use `mlwh_freshness` for an as-of caveat. `SampleProgress` passes through
baseline phase, QC, delivered timestamp, optional milestone fields, runs, and
platform/not-tracked signals.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_overview.go`
**Test file:** `internal/mlwh/tools_overview_test.go`

**Acceptance tests:**

1. Given `/run/52553/overview` returns `samples=10`, `studies=2`,
   `data_objects=4`, and `cache_synced_at`, when `mlwh_run_overview` is called,
   then those fields are returned and the path is `/run/52553/overview`.
2. Given `/run/52553/status` returns two events and `current="qc complete"`,
   when `mlwh_run_status` is called, then `cache_synced_at` is absent and the
   two events are returned in order.
3. Given `/sample/S1/progress` returns `baseline_phase="delivered"`,
   `qc="pass"`, `delivered_at`, `detailed_timeline=true`, one milestone, one
   run, and `cache_synced_at`, when `mlwh_sample_progress` is called, then
   every field is returned unchanged.
4. Given `/sample/ONT1/progress` returns `platforms:["ONT"]`,
   `qc:"not_tracked"`, `detailed_timeline=false`, and `runs:[]`, when called,
   then those exact values are returned.
5. Given the registered `mlwh_run_status` tool, when its description is
   inspected, then it says the response has no `cache_synced_at` and agents must
   call `mlwh_freshness` for the cache as-of caveat.

## C. Availability, IRODS, And Manifest

### C1: Samples with and without sequencing data

As an agent, I want bounded samples-with-data and samples-without-data tools, so
that I can list availability partitions only when counts are not enough.

Register:

- `mlwh_count_samples_with_data_for_study`: `study_lims_id`, optional `since`,
  `until`; call `CountSamplesWithDataSince` when any window arg is present,
  otherwise `CountSamplesWithData`.
- `mlwh_samples_with_data_for_study`: `study_lims_id`, optional `since`,
  `until`, `limit`, `offset`; call `SamplesWithDataSincePage`.
- `mlwh_samples_without_data_for_study`: `study_lims_id`, `limit`, `offset`;
  call `SamplesWithoutDataPage`.

`since`/`until` are RFC3339 over iRODS `created`, half-open `[since, until)`.
`until` without `since` and malformed timestamps are upstream 400s mapped to
actionable MCP errors.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_availability.go`
**Test file:** `internal/mlwh/tools_availability_test.go`

**Acceptance tests:**

1. Given `/study/S1/samples-with-data/count?since=2026-06-21T00:00:00Z`
   returns `{"count":2}`, when the count tool is called with that `since`, then
   the result is `{"count":2}` and the query contains that exact `since`.
2. Given `/study/S1/samples-with-data` receives
   `since=2026-06-21T00:00:00Z`, `until=2026-06-28T00:00:00Z`,
   `limit=100`, and `offset=0`, and returns one row with headers
   `X-Total-Count: 2`, `X-Next-Offset: 100`, when called with those `since`
   and `until` values and no limit, then the result has `samples` length 1,
   `total=2`, `next_offset=100`.
3. Given `/study/S1/samples-without-data` returns an ONT row with
   `platforms:["ONT"]`, when called, then that platform value is preserved.
4. Given upstream returns 400 for `until` without `since`, when the list tool is
   called with only `until`, then `IsError=true` and the message includes the
   upstream bad-request text.
5. Given upstream returns 400 for malformed `since=not-a-time`, when the count
   tool is called with that value, then `IsError=true` and the message includes
   the upstream bad-request text.
6. Given the samples-with-data and samples-without-data tool descriptions, when
   inspected, then each says bare list responses have no `cache_synced_at` and
   agents must call `mlwh_freshness` for the cache as-of caveat.

### C2: IRODS path tools with file-type counts

As an agent, I want run/study/sample iRODS path tools with suffix filtering and
counts, so that cram path questions can be answered with bounded transfer.

Register/update:

- `mlwh_irods_paths_for_sample`: add optional `file_type`, use
  `IRODSPathsForSampleByFileTypePage`.
- `mlwh_count_irods_paths_for_sample`: `sanger_name`, optional `file_type`.
- `mlwh_irods_paths_for_study`: add optional `file_type`, use
  `IRODSPathsForStudyByFileTypePage`.
- `mlwh_count_irods_paths_for_study`: `study_lims_id`, optional `file_type`.
- `mlwh_irods_paths_for_run`: `id_run`, optional `file_type`, use
  `IRODSPathsForRunByFileTypePage`.
- `mlwh_count_irods_paths_for_run`: `id_run`, optional `file_type`.

`file_type` semantics are upstream: one leading dot stripped,
case-insensitive filename suffix; valid unmatched suffix returns empty/0;
empty/whitespace, `%`, `_`, and `/` are 400s.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_availability.go`
**Test file:** `internal/mlwh/tools_availability_test.go`

**Acceptance tests:**

1. Given `/study/S1/irods?file_type=cram&limit=100&offset=0` returns one
   `IRODSPath` row with `id_run=52553` and `platform="illumina"` plus headers
   `X-Total-Count: 4`, `X-Next-Offset: -1`, when the study tool is called with
   `file_type:"cram"`, then those fields, `total=4`, and `next_offset=-1` are
   returned.
2. Given `/sample/S1/irods/count?file_type=.CRAM` returns `{"count":4}`, when
   the sample count tool is called with `.CRAM`, then the query contains
   `file_type=.CRAM` and the result is `{"count":4}`.
3. Given `/study/S1/irods/count?file_type=cram` returns `{"count":9}`, when
   the study count tool is called with `file_type:"cram"`, then the query
   contains `file_type=cram` and the result is `{"count":9}`.
4. Given `/sample/S1/irods?file_type=cram&limit=100&offset=0` returns one
   `IRODSPath` row with `id_product:"P1"`, `collection:"/seq/1"`,
   `data_object:"a.cram"`, `irods_path:"/seq/1/a.cram"`,
   `id_sample_tmp:123`, `name:"S1"`, `id_run:52553`, and
   `platform:"illumina"` plus headers `X-Total-Count: 1` and
   `X-Next-Offset: -1`, when the sample tool is called with
   `file_type:"cram"`, then the result field is `irods_paths`, the row exposes
   those fields, `total=1`, and `next_offset=-1`.
5. Given `/run/52553/irods?file_type=cram` returns two rows, when the run tool
   is called, then the path is `/run/52553/irods` and the result field is
   `irods_paths`, not `items`.
6. Given `/study/S1/irods?file_type=vcf&limit=100&offset=0` returns no rows
   with headers `X-Total-Count: 0`, `X-Next-Offset: -1`, when the study tool is
   called with unmatched valid suffix `vcf`, then the result is
   `irods_paths:[]`, `total=0`, and `next_offset=-1`.
7. Given `/run/52553/irods/count?file_type=vcf` returns `{"count":0}`, when
   the run count tool is called with unmatched valid suffix `vcf`, then the
   result is exactly `{"count":0}`.
8. Given upstream returns 400 for each of `file_type:" "`, `file_type:"%"`,
   and `file_type:"_"`, when any iRODS list or count tool is called with each
   value, then each call returns `IsError=true` and preserves the bad-request
   message.
9. Given upstream returns 400 for `file_type="/"`, when any iRODS tool is
   called with `/`, then `IsError=true` and the bad-request message is
   preserved.

### C3: Study manifest and count

As an agent, I want a paged study manifest tool, so that study manifests are one
server-side join with optional cram paths.

Register:

- `mlwh_study_manifest`: `study_lims_id`, optional `with_irods`, `file_type`,
  `limit`, `offset`; call `StudyManifestPage`.
- `mlwh_count_study_manifest`: `study_lims_id`; call `CountStudyManifest`.

The manifest output is flattened: upstream `StudyManifest` fields plus
top-level `total` and `next_offset`. `with_irods=true` includes `irods_path`;
without `file_type` it does not default to cram.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_availability.go`
**Test file:** `internal/mlwh/tools_availability_test.go`

**Acceptance tests:**

1. Given `/study/S1/manifest?with_irods=true&file_type=cram&limit=100&offset=0`
   returns study metadata, one row with `name:"S1"`,
   `supplier_name:"Supplier 1"`, `accession_number:"ERS1"`,
   `sanger_sample_id:"SANG1"`, `id_run:52553`, `lane:1`, `tag_index:2`, and
   `irods_path:"/irods/a.cram"`, `cache_synced_at`, and headers
   `X-Total-Count: 3`, `X-Next-Offset: -1`, when called, then the result has
   top-level `id_study_lims`, `rows`, `cache_synced_at`, `total=3`, and
   `next_offset=-1`, `rows[0]` exposes those row fields, and there is no
   `study_manifest` wrapper.
2. Given `/study/S1/manifest/count` returns `{"count":3}`, when the count tool
   is called, then the result is exactly `{"count":3}`.
3. Given `with_irods=false`, when called, then the query omits `with_irods` and
   returned rows omit `irods_path`.
4. Given `/study/S1/manifest?with_irods=true&limit=100&offset=0` returns one
   row with `irods_path="/irods/a.bam"`, when called with `with_irods:true` and
   no `file_type`, then the query has no `file_type`, the result returns that
   `.bam` path, and no `file_type=cram` request is made.
5. Given the output schema for `mlwh_study_manifest`, when
   `rows.items.properties` is inspected, then it exposes `name`,
   `supplier_name`, `accession_number`, `sanger_sample_id`, `id_run`, `lane`,
   `tag_index`, and optional `irods_path`.

## D. People, Study Lookup, And Count Coverage

### D1: Study-name lookup through search

As an agent, I want study-name lookup questions to use
`mlwh_search_studies`, so that I can find candidate study ids with enough
context to disambiguate.

For "What study id matches this name?" and partial or ambiguous study-name
questions, route to `mlwh_search_studies`. Keep the semantic result field
`studies`. Rows are upstream `wa.Study` values and must expose at least
`id_study_lims`, `name`, `study_title`, `programme`, `faculty_sponsor`, and
`accession_number` when upstream provides them. The tool description must state
the searched fields and that returned rows expose enough fields to
disambiguate candidate study ids.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_search.go`
**Test file:** `internal/mlwh/tools_search_test.go`

**Acceptance tests:**

1. Given `/search/study/cancer?limit=100&offset=0` returns rows with
   `id_study_lims:"S1"`, `name:"Cancer One"`, `study_title:"Tumour WGS"`,
   `programme:"Cancer"`, `faculty_sponsor:"Carl"`,
   `accession_number:"ERP1"` and `id_study_lims:"S2"`,
   `name:"Cancer Two"`, `study_title:"Tumour RNA"`,
   `programme:"Cancer"`, `faculty_sponsor:"Carla"`,
   `accession_number:"ERP2"`, plus headers `X-Total-Count: 2` and
   `X-Next-Offset: -1`, when `mlwh_search_studies` is called with
   `term:"cancer"`, then the stub receives `limit=100&offset=0`, the result has
   `studies` length 2, both rows expose those field values, `total=2`, and
   `next_offset=-1`.
2. Given the registered `mlwh_search_studies` description, when inspected, then
   it says the tool handles study-name/id lookup questions such as "What study
   id matches this name?", searches `name`, `study_title`, `programme`, and
   `faculty_sponsor`, and returns rows with enough fields to disambiguate
   candidate study ids.

### D2: Faculty sponsor, user, and person tools

As an agent, I want person-aware study lookup tools, so that sponsor questions
do not get conflated with `study_users` role membership.

Register:

- `mlwh_studies_for_faculty_sponsor`: `name`, `limit`, `offset`; call
  `StudiesForFacultySponsorPage`; rows omit `role`.
- `mlwh_count_studies_for_faculty_sponsor`: `name`.
- `mlwh_studies_for_user`: `person`, optional `role`, `limit`, `offset`; call
  `StudiesForUserPage`.
- `mlwh_count_studies_for_user`: `person`, optional `role`.
- `mlwh_resolve_person`: `term`, `limit`, `offset`; call
  `ResolvePersonPage`.
- `mlwh_count_resolve_person`: `term`.

Descriptions come from Registry and must state the sponsor/user distinction.
For user tools, omitted `role` sends no `role` query and uses upstream defaults:
`owner`, `manager`, `data_access_contact`. Supplied `role` overrides that set
exactly; matching is case-insensitive upstream.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_people.go`
**Test file:** `internal/mlwh/tools_people_test.go`

**Acceptance tests:**

1. Given `/studies/faculty-sponsor/Carl` returns one `PersonStudy` with no
   `role`, when called, then the result has `studies` length 1, `role` absent,
   and path `/studies/faculty-sponsor/Carl`.
2. Given `/studies/user/cwa?limit=100&offset=0` returns `owner` and
   `data_access_contact` rows, when called without `role`, then the query omits
   `role` and only those returned roles are present.
3. Given `/studies/user/cwa/count` returns `{"count":2}`, when the count tool is
   called without `role`, then the query omits `role` and uses the upstream
   default role set.
4. Given `/studies/user/cwa?role=Follower&limit=100&offset=0` returns one row
   with `role:"follower"`, when called with `role:"Follower"`, then the query
   preserves `role=Follower` and no default-role rows are returned.
5. Given `/studies/user/cwa/count?role=DATA_ACCESS_CONTACT` returns
   `{"count":1}`, when the count tool is called with that role, then the query
   preserves the supplied case and returns only the override count.
6. Given `/resolve-person/carl` returns candidates from `faculty_sponsor` and
   `study_users`, when called, then the result field is `people`, each row has
   `source`, `name`, and `study_count`, and page metadata is present.
7. Given the faculty sponsor and user tool descriptions, when inspected, then
   the former contains `faculty_sponsor` and the latter contains
   `study_users`.

### D3: Surface every missing upstream count

As an agent, I want count tools for every large upstream list, so that I can
size a result before transferring rows.

Add missing count tools for these Registry methods:

- `CountSamplesForRun` -> `mlwh_count_samples_for_run`.
- `CountRunsForStudy` -> `mlwh_count_runs_for_study`.
- `CountLibrariesForStudy` -> `mlwh_count_libraries_for_study`.
- `CountLanesForSample` -> `mlwh_count_lanes_for_sample`.
- `CountSamplesForLibrary` -> `mlwh_count_samples_for_library`.
- `CountSamplesForLibraryID` -> `mlwh_count_samples_for_library_id`.
- `CountSamplesForLibraryLimsID` ->
  `mlwh_count_samples_for_library_lims_id`.
- `CountSamplesForLibraryType` ->
  `mlwh_count_samples_for_library_type`.

Keep existing `mlwh_count_samples_for_study`, `mlwh_count_samples`,
`mlwh_count_studies_search`, and `mlwh_count_studies`.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_detail.go`
**Test file:** `internal/mlwh/tools_detail_test.go`

**Acceptance tests:**

1. Given `/run/52553/samples/count` returns `{"count":10}`, when
   `mlwh_count_samples_for_run` is called with `id_run:"52553"`, then the path
   is `/run/52553/samples/count` and the result is `{"count":10}`.
2. Given `/study/S1/runs/count` returns `{"count":2}`, when
   `mlwh_count_runs_for_study` is called, then the result is `{"count":2}`.
3. Given `/study/S1/libraries/count` returns `{"count":5}`, when
   `mlwh_count_libraries_for_study` is called, then the result is
   `{"count":5}`.
4. Given `/sample/S1/lanes/count` returns `{"count":4}`, when
   `mlwh_count_lanes_for_sample` is called, then the result is `{"count":4}`.
5. Given `/library/P1/study/S1/samples/count` returns `{"count":3}`, when
   `mlwh_count_samples_for_library` is called, then library and study path
   params appear in that order and the result is `{"count":3}`.
6. Given `/library-id/LIB123/samples/count` returns `{"count":6}`, when
   `mlwh_count_samples_for_library_id` is called with `library_id:"LIB123"`,
   then the path is `/library-id/LIB123/samples/count` and the result is
   `{"count":6}`.
7. Given `/library-lims-id/LIMS123/samples/count` returns `{"count":7}`, when
   `mlwh_count_samples_for_library_lims_id` is called with
   `library_lims_id:"LIMS123"`, then the path is
   `/library-lims-id/LIMS123/samples/count` and the result is `{"count":7}`.
8. Given `/library-type/WGS/samples/count` returns `{"count":8}`, when
   `mlwh_count_samples_for_library_type` is called with `library_type:"WGS"`,
   then the path is `/library-type/WGS/samples/count` and the result is
   `{"count":8}`.

### D4: Unified exact sample-finder count

As an agent, I want one `mlwh_count_find_samples` tool, so that exact
sample-finder counts use the same field enum as `mlwh_find_samples`.

Input: `field`, `value`. The `field` enum is the same as
`mlwh_find_samples`: `sanger_id`, `lims_id`, `accession`, `supplier_name`,
`library_type`. Dispatch to the matching `CountFindSamplesBy*` Registry method.
Output is `wa.Count`.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_resolve.go`
**Test file:** `internal/mlwh/tools_resolve_test.go`

**Acceptance tests:**

1. Given `/find/sample/sanger-id/ABC/count` returns `{"count":1}`, when called
   with `field:"sanger_id", value:"ABC"`, then the path is
   `/find/sample/sanger-id/ABC/count` and the result is `{"count":1}`.
2. Given `/find/sample/lims-id/LIMS1/count` returns `{"count":2}`, when called
   with `field:"lims_id", value:"LIMS1"`, then the path is
   `/find/sample/lims-id/LIMS1/count` and the result is `{"count":2}`.
3. Given `/find/sample/accession/ERS1/count` returns `{"count":1}`, when called
   with `field:"accession", value:"ERS1"`, then the path is
   `/find/sample/accession/ERS1/count` and the result is `{"count":1}`.
4. Given `/find/sample/supplier-name/Bob/count` returns `{"count":3}`, when
   called with `field:"supplier_name", value:"Bob"`, then the path is
   `/find/sample/supplier-name/Bob/count` and the result is `{"count":3}`.
5. Given `/find/sample/library-type/WGS/count` returns `{"count":5}`, when
   called with `field:"library_type", value:"WGS"`, then the path is
   `/find/sample/library-type/WGS/count` and the result is `{"count":5}`.
6. Given `field:"bad"`, when called, then schema validation or the handler
   rejects it before HTTP and the message names the supported fields.

## E. Existing Tool Updates And Dynamic Calls

### E1: Lean, paged study and run detail

As an agent, I want detail tools to expose lean bounded pages, so that detail
queries are still available without unbounded payloads.

Update only `mlwh_study_detail` and `mlwh_run_detail` to accept `limit`,
`offset`, and `lean`. Default `limit=100`, max `1000`, default `offset=0`.
Call `StudyDetailWithOptions` and `RunDetailWithOptions`. Flatten
`wa.StudyDetail`/`wa.RunDetail` fields with top-level `total` and
`next_offset`. Do not add `lean` to `mlwh_sample_detail`.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_detail.go`
**Test file:** `internal/mlwh/tools_detail_test.go`

**Acceptance tests:**

1. Given `/study/S1/detail?limit=100&offset=0&lean=true` returns a lean
   `StudyDetail` with headers `X-Total-Count: 200`,
   `X-Next-Offset: 100`, when called with `lean:true`, then the result has
   top-level `study`, `sample_ids`, `lean:true`, `total=200`, and
   `next_offset=100`.
2. Given `/run/52553/detail?limit=50&offset=50` returns a `RunDetail` with
   headers `X-Total-Count: 120`, `X-Next-Offset: 100`, when called with
   `limit:50, offset:50`, then those query params and metadata are preserved.
3. Given `mlwh_sample_detail` is inspected, then its input schema has no
   `lean`, `limit`, or `offset` properties.

### E2: Header-aware generic call endpoint

As an agent, I want `mlwh_call_endpoint` to expose pagination headers, so that
dynamic calls can still be paged safely.

Update `mlwh_call_endpoint` to call `CallWithHeaders`. If `X-Total-Count` or
`X-Next-Offset` is present, return `{"result":decoded,"total":N,
"next_offset":M}`. If neither header is present, keep returning the decoded
result directly. Do not infer semantic field names for dynamic calls.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_call.go`
**Test file:** `internal/mlwh/tools_call_test.go`

**Acceptance tests:**

1. Given dynamic method `AllStudies` returns a bare array and headers
   `X-Total-Count: 250`, `X-Next-Offset: 100`, when `mlwh_call_endpoint` is
   called, then `StructuredContent.result` is the array, `total=250`, and
   `next_offset=100`.
2. Given dynamic method `ResolveStudy` returns a `Match` with no pagination
   headers, when called, then `StructuredContent` is the `Match` object with no
   `result` wrapper.
3. Given an unknown method, when called, then the mapped tool error preserves
   the method name and is `IsError=true`.
4. Given dynamic method `RunStatus` returns a response with no
   `cache_synced_at`, when `mlwh_call_endpoint` is called, then the result does
   not synthesize `cache_synced_at` and the tool description directs agents to
   `mlwh_freshness` for the cache as-of caveat.

### E3: Workflow guidance chooses cheap tools first

As an agent, I want `mlwh://workflow` to route common real-world questions to
cheap tools, so that I do not pick large detail/list calls unnecessarily.

Update `workflowGuidance` before the Registry catalogue. It must mention:

- availability/counts: `mlwh_study_overview` or
  `mlwh_count_samples_with_data_for_study`; do not page iRODS or use
  `mlwh_study_detail` for availability/count questions;
- recency: prefer `added_last_7_days` from `mlwh_study_overview`; otherwise use
  explicit `since`/`until`; data "added to iRODS" means iRODS `created`;
- data access group: `mlwh_study_overview` or `mlwh_resolve_study`; do not use
  study detail;
- QC counts: `mlwh_study_status_breakdown`;
- sample/run progress: `mlwh_sample_progress`, `mlwh_run_status`; compute open
  phase elapsed time from `reached_at` / `entered_at` on the agent side;
- cram paths: count first, then iRODS tool with `file_type=cram`;
- manifest: `mlwh_study_manifest`, with `with_irods=true` and
  `file_type=cram` when a cram column is requested;
- people routing: sponsor to faculty-sponsor tools, login/email/membership to
  user tools, ambiguous names through `mlwh_resolve_person`;
- freshness: use response `cache_synced_at` when present; use
  `mlwh_freshness` for bare lists, counts, `mlwh_run_status`, and
  `mlwh_call_endpoint` responses that lack it.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/workflow.go`
**Test file:** `internal/mlwh/workflow_test.go`

**Acceptance tests:**

1. Given `mlwh://workflow` is read, then the body contains
   `mlwh_study_overview`, `mlwh_study_status_breakdown`,
   `mlwh_sample_progress`, `file_type=cram`, and `mlwh_resolve_person`.
2. Given the same body, then it says recency prefers `added_last_7_days`, uses
   explicit `since` and `until` otherwise, contains `added to iRODS`, and does
   not say `last_changed` or `last_updated` is new data.
3. Given the same body, then it says not to page iRODS or use
   `mlwh_study_detail` for availability/count questions and not to use study
   detail for data-access-group questions.
4. Given the same body, then it says sample/run progress open phase elapsed
   time is computed on the agent side from `reached_at` or `entered_at`.
5. Given the same body, then the Registry-derived endpoint catalogue is still
   appended and contains `/study/:id/overview`.
6. Given the same body, then it says to use response `cache_synced_at` when
   present and `mlwh_freshness` for bare lists, counts, `mlwh_run_status`, and
   `mlwh_call_endpoint` responses without `cache_synced_at`.

## F. Error, Schema, And Registration Consistency

### F1: Preserve actionable upstream errors

As an agent, I want all new tools to map upstream failures consistently, so that
I can fix input, disambiguate, or retry after sync.

Every new handler passes upstream errors through `mapToolError`. This includes
400 bad requests for malformed timestamps, invalid `file_type`, whitespace-only
person terms, not found, ambiguity, unsupported identifiers, impaired upstream,
and never-synced cache states.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/errmap.go`
**Test file:** `internal/mlwh/errmap_test.go`

**Acceptance tests:**

1. Given a new list tool receives upstream 400 `"invalid file_type"`, when
   called, then `IsError=true` and text content contains `invalid file_type`.
2. Given `mlwh_resolve_person` receives upstream 400 for a whitespace-only
   term, when called, then `IsError=true` and text content says the person term
   must not be blank.
3. Given a new study tool receives 409 `ambiguous_identifier`, when called, then
   `IsError=true` and text content tells the caller to disambiguate the study.
4. Given `mlwh_irods_paths_for_run` receives upstream 400
   `unsupported_identifier` for a non-Illumina run id, when called, then
   `IsError=true` and text content says the run identifier is unsupported.
5. Given a new aggregate tool receives upstream 404 `not_found`, when called,
   then `IsError=true` and text content tells the caller to check the
   identifier.
6. Given a new aggregate tool receives 503 `cache_never_synced`, when called,
   then text content mentions the cache has never synced.
7. Given a new manifest or list tool receives a 502 impaired cache/upstream
   error, when called, then text content advises retrying after cache or
   upstream recovery.
8. Given a new person tool receives a 502 impaired upstream error, when called,
   then text content advises fixing input or retrying later.

### F2: Register the full MLWH surface

As a developer, I want registration tests for the full tool set, so that no
required endpoint is missed.

Extend provider registration tests to include every new and updated tool. Tool
descriptions for curated tools use Registry/OpenAPI text unless this spec
requires extra MCP paging/size/freshness guidance.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/provider.go`
**Test file:** `internal/mlwh/provider_test.go`

**Acceptance tests:**

1. Given the MLWH provider is registered, when tools are listed, then the list
   includes `mlwh_study_overview`, `mlwh_study_status_breakdown`,
   `mlwh_run_overview`, `mlwh_run_status`, `mlwh_sample_progress`,
   `mlwh_samples_with_data_for_study`,
   `mlwh_samples_without_data_for_study`, `mlwh_study_manifest`,
   `mlwh_irods_paths_for_run`, `mlwh_studies_for_user`,
   `mlwh_resolve_person`, and `mlwh_count_find_samples`.
2. Given tools are listed, then every `/count` Registry method is surfaced by
   one MCP tool, with exact sample-finder counts covered only by
   `mlwh_count_find_samples`.
3. Given output schemas for `mlwh_samples_with_data_for_study` and
   `mlwh_study_manifest`, then each is an object schema with semantic fields
   plus required `total` and `next_offset`.
4. Given descriptions for all registered count tools, then each says `Count`
   responses have no `cache_synced_at` and agents must call `mlwh_freshness` for
   the cache as-of caveat.

### F3: Extend the hermetic MLWH harness

As an implementor, I want header-aware stub responses, so that page metadata and
guard behaviour are tested without a live warehouse.

Extend `stubResponse` with HTTP headers and add a helper:

```go
func (s *stubMLWH) respondJSONWithHeaders(
    path string,
    status int,
    body any,
    headers http.Header,
)
```

All provider acceptance tests remain hermetic. No test may call a live MLWH
server.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/harness_test.go`
**Test file:** `internal/mlwh/harness_test.go`

**Acceptance tests:**

1. Given `respondJSONWithHeaders` is configured with
   `X-Total-Count: 2`, when a tool calls that route, then the handler sees the
   same header and returns `total=2`.
2. Given an unmatched route, when any tool calls it, then the harness still
   returns a wa-shaped 404 error envelope.

## Implementation Order

1. [A1] Update `wa` dependency to API `1.7.0`; refresh schemas and fix
   compile breaks. This is prerequisite work.
2. [A2] Add core result-size guard and MLWH flag/env wiring. This can be tested
   with a fake provider before MLWH tools change.
3. [F3] Extend the MLWH test harness with response headers.
4. [A3] Add paged result wrappers and switch existing paged search/fan-out tools
   to bounded default pages with header metadata.
5. [B1, B2, B3] Add aggregate/status tools: study overview, status breakdown,
   run overview, run status, sample progress.
6. [C1, C2, C3] Add availability, iRODS, manifest, and related count tools.
7. [D1, D2, D3, D4] Add study-name lookup guidance, people tools, and missing
   count surfaces, including unified `mlwh_count_find_samples`.
8. [E1, E2] Update paged/lean detail and `mlwh_call_endpoint`.
9. [E3, F1, F2] Update workflow guidance, error mapping, and full
   registration/schema assertions.

Steps 4-8 are sequential after step 3 because they share page wrappers and the
header-aware harness. Individual tool groups inside steps 5-7 can be developed
in parallel once the wrappers exist.

## Appendix: Key Decisions

- **Upstream authority.** `wa.Registry`, `wa.OpenAPIDocument()`, and typed
  remote methods define paths, params, descriptions, and JSON fields. The MCP
  layer only wraps and presents them.
- **No MCP-side fan-out counts.** Aggregates and counts must call upstream
  aggregate/count endpoints, never fetch lists and count locally.
- **Bounded by default.** Existing fetch-all fan-out defaults are removed for
  MCP typed tools. Counts and `total` tell agents how much remains.
- **Semantic wrappers.** Curated tools keep `samples`, `studies`,
  `irods_paths`, `rows`, or native detail fields. Only `mlwh_call_endpoint`
  uses generic `result`.
- **Timestamp wording.** "New data" and "added to iRODS" mean the upstream iRODS
  `created` timestamp. `last_changed`, `last_updated`, and sync `last_run` are
  not new-data timestamps.
- **Freshness caveats.** Prefer `cache_synced_at` when the response carries it.
  Use `mlwh_freshness` for responses without it, especially bare lists, counts,
  and `mlwh_run_status`.
- **Platform fidelity.** Pass through `platforms`, `platform`,
  `qc:"not_tracked"`, ONT, registered-only, and upstream not-tracked signals.
  Never collapse them to a bare "no data".
- **People routing.** Faculty sponsor and `study_users` membership are separate
  sources. A zero result in one source does not prove the person has no studies.
- **Testing.** Implementors use GoConvey per `go-conventions` and behavioural,
  public-boundary tests per `testing-principles`. Reviewers use
  `go-reviewer`. Every acceptance test above needs a corresponding hermetic
  GoConvey assertion; no live MLWH, hardcoded bypasses, or build-tag exclusions.
