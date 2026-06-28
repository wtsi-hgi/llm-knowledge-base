# Feature: make sequencing-data availability, recency & sample progress easy via the MCP server

## Summary

Add MCP tooling so an agent can answer — **cheaply, in a single tool call** — the
most common class of question users ask this server:

- **"How many samples in study X have sequencing data available?"** (and "how many
  don't", "which are missing data", "how much is there").
- **"Is there any *new* sequencing data available for study X this week?"** — data
  **added to iRODS** within a recent window.
- **"What's happening with my sample?"** — where it is in the sequencing pipeline
  now, and how long it has spent in each phase.
- "What's in study X?" / "What's on this run?"

Today this server (`mlwh-mcp-server`, specified in [`../mcp/spec.md`](../mcp/spec.md))
cannot answer these without abuse, because the upstream `wa mlwh serve` API exposes
no such aggregates and its giant pass-through endpoints overflow the agent's token
budget. This feature surfaces the new upstream availability/recency/overview
capabilities as tools, and hardens the MCP layer so large results stop being
dead-ends.

**Scope rule for this spec: everything described here is in scope to build.** The
"Design decisions" section settles *how* each item is implemented, never *whether*.
There are no optional items.

## Dependency (read first)

This builds on a companion feature in the `wa` repo, [`wa-prompt.md`](./wa-prompt.md),
that adds the underlying endpoints (study sequencing-availability summary/overview,
samples-with/without-data counts and lists, sample identity on iRODS rows, a
mirrored iRODS-creation timestamp with date-windowed "added since" queries, a run
overview, `/count` counterparts for every list, list sizing metadata, lean detail,
and **sample pipeline-progress endpoints** backed by an always-available baseline
(per-platform product-metrics + iRODS), a detailed milestone timeline from the
newly-mirrored `seq_ops_tracking_per_sample`, and a within-sequencing status timeline
per platform). All of it is **platform-aware** — the upstream iRODS↔sample linkage
spans every sequencing platform (Illumina, PacBio, ONT, Elembio, Ultimagen), so the
tools work for non-Illumina samples and carry a `platform` field. **The tools here are
thin pass-throughs over those endpoints** and assume
they exist; the MCP size guard (deliverable G) is the one genuinely new MCP-layer
behaviour. Tool descriptions and output schemas are sourced from the upstream
`Registry`/OpenAPI (see Background), so the two specs must agree on wording and
semantics. Adapt to whatever exact endpoint shapes the `wa` spec finalises.

**Upstream → tool traceability** (each `wa-prompt.md` deliverable and how it surfaces
here; if `wa` drops one, the matching tool drops too):

| wa-prompt deliverable | MCP tool / behaviour |
|---|---|
| (S) summary, (O1) study overview | `mlwh_study_sequencing_summary` / `mlwh_study_overview` |
| (C) samples-with-data count | `mlwh_count_samples_with_data_for_study` |
| (E) with/without-data lists + iRODS sample identity | `mlwh_samples_with[out]_data_for_study`; sample fields on `mlwh_irods_paths_*` |
| (R)+(T) iRODS `created` + date window | `since`/`until` recency params on the availability tools |
| (O2) run overview | `mlwh_run_overview` |
| (N) `/count` counterparts | `mlwh_count_*` tools |
| (M) list sizing metadata | bounded paging + hints (deliverable P) |
| (L) lean detail | lean/overview tools (deliverable L) |
| (F) per-response freshness field | surfaced on every result |
| (P0) baseline + (P1–P2) milestones + (P5) per-platform status | `mlwh_sample_progress`, `mlwh_run_status` |
| (P3) study status breakdown | `mlwh_study_status_breakdown` |
| Platform coverage + HARD REQUIREMENT 11 | `platform` field + "not tracked" passthrough |
| *(no upstream — MCP-only)* | (G) response-size guard |

## Why this is needed (the motivating incident — read this)

An agent asked "how many of study 7607's 428 samples have sequencing data?" had to
abandon the MCP server entirely:

- `mlwh_count_samples_for_study` → `428` (samples, not samples *with data*).
- `mlwh_irods_paths_for_study` → **735 rows / ~170 KB**, which **exceeded the MCP
  token limit** and was spilled to a file; rows carry **no sample identity**, so
  they can't be aggregated to "distinct samples with data" anyway.
- `mlwh_study_detail` → **~600 KB**, also over the limit, no per-sample iRODS/lane
  info.

The only thing that worked was bypassing MCP and hitting REST directly, once per
sample (428 calls → 428 tool calls through MCP, a non-starter). And the recency
question ("new this week") is impossible today. This is the single most common
question shape, so it must be one call with a small response.

## Three timestamps — the tools must not conflate them

The recency question depends on presenting the right time (see
[`wa-prompt.md`](./wa-prompt.md) for the upstream detail):

1. **When data was added to iRODS** — the iRODS-location creation time (source
   column `seq_product_irods_locations.created`). The *only* thing "new this week?"
   is about; the upstream adds this signal (it is not mirrored today).
2. **`last_updated`** — the warehouse row's last-*changed* time (source
   `seq_product_irods_locations.last_changed`; what the cache syncs on). A proxy
   that conflates new with merely-modified data → never present it as "new".
