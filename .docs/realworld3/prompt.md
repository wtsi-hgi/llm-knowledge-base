# Feature: complete MLWH MCP support for `wa` v0.8.0 / API 1.8.0

## Summary

Set the dependency to `github.com/wtsi-hgi/wa` **v0.8.0** and expose its complete
MLWH read surface through the MCP server. The package release and MLWH API use
different version lines: `wa` v0.8.0 exports
`mlwh.APIVersion == "1.8.0"`. Provider version metadata must therefore report
MLWH API 1.8.0.

The required API shape is:

- no manifest methods, types, endpoints, or MCP tools;
- `GET /export/:children/:parent_kind/:parent_id` as the generic relationship
  projection endpoint, including `products` of `study`;
- no `/export/count` endpoint or `CountExport` method; a bounded
  `ExportResult` carries `Total`, `Complete`, and, for keyset-backed relationships,
  `NextCursor` in its body;
- Export output as an ordered matrix, not a slice of arbitrary named objects:
  `ExportResult{Columns, Rows, Total, NextCursor, Complete, Format}` where each row
  is `[]string` aligned with `Columns`;
- products exports bounded to 1000 rows by default, including products without
  iRODS objects, and support keyset continuation. `file_type` changes only an
  attached product `irods_path`; it does not remove product rows or change
  `Total`;
- literal-prefix sample search, optioned iRODS listings/counts, latest-data
  pages/counts, global run listing/count and aggregates, programme enumeration,
  study→users, per-study sample CRAMs, and runs-for-sample.

The result must make the common real-world questions one cheap, correct MCP call:

- export a chosen-column TSV/CSV/JSON-shaped table of a study's products or files,
  including `manual_qc`, deliverability, iRODS paths, and merged-CRAM caveats;
- list every selected CRAM for a study without silently losing merged multi-lane
  objects;
- search literal prefixes such as `hek_r` correctly, with optional organism,
  library type, QC, and deliverability filters;
- find the newest data added to iRODS for a study or faculty sponsor;
- report runs per month and grouped sequencing by programme/platform/manufacturer;
- list runs for a sample, studies in a programme, or users and roles for a study.

Everything in this prompt is downstream MCP work. The `wa` v0.8.0 code is the
contract; do not add or change upstream MLWH behavior.

## Authority And Version Baseline

Use the checked-out `~/wa` code at tag `v0.8.0` as the only contract.

Authoritative files:

- `~/wa/mlwh/openapi.go`: `APIVersion == "1.8.0"` and generated schemas.
- `~/wa/mlwh/registry.go`: exact endpoint `Method`, `Path`, path/query params,
  descriptions, defaults, and response types.
- `~/wa/mlwh/queryer.go`: complete typed query surface.
- `~/wa/mlwh/remote.go`: exact `RemoteClient` methods, header-aware page helpers,
  option propagation, and export streaming behavior.
- `~/wa/mlwh/export.go`: export relationships, column vocabularies, defaults,
  validation, filters, pagination, and rendering methods.
- `~/wa/mlwh/types.go`, `count.go`, `runs_agg.go`: exact output shapes and option
  types.
- `~/wa/mlwh/server.go`: HTTP query parsing and export materialization.
- `~/wa/mlwh/search.go`, `hierarchy.go`, `availability.go`, `people.go`: exact
  behavior.
- `~/wa/mlwh/*_test.go`: executable edge-case and parity contract.
- `~/wa/.docs/mcp/api-reference.md`: generated human mirror of the Registry; useful
  for review, but code remains authoritative.

Do not treat `~/wa/.docs/realworld*`, `~/wa/.docs/nomanifest`, or phase/spec prompt
files as a contract. Implementation and tests must not query, sync, migrate, or
write either real database.

Update `go.mod`/`go.sum` to `github.com/wtsi-hgi/wa v0.8.0`. Provider version
reporting must use the compiled `wa.APIVersion`, and therefore report
MLWH API `1.8.0`.

## Definition Of Complete MCP Support

Complete support has two layers:

1. **Registry parity:** every entry in `wa.Registry` remains callable through
   `mlwh_call_endpoint`, and the endpoint catalogue/schema resources are generated
   from the v0.8.0 Registry/OpenAPI. This covers lower-frequency endpoints that
   do not need another curated tool.
