# Feature: MLWH MCP tools for cheap real-world sequencing questions

## Summary

Add MCP tooling so an agent can answer the common MLWH questions cheaply: one small
call for aggregates, and bounded/paged calls for lists.

- "How many samples in study X have sequencing data, and how many do not?"
- "Is there new sequencing data for study X this week?"
- "What is happening with my sample, run, or study?"
- "What is in this study or run?"
- "What data access group is study X in?"
- "What are the cram iRODS paths for run/study/sample X?"
- "Give me a study manifest with run id, sample name, supplier name, accession, and
  optionally cram path."
- "How many study samples are not sequenced, sequenced, and passed manual QC?"
- "What study id matches this name?"
- "Which studies are associated with this sponsor/person/login/email?"

The upstream `wa` MLWH REST API already exposes the needed endpoints (API 1.7.0),
and the upstream `wa` Go client now exposes header-aware remote-client methods for
paged lists, paged manifest/detail envelopes, and dynamic calls. This feature wraps
those endpoints as MCP tools and hardens the MCP result path so large responses fail
with actionable guidance instead of overflowing the agent's budget.

Everything in this prompt is in scope to build. The `wa` code is authoritative; this
prompt describes the downstream MCP work only.

## Authority

Use the current `wa` Go code as the only contract for endpoint paths, query params,
field names, descriptions, and semantics:

- `~/wa/mlwh/registry.go`: endpoint `Method`, `Path`, `Query`, `Summary`,
  `Description`, and `QueryParams`.
- `~/wa/mlwh/types.go`: exact `json:` field tags and output shapes.
- `~/wa/mlwh/availability.go`, `progress.go`, `manifest.go`, `people.go`,
  `remote.go`, and `server.go`: behavior and typed client methods.
- `~/wa/.docs/mcp/api-reference.md` and `wa.OpenAPIDocument()`: generated mirrors of
  the code.

Do not use `~/wa/.docs/realworld*` prompt/spec/phase files as a contract. They are
known to have drifted.

Descriptions and output schemas in the MCP layer must be sourced from the upstream
`Registry`/OpenAPI wherever the existing code pattern supports that. Re-verify
against `~/wa` before implementing.

The header-aware pagination surface in `wa` is part of the upstream contract. Use
the current `~/wa` code, and the corresponding released tag or exact commit that
contains it, when updating the `github.com/wtsi-hgi/wa` dependency. The local
dependency is currently older than this surface and must be updated before the MCP
tools can use it.

## Upstream Surface

All endpoints are `GET`. Aggregate/progress endpoints return one small object. Bare
list endpoints return a JSON array plus `X-Total-Count` and `X-Next-Offset`; typed
client page variants expose those headers as `Page[T]` (`items`, `total`,
`next_offset`). `StudyManifest` is the exception: it returns an envelope object with
study metadata, `rows`, and `cache_synced_at`; its `rows` are paged and sized by the
same headers, and the upstream client exposes `StudyManifestPage` returning
`PagedStudyManifest`. `StudyDetailWithOptions` and `RunDetailWithOptions` expose
the paged/lean detail responses with header metadata. `CallWithHeaders` exposes
headers for Registry-driven dynamic calls.