3. **`last_run` / cache freshness** — when `wa` last synced from MLWH
   (`mlwh_freshness`). Users don't care about it as the answer; it only **bounds
   completeness** ("new this week" can't include data added since the last sync).
   The tools must surface it as a **caveat**, clearly separate from the
   data-added time — never as the answer.

## Background: what exists today (this code is authoritative — read it)

- **How tools are built.** `internal/mlwh/provider.go` (`Register` →
  `registerSearchTools` / `registerResolveTools` / `registerDetailTools` /
  `registerFreshnessTool` / `registerCallTool` / `registerWorkflowResource`); each
  tool is `mcp.AddTool(r.Server(), &mcp.Tool{Name, Description, OutputSchema},
  handler)` over the service-agnostic `internal/core` seam
  (`provider.go` `Registrar` ~42–43, `errs.go` `ToolError` ~34–45).
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
  `outputSchemaFor("IRODSPath")` (~97–114). The MCP **passes responses through
  verbatim** — no reshaping, truncation, or size checks anywhere.
- **Descriptions/schemas sourced from upstream.** `internal/mlwh/tools_resolve.go`
  (`resolveDescription`) and `internal/mlwh/schema.go` (output schemas from
  `wa.OpenAPIDocument()`): a new tool's description/output-schema come from the
  upstream `Registry` entry.
- **The generic escape hatch.** `internal/mlwh/tools_call.go` `mlwh_call_endpoint`
  reaches any method by name and deliberately leaves `OutputSchema` nil
  (untyped passthrough, ~62) — also a budget risk.
- **The workflow resource.** `internal/mlwh/workflow.go` (`mlwh://workflow`,
  ~37–89) serves guidance + live `wa.EndpointReference()`; it already tells agents
  "to size a search before transferring rows use the count tools"
  (~52–53).
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

- **(A1) A study sequencing summary/overview tool** (e.g.
  `mlwh_study_sequencing_summary` / `mlwh_study_overview`) returning the small
  upstream summary in one call: samples-with/without-data, a "how much" figure,
  the recency fields (newest data added, count added in the last N days), and the
  cache-freshness caveat field.
- **(A2) A count tool** for samples-with-data
  (`mlwh_count_samples_with_data_for_study`), built like the existing count tools.
- **(A3) Enumeration tools** — `mlwh_samples_with_data_for_study` /
  `..._without_data` (paginated fan-outs), so the agent can act on the samples
  still missing data; and surface the new sample-identity fields on
  `mlwh_irods_paths_for_*` (additive output-schema change).
- **(A4) Recency tools** — pass an explicit `since` (and optional `until`) through
  to the upstream date-windowed count/list of data **added to iRODS** in the
  window. The **agent supplies the date** (it knows "today"; note workflow scripts
  cannot call `Date.now()`, so the date is computed by the calling agent, not in a
  workflow). Provide a convenience that accepts a window but resolves to explicit
  dates before calling upstream.
- **(A5) A run overview tool** mirroring the upstream run overview.

### Budget-safety hardening

- **(G) A response-size guard in the MCP layer.** Before returning, a tool
  measures its serialised result; if it would exceed a configurable budget, it
  returns a **structured, actionable error** instead of the oversized payload —
  e.g. "this result is ~X KB (~Y rows); call `mlwh_<count tool>` to size it then
  page with `limit`/`offset`, or use `mlwh_study_sequencing_summary`." This covers
  `mlwh_irods_paths_for_study`, `mlwh_study_detail`, `mlwh_run_detail`,
  `mlwh_all_studies`, and the untyped `mlwh_call_endpoint`. Because it measures
  serialised bytes generically, it lives in the **shared `internal/core`** result
  path (alongside `errs.go`/the `Registrar` seam) so every current and future
  provider inherits it and the core stays service-agnostic. The threshold is a
  **core-level Option** (not MLWH-specific), set by each per-service binary from its
  own flag/env — for `mlwh-mcp-server`, an `MLWH_*` var: the core defines the
  mechanism, the binary names the knob.
- **(N) Count tools for every upstream `/count` counterpart**
  (`mlwh_count_irods_paths_for_study`, `..._for_sample`, `mlwh_count_runs_for_study`,
  `mlwh_count_libraries_for_study`, `mlwh_count_lanes_for_sample`,
  `mlwh_count_samples_for_run`, …), next to the existing count tools.
- **(P) Bounded-by-default paging with sizing hints.** Change `fanOutPagination`'s
  fetch-all default to a bounded page (the agent opts into more via an explicit
  `limit`), and include a hint in every paged result — "returned N of M; pass
  `offset=N` for the next page" — using the upstream sizing metadata. Reconcile
  with the C2 fetch-all stories in [`../mcp/spec.md`](../mcp/spec.md).
- **(L) Lean tools.** Surface the upstream lean/projected detail; steer "tell me
  about study X / this run" to the overview tools (A1/A5), never the giant
  `mlwh_study_detail` / `mlwh_run_detail`.

### Sample progress / pipeline status tools

The upstream progress endpoint always returns a **baseline** (registered →
sequenced[+QC] → delivered[+date], so it works for every sample on every platform)
and layers on, when available, **(a)** the detailed milestone timeline (submission →
labware → order → library → sequencing → qc-complete, from
`seq_ops_tracking_per_sample`, for in-window samples) and **(b)** the within-
sequencing **status** timeline from the sample's platform tables (Illumina
`iseq_run_status`; PacBio/Elembio/Ultimagen their own run/well metrics; ONT has none →
"not tracked for ONT"). So there is **no per-sample cliff** — absent layers are *less
detail*, never an error, and every result carries its `platform`.