2. **Curated workflows:** every capability in the table below has a dedicated
   typed tool, correct output schema, useful description, safe pagination, and
   workflow routing. Agents must not need
   `mlwh_call_endpoint` for the target questions in this prompt.

Do not add one MCP tool per Registry entry merely to duplicate the generic escape
hatch. Keep the existing consolidated tools (`mlwh_find_samples`, resolver tools,
detail tools, etc.) where they already cover a family correctly.

## Manifest Removal

`wa` v0.8.0 has no manifest API. Remove all downstream manifest code:

- remove `mlwh_study_manifest`;
- remove `mlwh_count_study_manifest`;
- remove `pagedStudyManifestResult`, manifest input types, schemas, registration,
  tests, and workflow guidance;
- remove references to `StudyManifest`, `PagedStudyManifest`, `ManifestRow`,
  `StudyManifestPage`, and `CountStudyManifest` so the v0.8.0 dependency compiles;
- ensure `mlwh_call_endpoint` and the endpoint catalogue do not advertise
  manifest Registry methods.

Use the product export for product-grained study rows:

```text
mlwh_export(
  children="products",
  parent_kind="study",
  parent_id=<study>,
  columns=[...],
  file_type=<optional attachment suffix>,
  deliverables_only=<optional product-row filter>,
  limit=<bounded page>,
  cursor=<continuation>
)
```

Do not add `mlwh_count_export`. Read `Total` from the bounded export response. Use
`mlwh_study_overview` or `mlwh_study_detail` for study metadata and
`mlwh_freshness` for cache as-of state.

## Curated Tool Matrix

Names below are firm unless an existing repository convention makes a narrowly
different name materially clearer. Registry method names and HTTP paths are firm.

| Concern | `wa` Registry methods | MCP work |
| --- | --- | --- |
| Generic projection | `Export` → `/export/:children/:parent_kind/:parent_id` | add `mlwh_export`; no count tool |
| Sample search | `SearchSamples`, `CountSampleSearch` | update `mlwh_search_samples` and `mlwh_count_samples` with `words`, `organism`, `library_type`, `qc`, `deliverables_only` |
| iRODS lists/counts | `IRODSPathsFor{Sample,Study,Run}`, `CountIRODSPathsFor{Sample,Study,Run}` | update all six existing tools with the v0.8.0 options and fields |
| Latest data | `LatestDataForStudy`, `CountLatestDataForStudy`, `LatestDataForFacultySponsor`, `CountLatestDataForFacultySponsor` | add four matching latest-data tools |
| Sample→runs | `RunsForSample`, `CountRunsForSample` | add `mlwh_runs_for_sample`, `mlwh_count_runs_for_sample` |
| Sample→studies count | `StudiesForSample`, `CountStudiesForSample` | retain `mlwh_studies_for_sample`; add `mlwh_count_studies_for_sample` |
| Global runs | `RunListing`, `CountRunListing` | add `mlwh_runs`, `mlwh_count_runs` |
| Monthly runs | `MonthlyRunCounts` | add `mlwh_monthly_run_counts` |
| Grouped sequencing | `SequencingAggregate` | add `mlwh_sequencing_aggregate` |
| Programme | `StudiesForProgramme`, `CountStudiesForProgramme`, `Programmes` | add `mlwh_studies_for_programme`, `mlwh_count_studies_for_programme`, `mlwh_programmes` |
| Study→users | `StudyUsers`, `CountStudyUsers` | add `mlwh_study_users`, `mlwh_count_study_users` |
| Per-sample CRAM | `SampleCRAMsForStudy`, `CountSampleCRAMsForStudy` | add `mlwh_sample_crams_for_study`, `mlwh_count_sample_crams_for_study` |
| Study aggregate | `StudyOverview` | update `mlwh_study_overview` for `programme` |
| Empty study status | `StatusBreakdown` | ensure `per_platform` is always `[]`, never `null` |
| Added-data windows | `SamplesWithData`, `CountSamplesWithData` | retain `since`/`until` behavior and verify it against v0.8.0 |

