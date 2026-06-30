# Feature: make sequencing-data availability, recency & sample progress easy via the MCP server

## Summary

Add MCP tooling so an agent can answer — **cheaply, in a single tool call** — the
most common class of question users ask this server:

- **"How many samples in study X have sequencing data available?"** (and "how many
  don't", "which are missing data", "how much is there").
- **"Is there any _new_ sequencing data available for study X this week?"** — data
  **added to iRODS** within a recent window.
- **"What's happening with my sample?"** — where it is in the sequencing pipeline
  now, and how long it has spent in each phase.
- "What's in study X?" / "What's on this run?"

Today this server (`mlwh-mcp-server`, specified in [`../mcp/spec.md`](../mcp/spec.md))
cannot answer these without abuse, because its giant pass-through endpoints overflow
the agent's token budget (see the motivating incident below). The **upstream `wa`
API has now landed** the aggregate/recency/overview/progress/manifest/people
endpoints that make clean answers possible (MLWH API **1.7.0**). This feature
surfaces those upstream capabilities as tools, and hardens the MCP layer so large
results stop being dead-ends.

**Scope rule for this spec: everything described here is in scope to build.** The
"Design decisions" section settles _how_ each item is implemented, never _whether_.
There are no optional items.

## Dependency: the upstream is already implemented — the `wa` CODE is the authority

The companion upstream feature has **already been implemented and merged** in the
`wa` repo (`~/wa`). This MCP feature is a set of **thin pass-throughs** over those
endpoints. Because the original upstream design drifted during implementation and
bugfixing, **the only authority for endpoint shapes, field names and semantics is
the `wa` Go code** — specifically `~/wa/mlwh/registry.go` (the per-endpoint
`Description`/`Summary`/`Query` entries that this server sources tool descriptions
from), `~/wa/mlwh/types.go` (the exact `json:` field tags), and the
availability/progress/manifest/people/remote handlers. The generated reference at
`~/wa/.docs/mcp/api-reference.md` and the OpenAPI document
(`wa.OpenAPIDocument()`) mirror that code. Do **not** trust `~/wa/.docs/realworld/`
spec/phase docs for exact names -- verify against the code.

Tool descriptions and output schemas here are sourced from the upstream
`Registry`/OpenAPI (see Background), so this spec must agree with the upstream code's
wording and semantics. The endpoint surface below was read from the code at API
1.7.0; re-verify before building.

### The landed upstream surface (verified against `~/wa` at API 1.7.0)

All new endpoints are `GET`. Aggregate/progress endpoints return a single small
object; array-list endpoints return a **bare JSON array** plus `X-Total-Count` /
`X-Next-Offset` headers (the typed Go client exposes page variants as `Page[T]` =
`{items, total, next_offset}`). `StudyManifest` is the exception: it is a paged
envelope object with `rows`, study metadata once, and `cache_synced_at`; its row
collection is sized by the same headers and by `/study/:id/manifest/count`.

| Endpoint                             | Registry method                  | Returns                         | Notes                                                        |
| ------------------------------------ | -------------------------------- | ------------------------------- | ------------------------------------------------------------ |
| `/study/:id/overview`                | `StudyOverview`                  | `StudyOverview`                 | cheap study aggregate + metadata + recency                   |
| `/study/:id/status-breakdown`        | `StatusBreakdown`                | `StatusBreakdown`               | phase ladder + per-platform + QC split                       |
| `/study/:id/samples-with-data`       | `SamplesWithData`                | `[]SampleWithData` (paged)      | accepts `since`,`until`                                      |
| `/study/:id/samples-with-data/count` | `CountSamplesWithData`           | `Count`                         | accepts `since`,`until`                                      |
| `/study/:id/samples-without-data`    | `SamplesWithoutData`             | `[]SampleWithData` (paged)      |                                                              |
| `/run/:id/overview`                  | `RunOverview`                    | `RunOverview`                   | cheap run aggregate                                          |
| `/run/:id/status`                    | `RunStatus`                      | `RunStatusTimeline`             | within-sequencing status timeline                            |
| `/sample/:id/progress`               | `SampleProgress`                 | `SampleProgress`                | baseline + milestones + per-run status                       |
| `/run/:id/irods`                     | `IRODSPathsForRun`               | `[]IRODSPath` (paged)           | accepts `file_type`; Illumina NPG run id                     |
| `/run/:id/irods/count`               | `CountIRODSPathsForRun`          | `Count`                         | accepts `file_type`                                          |
| `/study/:id/manifest`                | `StudyManifest`                  | `StudyManifest` (paged `rows`)  | accepts `with_irods`, `file_type`; carries `cache_synced_at` |
| `/study/:id/manifest/count`          | `CountStudyManifest`             | `Count`                         | product-grained count; ignores `with_irods`/`file_type`      |
| `/studies/faculty-sponsor/:name`     | `StudiesForFacultySponsor`       | `[]PersonStudy` (paged)         | named PI/sponsor, substring on `study.faculty_sponsor`       |
| `/studies/faculty-sponsor/:name/count` | `CountStudiesForFacultySponsor` | `Count`                         |                                                              |
| `/studies/user/:person`              | `StudiesForUser`                 | `[]PersonStudy` (paged)         | `study_users` membership; accepts role override              |
| `/studies/user/:person/count`        | `CountStudiesForUser`            | `Count`                         | accepts `role`                                               |
| `/resolve-person/:term`              | `ResolvePerson`                  | `[]PersonCandidate` (paged)     | candidates from `faculty_sponsor` and `study_users`          |
| `/resolve-person/:term/count`        | `CountResolvePerson`             | `Count`                         |                                                              |
| all other large lists' `/count` endpoints | `Count*`                    | `Count` (`{count:N}`)           | `/count` counterpart for sizing before transfer              |

**Changed existing endpoints:**

- `/study/:id/irods` and `/sample/:id/irods` rows (`IRODSPath`) now carry sample
  identity: **`id_sample_tmp`** (int64) and **`name`** (Sanger sample name; empty
  when the sample isn't in the sample mirror), plus **`id_run`** (0 when not
  derivable) and **`platform`**. This is what finally lets iRODS rows be aggregated
  to "distinct samples with data" and tied back to an Illumina run when possible.
- `/study/:id/irods`, `/sample/:id/irods`, and their `/count` endpoints now accept
  `file_type`, a case-insensitive filename-suffix filter with a leading dot stripped
  (`cram`, `.CRAM`, and `CRAM` are equivalent). Empty/whitespace values, `%`, `_`,
  or `/` are 400s; a valid but unmatched suffix returns an empty result/zero count.
- `/study/:id/overview` now carries cheap study metadata: `name`,
  `accession_number`, `faculty_sponsor`, and `data_access_group`.
- `/study/:id/status-breakdown` now carries the study QC split as `qc`.
- `/search/study/:term` still searches by substring, but its description now makes
  clear that it matches `name`, `study_title`, `programme`, and `faculty_sponsor`,
  and that rows carry enough fields to disambiguate duplicate names.
- `/study/:id/detail` and `/run/:id/detail` gained a **`lean`** boolean query param,
  and their nested sample collections are paginated (`limit`/`offset` + sizing
  headers). `/sample/:id/detail` did **not** gain `lean` in the landed code. The
  detail endpoints are still large; prefer the overview/progress/manifest tools.
- `/study/` (`AllStudies`) is unchanged and still large. There is **no** all-studies
  overview endpoint.

### Exact result shapes (the `json:` field names the tools surface)

- **`StudyOverview`**: `id_study_lims`, `name`, `accession_number`,
  `faculty_sponsor`, `data_access_group`, `samples_total`, `samples_with_data`,
  `samples_without_data`, `samples_sequenced_no_data`, `data_objects`, `runs`,
  `libraries`, `library_types` (`[]string`), `sequencing_date_range`
  (`{earliest, latest}`, omitempty), `newest_data_added` (latest iRODS `created`,
  RFC3339, empty if none), `added_last_7_days` (distinct samples with data added in
  `[now-7d, now)`), `cache_synced_at`.
- **`RunOverview`**: `id_run`, `samples`, `studies`, `data_objects`,
  `sequencing_date_range` (omitempty), `cache_synced_at`.
- **`StatusBreakdown`**: `id_study_lims`, `distinct`
  (`PhaseLadder` = `{with_data, sequenced_no_data, registered}`, partitions
  `samples_total`), `per_platform` (`[]{platform, ladder:PhaseLadder}`; a
  multi-platform sample is counted under every platform, so the grand total may
  exceed `samples_total`), `qc` (`{qc_pass, qc_fail, qc_pending}` for sequenced
  distinct samples), `with_detailed_timeline`, `cache_synced_at`.
- **`SampleProgress`**: `sample`, `platforms` (`[]string`), `baseline_phase`
  (`registered|sequenced|delivered`), `qc` (`pass|fail|pending|not_tracked`),
  `delivered_at` (earliest iRODS `created`, empty if none), `detailed_timeline`
  (bool), `timeline_reason` (omitempty; why it's false), `milestones` (omitempty;
  each `{name, reached_at, duration_to_next}`), `current_milestone` (omitempty),
  `runs` (omitempty; `[]RunStatusTimeline`), `cache_synced_at`.
- **`RunStatusTimeline`**: `id_run` (0 for non-Illumina), `platform`, `events`
  (`[]{phase, entered_at, duration}`; empty for ONT), `current` (derived: phase of
  the latest `entered_at`), `not_tracked` (omitempty; reserved, currently always
  empty — no supported platform sets it). **No `cache_synced_at`.**
- **`SampleWithData`** (the with/without-data row): `sample`, `platforms`
  (`[]string`; `[]` for registered-only, `["ONT"]` for ONT). **No
  `cache_synced_at`.**
- **`IRODSPath`**: `id_product`, `collection`, `data_object`, `irods_path`,
  `id_sample_tmp`, `name`, `id_run` (0 when not derivable), `platform`.
- **`StudyManifest`**: `id_study_lims`, `name`, `accession_number`,
  `faculty_sponsor`, `data_access_group`, `rows` (`[]ManifestRow`),
  `cache_synced_at`. `ManifestRow` is `name`, `supplier_name`, `accession_number`,
  `sanger_sample_id`, `id_run`, `lane`, `tag_index`, and optional `irods_path`
  (present only when `with_irods` is set).
- **`PersonStudy`**: `study` (`Study`), `role` (omitempty; empty for faculty
  sponsor, set for `study_users` membership).
- **`PersonCandidate`**: `source` (`faculty_sponsor|study_users`), `name`, `login`
  (omitempty), `email` (omitempty), `role` (omitempty), `study_count`.
- **`Count`**: `{count}`. **No `cache_synced_at`.**

## Why this is needed (the motivating incident — read this)

An agent asked "how many of study 7607's 428 samples have sequencing data?" had to
abandon the MCP server entirely:

- `mlwh_count_samples_for_study` → `428` (samples, not samples _with data_).
- `mlwh_irods_paths_for_study` → **735 rows / ~170 KB**, which **exceeded the MCP
  token limit** and was spilled to a file; rows carried **no sample identity** (now
  fixed upstream — see `id_sample_tmp`/`name`), so they couldn't be aggregated to
  "distinct samples with data" anyway.
- `mlwh_study_detail` → **~600 KB**, also over the limit, no per-sample iRODS/lane
  info.

The only thing that worked was bypassing MCP and hitting REST directly, once per
sample (428 calls → 428 tool calls through MCP, a non-starter). The recency
question ("new this week") was impossible. With the upstream now landed, the right
answer is **one `mlwh_study_overview` call** (`samples_with_data`,
`added_last_7_days`, `cache_synced_at`) with a tiny response. This is the single
most common question shape, so it must be one call with a small response.

## Three timestamps — the tools must not conflate them

The recency question depends on presenting the right time:

1. **When data was added to iRODS** — the iRODS-location creation time (source
   `seq_product_irods_locations.created`, now mirrored). This is the _only_ thing
   "new this week?" is about, and the basis of the `since`/`until` window. Surfaced
   as `newest_data_added`, `sequencing_date_range.{earliest,latest}`,
   `delivered_at`, and the run-status `entered_at`.
2. **`last_changed`** — the warehouse row's last-_changed_ time (what the cache
   syncs on). **Not surfaced as a field**; never present it as "new" — it conflates
   new with merely-modified data.
