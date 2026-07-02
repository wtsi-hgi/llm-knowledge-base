# Feature: MLWH MCP tools for fast study→iRODS TSV, recency, correct prefix/contains search, and run aggregation

## Summary

Wrap the third wave of upstream `wa` MLWH endpoints (API 1.8.0) as MCP tools, and fix
the MCP surface so a set of common real-world questions become **one cheap, correct
call** instead of a long manual fan-out that ends in a caveated guess. The target
questions (from real agent transcripts that currently fail or answer wrongly):

- "Write a TSV of the cram iRODS files for study X with columns
  `[supplier_sample_name, study_accession_number, sanger_sample_id, manual_qc,
  irods_path]`, primary/target deliverables only." (today: no tool exposes
  `manual_qc` or a target filter; the manifest is ~3 s for a big study)
- "Give me all samples that start with `hek_r`." (today: 55 results, not the 4 wanted)
- "Which samples' names contain `<substring>`?" (today: 0 results for real substrings)
- "The most recently sequenced sample / latest iRODS data for study (or lab) X."
  (today: `IRODSPath` has no `created`; recency is unanswerable from a listing)
- "Plot runs per month by manufacturer and platform for the last 3 years." (today: no
  global run tool at all)

The upstream `wa` code is authoritative and already exposes the needed endpoints and
header-aware client methods (API 1.8.0). This feature is the **downstream MCP work
only**: wrap them, expose the new fields/params, fix the search tools' semantics, add
run-aggregation and recency tools, and rewrite the workflow guidance so agents pick the
cheap correct tool first and stop reaching for the generic escape hatch.

Everything in this prompt is in scope to build. The `wa` code is the contract.

## Authority

Use the current `wa` Go code as the only contract for endpoint paths, query params,
field names, descriptions, and semantics:

- `~/wa/mlwh/registry.go`: endpoint `Method`, `Path`, `Query`, `Summary`,
  `Description`, `QueryParams`.
- `~/wa/mlwh/types.go`: exact `json:` field tags and output shapes.
- `~/wa/mlwh/manifest.go`, `hierarchy.go`, `search.go`, `availability.go`,
  `people.go`, `remote.go`, `server.go`: behaviour and typed client methods.
- `~/wa/.docs/mcp/api-reference.md` and `wa.OpenAPIDocument()`: generated mirrors.

Do not use `~/wa/.docs/realworld*` prompt/spec/phase files as a contract; they can
drift. Re-verify every field/param/description against `~/wa` before implementing.
The MCP layer must source descriptions and output schemas from the upstream
`Registry`/OpenAPI wherever the existing pattern supports it. Update the
`github.com/wtsi-hgi/wa` dependency to the tag/commit that contains the API 1.8.0
surface (the new TSV/export, recency, search-mode, and run-aggregation methods and
their header-aware client variants) before wrapping it.

## Upstream Surface (new/changed in this wave — verify names against `~/wa`)

All endpoints are `GET`. Bare list endpoints return a JSON array plus `X-Total-Count`
and `X-Next-Offset`; typed client page variants expose those as `Page[T]`. Envelope
tools (manifest/detail-style) keep their body shape and add top-level `total` /
`next_offset`. **The exact paths/method names below are indicative — bind to whatever
`~/wa/mlwh/registry.go` actually ships; do not invent names it does not have.**

| Concern | Upstream (indicative) | Returns | Notes |
| --- | --- | --- | --- |
| Study→iRODS TSV/export rows | `StudyExport`/extended `StudyManifest` | rows w/ chosen columns | column set, `file_type`, deliverable-only, `qc` filter; keyset-paged; `/count` |
| iRODS paths (+recency) | `IRODSPathsFor{Study,Sample,Run}` | `[]IRODSPath` (now incl. `created`) | `file_type`, `order_by=created_desc`, `since`/`until` |
| Latest iRODS for study/sponsor | `LatestIRODSForStudy` / `…ForFacultySponsor` | rows sorted `created DESC` | one call for "most recent sample/data" |
| Search — prefix/contains/exact | `SearchSamples`/`SearchStudies` w/ `mode`, or mode-specific methods | list + `total`/`next_offset` | literal-prefix, indexed contains, exact-short-token |
| Runs per month | `RunsMonthlyCounts` | `[]{month, manufacturer, platform, count, date_basis, cache_synced_at}` | `since`/`until`, all platforms |
| Global run listing | `Runs` | `[]Run` | one row per run; paged; `/count` |
| Study QC/status (fixed) | `StatusBreakdown` | `StatusBreakdown` | `per_platform` now always `[]`, never null; big studies now <1 s |