Every list with an upstream `Page[T]` helper keeps the repository's semantic
wrapper (`samples`, `studies`, `irods_paths`, `users`, etc.) plus `total` and
`next_offset`. Counts remain separate `{"count": N}` tools except for Export,
whose total is in the response body.

## `mlwh_export` Exact Contract

### Input

Map directly to `wa.ExportRelationship` and `wa.ExportOptions`:

- `children` (required string);
- `parent_kind` (required string);
- `parent_id` (required string);
- `columns` (optional ordered string array);
- `file_type` (optional string);
- `deliverables_only` (**optional pointer boolean**, not a plain boolean: omitted,
  explicit `false`, and explicit `true` have different semantics);
- `role` (optional comma-separated string);
- `qc` (optional `pass|fail|pending`);
- `library_type` (optional string);
- `organism` (optional string);
- `sort` (optional; `created-desc`/`created_desc`, iRODS only);
- `since`, `until` (optional RFC3339 iRODS-created window; `until` requires
  `since`);
- `limit`, `offset` (optional non-negative integers);
- `all` (optional boolean);
- `cursor` (optional opaque string);
- `format` (optional `tsv|csv|json`, default `tsv`).

Use `wa.ExportRelationshipDescriptions()` and `wa.ExportColumnVocabularies()` to
build descriptions/enums and keep the MCP input contract aligned with upstream.
Do not maintain a second hand-written validation implementation.

### Relationship Grammar And Columns

The complete export grammar is:

| `children` | allowed `parent_kind` | default columns | available columns |
| --- | --- | --- | --- |
| `products` | `study` | `name,supplier_name,accession_number,sanger_sample_id,id_run,lane,tag_index,manual_qc` | `name,supplier_name,accession_number,sanger_sample_id,id_run,lane,tag_index,manual_qc,irods_path,irods_unmatched,reason,id_study_lims,study_accession_number` |
| `sample-crams` | `study` | `name,accession_number,irods_path,merged` | all file columns below |
| `irods` (alias `files`) | `study`, `sample`, `run` | `supplier_name,sanger_sample_id,manual_qc,irods_path` | `supplier_name,sanger_sample_id,name,accession_number,study_accession_number,id_study_lims,manual_qc,id_run,lane,tag_index,platform,created,merged,deliverable,irods_path,id_product,id_sample_tmp,collection,data_object` |
| `samples` | `study`, `run`, `library` | `name,sanger_sample_id,supplier_name,accession_number` | `id_sample_tmp,id_lims,id_sample_lims,uuid_sample_lims,name,sanger_sample_id,supplier_name,accession_number,donor_id,taxon_id,common_name,description` |
| `runs` | `study`, `sample` | `id_run` | `id,native_id,id_run,platform,manufacturer,run_date,date_basis` |
| `libraries` | `study` | `pipeline_id_lims,library_id,id_library_lims` | `pipeline_id_lims,id_study_lims,library_id,id_library_lims` |
| `lanes` | `sample` | `id_run,lane,tag_index` | `id_run,lane,tag_index` |
| `studies` | `sample`, `faculty-sponsor`, `user`, `programme` | `id_study_lims,name,accession_number` | `id_study_lims,name,accession_number,study_title,faculty_sponsor,programme,role` |
| `users` | `study` | `role,name,login,email` | `role,name,login,email` |

Column aliases accepted upstream: `supplier_sample_name` → `supplier_name` and
`position` → `lane`. Do not add other aliases. Sample-CRAM columns use the
canonical names `name`, `accession_number`, and `irods_path`.

Parent resolution is upstream behavior: study, sample, run, and library parents
use their `wa` resolvers; faculty sponsor, user, and programme parent values use
the corresponding upstream matching semantics. Pass the caller's identifier to
`RemoteClient.Export`; do not resolve and fan out MCP-side.

### Output And Rendering

Return the `wa.ExportResult` shape and OpenAPI schema:

- `Columns []string`: canonical selected columns in output order;
- `Rows [][]string`: string values aligned positionally with `Columns`;
- `Total int`: total matching rows for bounded calls; `-1` for a complete
  streaming (`all=true`) result;