3. **`cache_synced_at` / cache freshness** — when `wa` last synced from MLWH
   (oldest `last_run` across the endpoint's feeding tables; the dedicated
   `mlwh_freshness` tool gives per-table `high_water`/`last_run`). Users don't care
   about it as the answer; it only **bounds completeness** ("new this week" can't
   include data added since the last sync). Present it as a **caveat**, clearly
   separate from the data-added time — never as the answer.

**Freshness is NOT on every response.** `cache_synced_at` is present on
`StudyOverview`, `RunOverview`, `StatusBreakdown`, `SampleProgress`, and
`StudyManifest`. It is **absent** from bare list responses (`SamplesWithData`,
`IRODSPath` arrays, people/study arrays), `Count` responses, and the standalone
`RunStatusTimeline` (`/run/:id/status`). For those, the freshness caveat must come
from `mlwh_freshness` (and for run status, note that `mlwh_sample_progress` embeds
the same run timelines _and_ carries `cache_synced_at`).

## Background: what exists today in THIS repo (this code is authoritative — read it)

- **How tools are built.** `internal/mlwh/provider.go` (`Register` →
  `registerSearchTools` / `registerResolveTools` / `registerDetailTools` /
  `registerFreshnessTool` / `registerCallTool` / `registerWorkflowResource`); each
  tool is `mcp.AddTool(r.Server(), &mcp.Tool{Name, Description, OutputSchema},
handler)` over the service-agnostic `internal/core` seam (`provider.go`
  `Registrar` ~42–43, `errs.go` `ToolError` ~34–45).