Also surface any new `/count` counterparts (TSV/export count, runs count) as count
tools, following the existing count-tool pattern.

## Output Shapes (additions — confirm against `~/wa/mlwh/types.go`)

- **`IRODSPath`** gains **`created`** (RFC3339 UTC, omitempty when the source value is
  null). Existing fields unchanged (`id_product`, `collection`, `data_object`,
  `irods_path`, `id_sample_tmp`, `name`, `id_run`, `platform`).
- **Manifest/export row** gains **`manual_qc`** (the upstream pass/fail/pending
  roll-up, or the surface form `wa` ships) and whatever target/deliverable indicator
  `wa` exposes. The export tool returns the caller-selected columns as named fields.
- **Run row / monthly count** per the upstream `Run` / monthly-count types: month,
  manufacturer, platform, count, `date_basis`, plus `cache_synced_at` on the aggregate.
- **`StatusBreakdown.per_platform`** is always an array (`[]` for empty studies), never
  null — the tool's output schema and the wrapper must accept and assert `[]`.

## MCP Tools To Add Or Update

### Study→iRODS TSV / manifest (the flagship)

- **`mlwh_study_irods_tsv`** (or extend `mlwh_study_manifest`): the default tool for
  "give me a table/TSV of a study's iRODS files with these columns". Params: study id
  (required), `file_type` (default `cram`), `columns` (ordered selection over the
  documented vocabulary incl. `supplier_name`/`supplier_sample_name`,
  `sanger_sample_id`, `name`, `study_accession_number`, `manual_qc`, `id_run`, `lane`,
  `tag_index`, `platform`, `irods_path`), `deliverables_only` (default true),
  optional `qc` filter (`pass|fail|pending`), plus paging. It MUST return
  `manual_qc` and the deliverable filter — the whole point is that the agent does not
  reconstruct target/QC semantics itself. Bounded-by-default with `total`/`next_offset`
  and a `/count` tool; document that the full set is retrieved by paging the cursor,
  and that assembling a file is the agent's job (the MCP result is bounded by the size
  guard — see Hardening).
- **`mlwh_count_study_irods_tsv`** (or the manifest count already present) honouring
  the same `file_type`/`deliverables_only`/`qc` filters, so an agent can size the
  export before paging.

### iRODS recency and "latest data"

- Update **`mlwh_irods_paths_for_{study,sample,run}`** to expose the new `created`
  field and accept `order_by=created_desc`, `since`, `until`. Descriptions must state
  `created` = "data added to iRODS" and that recency ordering is available.
- Add **`mlwh_latest_irods_for_study`** and **`mlwh_latest_irods_for_faculty_sponsor`**
  (bind to the upstream latest endpoints): return the newest iRODS rows with `created`,
  `irods_path`, sample `name`, `supplier_name`, study id/name, `id_run`, lane/tag,
  platform — sorted newest-first, all ties at the max returned. These make "the most
  recently sequenced sample for study/lab X" one call.

### Search — explicit prefix / contains / exact

- Rework the sample/study search tools so **intent is explicit**. Either add a `mode`
  param (`prefix` = literal whole-value prefix; `contains` = substring; `exact`;
  keep the current word-prefix as its own value) or provide mode-specific tools. The
  descriptions MUST state, per mode, what matches and over which fields, with worked
  examples: `prefix "hek_r"` → the 4 `Hek_R*`; `contains "usculus"` → the Mus musculus
  set; `exact "gt"` for controlled short tokens. Preserve the existing word-prefix
  behaviour under its own mode/value, but make it no longer the only thing "search"
  can do.
- Keep **`mlwh_find_samples`** / **`mlwh_count_find_samples`** as the exact-match path
  for the `find/sample/*` fields, and route short controlled tokens (`gt`) there
  rather than dead-ending on the 3-char free-text minimum.

### Run aggregation

- **`mlwh_runs_monthly`**: wrap the monthly grouped-count endpoint; params
  `since`/`until` and optional platform filter; returns `{month, manufacturer,
  platform, count, date_basis, cache_synced_at}` rows. This is the default for
  "runs per month" / run time-series questions.