- `NextCursor string`: continuation for an incomplete iRODS/products page;
- `Complete bool`;
- `Format string`.

Do not represent rows as arbitrary named JSON objects. `format` is format metadata
on the HTTP response;
if MCP rendering is needed, use `ExportResult.RenderAs` rather than implementing
CSV/TSV escaping locally. Do not duplicate both a large row matrix and rendered
text in the default result.

`RemoteClient.Export(..., All: true)` returns a streaming result (`Rows == nil`,
unexported row iterator). An MCP handler cannot serialize that iterator. Drain it
through `ForEachRow`/`RenderAs` into an explicit MCP result before returning,
honor context cancellation, and let the global MCP result-size guard reject an
oversized result. Default workflows must page rather than set `all=true` for a
large study.

### Pagination

- Default bounded export limit is **1000**, not the MCP list default of 100.
- iRODS and products exports support opaque keyset `cursor` continuation and put
  the next cursor in `NextCursor` when incomplete.
- A created-desc iRODS export cannot use `cursor`; it uses bounded
  `limit`/`offset` pages.
- Samples, runs, libraries, lanes, studies, users, and sample-CRAM exports are
  offset-backed. They reject `cursor`; when `Complete == false`, continue with
  `offset + len(Rows)`.
- `offset` without an explicit `limit` is rejected unless `all=true`.
- `all=true` streams the complete set internally and reports `Total == -1`.
- There is no export count method or count endpoint. Never route agents to a
  nonexistent `mlwh_count_export`.

### Product Export

`children=products,parent_kind=study` has this contract:

- row grain is one distinct `(id_run, position/lane, tag_index)` product;
- products with no iRODS object remain in `Rows` and in `Total`;
- requesting no iRODS-derived column avoids the iRODS attachment join;
- requesting `irods_path`, `irods_unmatched`, or `reason` attaches at most one
  matching object per product;
- `file_type` filters only that attached path and never removes a product row or
  changes `Total`;
- `deliverables_only=true` filters product rows independently of attached iRODS,
  using Illumina `iseq_flowcell.entity_type IN ('library','library_indexed')`;
- `manual_qc` is the upstream product QC roll-up;
- a known direct-product gap caused by a merged multi-lane CRAM is represented as
  empty `irods_path`, `irods_unmatched=true`, `reason=merged_multilane`;
- products pagination is keyset-backed and default-bounded to 1000 rows.

Export does not repeat study name/sponsor and does not carry
`products_without_irods` or `cache_synced_at`. Do not add those fields. Use
row-level `irods_unmatched`/`reason`, `Total`, `mlwh_study_overview`, and
`mlwh_freshness` for those concerns.

### Shared Export Filters

The `qc`, `library_type`, `organism`, and `deliverables_only` family is backed only
for iRODS/files, samples, sample-CRAMs, and products. Passing one to runs,
libraries, lanes, studies, or users must preserve upstream's actionable
unsupported error; do not silently ignore it.

- `file_type`: filename suffix, case-insensitive, one leading dot stripped;
  empty/whitespace or `%`, `_`, `/` is a 400; valid unmatched suffix is an empty
  file result. For file/sample-CRAM exports omission defaults to CRAM. For
  products omission means any attached file type.
- `deliverables_only`: nil uses the relationship default. CRAM iRODS/files and
  sample-CRAM exports default true. Explicit false includes controls/sub-products.
  Product exports default unfiltered; explicit true changes the product row set.
  The discriminator approximates iRODS `target=1`, is not `is_spiked`, and is
  pass-through where PacBio/ONT have no discriminator.
- `qc`: `pass|fail|pending`; export is product-grained/raw product QC. This is
  intentionally different from sample search's per-sample roll-up.
- `library_type`: exact `library_samples.pipeline_id_lims` match.
- `organism`: whole-word membership resolved over `common_name`; it includes
  matching subspecies and never becomes a mid-word substring.
- `sort`, `since`, `until`: iRODS exports only, over iRODS `created` (data added),
  with `[since, until)` semantics and `until` requiring `since`.
- `role`: users-of-study omits to all roles; studies-of-user omits to the default
  `owner,manager,data_access_contact` set. Preserve that directional difference.

