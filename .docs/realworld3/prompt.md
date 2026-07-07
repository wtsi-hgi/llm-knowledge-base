# Feature: MLWH MCP tools for a fast generic column-selectable export, iRODS recency, correct default search, run + programme aggregation, a study→users inverse, and merged-CRAM attribution

## Summary

Wrap the third wave of upstream `wa` MLWH endpoints (API 1.8.0) as MCP tools, and fix
the MCP surface so a set of common real-world questions become **one cheap, correct
call** instead of a long manual fan-out that ends in a caveated guess or a silently
incomplete answer. The target questions (from real agent transcripts that currently
fail or answer wrongly):

- "Write a TSV of the cram iRODS files for study X with columns
  `[supplier_sample_name, study_accession_number, sanger_sample_id, manual_qc,
  irods_path]`, primary/target deliverables only." (today: no tool exposes
  `manual_qc`, a deliverable filter, or column selection; the manifest is ~3 s for a big
  study and silently drops merged CRAMs)
- "Give me all samples that start with `hek_r`." (today: 55 results, not the 4 wanted)
- "The Mus musculus samples." (today: no clean way to constrain a search to an organism)
- "The most recently sequenced sample / latest iRODS data for study (or lab) X."
  (today: `IRODSPath` has no `created`; recency is unanswerable from a listing)
- "Runs per month by manufacturer and platform for the last 3 years." (today: no
  global run tool at all)
- "Break down PacBio sequencing by programme for the last year; which 5 studies? Then a
  per-study table with programme, faculty sponsor, and the study owners/managers/
  followers." (today: no platform/date-scoped grouped aggregate, `programme` is not a
  grouping dimension and is absent from `mlwh_study_overview`, and the study↔person graph
  runs person→studies only — there is no study→users tool)
- "List every sample in study 7568 with sample name, EGA id, and an iRODS CRAM path."
  (today: `mlwh_study_manifest --with-irods` silently returns an empty `irods_path` for
  the 48 samples whose CRAM is a merged multi-lane composite object)

The upstream `wa` code is authoritative and already exposes the needed endpoints and
header-aware client methods (API 1.8.0). This feature is the **downstream MCP work
only**: wrap them, expose the new fields/params, add the generic export tool, fix the
search tools' semantics, add run/programme aggregation, recency, the study→users
inverse, and merged-CRAM attribution, and rewrite the workflow guidance so agents pick
the cheap correct tool first and stop reaching for the generic escape hatch.

Everything in this prompt is in scope to build. The `wa` code is the contract.

## Authority

Use the current `wa` Go code as the only contract for endpoint paths, query params,
field names, descriptions, and semantics:

- `~/wa/mlwh/registry.go`: endpoint `Method`, `Path`, `Query`, `Summary`,
  `Description`, `QueryParams`.
- `~/wa/mlwh/types.go` (and `~/wa/mlwh/mlwh.go` for `Run`/`Library`): exact `json:`
  field tags and output shapes.
- `~/wa/mlwh/manifest.go`, `hierarchy.go`, `search.go`, `availability.go`,
  `people.go`, `count.go`, `remote.go`, `server.go`: behaviour and typed client methods.
- `~/wa/.docs/mcp/api-reference.md` and `wa.OpenAPIDocument()`: generated mirrors.

Do not use `~/wa/.docs/realworld*` prompt/spec/phase files as a contract; they can
drift. Re-verify every field/param/description against `~/wa` before implementing.
The MCP layer must source descriptions and output schemas from the upstream
`Registry`/OpenAPI wherever the existing pattern supports it. Update the
`github.com/wtsi-hgi/wa` dependency to the tag/commit that contains the API 1.8.0
surface (the new generic export, recency, search default, run + programme aggregation,
study→users, and merged-CRAM-aware methods and their header-aware client variants)
before wrapping it.

## Upstream Surface (new/changed in this wave — verify names against `~/wa`)

All endpoints are `GET`. Bare list endpoints return a JSON array plus `X-Total-Count`
and `X-Next-Offset`; typed client page variants expose those as `Page[T]`. Envelope
tools (manifest/detail-style) keep their body shape and add top-level `total` /
`next_offset`. **The exact paths/method names below are indicative — bind to whatever
`~/wa/mlwh/registry.go` actually ships; do not invent names it does not have.**