- **(Q1) `mlwh_sample_progress`** — pass a Sanger sample name to the upstream
  endpoint and return its result as-is: the always-present baseline, plus — when
  present — the milestone timeline (each milestone with its `reached_at` and duration
  to the next; the **current phase** after the latest reached milestone) and the
  per-run run-status timeline. When a layer is absent, present what's there and say
  *why* (`detailed_timeline: false` / sample outside the tracking window) — never
  imply no progress.
- **(Q2) `mlwh_run_status`** — a run's status timeline (each phase with `entered_at`,
  duration, and the **derived** current phase); the building block Q1 composes per
  run, also usable directly.
- **(Q3) `mlwh_study_status_breakdown`** — counts of **all** the study's samples by
  baseline phase, plus the count that also have a detailed timeline (e.g. "428
  delivered; 11 with detailed timeline"), in one small call (never N per-sample
  lookups). Nothing silently dropped.
- **(Q4) Honest progress presentation.** Present the **current phase**; for an open
  phase, "time in phase so far" computed by the **agent** (which knows "now") from the
  upstream `entered_at`/`reached_at`; show completed-phase durations from upstream;
  always attach the `mlwh_freshness` caveat (current phase is **as-of last sync**).
  Frame layer differences as detail level, never pass/fail; render recurrences /
  on-hold / cancelled faithfully; never fabricate phases the data doesn't record.

### Guidance

- **(W) Update the `mlwh://workflow` resource** with explicit workflows: (i)
  **availability** — use the summary/count tool, never `mlwh_irods_paths_for_study`
  or `mlwh_study_detail`; (ii) **recency** — supply an explicit `since`, read the
  result as "added to iRODS", and **caveat with `mlwh_freshness`** (the answer is
  complete only up to the last sync); (iii) the general **count/summarise → decide
  → page** recipe naming each large list's count tool; (iv) **progress** — route
  "what's happening with my sample / study" to the progress/breakdown tools, read
  the current phase as **as-of last sync**, let the agent compute elapsed time in the
  open phase from `reached_at`, and present the baseline-vs-detailed difference as
  detail level (never "broken for this sample"). Make the new tools'
  descriptions unambiguously the right pick for these questions, including the
  definition of "available", the recency semantics, and the study-scoping caveat.

## HARD REQUIREMENTS

1. **One call, small response** for every count/summary/overview/recency question;
   payload size independent of study/run size. No agent should page iRODS or fan
   out per sample to answer availability or recency.