| Endpoint | Registry method | Returns | Notes |
| --- | --- | --- | --- |
| `/study/:id/overview` | `StudyOverview` | `StudyOverview` | Study aggregate, study metadata, availability, recency |
| `/study/:id/status-breakdown` | `StatusBreakdown` | `StatusBreakdown` | Phase ladder, per-platform ladder, QC split |
| `/study/:id/samples-with-data` | `SamplesWithData` | `[]SampleWithData` | Paged; accepts `since`,`until` |
| `/study/:id/samples-with-data/count` | `CountSamplesWithData` | `Count` | Accepts `since`,`until` |
| `/study/:id/samples-without-data` | `SamplesWithoutData` | `[]SampleWithData` | Paged |
| `/run/:id/overview` | `RunOverview` | `RunOverview` | Run aggregate |
| `/run/:id/status` | `RunStatus` | `RunStatusTimeline` | Within-sequencing status timeline |
| `/sample/:id/progress` | `SampleProgress` | `SampleProgress` | Baseline, milestones, per-run status |
| `/sample/:id/irods` | `IRODSPathsForSample` | `[]IRODSPath` | Paged; accepts `file_type` |
| `/sample/:id/irods/count` | `CountIRODSPathsForSample` | `Count` | Accepts `file_type` |
| `/study/:id/irods` | `IRODSPathsForStudy` | `[]IRODSPath` | Paged; accepts `file_type` |
| `/study/:id/irods/count` | `CountIRODSPathsForStudy` | `Count` | Accepts `file_type` |
| `/run/:id/irods` | `IRODSPathsForRun` | `[]IRODSPath` | Paged; accepts `file_type`; Illumina NPG run id |
| `/run/:id/irods/count` | `CountIRODSPathsForRun` | `Count` | Accepts `file_type` |
| `/study/:id/manifest` | `StudyManifest` | `StudyManifest` | Paged `rows`; accepts `with_irods`,`file_type` |
| `/study/:id/manifest/count` | `CountStudyManifest` | `Count` | Product-grained; unaffected by `with_irods`/`file_type` |
| `/studies/faculty-sponsor/:name` | `StudiesForFacultySponsor` | `[]PersonStudy` | Paged; named sponsor/PI substring |
| `/studies/faculty-sponsor/:name/count` | `CountStudiesForFacultySponsor` | `Count` | Count counterpart |
| `/studies/user/:person` | `StudiesForUser` | `[]PersonStudy` | Paged; `study_users`; accepts `role` |
| `/studies/user/:person/count` | `CountStudiesForUser` | `Count` | Accepts `role` |
| `/resolve-person/:term` | `ResolvePerson` | `[]PersonCandidate` | Paged; candidates from sponsors and `study_users` |
| `/resolve-person/:term/count` | `CountResolvePerson` | `Count` | Count counterpart |

Also add MCP count tools for every upstream `/count` counterpart that is not already
surfaced, including existing large lists such as study samples, run samples, study
runs, study libraries, and sample lanes.

## Output Shapes

- **`StudyOverview`**: `id_study_lims`, `name`, `accession_number`,
  `faculty_sponsor`, `data_access_group`, `samples_total`, `samples_with_data`,
  `samples_without_data`, `samples_sequenced_no_data`, `data_objects`, `runs`,
  `libraries`, `library_types`, `sequencing_date_range` (`{earliest, latest}`,
  omitempty), `newest_data_added`, `added_last_7_days`, `cache_synced_at`.
- **`RunOverview`**: `id_run`, `samples`, `studies`, `data_objects`,
  `sequencing_date_range` (omitempty), `cache_synced_at`.
- **`StatusBreakdown`**: `id_study_lims`, `distinct` (`{with_data,
  sequenced_no_data, registered}`), `per_platform` (`[]{platform, ladder}`), `qc`
  (`{qc_pass, qc_fail, qc_pending}`), `with_detailed_timeline`, `cache_synced_at`.
- **`SampleProgress`**: `sample`, `platforms`, `baseline_phase`
  (`registered|sequenced|delivered`), `qc` (`pass|fail|pending|not_tracked`),
  `delivered_at`, `detailed_timeline`, `timeline_reason` (omitempty), `milestones`
  (omitempty; `{name, reached_at, duration_to_next}`), `current_milestone`
  (omitempty), `runs` (omitempty), `cache_synced_at`.
- **`RunStatusTimeline`**: `id_run`, `platform`, `events` (`[]{phase, entered_at,
  duration}`), `current`, `not_tracked` (omitempty; currently always empty). No
  `cache_synced_at`.
- **`SampleWithData`**: `sample`, `platforms`. No `cache_synced_at`.
- **`IRODSPath`**: `id_product`, `collection`, `data_object`, `irods_path`,
  `id_sample_tmp`, `name`, `id_run` (0 when not derivable), `platform`.
- **`StudyManifest`**: `id_study_lims`, `name`, `accession_number`,
  `faculty_sponsor`, `data_access_group`, `rows`, `cache_synced_at`. Each row has
  `name`, `supplier_name`, `accession_number`, `sanger_sample_id`, `id_run`, `lane`,
  `tag_index`, and optional `irods_path` when `with_irods=true`.