## Search Contract

Update `mlwh_search_samples` and `mlwh_count_samples` to accept the exact upstream
`wa.SampleSearchOptions` fields. Use `words` as the public boolean; do not expose
a `mode` parameter.

- Default free-text behavior is a case-insensitive literal whole-value prefix
  over `name`, `supplier_name`, `common_name`, and `donor_id`.
- `words=true` opts into separator-agnostic word-prefix behavior over those same
  fields. It is not the default.
- There is no contains/substring/n-gram mode. A mid-word fragment such as
  `usculus` must not be described as supported.
- `organism`, `library_type`, `qc`, and `deliverables_only` AND-combine with the
  term. The exact filters are exempt from the free-text three-character minimum;
  the MCP guard must not reject a short term when a valid exact filter makes the
  upstream request legal.
- `organism` is whole-word common-name membership, including subspecies;
  `library_type` is exact; `qc` uses the authoritative per-sample
  fail>pending>pass roll-up; deliverability uses the upstream discriminator and
  PacBio/ONT pass-through.
- The matching count tool must forward the same options. Preserve the upstream
  caveat that very common sample-search counts are exact up to the configured
  bound and become a floor at the bound.
- Keep `mlwh_find_samples` / `mlwh_count_find_samples` for exact controlled-field
  matches and resolver tools for exact identifiers.

For an optioned page, `RemoteClient` provides `SearchSamplesWithOptions` but no
`SearchSamplesWithOptionsPage`. Use
`CallWithHeaders("SearchSamples", ...)` (or an equivalently single-request,
Registry-driven helper) to preserve `X-Total-Count` and `X-Next-Offset`; do not
drop page metadata or issue a per-row fan-out.

Acceptance example: a default search for `hek_r` returns the four literal-prefix
samples, while `words=true` retains the broader word-prefix behavior.

## iRODS And Latest-Data Contract

### iRODS Lists And Counts

All three iRODS list tools and all three count tools accept:

- `file_type`;
- `deliverables_only`;
- `since` and `until` over `created`;
- list tools additionally accept `order_by=created_desc`;
- list tools keep bounded `limit`/`offset` and return page metadata.

The exact `IRODSPath` fields are:

`id_product`, `collection`, `data_object`, `irods_path`, `id_sample_tmp`, `name`,
`supplier_name`, `sanger_sample_id`, `accession_number`, `id_study_lims`,
`study_accession_number`, `created`, `id_run`, `lane`, `tag_index`, `platform`,
`merged`, `manual_qc`, and nullable `deliverable`.

`created` means data added to iRODS, UTC RFC3339. `merged=true` marks a composite
object; public `id_run`, `lane`, and `tag_index` are all zero because there is no
single product coordinate, while sample/study attribution remains populated.
`deliverable` is tri-state: true/false where a discriminator exists and null for
PacBio/ONT/no discriminator.

`RemoteClient` has optioned list methods but no optioned `Page` variants. Use
`CallWithHeaders` for optioned list tools so the exact filtered
request still returns `total` and `next_offset`. Count tools use the corresponding
`CountIRODSPathsFor*WithOptions` methods. Do not call a file-type-only Page helper
and accidentally drop deliverability, order, or date-window options.

### Latest Data

Add list and count tools for both latest-data families:

- study: `LatestDataForStudy` / `CountLatestDataForStudy`;
- faculty sponsor: `LatestDataForFacultySponsor` /
  `CountLatestDataForFacultySponsor`.

Lists return `RecentDataRow` pages newest-first with `created`, `irods_path`,
`id_study_lims`, `study_name`, sample `name`, `supplier_name`, `id_run`, `lane`,
`tag_index`, `platform`, and `merged`. They accept `file_type`, default to 10
rows, allow at most 1000, and use offset pagination with upstream header totals.
They are bounded pages, **not** "all ties at MAX(created)". Faculty sponsor is a
substring match on the `Study.faculty_sponsor` field, not a study-users role.

## Runs And Sequencing Aggregates

### Runs For Sample

Wrap `RunsForSamplePage` and `CountRunsForSample` with the normal semantic list
wrapper and `total`/`next_offset`.