- **The count-tool template.** `internal/mlwh/tools_search.go` `mlwh_count_samples`
  (~221–240) and `internal/mlwh/tools_detail.go` `mlwh_count_samples_for_study`
  (~666–693): one-field input → `wa.Count` (`{count:N}`) → typed client call →
  `mapToolError`.
- **The large-payload tools + pagination contract.**
  `internal/mlwh/tools_detail.go`: `addIRODSPathsForStudy` (~591–615),
  `addStudyDetail` (~206–228), `addAllStudies` (~371), and `fanOutPagination` /
  `fetchAllLimit = 1_000_000` (~50–66) — fan-outs **default to fetch-all**, the
  direct cause of the overflow.
- **The iRODS result shape.** `internal/mlwh/schema.go` `irodsPathsResult`
  (~69–72) wraps `[]wa.IRODSPath`; field docs come from upstream OpenAPI via
  `outputSchemaFor("IRODSPath")` (~97–114) -- now including `id_sample_tmp`,
  `name`, `id_run`, and `platform`.
  The MCP **passes responses through verbatim** — no reshaping, truncation, or size
  checks anywhere.
- **Descriptions/schemas sourced from upstream.** `internal/mlwh/tools_resolve.go`
  (`resolveDescription`) and `internal/mlwh/schema.go` (output schemas from
  `wa.OpenAPIDocument()`): a new tool's description/output-schema come from the
  upstream `Registry` entry — so adopt the upstream wording verbatim.