- **`PersonStudy`**: `study`, `role` (omitempty; empty for faculty sponsor, set for
  `study_users` membership).
- **`PersonCandidate`**: `source` (`faculty_sponsor|study_users`), `name`, optional
  `login`, optional `email`, optional `role`, `study_count`.
- **`Count`**: `{count}`. No `cache_synced_at`.

## MCP Paged Result Shape

Paged MCP list tools must preserve the existing semantic wrapper convention and add
top-level sizing fields. Use named list fields plus `total` and `next_offset`, for
example `{samples:[...], total, next_offset}` or `{irods_paths:[...], total,
next_offset}`. Do not switch normal typed tools to a generic `{items:[...], ...}`
shape. `next_offset` is `-1` when there is no next page.

The sizing fields must come from the upstream `wa` header-aware client result for
the same request and filters. Count tools still return `{count}` and are used when
the caller explicitly asks for a count, not as hidden MCP-side fan-out for paged
list metadata.

## Query Semantics

- `since` / `until`: optional RFC3339 window on `/study/:id/samples-with-data` and
  `/count`. The window is half-open `[since, until)` over iRODS `created`; `since`
  is inclusive, `until` is exclusive. `until` without `since` and malformed
  timestamps are upstream 400s and must map to actionable MCP errors.
- `file_type`: optional on sample/study/run iRODS list and count tools. It is a
  case-insensitive filename suffix filter with one leading dot stripped, so `cram`,
  `.CRAM`, and `CRAM` are equivalent. Empty/whitespace values, `%`, `_`, and `/` are
  upstream 400s. A valid but unmatched suffix returns an empty result or count 0.
- `with_irods`: optional boolean on `StudyManifest`. When true, include
  `irods_path`; when false, omit it. `with_irods=true` without `file_type` does not
  default to cram; upstream returns any one object for the product.
- `role`: optional on `StudiesForUser` list and count. Omitted means the upstream
  default role set: `owner`, `manager`, `data_access_contact`. Supplying `role`
  overrides that set exactly and case-insensitively; for example `role=follower`
  returns only follower rows.
- `lean`: optional boolean on `/study/:id/detail` and `/run/:id/detail` only. There
  is no landed `lean` param for `/sample/:id/detail`.

## Time And Freshness

Never conflate these timestamps:

1. **Data added to iRODS**: `seq_product_irods_locations.created`, mirrored upstream.
   This is the only basis for "new this week?", `since`/`until`,
   `newest_data_added`, `sequencing_date_range`, and `delivered_at`.
2. **`last_changed` / `last_updated`**: the warehouse row-change time used for cache
   sync. Do not present this as "new data"; it includes later modifications.
3. **`cache_synced_at` / freshness**: the oldest relevant `last_run`; it bounds
   completeness only. Present it as a caveat, not as the answer.

`cache_synced_at` is present on `StudyOverview`, `RunOverview`, `StatusBreakdown`,
`SampleProgress`, and `StudyManifest`. It is absent from bare list responses,
`Count` responses, and standalone `RunStatusTimeline`; use `mlwh_freshness` for the
as-of caveat there.

## MCP Tools To Add Or Update

### Aggregates And Status

- `mlwh_study_overview`: default for study availability, recency, "what is in this
  study?", and study metadata/data-access-group questions.
- `mlwh_study_status_breakdown`: default for study phase counts, manual-QC counts,
  and "how many are received/sequenced/not sequenced/passed QC?" questions.
- `mlwh_run_overview`: default for "what is on this run?" aggregate questions.
- `mlwh_run_status`: run lifecycle timeline. Add freshness caveat through
  `mlwh_freshness`, because the response itself has no `cache_synced_at`.
- `mlwh_sample_progress`: default for "what is happening with my sample?" It must
  pass through the always-present baseline and the optional milestone/run layers.

### Availability, Recency, IRODS, And Manifest Lists

- `mlwh_count_samples_with_data_for_study`: wraps `CountSamplesWithData`; accepts
  `since`/`until`.
- `mlwh_samples_with_data_for_study`: wraps `SamplesWithData`; paged; accepts
  `since`/`until`.