Also add `mlwh_count_studies_for_sample` as the `CountStudiesForSample`
counterpart to `mlwh_studies_for_sample`.

### Global Run Listing

`mlwh_runs` wraps `RunListing` with `since`, `until`, repeatable/array `platform`,
`limit`, and `cursor`. A `RunListingRow` contains `id`, `platform`, `native_id`,
`manufacturer`, `run_date`, `date_basis`, and `cache_synced_at`.

The stable `id` is `<platform>:<native_id>` and is the keyset cursor. The HTTP
response is a bare list and has no `Page[T]` headers; do not invent
`next_offset`. Document that the next request uses the last returned row's `id`
as `cursor`. `mlwh_count_runs` wraps `CountRunListing` with the same date/platform
filters.

### Monthly Counts

`mlwh_monthly_run_counts` wraps `MonthlyRunCounts` with optional `since`, `until`,
and platform array. Rows are `{month, manufacturer, platform, count, date_basis,
cache_synced_at}`.

Run grain is one native run identifier: Illumina/Elembio/Ultimagen distinct
`id_run`, PacBio distinct `pac_bio_run_name`, ONT distinct `experiment_name`.
Date basis must remain visible: Illumina/Elembio run complete, Ultimagen run
archived, PacBio `run_complete`, ONT warehouse load time (not a true sequencing
date).

### General Aggregate

`mlwh_sequencing_aggregate` wraps `SequencingAggregate`. Inputs:

- required `group_by` array over `month`, `platform`, `manufacturer`, `programme`,
  `faculty_sponsor`;
- required `unit` = `runs|samples|products`;
- optional `since`, `until`, and platform array.

Rows are `SequencingAggregateRow{group, unit, count, date_basis,
cache_synced_at}`. `group` contains only requested keys. For `unit=runs`, dates
use the per-platform run basis and a run spanning multiple requested study groups
counts once in each group it touches. For samples/products, dates use iRODS
`created`, and each data row is attributed through its one study programme/sponsor.
Do not reconstruct this aggregate from lists in MCP.

## Programme, Study Users, And Sample CRAMs

- `mlwh_studies_for_programme`: exact programme match, bounded page of `Study`;
  `mlwh_count_studies_for_programme` is its exact count counterpart.
- `mlwh_programmes`: unpaged distinct non-empty programme vocabulary as
  `{name, study_count}` rows.
- `mlwh_study_overview`: expose the additive `programme` field.
- `mlwh_study_users`: page of `{role,name,login,email}`. Omitted role means all
  roles present. Optional role is a comma-separated exact case-insensitive set
  over `owner`, `manager`, `data_access_contact`, `follower`, `slf_manager`,
  `lab_manager`, `administrator`. `mlwh_count_study_users` must use the same
  filter. This differs from person→studies, whose omitted role uses only owner,
  manager, and data-access contact.
- `mlwh_sample_crams_for_study`: one selected CRAM per sample as
  `{name,accession_number,irods_path,merged}`, preferring a merged composite when
  present. Add the matching count tool and use canonical upstream field names.
  Do not infer sample-level CRAM absence from an empty product-level iRODS
  attachment.

## Time, Freshness, And Result Shapes

Never conflate:

1. `created` / `newest_data_added`: data added to iRODS and the basis for latest
   data and iRODS windows;
2. `last_changed`/`last_updated`: source row mutation, not new data;
3. `cache_synced_at` and `/freshness`: cache completeness/as-of state.

Monthly/aggregate rows carry `cache_synced_at`. Export, bare iRODS lists, latest
data, programmes, study users, and sample-CRAM responses do not; their tool
descriptions must direct callers to `mlwh_freshness`. Counts likewise have no
freshness timestamp.

Use upstream OpenAPI components for output schemas. Preserve semantic wrappers
for MCP list results rather than returning a root array. Assert
`StatusBreakdown.per_platform` is an empty array for an empty study, never null.

## MCP Hardening

- Keep `MLWH_MAX_TOOL_RESULT_BYTES` / `--mlwh-max-tool-result-bytes` and the
  default 1 MiB guard.