- **The generic escape hatch.** `internal/mlwh/tools_call.go` `mlwh_call_endpoint`
  reaches any method by name and deliberately leaves `OutputSchema` nil (untyped
  passthrough, ~62) — also a budget risk.
- **The workflow resource.** `internal/mlwh/workflow.go` (`mlwh://workflow`,
  ~37–89) serves guidance + live `wa.EndpointReference()`; it already tells agents
  "to size a search before transferring rows use the count tools" (~52–53).
- **Freshness + errors.** `internal/mlwh/tools_freshness.go` (`mlwh_freshness`)
  and `internal/mlwh/errmap.go` (`mapToolError`, upstream sentinels → actionable
  hints).
- **The hermetic harness.** `internal/mlwh/harness_test.go`: stub MLWH
  `httptest.Server` (`newStubMLWH`, `respondJSON`, `respondError`, ~94–128) and
  `runMLWHServerWithClient` (~201–249), asserting the request path/query the stub
  received. Never a live warehouse.
- **Spec conventions.** `../mcp/spec.md` — the search/count stories and the
  fan-out + fetch-all (C2) stories any new tool must follow.

## What the feature must deliver

### Availability & recency tools (thin pass-throughs)

- **(A1) A study overview tool** (`mlwh_study_overview`) returning `StudyOverview`
  in one call: `samples_with_data` / `samples_without_data` /
  `samples_sequenced_no_data`, `data_objects` (the "how much"), the recency fields
  (`newest_data_added`, `added_last_7_days`), the cheap study metadata
  (`name`, `accession_number`, `faculty_sponsor`, `data_access_group`), and
  `cache_synced_at`. This is the default answer to "what's the sequencing-data
  situation for study X", "what data access group is study X in?", and to the
  zero-argument "anything new this week?" (`added_last_7_days`). There is **one**
  overview endpoint -- do not invent a separate "summary".
- **(A2) A count tool** for samples-with-data
  (`mlwh_count_samples_with_data_for_study` → `CountSamplesWithData`), built like the
  existing count tools; it also accepts the recency window (see A4).
- **(A3) Enumeration tools** — `mlwh_samples_with_data_for_study` /
  `..._without_data_for_study` (paginated fan-outs over `SamplesWithData`), so the
  agent can act on the samples still missing data; and surface the new
  sample/run/platform fields (`id_sample_tmp`, `name`, `id_run`, `platform`) on
  `mlwh_irods_paths_for_*` (additive output-schema change).
- **(A4) Recency window** — pass an explicit `since` (and optional `until`,
  RFC3339) through to `mlwh_count_samples_with_data_for_study` and
  `mlwh_samples_with_data_for_study`. The window is **half-open `[since, until)`
  over iRODS `created`** ("added to iRODS"). The **agent supplies the date** (it
  knows "today"; note workflow scripts cannot call `Date.now()`, so the date is
  computed by the calling agent, not in a workflow). Upstream rejects `until`
  without `since` and malformed timestamps with 400 — map those to actionable
  errors. For the common "this week" case, prefer `mlwh_study_overview`'s
  `added_last_7_days` (no date arithmetic needed). A convenience that accepts a
  window and resolves it to explicit `since`/`until` before calling upstream is in
  scope.
- **(A5) A run overview tool** (`mlwh_run_overview` → `RunOverview`).

### Budget-safety hardening

- **(G) A response-size guard in the MCP layer.** Before returning, a tool measures
  its serialised result; if it would exceed a configurable budget, it returns a
  **structured, actionable error** instead of the oversized payload — e.g. "this
  result is ~X KB (~Y rows); call `mlwh_<count tool>` to size it then page with
  `limit`/`offset`, or use `mlwh_study_overview`." This covers
  `mlwh_irods_paths_for_study`, `mlwh_irods_paths_for_sample`,
  `mlwh_irods_paths_for_run`, `mlwh_study_manifest`, the people/study list tools,
  `mlwh_study_detail`, `mlwh_run_detail`, `mlwh_all_studies`, and the untyped
  `mlwh_call_endpoint`. Because it measures
  serialised bytes generically, it lives in the **shared `internal/core`** result
  path (alongside `errs.go`/the `Registrar` seam) so every current and future
  provider inherits it and the core stays service-agnostic. The threshold is a
  **core-level Option** (not MLWH-specific), set by each per-service binary from its
  own flag/env — for `mlwh-mcp-server`, an `MLWH_*` var: the core defines the
  mechanism, the binary names the knob.