- `mlwh_samples_without_data_for_study`: wraps `SamplesWithoutData`; paged.
- Update `mlwh_irods_paths_for_study` and `mlwh_irods_paths_for_sample` to expose
  `file_type` and the additive `IRODSPath` fields.
- Add `mlwh_irods_paths_for_run` and `mlwh_count_irods_paths_for_run`; both accept
  `file_type`.
- Add or update count tools for sample/study iRODS so counts honor `file_type`.
- `mlwh_study_manifest`: wraps `StudyManifest`; paged envelope; accepts
  `with_irods` and `file_type`.
- `mlwh_count_study_manifest`: wraps `CountStudyManifest`.

### People And Study Lookup

- `mlwh_studies_for_faculty_sponsor` and
  `mlwh_count_studies_for_faculty_sponsor`: named PI/sponsor lookup over
  `study.faculty_sponsor`; rows have empty `role`.
- `mlwh_studies_for_user` and `mlwh_count_studies_for_user`: `study_users`
  membership lookup across `name`, `login`, and `email`, with optional `role`
  override.
- `mlwh_resolve_person` and `mlwh_count_resolve_person`: directory-style candidate
  resolution across sponsor names and `study_users` stored forms.
- Keep `mlwh_search_studies` useful for ambiguous study-name searches: descriptions
  should state it searches `name`, `study_title`, `programme`, and
  `faculty_sponsor`, and that rows expose enough fields to disambiguate.

## MCP Hardening

- Add a generic response-size guard in `internal/core`. Before returning a tool
  result, measure the serialized payload; if it exceeds a configurable budget, return
  a structured actionable error instead of the payload. The error should point the
  caller to the relevant overview/count/page workflow.
- The guard must cover all tools, including large MLWH tools and
  `mlwh_call_endpoint`. The mechanism is core-level and service-agnostic; the
  `mlwh-mcp-server` binary names and wires its own `MLWH_*` flag/env knob. For
  MLWH, expose `MLWH_MAX_TOOL_RESULT_BYTES` and
  `--mlwh-max-tool-result-bytes`; default the budget to 1 MiB. Over-budget tools
  must return `IsError=true` with structured, actionable error content.
- Change the MLWH paged fan-out default from fetch-all to a bounded page. Use
  default `limit=100` and maximum `limit=1000`, matching existing search-tool
  bounds. Include `total` and `next_offset` in paged results using upstream
  header-aware `wa` client results for the exact filtered request.
- Surface `lean` on `mlwh_study_detail` and `mlwh_run_detail` only.
- Do not implement aggregates by fetching lists and counting in MCP. Use upstream
  aggregate/count endpoints.

## Workflow Guidance

Update `mlwh://workflow` so agents choose the cheap tool first:

- Availability and recency: use `mlwh_study_overview` or the samples-with-data count
  tool; do not page iRODS or use study detail to answer counts.
- Recency: prefer `added_last_7_days`; otherwise pass explicit `since`/`until` and
  describe the result as data "added to iRODS".
- Data access group: use `mlwh_study_overview` or `mlwh_resolve_study`, not study
  detail.
- Study QC counts: use `mlwh_study_status_breakdown`.
- Sample/run progress: use `mlwh_sample_progress` / `mlwh_run_status`; compute open
  phase elapsed time from `reached_at`/`entered_at` on the agent side.
- Run/study/sample cram paths: use the appropriate iRODS tool with `file_type=cram`;
  count first if the result may be large.
- Study manifest: use `mlwh_study_manifest`; set `with_irods=true` and
  `file_type=cram` when a cram path column is requested.
- Person/study routing: sponsor/PI questions go to
  `mlwh_studies_for_faculty_sponsor`; "my studies", login, email, or membership
  questions go to `mlwh_studies_for_user`; ambiguous or partial names should go
  through `mlwh_resolve_person` before choosing a stored form.

## Hard Requirements

1. One-call aggregate answers for availability, recency, overview, progress, QC, and
   study metadata questions; no per-sample fan-out.
2. Bounded-by-default list tools with count counterparts and sizing hints. Paged
   list wrappers use semantic list fields with top-level `total` and `next_offset`,
   sourced from upstream header-aware `wa` client results.