- Update size-error guidance for Export to use a smaller `limit`, `Total`, and
  `NextCursor`/offset continuation. Never mention `mlwh_count_export`.
- Keep normal MCP page defaults at 100/max 1000 where the curated list already
  owns pagination. Export is the exception: preserve upstream's bounded default
  of 1000 unless a deliberate smaller MCP default is documented and tested; do
  not turn omission into an unbounded call.
- Preserve explicit `deliverables_only=false` on Export by using a pointer bool.
- Never implement target/QC/organism/recency/merged/programme/run aggregation in
  MCP by fetching and joining rows. Call the upstream endpoint.
- Map upstream bad request, not found, ambiguity, unsupported identifier,
  impaired upstream, and never-synced errors through the existing structured
  tool-error path.
- `all=true` remains opt-in and subject to the result-size guard; workflow text
  must recommend paging for large results.
- Keep all tests hermetic with `httptest.Server`; no real source or mirror DB and
  no live `wa mlwh` server are needed.

## Workflow Guidance

Rewrite `mlwh://workflow` so agents route as follows:

- **Chosen-column table of study products, including products with no file:**
  `mlwh_export(children=products,parent_kind=study,...)`. Use `Total` and cursor
  pages. `file_type` only narrows an attached path.
- **Chosen-column table of actual files:**
  `mlwh_export(children=irods,parent_kind=study|sample|run,...)`; CRAM and
  deliverables-only are the default for omitted file options.
- **Every sample in a study with one CRAM:**
  `mlwh_sample_crams_for_study`; this is merged-aware and one row per selected
  sample CRAM.
- **Literal "starts with":** `mlwh_search_samples` with default options.
  Use `words=true` only for separator-agnostic word-prefix intent. There is no
  substring mode.
- **Organism/library/QC/deliverability constrained samples:** pass exact filters
  to sample search or a supported export relationship.
- **Newest data added:** latest-data study/faculty-sponsor tools or optioned iRODS
  `order_by=created_desc`; always say "added to iRODS".
- **Runs for a sample:** `mlwh_runs_for_sample`.
- **Runs per month:** `mlwh_monthly_run_counts`; use `mlwh_runs` for global
  keyset-paged drill-down and `mlwh_count_runs` for sizing.
- **Sequencing by programme/sponsor/platform/manufacturer/month:**
  `mlwh_sequencing_aggregate`; do not fan out over studies.
- **Studies in a programme:** `mlwh_studies_for_programme`; discover exact values
  with `mlwh_programmes`.
- **Study owners/managers/followers:** `mlwh_study_users`; faculty sponsor is a
  separate Study field.
- **Generic fallback:** `mlwh_call_endpoint` only when no curated workflow covers
  the question. It must advertise the v0.8.0 Registry and no manifest entry.

## Hard Requirements

1. Build against `github.com/wtsi-hgi/wa v0.8.0`; report MLWH API `1.8.0` from
   `wa.APIVersion`.
2. Remove both manifest MCP tools and every reference to manifest methods/types.
3. Add one `mlwh_export` tool matching the complete relationship grammar,
   columns, filters, tri-state deliverability, formats, `all`, and pagination.
4. Do not add `mlwh_count_export`; bounded Export `Total` is the sizing contract.
5. Product export preserves product grain and products without iRODS, with
   row-level `irods_unmatched/reason` and correct file-type/deliverability
   semantics.
6. Every method in the curated tool matrix is directly usable via
   a typed MCP tool with the exact upstream options and output fields.
7. Optioned sample-search and iRODS list pages preserve upstream sizing headers in
   one request; no missing `total`/`next_offset` and no list+per-row fan-out.
8. Registry/OpenAPI/catalogue parity covers the complete v0.8.0 Registry, with
   Export present and manifest absent.
9. Correct wording and caveats for iRODS `created`, run `date_basis`, aggregate
   `unit`, merged composites, role defaults, and cache freshness.
10. Bounded defaults, context cancellation, size guard, and actionable errors work
    for Export and every new list/count tool.
11. All tests are hermetic and assert exact path/query propagation, response
    shapes, pagination metadata, descriptions, workflow routing, errors, and size
    behavior.

## Required Acceptance Coverage