- **`mlwh_runs`** (+ **`mlwh_count_runs`**): the global run listing for drill-down
  (one row per run: stable id, native id, platform, manufacturer, run dates), bounded
  and paged.

## Query Semantics

- `file_type`: unchanged filename-suffix filter (case-insensitive, one leading dot
  stripped); empty/`%`/`_`/`/` are upstream 400s → actionable MCP errors; a valid but
  unmatched suffix is an empty result / count 0.
- `deliverables_only`: boolean, default true on the cram TSV; when true, controls/
  spikes and non-primary sub-products are excluded per the upstream definition. State
  in the description that this approximates the iRODS `target=1` AVU and what it
  excludes — do not describe it as a plain column.
- `qc` filter and the `manual_qc` column: `manual_qc` is `iseq_product_metrics.qc`
  rolled up to pass/fail/pending (fail > pending > pass; not-tracked when no product).
  The column and the filter are independent (an agent can return the column without
  filtering).
- `order_by=created_desc`, `since`, `until`: recency over iRODS `created`, half-open
  `[since, until)`; `until` without `since` and malformed timestamps map to actionable
  errors.
- search `mode`: `prefix` (literal whole-value prefix), `contains` (substring,
  including mid-word), `exact`, and the legacy word-prefix — each documented; the
  3-char minimum applies only to the free-text prefix/contains paths.
- run aggregation `since`/`until` window and per-platform `date_basis` are surfaced
  from upstream verbatim; present `date_basis` and `cache_synced_at` as caveats.

## Time And Freshness

Never conflate: (1) **data added to iRODS** = `created` — the only basis for "latest",
recency ordering, `since`/`until`, `newest_data_added`; (2) **`last_changed`** = the
warehouse row-change / sync key — never presented as "new data"; (3) **`cache_synced_at`
/ freshness** = completeness caveat. `cache_synced_at` is present on aggregate/manifest
responses; it is absent on bare lists and counts — use `mlwh_freshness` for the as-of
caveat there, including for the new TSV/export, recency, and run tools where the
response has no `cache_synced_at`.

## MCP Hardening

- Keep the generic response-size guard (`MLWH_MAX_TOOL_RESULT_BYTES`, default 1 MiB;
  `IsError=true` with a structured actionable error over budget). The study→iRODS TSV
  can be very large (a big study is 100k+ product×irods rows); the tool MUST be
  bounded-by-default and paged, and its over-budget error must point the caller to
  `/count` + the page cursor, not to fetch-all. Do NOT let the export tool try to
  return a whole study inline.
- Bounded paged fan-out defaults (`limit=100`, max `1000`) with `total`/`next_offset`
  from upstream header-aware results for the exact filtered request; use keyset/offset
  as upstream exposes it.
- Do not implement aggregates or the target/QC/recency semantics MCP-side by fetching
  lists and post-processing. Use the upstream endpoints; the semantics live in `wa`.
- The generic `mlwh_call_endpoint` remains a fallback only. It must NOT be the
  recommended way to answer any of the five target questions; the workflow guidance
  must steer agents to the curated tools instead (see below).

## Workflow Guidance

Rewrite `mlwh://workflow` so agents choose the cheap correct tool first, and add
explicit routing for the previously-failing shapes:

- **Study cram TSV / "table of files with columns X":** use `mlwh_study_irods_tsv`
  with the requested `columns` (incl. `manual_qc`), `file_type=cram`,
  `deliverables_only=true`; count first with the count tool if it may be large; page
  the cursor to assemble the full file. Do NOT hand-write SQL via
  `mlwh_call_endpoint`, and do NOT page raw iRODS lists and try to join in QC/target
  yourself.
- **"Starts with" / "contains" / short exact token:** use the search tool's `prefix` /
  `contains` / `exact` mode respectively (or `mlwh_find_samples` for exact controlled
  fields). Word-prefix is a distinct mode; do not use it for "starts with".
- **"Most recent sample / latest data for study or lab":** use
  `mlwh_latest_irods_for_study` / `…_for_faculty_sponsor`, or an iRODS list with
  `order_by=created_desc`. Do NOT infer recency from `study_overview` alone (it reports
  a max timestamp you cannot follow to a row) or from list ordering (which is by
  product id).