| Concern | Upstream (indicative) | Returns | Notes |
| --- | --- | --- | --- |
| Generic column-selectable export | `Export`/extended list methods | rows w/ chosen columns | any parent→children relationship; `columns`, filters; keyset-paged; `/count` |
| iRODS paths (+recency, +merged) | `IRODSPathsFor{Study,Sample,Run}` | `[]IRODSPath` (now incl. `created`, merged attribution) | `file_type`, `order_by=created_desc`, `since`/`until`; merged/composite CRAMs attributed to their sample |
| Latest iRODS for study/sponsor | `LatestIRODSForStudy` / `…ForFacultySponsor` | rows sorted `created DESC` | one call for "most recent sample/data" |
| Per-sample study CRAMs | `StudySampleCrams` | one row per sample: name, ega_id, irods_cram_path | merged-aware, de-duplicated (Q7) |
| Search — default literal-prefix | `SearchSamples` (default) + `--words` mode | list + `total`/`next_offset` | literal whole-value prefix default; opt-in word-prefix; shared exact filters |
| Runs per month / grouped | `RunsMonthlyCounts` / grouped aggregate | `[]{month, manufacturer, platform, count, date_basis, cache_synced_at}` | `since`/`until`; group by platform AND study attribute (programme/faculty_sponsor); `unit` |
| Global run listing | `Runs` | `[]Run` | one row per run; paged; `/count` |
| Studies by programme + enumeration | `StudiesForProgramme` / `Programmes` | `[]Study` / `[]{programme, count}` | exact indexed programme filter; vocabulary |
| Study → users (inverse) | `UsersForStudy` | `[]{role, name, login, email}` | one indexed `id_study_tmp` lookup; role filter |
| Study QC/status (fixed) | `StatusBreakdown` | `StatusBreakdown` | `per_platform` now always `[]`, never null; big studies now <1 s |

Also surface any new `/count` counterparts (export count, runs count,
studies-for-programme count) as count tools, following the existing count-tool pattern.

## Output Shapes (additions — confirm against `~/wa/mlwh/types.go`)

- **`IRODSPath`** gains **`created`** (RFC3339 UTC, omitempty when the source value is
  null) and honest merged-object attribution: for a merged/composite CRAM, `name` /
  `id_sample_tmp` are populated (sourced from the iRODS mirror's own denormalised
  `id_sample_tmp`, not the single-lane product join), and the object is marked as merged
  (e.g. a `merged` flag / the contributing run set) rather than reporting a single
  misleading `id_run=0`. Existing fields otherwise unchanged (`id_product`, `collection`,
  `data_object`, `irods_path`, `platform`).
- **Export / manifest row** gains **`manual_qc`** (the upstream pass/fail/pending
  roll-up, resolved for composite products too) and whatever target/deliverable
  indicator `wa` exposes. The export tool returns the caller-selected columns as named
  fields.
- **`StudyManifest`** gains an envelope **`products_without_irods`** counter and per-row
  **`irods_unmatched`**/`reason` (`merged_multilane`) when the product-grained join
  cannot reach a product's merged CRAM (Q7), unless upstream instead resolves the merged
  path directly.
- **Per-sample study CRAMs row**: `sample_name`, `ega_id`, `irods_cram_path` — one per
  sample, merged-aware.
- **`StudyOverview`** gains **`programme`** (alongside `name`, `accession_number`,
  `faculty_sponsor`, `data_access_group`).
- **Study→users row**: `role`, `name`, `login`, `email`.
- **Grouped run/sequencing count row**: `{month?, programme?, faculty_sponsor?,
  manufacturer, platform, count, unit, date_basis, cache_synced_at}`.
- **Run row / monthly count** per the upstream `Run` / monthly-count types.
- **`StatusBreakdown.per_platform`** is always an array (`[]` for empty studies), never
  null — the tool's output schema and the wrapper must accept and assert `[]`.

## MCP Tools To Add Or Update

### Generic export (the flagship)