Extend the shared MLWH `httptest` harness and add behavior-focused tests for:

- dependency/provider version: `wa.APIVersion == "1.8.0"` and server version
  metadata reports it;
- manifest tools are absent; `StudyManifest`/`CountStudyManifest` are absent from
  the endpoint catalogue;
- `mlwh_export` registration and schema, including generated relationship/column
  vocabularies and optional pointer `deliverables_only`;
- table-driven request propagation for every export relationship and every export
  option;
- exact `ExportResult` matrix shape, canonical columns, `Total`, `Complete`,
  `NextCursor`, and `Format`;
- products default 1000 bounded page, cursor continuation, products without iRODS,
  `file_type` attachment-only behavior, deliverables product filtering,
  `manual_qc`, and merged gap reason;
- offset-backed export continuation and rejection of cursor on unsupported
  relationships;
- created-desc iRODS export rejecting cursor; `all=true` materialization,
  cancellation, and over-budget MCP error;
- sample search default vs `words=true`, exact filters, short filtered term,
  optioned count parity, and header-aware page metadata;
- all iRODS list/count options and every additive `IRODSPath` field, including
  nullable `deliverable` and merged zero coordinates;
- latest-data study/sponsor list and count paths, default/newest-first page
  behavior, and `file_type`;
- runs-for-sample list/count, studies-for-sample count, global run
  cursor/filter/count, monthly counts, and sequencing aggregate
  `group_by`/`unit`/platform/date options;
- programme list/count/vocabulary and `StudyOverview.programme`;
- study-users all-role default vs explicit role set and matching count;
- sample-CRAM canonical shape, merged preference, and matching count;
- empty-study `StatusBreakdown.per_platform == []`;
- Registry parity for `mlwh_call_endpoint`, endpoint catalogue, and OpenAPI-backed
  schemas, with Export present and no stale manifest methods;
- workflow text choosing curated tools for every target question and never naming
  `mlwh_count_export` or a manifest tool.

## Repo Pointers

- Dependency and provider version: `go.mod`, `go.sum`,
  `internal/mlwh/provider.go`.
- Tool registration: `internal/mlwh/provider.go` and `register*Tools` helpers.
- Manifest registration to remove: `internal/mlwh/tools_availability.go`; create a
  focused export file if that keeps registration clearer.
- Search: `internal/mlwh/tools_search.go`.
- Hierarchy/detail: `internal/mlwh/tools_detail.go`.
- iRODS/availability/latest data: `internal/mlwh/tools_availability.go`.
- Runs and aggregates: add a focused tools file rather than overloading overview.
- Programme and study users: `internal/mlwh/tools_people.go` or focused files.
- Output schemas: `internal/mlwh/schema.go`, sourced from
  `wa.OpenAPIDocument()`.
- Generic Registry fallback: `internal/mlwh/tools_call.go`.
- Workflow: `internal/mlwh/workflow.go`.
- Freshness/errors: `internal/mlwh/tools_freshness.go`,
  `internal/mlwh/errmap.go`.
- Size guard: `internal/core/`.
- Hermetic server harness: `internal/mlwh/harness_test.go`.
- Broader MCP conventions: `../mcp/spec.md`.

## Out Of Scope

- Upstream `wa` changes, new MLWH endpoints, cache schema changes, or API version
  bumps.
- Reintroducing a compatibility manifest endpoint/tool or a synthetic export
  count endpoint.
- Database sync/migration or any write to the real source/mirror databases.
- MCP-side SQL, joins, QC roll-ups, deliverability rules, recency aggregation,
  merged attribution, or sequencing aggregation.
- Web UI work or changing the MLWH HTTP server.

## Final Review Checklist

- `go.mod` targets `wa v0.8.0`; provider API version is 1.8.0.
- Manifest tools/types/method names are gone everywhere.
- Export is present, exact, and bounded.
- No `mlwh_count_export` exists or is mentioned in workflow guidance.
- All methods in the curated tool matrix have exact tools and options.
- Generic Registry fallback/catalogue covers all v0.8.0 endpoints.
- Workflow routes target questions to one cheap curated call.
- Tests are hermetic and pass without touching either real database.