- **"Runs per month / by platform / by manufacturer":** use `mlwh_runs_monthly`; use
  `mlwh_runs` for per-run drill-down. Do NOT fan out over studies.
- **Recency wording:** always describe `created`-based results as data "added to
  iRODS", with the `cache_synced_at`/freshness caveat.

## Hard Requirements

1. One cheap call for the flagship shapes: a study cram TSV with chosen columns +
   `manual_qc` + deliverable filter (bounded page, `total`, `/count`); a "starts
   with"/"contains"/exact search that returns the right rows; a "latest data" answer;
   a runs-per-month aggregate. No per-sample or per-study fan-out for any of these.
2. Correct semantics surfaced, not reconstructed: `manual_qc`, deliverable/target,
   `created` recency, run `date_basis`, and prefix-vs-contains-vs-exact all come from
   the upstream endpoints and their descriptions, exposed faithfully. The agent is
   never expected to write SQL to get these right.
3. `IRODSPath.created` and the new recency params are surfaced on every iRODS list
   tool; `manual_qc` and the deliverable filter are surfaced on the TSV/manifest tool.
4. Bounded-by-default lists with count counterparts and sizing hints
   (`total`/`next_offset`), sourced from upstream header-aware `wa` client results.
   The TSV tool never returns an unbounded payload; the size guard covers it.
5. Correct timestamp wording ("added to iRODS" = `created`), and clear freshness
   caveats from `cache_synced_at` / `mlwh_freshness`.
6. `StatusBreakdown.per_platform` empty-study handling: the tool accepts and returns
   `[]` (never null); add a regression test with an empty study stub.
7. Consistent actionable errors for upstream 400s / not-found / ambiguity /
   unsupported identifiers / impaired-or-never-synced cache, for all new tools.
8. Hermetic tests only: extend `internal/mlwh/harness_test.go` with stubbed MLWH
   responses. Assert tool registration, request path/query (incl. `columns`,
   `deliverables_only`, `qc`, `order_by`, `since`/`until`, search `mode`), returned
   shape (incl. `created`, `manual_qc`), `cache_synced_at` presence/absence, paging
   hints, error mapping, and over-budget guard behaviour for the TSV tool.

## Repo Pointers

- Tool registration: `internal/mlwh/provider.go` and the `register*Tools` helpers.
- Manifest/detail + paging: `internal/mlwh/tools_detail.go`. iRODS/availability:
  `internal/mlwh/tools_availability.go`, `tools_overview.go`. Search:
  `internal/mlwh/tools_search.go`. People: `internal/mlwh/tools_people.go`.
  Resolve: `internal/mlwh/tools_resolve.go`.
- Output schemas: `internal/mlwh/schema.go`, sourced from `wa.OpenAPIDocument()`.
- Generic fallback: `internal/mlwh/tools_call.go`. Workflow resource:
  `internal/mlwh/workflow.go`. Freshness/errors: `internal/mlwh/tools_freshness.go`,
  `internal/mlwh/errmap.go`. Size guard: `internal/core/`. Harness:
  `internal/mlwh/harness_test.go`. Broader conventions: `../mcp/spec.md`.

## Out Of Scope

- Further upstream `wa` API work. The API 1.8.0 endpoints and header-aware client
  surface are implemented in `~/wa`; update the dependency and wrap them.
- HTTP transport / web UI work; client-side caching or quotas beyond the core size
  guard.
- Re-deriving target/QC/recency/run semantics MCP-side — they are upstream concerns.

## Notes

- Bind tool names/params/fields to what `~/wa/mlwh/registry.go` and `types.go`
  actually ship for API 1.8.0; the names in this prompt are indicative. If upstream
  extends `StudyManifest` rather than adding a separate export endpoint, wrap the
  extension and keep the tool name that best communicates "TSV/table of a study's
  iRODS files with chosen columns" to the agent.
- All paginated typed list tools return top-level `total` and `next_offset`
  (including the new TSV/export, recency, and run tools), matching the flattened
  envelope rule already used for manifest/detail.
- The workflow resource text is the primary lever that stops agents defaulting to
  `mlwh_call_endpoint` for these questions — make the routing unambiguous and give the
  worked examples (hek_r → 4; usculus → contains; study cram TSV; latest-for-sponsor;
  runs-per-month).