- **`mlwh_export`**: the default tool for "give me a table/TSV of the `<children>` of
  `<entity>` with these columns". Params: `entity`/`children` selecting the relationship
  (at least: iRODS/files of study|sample|run; samples of study|run|library; runs of
  study|sample; libraries of study; lanes of sample; studies of sample|faculty-sponsor|
  user|programme; users of study; sample-crams of study), the parent id, `columns`
  (ordered selection over the relationship's documented vocabulary), the applicable
  filters (`file_type` default `cram` on file listings, `deliverables_only` default true
  on cram, `qc`, `organism`, `library_type`), recency (`order_by=created_desc`,
  `since`/`until` on iRODS), and paging. It returns the selected columns as structured
  rows; optionally accept a `format` to return pre-rendered TSV/CSV text for direct
  saving. It MUST expose `manual_qc` and the deliverable filter and include merged CRAMs
  (see below) — the whole point is that the agent does not reconstruct target/QC/merge
  semantics itself. Bounded-by-default with `total`/`next_offset` and a
  **`mlwh_count_export`** counterpart; document that the full set is retrieved by paging
  the cursor, that assembling a file is the agent's job, and that the MCP result is
  bounded by the size guard (see Hardening). This supersedes `mlwh_study_manifest` for
  "table of a study's files with chosen columns"; the fixed-shape list tools
  (`mlwh_irods_paths_for_*`, `mlwh_samples_for_study`, `mlwh_runs_for_study`, …) remain
  for quick lookups without column selection.

### iRODS recency and "latest data"

- Update **`mlwh_irods_paths_for_{study,sample,run}`** to expose the new `created`
  field, accept `order_by=created_desc`, `since`, `until`, and carry the merged-object
  attribution (see below). Descriptions must state `created` = "data added to iRODS" and
  that recency ordering is available.
- Add **`mlwh_latest_irods_for_study`** and **`mlwh_latest_irods_for_faculty_sponsor`**
  (bind to the upstream latest endpoints): return the newest iRODS rows with `created`,
  `irods_path`, sample `name`, `supplier_name`, study id/name, `id_run`, lane/tag,
  platform — sorted newest-first, all ties at the max returned. These make "the most
  recently sequenced sample for study/lab X" one call.

### Merged multi-lane CRAM attribution (Q7)

- **`mlwh_irods_paths_for_{study,sample}`** and **`mlwh_export`** must attribute
  merged/composite CRAMs to their sample: `name`/`id_sample_tmp` populated, never
  `""`/`0`, and the merged nature surfaced honestly. State in the description that a
  sample sequenced across several lanes has ONE merged CRAM (its own composite product
  id) and that it is attributed via the sample↔iRODS linkage.
- Add **`mlwh_study_sample_crams`**: one row per sample (`sample_name`, `ega_id`,
  `irods_cram_path`), merged-aware and de-duplicated — the clean single call for
  "every sample of study X with its iRODS CRAM path". For study 7568 it returns all 732
  samples with a path, not 684.
- Update **`mlwh_study_manifest`**: surface `products_without_irods` and per-row
  `irods_unmatched`/`reason` (or the resolved merged path), and its description must name
  merged multi-lane CRAMs as the common cause of an empty `irods_path`. The tool must not
  present an empty `irods_path` as a plain data gap.

### Search — literal-prefix default, opt-in word-prefix, shared exact filters

Align the sample search tools with the upstream (corrected) semantics — do NOT expose a
"contains"/substring mode; it does not exist upstream:

- **Default = literal whole-value prefix.** `mlwh_search_samples("hek_r")` with no mode
  returns **exactly** the 4 samples whose `name`/`supplier_name`/`common_name`/`donor_id`
  literally starts with the term (`Hek_R1..4`), NOT the 55 the old word-prefix returned.
- **Opt-in `mode=words`** preserves the separator-agnostic word-prefix behaviour (e.g.
  `10X Automation HEK` matching `10X_Automation_HEK`); it is no longer the default and its
  cross-field AND behaviour is not what a bare search does.
- **No `contains`/substring/n-gram mode** — mid-word fragments like `usculus` match
  **nothing** (say so in the description); do not offer or imply substring search.
- **Shared exact filters** on `mlwh_search_samples` (and `mlwh_export`), each exempt from
  the 3-char minimum, AND-combined: `organism` (WORD-MEMBERSHIP over the low-cardinality
  `common_name` vocabulary — `organism="musculus"` matches every `common_name` containing
  the whole word `musculus`, INCLUDING subspecies like `Mus musculus castaneus`;
  `usculus` matches nothing; NOT exact-whole-value), `library_type` (exact
  `pipeline_id_lims`), `qc pass|fail|pending` (per-sample roll-up on search, per-product
  on the export — state the grain), `deliverables_only` (pass-through for PacBio/ONT).
- Keep **`mlwh_find_samples`** / **`mlwh_count_find_samples`** as the exact-match path
  for the `find/sample/*` fields, and route short controlled tokens (`gt`) there rather
  than dead-ending on the 3-char free-text minimum. Exact single-identifier lookups stay
  the resolve/`info` path — do not add an `exact` search mode.

### Run and programme aggregation (Q5, Q6)

- **`mlwh_runs_monthly`**: wrap the monthly grouped-count endpoint; params
  `since`/`until` and optional platform filter; returns `{month, manufacturer,
  platform, count, date_basis, cache_synced_at}` rows. Default for "runs per month".
- **`mlwh_runs`** (+ **`mlwh_count_runs`**): the global run listing for drill-down
  (one row per run: composite `<platform>:<native_id>` id, native id, platform,
  manufacturer, run date + `date_basis`), bounded and paged.
- **`mlwh_sequencing_counts`** (the generalised grouped aggregate): params `group_by`
  (combinable over `programme`, `faculty_sponsor`, `platform`, `manufacturer`, `month`),
  optional `platform` filter, `since`/`until`, and `unit` (`runs` | `samples`/`products`).
  Returns per-group counts, each row stating `unit` and `date_basis`. This answers
  "PacBio sequencing by programme for the last year" in one call. Describe the
  multi-study run-attribution rule verbatim from upstream.

### Programme as a dimension (Q6)

- Add **`mlwh_studies_for_programme`** (+ **`mlwh_count_studies_for_programme`**): the
  exact, indexed "studies in programme X" list (NOT the `mlwh_search_studies` conflation
  of name/title/programme/sponsor) — answers "which 5 studies".
- Add **`mlwh_programmes`**: the distinct `programme` vocabulary with study counts.
- Update **`mlwh_study_overview`** to carry **`programme`**, so a per-study pass can group
  by programme without a second `mlwh_resolve_study`/`mlwh_study_detail` call.

### Study → users inverse (Q6)

- Add **`mlwh_study_users`** (bind to `/study/:id/users`): given a study, list its role
  members — `role`, `name`, `login`, `email` — with an optional `role` filter over the
  stored vocabulary (`owner`, `manager`, `data_access_contact`, `follower`, `slf_manager`,
  `lab_manager`, `administrator`). This is the inverse of `mlwh_studies_for_user` and
  closes the direction gap (owners/managers/followers per study in one call). The
  description must keep the distinction that `faculty_sponsor` is a `Study` field, NOT a
  `study_users` role.

## Query Semantics

- `file_type`: unchanged filename-suffix filter (case-insensitive, one leading dot
  stripped); empty/`%`/`_`/`/` are upstream 400s → actionable MCP errors; a valid but
  unmatched suffix is an empty result / count 0.
- `deliverables_only`: boolean, default true on the cram export; when true, controls/
  spikes and non-primary sub-products are excluded per the upstream `entity_type`
  definition; **pass-through for PacBio/ONT** (no discriminator → their samples are never
  dropped). State in the description that this approximates the iRODS `target=1` AVU and
  what it excludes — do not describe it as a plain column.
- `qc` filter and the `manual_qc` column: `manual_qc` is `iseq_product_metrics.qc`
  rolled up to pass/fail/pending (fail > pending > pass; not-tracked when no product),
  resolved for composite products too. The column and the filter are independent, and
  the filter grain differs (per-sample roll-up on search, per-product on the export).
- `organism`/`library_type`: exact filters over low-cardinality vocabularies; `organism`
  is whole-word membership over `common_name` (incl. subspecies), never mid-word substring.
- `order_by=created_desc`, `since`, `until`: recency over iRODS `created`, half-open
  `[since, until)`; `until` without `since` and malformed timestamps map to actionable
  errors.
- search: default literal whole-value prefix; `mode=words` is the opt-in word-prefix;
  no contains/substring; the 3-char minimum applies only to the free-text prefix/`words`
  paths, not the exact filters.
- run/sequencing aggregation `since`/`until` window, per-platform `date_basis`, `unit`,
  and the multi-study run-attribution rule are surfaced from upstream verbatim; present
  `date_basis`, `unit`, and `cache_synced_at` as caveats.
- study→users `role`: optional filter over the stored role vocabulary; default returns
  all roles present (state the default).

## Time And Freshness

Never conflate: (1) **data added to iRODS** = `created` — the only basis for "latest",
recency ordering, `since`/`until`, `newest_data_added`; (2) **`last_changed`** = the
warehouse row-change / sync key — never presented as "new data"; (3) **`cache_synced_at`
/ freshness** = completeness caveat. `cache_synced_at` is present on aggregate/manifest
responses; it is absent on bare lists and counts — use `mlwh_freshness` for the as-of
caveat there, including for the new export, recency, run/programme aggregation,
study→users, and sample-crams tools where the response has no `cache_synced_at`.
Run/sequencing counts carry a per-platform `date_basis` (Illumina/Element `run complete`,
Ultima `run archived`, PacBio `run_complete`, ONT the labelled warehouse-load fallback);
never present the ONT bucket's month as a true sequencing month.

## MCP Hardening

- Keep the generic response-size guard (`MLWH_MAX_TOOL_RESULT_BYTES`, default 1 MiB;
  `IsError=true` with a structured actionable error over budget). The export can be very
  large (a big study is 100k+ product×irods rows); the tool MUST be bounded-by-default
  and paged, and its over-budget error must point the caller to `mlwh_count_export` + the
  page cursor, not to fetch-all. Do NOT let the export tool try to return a whole study
  inline.
- Bounded paged fan-out defaults (`limit=100`, max `1000`) with `total`/`next_offset`
  from upstream header-aware results for the exact filtered request; use keyset/offset
  as upstream exposes it.
- Do not implement aggregates or the target/QC/recency/programme/merge semantics
  MCP-side by fetching lists and post-processing. Use the upstream endpoints; the
  semantics live in `wa`.
- The generic `mlwh_call_endpoint` remains a fallback only. It must NOT be the
  recommended way to answer any of the target questions; the workflow guidance must steer
  agents to the curated tools instead (see below).

## Workflow Guidance

Rewrite `mlwh://workflow` so agents choose the cheap correct tool first, and add
explicit routing for the previously-failing shapes:

- **"Table/TSV of files (or any children) with columns X":** use `mlwh_export` with the
  requested `columns` (incl. `manual_qc`), `file_type=cram`, `deliverables_only=true`;
  count first with `mlwh_count_export` if it may be large; page the cursor to assemble
  the full file. Do NOT hand-write SQL via `mlwh_call_endpoint`, and do NOT page raw
  iRODS lists and try to join in QC/target yourself.
- **"Every sample of a study with its CRAM path":** use `mlwh_study_sample_crams` (one
  merged-aware row per sample). Do NOT use `mlwh_study_manifest --with-irods` and treat
  an empty `irods_path` as "no data" — that misses merged multi-lane CRAMs (48 of 732 in
  study 7568); the manifest now flags them via `products_without_irods`.
- **"Starts with" / short exact token:** use the default (literal-prefix) sample search,
  or `mlwh_find_samples` for exact controlled fields. `mode=words` is a distinct opt-in;
  do not use it for "starts with". There is no "contains"/substring search.
- **"The <organism> samples" / narrow by organism/library-type/qc/deliverable:** pass the
  `organism`/`library_type`/`qc`/`deliverables_only` filters on `mlwh_search_samples` or
  `mlwh_export`. `organism` is whole-word membership over `common_name`.
- **"Most recent sample / latest data for study or lab":** use
  `mlwh_latest_irods_for_study` / `…_for_faculty_sponsor`, or an iRODS list with
  `order_by=created_desc`. Do NOT infer recency from `study_overview` alone or from list
  ordering (which is by product id).
- **"Runs per month / by platform / by manufacturer":** use `mlwh_runs_monthly`; use
  `mlwh_runs` for per-run drill-down. Do NOT fan out over studies.
- **"Sequencing by programme (± platform, date)":** use `mlwh_sequencing_counts` with
  `group_by=programme` (+ optional `platform`, `since`/`until`, `unit`). For "which
  studies in programme X" use `mlwh_studies_for_programme`; discover the vocabulary with
  `mlwh_programmes`; read `programme` per study from `mlwh_study_overview`. Do NOT fan out
  two calls per study over 8,223 studies.
- **"A study's owners / managers / followers":** use `mlwh_study_users` (study→people).
  Do NOT try to invert `mlwh_studies_for_user` by guessing candidate people.
- **Recency wording:** always describe `created`-based results as data "added to
  iRODS", with the `cache_synced_at`/freshness caveat.

## Hard Requirements

1. One cheap call for the target shapes: a column-selected cram export with `manual_qc` +
   deliverable filter + merged CRAMs (bounded page, `total`, `/count`); a "starts with" /
   organism-narrowed search that returns the right rows; a "latest data" answer; a
   runs-per-month and a sequencing-by-programme aggregate; a studies-in-programme list; a
   study→users listing; a per-sample cram list. No per-sample or per-study fan-out for
   any of these.
2. Correct semantics surfaced, not reconstructed: `manual_qc`, deliverable/target,
   `created` recency, run `date_basis`, literal-prefix-vs-words, the exact filters,
   `programme` grouping, study→users direction, and merged-CRAM attribution all come from
   the upstream endpoints and their descriptions, exposed faithfully. The agent is never
   expected to write SQL to get these right.
3. `IRODSPath.created`, the recency params, and merged-object attribution are surfaced on
   every iRODS list tool and the export; `manual_qc`, the deliverable filter, and
   column selection are surfaced on the export; `programme` is surfaced on
   `mlwh_study_overview`.
4. Bounded-by-default lists with count counterparts and sizing hints
   (`total`/`next_offset`), sourced from upstream header-aware `wa` client results.
   The export never returns an unbounded payload; the size guard covers it.
5. Correct timestamp wording ("added to iRODS" = `created`), correct `date_basis`/`unit`
   labels on aggregates (incl. the ONT warehouse-load caveat), and clear freshness
   caveats from `cache_synced_at` / `mlwh_freshness`.
6. No silently dropped rows: merged/composite CRAMs are attributed and returned by the
   study/sample iRODS tools, the export, and `mlwh_study_sample_crams`; the manifest makes
   any shortfall explicit. Study 7568 sample-crams = 732 populated rows.
7. `StatusBreakdown.per_platform` empty-study handling: the tool accepts and returns
   `[]` (never null); add a regression test with an empty study stub.
8. Consistent actionable errors for upstream 400s / not-found / ambiguity /
   unsupported identifiers / impaired-or-never-synced cache, for all new tools.
9. Hermetic tests only: extend `internal/mlwh/harness_test.go` with stubbed MLWH
   responses. Assert tool registration, request path/query (incl. `columns`, relationship
   selection, `deliverables_only`, `qc`, `organism`, `library_type`, `order_by`,
   `since`/`until`, search default vs `mode=words`, `group_by`/`unit`, `role`), returned
   shape (incl. `created`, `manual_qc`, `programme`, merged attribution,
   `products_without_irods`, study→users rows), `cache_synced_at` presence/absence,
   paging hints, error mapping, and over-budget guard behaviour for the export tool.

## Repo Pointers

- Tool registration: `internal/mlwh/provider.go` and the `register*Tools` helpers.
- Manifest/detail + paging: `internal/mlwh/tools_detail.go`. iRODS/availability:
  `internal/mlwh/tools_availability.go`, `tools_overview.go`. Search:
  `internal/mlwh/tools_search.go`. People (person→studies today; add study→users):
  `internal/mlwh/tools_people.go`. Resolve: `internal/mlwh/tools_resolve.go`.
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
- Re-deriving target/QC/recency/run/programme/merge semantics MCP-side — they are
  upstream concerns.

## Notes

- Bind tool names/params/fields to what `~/wa/mlwh/registry.go` and `types.go`/`mlwh.go`
  actually ship for API 1.8.0; the names in this prompt are indicative. If upstream
  extends `StudyManifest` or the list endpoints rather than adding separate export/
  sample-crams/users endpoints, wrap the extension and keep the tool name that best
  communicates the job to the agent.
- The generic export tool is the primary lever for Q1 and for "table of X with columns":
  it must clearly out-rank both `mlwh_study_manifest` and `mlwh_call_endpoint` in the
  workflow text for column-selected/large listings, while the simple per-relationship
  list tools stay for quick lookups.
- All paginated typed list tools return top-level `total` and `next_offset`
  (including the new export, recency, run, programme, and study→users tools), matching the
  flattened envelope rule already used for manifest/detail.
- The workflow resource text is the primary lever that stops agents defaulting to
  `mlwh_call_endpoint` (or to a manual manifest+backfill) for these questions — make the
  routing unambiguous and give the worked examples (hek_r → 4; organism musculus →
  word-membership incl. subspecies; column-selected cram export; sample-crams → 732;
  latest-for-sponsor; runs-per-month; PacBio-by-programme; study→users owners/managers/
  followers).