- **(N) Count tools for every upstream `/count` counterpart** —
  `mlwh_count_samples_with_data_for_study`, `mlwh_count_irods_paths_for_study`,
  `mlwh_count_irods_paths_for_sample`, `mlwh_count_irods_paths_for_run`,
  `mlwh_count_study_manifest`, `mlwh_count_studies_for_faculty_sponsor`,
  `mlwh_count_studies_for_user`, `mlwh_count_resolve_person`,
  `mlwh_count_runs_for_study`, `mlwh_count_libraries_for_study`,
  `mlwh_count_lanes_for_sample`, `mlwh_count_samples_for_run` (and the existing
  `mlwh_count_samples_for_study`), next to the existing count tools. (Each maps to
  a real `/count` endpoint listed above.)
- **(P) Bounded-by-default paging with sizing hints.** Change `fanOutPagination`'s
  fetch-all default to a bounded page (the agent opts into more via an explicit
  `limit`), and include a hint in every paged result — "returned N of M; pass
  `offset=N` for the next page" — using the upstream sizing metadata
  (`X-Total-Count` / `X-Next-Offset`, exposed by the typed client as `Page[T]`
  `{items, total, next_offset}`). Reconcile with the C2 fetch-all stories in
  [`../mcp/spec.md`](../mcp/spec.md).
- **(L) Lean detail.** Surface the upstream **`lean`** query param on
  `mlwh_study_detail` and `mlwh_run_detail` only (there is no landed sample-detail
  `lean` param), and steer "tell me about study X / this run" to the overview tools
  (A1/A5), never the giant full-detail payloads.

### Sample progress / pipeline status tools

The upstream `SampleProgress` always returns a **baseline** (`baseline_phase`:
`registered → sequenced → delivered`, plus a rolled-up `qc`, so it works for every
sample on every platform) and layers on, when available, **(a)** the detailed
milestone timeline (`milestones` from `seq_ops_tracking_per_sample`, gated by
`detailed_timeline` with a `timeline_reason` when false) and **(b)** the
within-sequencing `runs` status timelines from the sample's platform tables. So
there is **no per-sample cliff** — absent layers are _less detail_, never an error,
and every result carries `platforms` and `qc` (`"not_tracked"` where a platform has
no QC, e.g. ONT).

- **(Q1) `mlwh_sample_progress`** (→ `SampleProgress`) — pass a Sanger sample name
  and return the result as-is: the always-present baseline (`baseline_phase`, `qc`,
  `delivered_at`, `platforms`), plus — when present — `milestones` (each with
  `reached_at` and `duration_to_next`; the **current phase** via
  `current_milestone`) and the per-run `runs` timelines. When a layer is absent,
  present what's there and say _why_ (`detailed_timeline: false` + `timeline_reason`)
  — never imply no progress. Carries `cache_synced_at`.
- **(Q2) `mlwh_run_status`** (→ `RunStatusTimeline`) — a run's status timeline
  (`events` each `{phase, entered_at, duration}`, plus the **derived** `current`
  phase); the building block Q1 composes per run, also usable directly. **This
  response has no `cache_synced_at`** — the tool/workflow must attach
  `mlwh_freshness` for the as-of-sync caveat.
- **(Q3) `mlwh_study_status_breakdown`** (→ `StatusBreakdown`) — counts of **all**
  the study's samples by phase: the `distinct` ladder
  (`with_data`/`sequenced_no_data`/`registered`, partitioning `samples_total`), the
  `per_platform` ladders, `qc` (`qc_pass`/`qc_fail`/`qc_pending` over sequenced
  distinct samples), and `with_detailed_timeline`, in one small call (never N
  per-sample lookups). Carries `cache_synced_at`. Nothing silently dropped (ONT and
  registered-only samples land in `registered` and are excluded from the QC split).
- **(Q4) Honest progress presentation.** Present the **current phase**
  (`current_milestone` / run `current`); for an open phase, "time in phase so far"
  computed by the **agent** (which knows "now") from the upstream
  `reached_at`/`entered_at`; show completed-phase durations from upstream
  (`duration_to_next` / `duration`); attach the freshness caveat (current phase is
  **as-of last sync** — from `cache_synced_at` where present, else `mlwh_freshness`).
  Frame layer differences as detail level, never pass/fail; render recurrences /
  on-hold / cancelled faithfully; never fabricate phases the data doesn't record.

### Guidance