3. Correct timestamp wording: "added to iRODS" means iRODS `created`, never
   `last_changed`.
4. Clear cache-freshness caveats, sourced from `cache_synced_at` when present and
   `mlwh_freshness` otherwise.
5. Thin pass-through behavior except for the generic size guard and MCP paging/error
   presentation.
6. Consistent actionable errors for upstream 400s, not found, ambiguity, unsupported
   identifiers, impaired upstream/cache, and never-synced cache states.
7. Platform-aware presentation. `platforms`, `platform`, `qc:"not_tracked"`, and
   upstream "not tracked" signals must pass through; never collapse them to a bare
   "no data". ONT and registered-only samples belong in the `registered` ladder
   bucket and are excluded from the QC split.
8. People routing must not conflate `faculty_sponsor` with `study_users` role
   membership. A zero result for one spelling/source is not proof of no studies.
9. Hermetic tests only: extend `internal/mlwh/harness_test.go` with stubbed MLWH
   responses. Assert tool registration, request path/query, returned shape,
   `cache_synced_at` presence/absence, error mapping, paging hints, and over-budget
   guard behavior.

## Repo Pointers

- Tool registration: `internal/mlwh/provider.go` and existing
  `register*Tools` helpers.
- Count-tool pattern: `internal/mlwh/tools_search.go` and
  `internal/mlwh/tools_detail.go`.
- Existing pagination/fan-out: `internal/mlwh/tools_detail.go`.
- Output schemas: `internal/mlwh/schema.go`, sourced from `wa.OpenAPIDocument()`.
- Generic fallback: `internal/mlwh/tools_call.go` (`mlwh_call_endpoint`).
- Workflow resource: `internal/mlwh/workflow.go`.
- Freshness/errors: `internal/mlwh/tools_freshness.go` and
  `internal/mlwh/errmap.go`.
- Harness: `internal/mlwh/harness_test.go`.
- Broader MCP conventions: `../mcp/spec.md`.

## Out Of Scope

- Further upstream `wa` API work. The required API 1.7.0 endpoints and
  header-aware remote-client surface are already implemented in `~/wa`; update the
  dependency and wrap that surface rather than rebuilding it downstream.
- HTTP transport or web UI work.
- Client-side caching, quotas, or non-MLWH provider behavior beyond the generic core
  size guard.

## Notes

Header-aware envelope tools should keep the existing envelope body shape and add
`total` and `next_offset` at the same top level. For example, `mlwh_study_manifest`
should return its normal manifest fields plus top-level paging metadata, not nest
the manifest under an upstream-style wrapper.

MCP should expose every upstream `/count` Registry method that is not already
surfaced as a tool, even when the matching list tool is narrow or not part of the
main curated workflow.

Exact sample-finder count endpoints should be surfaced through one unified
`mlwh_count_find_samples` tool using the same `field` enum pattern as
`mlwh_find_samples`, rather than through one dedicated tool per upstream finder.

Over-budget tool results should return `IsError=true` with a provider-neutral
structured error object containing at least `code`, `message`, `limit_bytes`,
`actual_bytes`, and `guidance`. MLWH may provide MLWH-specific guidance text, but
the core error shape should remain service-agnostic.

The response-size guard should measure the serialized MCP tool result that will be
returned to the client. If the implementation cannot access that final envelope, it
must conservatively measure the larger of the structured content JSON and any
generated text content.

All paginated typed MCP list tools should return top-level `total` and
`next_offset`, including existing tools such as `mlwh_search_samples`,
`mlwh_search_studies`, `mlwh_all_studies`, and existing fan-out/list tools. The
rule should not apply only to newly added tools.

`mlwh_study_detail` and `mlwh_run_detail` should expose upstream `limit`, `offset`,
and `lean` support while keeping their existing detail fields at the top level and
adding top-level `total` and `next_offset`, matching the flattened envelope rule
used for manifest.

`mlwh_call_endpoint` should use upstream `CallWithHeaders` and, when pagination
headers are present on the dynamic response, return a generic wrapper containing
`result`, `total`, and `next_offset`. Typed curated tools should keep their
semantic wrappers; the generic endpoint should not try to infer semantic field
names.