2. **Correct recency presentation.** Present windowed results as "added to iRODS",
   never `last_updated`; always attach the cache-freshness caveat, clearly distinct
   from the data-added time.
3. **Thin pass-throughs (except G).** Counts/summaries come from upstream
   aggregates, not MCP-side counting of fetched lists; follow the existing
   count-tool pattern, derive descriptions/output-schemas from the upstream
   `Registry`/OpenAPI, reuse `mapToolError` and the input-guard conventions.
4. **The size guard is generic and in `internal/core`**, leaving the core
   service-agnostic; it applies to every tool including `mlwh_call_endpoint`.
5. **Consistent errors.** Never-synced / empty / unknown-study map to the same
   actionable hints the existing study tools produce.
6. **Hermetic tests.** Extend `harness_test.go`: stub the new endpoints with
   `respondJSON`, call each tool through the MCP client, assert request path/query
   and returned shape (including the recency `since`/`until` params and the
   freshness field); cover error paths via `respondError`. For the size guard,
   stub an over-large upstream response and assert the structured guard error
   (not a raw dump) and that an under-budget response is unaffected.
7. **Keep the surface coherent.** Update `mlwh://workflow` and the relevant
   `../mcp/spec.md` stories; keep version-surfacing and existing behaviour intact
   except the deliberate (P) default change, which must be documented.
8. **Every sample resolves; tiers are detail-level, not pass/fail.** `mlwh_sample_progress`
   always presents the baseline; the milestone timeline and run-status layers are
   shown when available and their absence is framed as *less detail* (with the
   reason), never an error or "no progress". Current phase is **as-of last sync**
   (freshness caveat); open-phase elapsed time is computed by the agent from
   `entered_at`/`reached_at`, not fabricated. The study breakdown (Q3) is one
   aggregate call counting **all** samples, nothing hidden.
9. **Platform-aware; never a false "no data".** Upstream covers all sequencing
   platforms uniformly (Illumina, PacBio, Elembio, Ultimagen via their own
   product-metrics; ONT via `oseq_flowcell`), carrying a `platform` on results and an
   explicit "not supported/tracked for &lt;platform&gt;" where a platform lacks a
   capability (e.g. ONT iRODS/QC/status). Surface `platform` in tool output and pass
   the upstream "not tracked" message through **verbatim** — never collapse it to a
   bare "no data". (Mirrors upstream HARD REQUIREMENT 11.)

## Design decisions for the spec to settle (HOW, not WHETHER)

Each item below **will be built**; settle only the implementation:

- **Tool names/shapes**, tracking the upstream endpoints (combined summary/overview
  vs separate; the `since`/`until` parameter names; the count/list tool names) —
  consistent with the existing `mlwh_*_for_study` / `mlwh_count_*` conventions.
- **The size-guard threshold default and config name**, the structured error's
  shape/wording, and how it estimates size (serialised bytes vs a token estimate).
- **The bounded-default page size for (P)** and the exact sizing-hint wording,
  reconciled with the C2 fetch-all stories.
- **How "added to iRODS" is worded** in tool descriptions, and the exact
  field/wording of the freshness caveat. The caveat is **always present on the
  response** (the upstream surfaces it per its deliverable F — see HARD REQUIREMENT
  2); this decision settles only its presentation, not whether it appears.
  `mlwh_freshness` stays available for an explicit re-check but is **not** a
  substitute for the per-response field.
- **Progress tool shapes** — names (`mlwh_sample_progress`, `mlwh_run_status`,
  `mlwh_study_status_breakdown`); how the open phase's elapsed time, the freshness
  caveat, and the three-layer (baseline / milestone / run-status) presentation
  (incl. `detailed_timeline: false` + reason) are surfaced — matching the upstream
  progress endpoints.

## Out of scope

- The upstream API work itself (see [`wa-prompt.md`](./wa-prompt.md)); this spec
  assumes those endpoints exist.
- Any `internal/core` change beyond the generic size guard (G) and what
  registering new MLWH tools requires.
- HTTP transport / web UI work (separate features); client-side caching or quotas.

## Pointers / prior art (in order of authority)

1. **This repo's code**: `internal/mlwh/tools_detail.go`
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
2. **The companion upstream feature**, [`wa-prompt.md`](./wa-prompt.md), and the
   `wa` repo it targets — the contract these tools wrap; the upstream `Registry`
   `Description` becomes each tool's description.