- **(W) Update the `mlwh://workflow` resource** with explicit workflows: (i)
  **availability** — use `mlwh_study_overview` / the count tool, never
  `mlwh_irods_paths_for_study` or `mlwh_study_detail`; (ii) **recency** — prefer
  `added_last_7_days` from the overview, or supply an explicit `since`, read the
  result as "added to iRODS", and **caveat with `cache_synced_at` / `mlwh_freshness`**
  (the answer is complete only up to the last sync); (iii) the general
  **count/summarise → decide → page** recipe naming each large list's count tool;
  (iv) **progress** — route "what's happening with my sample / study" to the
  progress/breakdown tools, read the current phase as **as-of last sync**, let the
  agent compute elapsed time in the open phase from `reached_at`/`entered_at`, and
  present the baseline-vs-detailed difference as detail level (never "broken for
  this sample"); (v) **realworld2 workflows** -- route data-access-group questions
  to `mlwh_study_overview`, run cram-path questions to `mlwh_irods_paths_for_run`
  with `file_type`, tabular sample/run/accession questions to `mlwh_study_manifest`,
  QC count questions to `mlwh_study_status_breakdown`, and person/study questions
  to the faculty-sponsor, user-membership, or resolve-person tools as appropriate.
  Make the new tools' descriptions unambiguously the right pick for
  these questions, including the definition of "available", the recency semantics,
  and the study-scoping caveat.

## HARD REQUIREMENTS

1. **One call, small response** for every count/summary/overview/recency question;
   payload size independent of study/run size. No agent should page iRODS or fan
   out per sample to answer availability or recency.
2. **Correct recency presentation.** Present windowed results as "added to iRODS"
   (iRODS `created`), never `last_changed`; always attach the cache-freshness caveat
   (`cache_synced_at` where the response carries it, otherwise `mlwh_freshness`),
   clearly distinct from the data-added time. Note the freshness field is present
   only on `StudyOverview`/`RunOverview`/`StatusBreakdown`/`SampleProgress`/
   `StudyManifest` among the new response types.
3. **Thin pass-throughs (except G).** Counts/summaries come from upstream
   aggregates, not MCP-side counting of fetched lists; follow the existing
   count-tool pattern, derive descriptions/output-schemas from the upstream
   `Registry`/OpenAPI, reuse `mapToolError` and the input-guard conventions.
4. **The size guard is generic and in `internal/core`**, leaving the core
   service-agnostic; it applies to every tool including `mlwh_call_endpoint`.
5. **Consistent errors.** Never-synced / empty / unknown-study, and the recency
   400s (`until` without `since`, malformed timestamp), map to the same actionable
   hints the existing study tools produce.
6. **Hermetic tests.** Extend `harness_test.go`: stub the new endpoints with
   `respondJSON`, call each tool through the MCP client, assert request path/query
   (including the recency `since`/`until` params, the study/run detail `lean` params,
   and which responses carry `cache_synced_at`) and returned shape; cover error paths via
   `respondError`. For the size guard, stub an over-large upstream response and
   assert the structured guard error (not a raw dump) and that an under-budget
   response is unaffected.
7. **Keep the surface coherent.** Update `mlwh://workflow` and the relevant
   `../mcp/spec.md` stories; keep version-surfacing and existing behaviour intact
   except the deliberate (P) default change, which must be documented.
8. **Every sample resolves; tiers are detail-level, not pass/fail.**
   `mlwh_sample_progress` always presents the baseline; `milestones` and `runs` are
   shown when available and their absence is framed as _less detail_ (with
   `timeline_reason`), never an error or "no progress". Current phase is **as-of
   last sync**; open-phase elapsed time is computed by the agent from
   `reached_at`/`entered_at`, not fabricated. The study breakdown (Q3) is one
   aggregate call counting **all** samples, nothing hidden.
9. **Platform-aware; never a false "no data".** Upstream covers all sequencing
   platforms uniformly — canonical names `Illumina`, `PacBio`, `Elembio`,
   `Ultimagen` (via their own product-metrics) and `ONT` (via `oseq_flowcell`,
   identity/study only: no product-metrics/iRODS/QC/run-status). Results carry
   `platforms` (`["ONT"]` for ONT, `[]` for registered-only) and `qc:"not_tracked"`
   where a platform lacks QC; ONT and registered-only samples are counted in the
   `registered` ladder bucket, never dropped. Surface `platforms`/`qc` in tool
   output and pass any upstream "not tracked" signal through **verbatim** — never
   collapse it to a bare "no data". (The `RunStatusTimeline.not_tracked` field is
   reserved but currently always empty.)

## Design decisions for the spec to settle (HOW, not WHETHER)

Each item below **will be built**; settle only the implementation:

- **Tool names/shapes**, tracking the landed upstream endpoints (`mlwh_study_overview`,
  `mlwh_run_overview`, `mlwh_run_status`, `mlwh_sample_progress`,
  `mlwh_study_status_breakdown`, `mlwh_samples_with[out]_data_for_study`,
  `mlwh_irods_paths_for_run`, `mlwh_study_manifest`,
  `mlwh_studies_for_faculty_sponsor`, `mlwh_studies_for_user`,
  `mlwh_resolve_person`, and the `mlwh_count_*` counterparts) -- consistent with
  the existing `mlwh_*_for_study` / `mlwh_count_*` conventions and with the
  upstream registry method names.
- **The size-guard threshold default and config name**, the structured error's
  shape/wording, and how it estimates size (serialised bytes vs a token estimate).
- **The bounded-default page size for (P)** and the exact sizing-hint wording,
  reconciled with the C2 fetch-all stories.
- **How "added to iRODS" is worded** in tool descriptions, the exact `since`/`until`
  parameter surfacing, and how the freshness caveat is presented — including the
  fact that `cache_synced_at` is on `StudyOverview`, `RunOverview`,
  `StatusBreakdown`, `SampleProgress`, and `StudyManifest`, so the caveat for bare
  lists/counts/`mlwh_run_status` is sourced from `mlwh_freshness`.
- **Progress tool shapes** — how the open phase's elapsed time, the freshness
  caveat, and the three-layer (baseline / `milestones` / `runs`) presentation (incl.
  `detailed_timeline: false` + `timeline_reason`) are surfaced — matching the
  upstream `SampleProgress`/`RunStatusTimeline`.
- **Lean surfacing** — how the upstream `lean` param is exposed on study/run detail
  tools only.

## Second wave (realworld2): more question shapes to make easy

A further batch of real user questions must also be one cheap call. These build on a
**second upstream feature** that has now landed in the `wa` repo
([`~/wa/.docs/realworld2/prompt.md`](../../../wa/.docs/realworld2/prompt.md) is only
background; the contract is the `wa` code at API 1.7.0). Same rules as above: thin
pass-throughs, one small/bounded response, the freshness/timestamp discipline, the
size guard (G), bounded paging (P), `/count` tools (N), descriptions sourced from
the upstream Registry/OpenAPI. Source-schema facts below were verified against the
live MLWH source, but exact names/fields come from current `wa` code.

| Question (study/run/person)                                            | Upstream `wa` contract now landed                                      | MCP tool / behaviour to add                                                                     |
| --------------------------------------------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| Q1 "data access groups for study X"                                   | `StudyOverview` carries `data_access_group` plus study metadata          | (A10) surface it via `mlwh_study_overview`; never route to `mlwh_study_detail`                   |
| Q2 "iRODS path for cram files from run X"                             | `IRODSPathsForRun` + `file_type` on run/study/sample iRODS endpoints     | (A6) `mlwh_irods_paths_for_run` + `file_type` filter on iRODS tools                              |
| Q3 "list study details, run_id, sample name, supplier_name, accession" | `StudyManifest` envelope with paged `rows` and count                     | (A7) `mlwh_study_manifest` (paginated tabular + count)                                           |
| Q4 Q3 + iRODS path to the cram files                                  | `StudyManifest` accepts `with_irods=true&file_type=cram`                 | (A7) `mlwh_study_manifest` with iRODS path + `file_type`                                         |
| Q5 "study ID for `<name>`" (ambiguous)                               | `SearchStudies` rows expose `id_study_lims`/`name`/`faculty_sponsor`     | workflow note on multiple matches                                                               |
| Q6 "not-sequenced / sequenced / passed manual QC counts"             | `StatusBreakdown.qc` gives `qc_pass`/`qc_fail`/`qc_pending`              | (A8) study **QC counts** via `mlwh_study_status_breakdown`                                       |
| Q7 "studies for `<person>`" / "my studies"                          | `StudiesForFacultySponsor`, `StudiesForUser`, `ResolvePerson`            | (A9/A9b) sponsor, role-membership, and person-resolution tools with routing                      |

### New MCP deliverables

- **(A6) iRODS by file type + run-scoped iRODS.** A `mlwh_irods_paths_for_run`
  (+ `mlwh_count_irods_paths_for_run`) wrapping the new `/run/:id/irods`, and a
  `file_type` param (e.g. `cram`) on `mlwh_irods_paths_for_{study,sample,run}` and
  their count tools. Upstream `file_type` is a filename-suffix match with the
  leading dot stripped; invalid values are 400s, while a valid but unmatched suffix
  is an empty result/zero count. Run-scope is an Illumina product-to-run join; pass
  through `id_run` and `platform` from `IRODSPath`.
- **(A7) Study manifest.** `mlwh_study_manifest` wrapping the new paginated study
  manifest envelope: study metadata (`id_study_lims`, `name`, `accession_number`,
  `faculty_sponsor`, `data_access_group`) returned once, plus paged product rows
  (`name`, `supplier_name`, `accession_number`, `sanger_sample_id`, `id_run`, `lane`,
  `tag_index`, optional `irods_path`). Set `with_irods=true` to include `irods_path`;
  set `file_type=cram` to narrow that joined path. `with_irods` without `file_type`
  does **not** default to cram; it returns any one object for the product. The count
  counterpart is `mlwh_count_study_manifest` / `CountStudyManifest`, product-grained
  and unaffected by `with_irods`/`file_type`. This is the most budget-sensitive new
  tool: bounded-by-default, paged with sizing hints, and **fully under the size guard
  (G)**.
- **(A8) Study QC counts.** Surface the new study-level QC dimension —
  **received** (`samples_total`), **sequenced** (has product-metrics),
  **not-sequenced** (registered), and **qc_pass / qc_fail / qc_pending** — on the
  `mlwh_study_status_breakdown` tool, in one cheap call (no per-sample fan-out),
  consistent with `mlwh_sample_progress`'s `qc`. The landed upstream does **not** put
  these QC fields on `StudyOverview`.
- **(A9) People -> studies, with routing.** Add `mlwh_studies_for_faculty_sponsor`
  (+ `mlwh_count_studies_for_faculty_sponsor`) for the named **faculty sponsor** and
  `mlwh_studies_for_user` (+ `mlwh_count_studies_for_user`) for `study_users` role
  membership. `StudiesForUser` matches the person term case-insensitively across
  `name`, `login`, and `email`; its default role set is `owner`, `manager`, and
  `data_access_contact`. A supplied `role` is a comma-separated **override** of that
  default, so `role=follower` deliberately widens to followers only. Rows are
  `PersonStudy` (`study`, optional `role`).
- **(A9b) Person resolution (name -> stored identifier).** A `mlwh_resolve_person`
  tool wrapping the upstream `/resolve-person/:term` endpoint, plus
  `mlwh_count_resolve_person`: given a partial term it returns distinct
  `PersonCandidate` rows from both sources (`source`, `name`, optional `login` /
  `email` / `role`, `study_count`) so the agent can translate a spoken/partial name
  into the exact stored value and disambiguate, instead of guessing a spelling and
  dead-ending.
- **(A10) Cheap study metadata.** Ensure `data_access_group`, `faculty_sponsor`,
  `name`, `accession_number` come from a cheap study tool (`mlwh_study_overview` /
  `mlwh_resolve_study`), never the giant `mlwh_study_detail` (Q1).

### Routing & semantics the descriptions + `mlwh://workflow` must encode

- **`faculty_sponsor` (named PI/sponsor) ≠ `study_users` (role membership).** "Studies
  for `<person>`" → faculty_sponsor (e.g. ~91 studies for "Carl Anderson"); "my
  studies"/a login → `study_users`, role-filtered by default to `owner`, `manager`,
  and `data_access_contact`; `follower` is noisy and must be requested explicitly.
  They return different sets — never conflate; route by name-vs-login.
- **Translate the user's name to what's stored; never dead-end on a narrow search.**
  People are stored as free-text full names (`faculty_sponsor`) and as
  `name`/`login`/`email` (`study_users`) — a user won't type these exactly. The
  workflow guidance must tell the agent to: match across name/login/email; if a
  partial/first-name/initials query returns nothing or is ambiguous, **resolve via
  `mlwh_resolve_person` and pick/confirm a candidate** rather than reporting "no
  results"; and for **"my studies"**, prefer the user's **email/login** — which the
  host/session usually knows (the MCP session carries the user's email) — over
  guessing their name spelling. A zero result from one spelling is not evidence of
  zero studies.
- **"cram" is a file-type (filename-suffix) filter** (no file-type column upstream);
  run-scoped iRODS exists upstream via the product→run join.
- **QC:** "sequenced" = has product-metrics; "passed manual QC" = `qc` pass;
  "not got sequence data" = not sequenced (registered). One call, no per-sample
  fan-out.
- The manifest and run-iRODS **lists are large** → count/summarise → page; the size
  guard (G) applies. `StudyManifest` carries `cache_synced_at`; the run-iRODS and
  people list responses do not, so their freshness caveat must come from
  `mlwh_freshness`.

These reuse the same MCP-layer machinery specified above (size guard G, bounded
paging P, count tools N, `mapToolError`, upstream-sourced descriptions, the hermetic
harness). The realworld2 endpoints have landed; keep re-verifying exact paths/field
names against the `wa` code before building.

## Out of scope

- The upstream API work itself — the first and second waves are **already implemented**
  in `~/wa` (API 1.7.0). This spec wraps those endpoints and treats the `wa` code as
  the contract.
- Any `internal/core` change beyond the generic size guard (G) and what
  registering new MLWH tools requires.
- HTTP transport / web UI work (separate features); client-side caching or quotas.

## Pointers / prior art (in order of authority)

1. **The upstream `wa` CODE** (the contract these tools wrap; descriptions/schemas
   are sourced from it): `~/wa/mlwh/registry.go` (per-endpoint `Description` /
   `Summary` / `Query`), `~/wa/mlwh/types.go` (exact `json:` field tags),
   `~/wa/mlwh/availability.go` + `~/wa/mlwh/progress.go` + `~/wa/mlwh/manifest.go`
   + `~/wa/mlwh/people.go` (semantics), `~/wa/mlwh/remote.go` (the typed client +
   `Page[T]` where available), and
   `~/wa/.docs/mcp/api-reference.md` / `wa.OpenAPIDocument()` (generated mirrors).
   MLWH API **1.7.0**.
2. **This repo's code**: `internal/mlwh/tools_detail.go`
   (`mlwh_count_samples_for_study`, `addIRODSPathsForStudy`, `addStudyDetail`,
   `addAllStudies`, `fanOutPagination`/`fetchAllLimit`),
   `internal/mlwh/tools_search.go` (count-tool template),
   `internal/mlwh/schema.go` (`irodsPathsResult`, `outputSchemaFor`),
   `internal/core/errs.go` + `internal/core/provider.go` (the result/Registrar seam
   for the size guard), `internal/mlwh/workflow.go` (the resource to extend),
   `internal/mlwh/tools_freshness.go` + `internal/mlwh/errmap.go`,
   `internal/mlwh/tools_call.go` (the untyped fallback the guard must also cover),
   `internal/mlwh/harness_test.go` (the hermetic harness to extend), and
   `../mcp/spec.md`.
