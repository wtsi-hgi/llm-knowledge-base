# Complete MLWH MCP Support for wa v0.8.0 Specification

## Overview

Upgrade the MCP server to `github.com/wtsi-hgi/wa v0.8.0`, whose compiled
MLWH API version is `1.8.0`. Keep the MCP provider a thin, read-only adapter:
typed tools call the upstream `wa/mlwh` remote client, while
`mlwh_call_endpoint` preserves complete Registry coverage.

Remove the obsolete manifest surface. Add a bounded matrix export and typed
tools for optioned search, iRODS, latest data, runs, aggregates, programmes,
study users, and sample CRAMs. Preserve upstream row grain, filters,
pagination, errors, time meanings, and cache-freshness caveats.

## Architecture

### Authority and dependency

- `go.mod` and `go.sum` require `github.com/wtsi-hgi/wa v0.8.0`.
- `wa.APIVersion` is the only provider-version source and equals `1.8.0`.
- `wa.Registry`, `wa.OpenAPIDocument()`,
  `wa.ExportRelationshipDescriptions()`, and
  `wa.ExportColumnVocabularies()` are runtime sources of truth.
- No implementation or test connects to a real source or mirror database.

The public upstream calls used by this feature keep these signatures:

```go
func (*wa.RemoteClient) CallWithHeaders(
    context.Context, string, []string, url.Values,
) (any, http.Header, error)

func (*wa.RemoteClient) Export(
    context.Context, wa.ExportRelationship, string, wa.ExportOptions,
) (wa.ExportResult, error)
```

Other typed calls use their exact `wa v0.8.0` signatures. MCP code must not
join, aggregate, resolve fan-outs, or reproduce upstream validation.

### Files

```text
go.mod, go.sum                       wa dependency
README.md                            public workflows and tool catalogue
cmd/mlwh-mcp-server/readme_test.go   public documentation regression tests
internal/mlwh/provider.go            version, registration, size guidance
internal/mlwh/schema.go              OpenAPI schemas and semantic wrappers
internal/mlwh/tools_export.go        generic export tool
internal/mlwh/tools_search.go        optioned sample search/count
internal/mlwh/tools_availability.go  iRODS, latest data, sample CRAMs
internal/mlwh/tools_detail.go        sample runs/studies counts
internal/mlwh/tools_runs.go          global runs and aggregates
internal/mlwh/tools_people.go        programmes and study users
internal/mlwh/tools_overview.go      programme/status compatibility
internal/mlwh/tools_call.go          Registry escape hatch
internal/mlwh/workflow.go            workflow routing and catalogue
internal/mlwh/harness_test.go        hermetic header-aware HTTP stub
```

Focused test files mirror production files. Existing files may be split when
needed, but `cmd/` remains wiring-only and `internal/core/` remains
service-agnostic.

### MCP list and count shapes

All typed list results are objects. Header-paged lists add exact upstream page
metadata:

```json
{"<semantic_plural>": [], "total": 0, "next_offset": -1}
```

Semantic fields are `samples`, `studies`, `runs`, `irods_paths`,
`latest_data`, `users`, and `sample_crams`. Typed list results without
header-based page metadata use `runs`, `monthly_run_counts`, `aggregates`, or
`programmes`, as appropriate. Global run listing is a keyset page and
therefore has neither `total` nor `next_offset`. Counts remain the upstream
`{"count": N}` object.

Curated offset lists default to 100 rows and reject limits above 1000, except
latest-data lists, which preserve the upstream default of 10. Export preserves
its upstream default of 1000. Every offset must be non-negative.

For optioned sample search and iRODS lists, call `CallWithHeaders` once, assert
the Registry result is the expected pointer-to-slice type, and parse
`X-Total-Count` and `X-Next-Offset`. Missing or invalid headers yield `total=0`
and `next_offset=-1`, matching `wa.Page[T]`. Do not make a count request or a
per-row request to construct a page.

### Export contract

`mlwh_export` has this public input:

```go
type exportInput struct {
    Children         string   `json:"children"`
    ParentKind       string   `json:"parent_kind"`
    ParentID         string   `json:"parent_id"`
    Columns          []string `json:"columns,omitempty"`
    FileType         string   `json:"file_type,omitempty"`
    DeliverablesOnly *bool    `json:"deliverables_only,omitempty"`
    Role             string   `json:"role,omitempty"`
    QC               string   `json:"qc,omitempty"`
    LibraryType      string   `json:"library_type,omitempty"`
    Organism         string   `json:"organism,omitempty"`
    Sort             string   `json:"sort,omitempty"`
    Since            string   `json:"since,omitempty"`
    Until            string   `json:"until,omitempty"`
    Limit            int      `json:"limit,omitempty"`
    Offset           int      `json:"offset,omitempty"`
    All              bool     `json:"all,omitempty"`
    Cursor           string   `json:"cursor,omitempty"`
    Format           string   `json:"format,omitempty"`
}
```

`deliverables_only` is a pointer so omission, false, and true remain distinct.
`sort` accepts `created-desc` and `created_desc`; `format` accepts `tsv`,
`csv`, and `json`, defaulting to `tsv`. The input schema derives child,
parent, and column enums and descriptions from upstream export vocabulary
functions. Upstream `RemoteClient.Export` performs relationship and option
validation.

The complete relationship grammar and default columns are:

- `products` of `study`:
  `name,supplier_name,accession_number,sanger_sample_id,id_run,lane,`
  `tag_index,manual_qc`.
- `sample-crams` of `study`:
  `name,accession_number,irods_path,merged`.
- `irods` (alias `files`) of `study`, `sample`, or `run`:
  `supplier_name,sanger_sample_id,manual_qc,irods_path`.
- `samples` of `study`, `run`, or `library`:
  `name,sanger_sample_id,supplier_name,accession_number`.
- `runs` of `study` or `sample`: `id_run`.
- `libraries` of `study`:
  `pipeline_id_lims,library_id,id_library_lims`.
- `lanes` of `sample`: `id_run,lane,tag_index`.
- `studies` of `sample`, `faculty-sponsor`, `user`, or `programme`:
  `id_study_lims,name,accession_number`.
- `users` of `study`: `role,name,login,email`.

Available columns are the exact arrays returned by
`wa.ExportColumnVocabularies()`. Accepted aliases are only
`supplier_sample_name -> supplier_name`, `position -> lane`, and
`files -> irods`. The result is the OpenAPI `wa.ExportResult` matrix:

```text
Columns []string
Rows [][]string
Total int
NextCursor string
Complete bool
Format string
```

JSON field names are the upstream wire names shown above. Rows align
positionally with canonical `Columns`; no named-object row projection or
additional rendered field is added.

Bounded product pages and iRODS pages without created-desc sorting use
`NextCursor`. Created-desc iRODS and all other relationships continue by
`offset + len(Rows)`. Offset-backed relationships reject cursors. `offset`
without an explicit limit is invalid unless `all=true`. `all=true` returns
`Total=-1`, `Complete=true`, and an empty cursor after the handler drains
`ForEachRow` into `Rows` with the request context. The core result-size guard
then applies.

### Shared option semantics

- iRODS list/count inputs: `file_type`, `deliverables_only`, `since`, and
  `until`; lists also accept `order_by=created_desc`, `limit`, and `offset`.
- Sample search inputs: `term`, `words`, `organism`, `library_type`, `qc`,
  `deliverables_only`, `limit`, and `offset`; the count omits pagination.
- Run filters: `since`, `until`, and array `platform`; run listing adds
  `limit` and `cursor`.
- Sequencing aggregate adds required array `group_by` and required `unit`.
- Date windows are half-open. `until` requires `since`.
- iRODS `created` means data added to iRODS, never source mutation or cache
  sync time.

MCP passes values to the corresponding upstream methods. Unsupported export
filters, invalid enums, invalid suffixes, malformed timestamps, ambiguity,
not-found, impaired-upstream, and never-synced conditions use the existing
structured tool-error path.

### Curated input keys

Public MCP JSON keys are exact:

- Sample search uses `term`, `words`, `organism`, `library_type`, `qc`,
  `deliverables_only`, `limit`, and `offset`; its count omits the last two.
- Sample, study, and run iRODS tools use `sanger_name`, `study_lims_id`, or
  `id_run`, respectively, plus the shared iRODS options above.
- Study latest-data uses `study_lims_id`; sponsor latest-data uses
  `faculty_sponsor`; list tools add `file_type`, `limit`, and `offset`, while
  counts accept only the identifier and `file_type`.
- Sample run/study tools use `sanger_name`; run lists add `limit` and `offset`.
- Global runs and monthly counts use `since`, `until`, and array `platform`;
  global runs add `limit` and `cursor`.
- Sequencing aggregate uses required array `group_by`, required `unit`, and
  optional `since`, `until`, and array `platform`.
- Programme tools use `programme`; programme vocabulary takes `{}`.
- Study-user tools use `study_lims_id` and optional `role`; the list adds
  `limit` and `offset`.
- Sample-CRAM tools use `study_lims_id`; the list adds `limit` and `offset`.

## A. Version, Removal, and Registry Parity

### A1: Upgrade the contract and remove manifests

As an operator, I want the provider compiled against MLWH API 1.8.0, so that
its advertised surface matches the deployed server.

Set `wa v0.8.0`; continue returning `wa.APIVersion` from the provider. Remove
`mlwh_study_manifest`, `mlwh_count_study_manifest`, their input/result types,
schemas, registration, tests, workflow text, and all downstream references to
removed manifest APIs. Do not add compatibility replacements.

**Package:** `internal/mlwh/`, `cmd/mlwh-mcp-server/`
**File:** `go.mod`, `go.sum`, `internal/mlwh/provider.go`,
`internal/mlwh/tools_availability.go`
**Test file:** `internal/mlwh/provider_test.go`,
`cmd/mlwh-mcp-server/main_test.go`

**Acceptance tests:**

1. Given the compiled provider, when versions are read through provider
   metadata, server instructions, version resource, `--version`, and startup
   logging, then each MLWH API value equals `wa.APIVersion == "1.8.0"`.
2. Given listed tools, when names are inspected, then neither manifest tool is
   present and every other existing supported tool remains present.
3. Given `mlwh://workflow` and its Registry catalogue, when read, then they
   contain neither `StudyManifest`, `CountStudyManifest`, nor `/manifest`.
4. Given repository Go sources, when compiled against `wa v0.8.0`, then no
   downstream manifest method or type reference remains.

### A2: Preserve complete Registry and OpenAPI parity

As an agent, I want every `wa.Registry` entry callable, so that curated tools
do not hide less-common reads.

Keep `mlwh_call_endpoint` Registry-driven. Build the endpoint catalogue from
`wa.EndpointReference()` and typed output schemas from
`wa.OpenAPIDocument()`. The v0.8.0 Registry has 90 methods, includes `Export`,
and excludes manifest methods. Do not add a curated tool for every entry.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_call.go`, `internal/mlwh/schema.go`,
`internal/mlwh/workflow.go`
**Test file:** `internal/mlwh/tools_call_test.go`,
`internal/mlwh/schema_test.go`, `internal/mlwh/workflow_test.go`

**Acceptance tests:**

1. Given every v0.8.0 Registry entry, when its method is compared with the
   generic call catalogue, then all 90 are advertised and dispatchable.
2. Given Registry `Export`, when called generically with path parameters
   `products,study,S1`, then the stub receives
   `/export/products/study/S1` and the matrix response is returned.
3. Given OpenAPI components for every new result type, when MCP schemas are
   built, then all `$ref`s are resolved, field descriptions are retained, and
   each typed output schema is an object.
4. Given Registry and catalogue methods, when inspected, then `Export` is
   present and manifest names are absent.

## B. Generic Export

### B1: Register an exact generated export input

As an agent, I want one `mlwh_export` tool, so that I can request a
chosen-column relationship table in one call.

Register the `exportInput` contract from Architecture. Generate child/parent
and column descriptions and enums from the two upstream vocabulary functions.
Map the input directly to `wa.ExportRelationship` and `wa.ExportOptions`.
There is no `mlwh_count_export`; `Total` sizes bounded calls.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_export.go`, `internal/mlwh/provider.go`,
`internal/mlwh/schema.go`
**Test file:** `internal/mlwh/tools_export_test.go`

**Acceptance tests:**

1. Given registered tools, when `mlwh_export` is inspected, then its required
   fields are `children`, `parent_kind`, and `parent_id`; every optional field
   in `exportInput` is present; and `mlwh_count_export` is absent.
2. Given upstream relationship descriptions and column vocabularies, when the
   input schema is inspected, then its child aliases, allowed parent kinds,
   defaults, canonical columns, and column aliases exactly match upstream.
3. Given omitted, false, and true `deliverables_only`, when three requests are
   recorded, then the first omits the query parameter and the others send
   `false` and `true`, respectively.
4. Given table-driven calls covering every relationship and every option, when
   invoked, then the stub receives the exact v0.8.0 path and only the supplied
   query values, with ordered columns encoded as one comma-separated value.
5. Given an upstream validation error for a relationship, column, filter, or
   parent, when returned, then the existing structured error mapping preserves
   its actionable message.

### B2: Return bounded matrices and materialize streams

As an agent, I want explicit export continuation metadata, so that large
tables are safe and complete.

Return `wa.ExportResult` unchanged for bounded calls. For `all=true`, consume
the upstream iterator with `ForEachRow`, copy each row into `Rows`, and stop on
context cancellation or iteration error. Do not add rendered CSV/TSV text to
the structured result.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_export.go`
**Test file:** `internal/mlwh/tools_export_test.go`,
`internal/core/server_test.go`

**Acceptance tests:**

1. Given columns `name,irods_path` and rows `[["S1","/a.cram"]]`, when the
   tool returns, then structured content has exactly `Columns`, `Rows`,
   `Total`, `NextCursor`, `Complete`, and `Format`, with row values aligned to
   the two canonical columns.
2. Given an omitted limit for a product export, when called, then the upstream
   request remains bounded at the 1000-row default and never sets `all=true`.
3. Given an incomplete keyset page, when returned, then `Complete=false` and
   the exact opaque `NextCursor` are preserved for the next request.
4. Given an incomplete offset-backed page of 40 rows at offset 100, when
   returned, then workflow and tool descriptions direct the next request to
   offset 140 and never to a cursor.
5. Given a cursor on samples or a created-desc iRODS export, when invoked, then
   the result is a mapped upstream unsupported-input tool error.
6. Given `all=true` with three streamed rows, when invoked, then `Rows` has all
   three rows, `Total=-1`, `Complete=true`, and `NextCursor` is empty.
7. Given cancellation during streamed materialization, when the request context
   ends, then the tool returns a cancellation error and no successful partial
   matrix.
8. Given a materialized result above `MLWH_MAX_TOOL_RESULT_BYTES`, when the
   core guard runs, then the tool error recommends a smaller limit and
   `Total` plus cursor/offset continuation, without naming a count-export tool.

### B3: Preserve product and shared-filter semantics

As an agent, I want product exports to retain product grain, so that missing
or merged files do not silently remove sequencing products.

Products are distinct `(id_run,lane,tag_index)` rows. Without iRODS-derived
columns, no attachment join is required. With `irods_path`,
`irods_unmatched`, or `reason`, attach at most one object per product.
`file_type` changes only that attachment. Default products are unfiltered;
explicit true deliverability filters product rows. Shared export filters are
supported only for iRODS/files, samples, sample-CRAMs, and products. Export
never adds study name/sponsor, `products_without_irods`, or `cache_synced_at`;
callers use overview, row reasons, and freshness instead.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_export.go`
**Test file:** `internal/mlwh/tools_export_test.go`

**Acceptance tests:**

1. Given three study products and only two matching iRODS objects, when
   products are exported, then `Rows` and `Total` both describe all three
   products; the unmatched row has an empty path.
2. Given the same products and `file_type=bam` with no BAM attachment, when
   exported, then all three product rows and the same `Total` remain and all
   attached paths are empty.
3. Given one control and one deliverable product, when `deliverables_only` is
   omitted, false, and true, then upstream results contain 2, 2, and 1 product
   rows, respectively.
4. Given product QC values, when `manual_qc` is selected or `qc=fail` is used,
   then the exact upstream product roll-up values and row filter are preserved.
5. Given a product represented only by a merged multi-lane CRAM, when selected,
   then its row has empty `irods_path`, `irods_unmatched=true`, and
   `reason=merged_multilane`.
6. Given iRODS/sample-CRAM omission of `file_type` and
   `deliverables_only`, when exported, then CRAM and deliverables-only defaults
   apply; explicit false includes controls/sub-products.
7. Given valid `qc`, `library_type`, `organism`, date, sort, role, and file
   filters, when forwarded, then upstream semantics are unchanged; given an
   unsupported relationship/filter pair, then the filter is rejected rather
   than ignored.

## C. Search, iRODS, and Latest Data

### C1: Support literal-prefix sample search options

As an agent, I want filtered literal-prefix search, so that terms such as
`hek_r` mean starts-with rather than substring.

Extend `mlwh_search_samples` and `mlwh_count_samples` with `words`,
`organism`, `library_type`, `qc`, and `deliverables_only`. The list also keeps
`limit` and `offset`. Use `words`, never `mode`. Default search is a
case-insensitive literal whole-value prefix over `name`, `supplier_name`,
`common_name`, and `donor_id`; `words=true` opts into separator-agnostic
word-prefix matching. Exact filters AND-combine and allow a term shorter than
three characters. Organism is whole-word common-name membership including
matching subspecies; library type is exact; QC is the sample roll-up
`fail>pending>pass`; deliverability uses the upstream discriminator, not
`is_spiked`, with PacBio/ONT pass-through. Counts forward identical options
and retain the configured exact-count floor caveat.

Use one `CallWithHeaders("SearchSamples", ...)` for optioned list calls. A call
without options may use the existing page helper.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_search.go`, `internal/mlwh/schema.go`
**Test file:** `internal/mlwh/tools_search_test.go`

**Acceptance tests:**

1. Given stub rows for four names beginning `hek_r`, when default search uses
   `term=hek_r`, then exactly those four rows are returned with the stub's
   `total` and `next_offset` headers.
2. Given the same term with `words=true`, when invoked, then `words=true` is
   sent and the broader upstream word-prefix rows are returned.
3. Given each exact filter and all filters together, when list and count tools
   are invoked, then `organism`, `library_type`, `qc`, and
   `deliverables_only=true` are propagated identically.
4. Given a two-character term plus a valid exact filter, when invoked, then no
   local minimum-length guard rejects it and the single upstream request runs.
5. Given a two-character term with no exact filter, when invoked, then the
   existing three-character error is returned before HTTP.
6. Given optioned list response headers `17` and `10`, when returned, then the
   wrapper is `{"samples":[...],"total":17,"next_offset":10}` and only one
   HTTP request was made.
7. Tool descriptions state literal prefix is default, `words=true` is opt-in,
   mid-word substring is unsupported, organism is whole-word, library type is
   exact, QC is sample-level `fail>pending>pass`, and counts may be a floor.

### C2: Preserve optioned iRODS pages and fields

As an agent, I want filterable iRODS listings and matching counts, so that I
can page actual data objects without losing metadata.

Update all sample, study, and run iRODS list/count tools with shared iRODS
options. The lists are `mlwh_irods_paths_for_sample`,
`mlwh_irods_paths_for_study`, and `mlwh_irods_paths_for_run`; their counts are
`mlwh_count_irods_paths_for_sample`,
`mlwh_count_irods_paths_for_study`, and
`mlwh_count_irods_paths_for_run`. Lists call `CallWithHeaders` once so all
filters and page headers come from one request. Counts call the corresponding
`CountIRODSPathsFor*WithOptions` method. `order_by` is list-only.

Output rows are exact `wa.IRODSPath` values, including `id_product`,
`collection`, `data_object`, `irods_path`, `id_sample_tmp`, `name`,
`supplier_name`, `sanger_sample_id`, `accession_number`, `id_study_lims`,
`study_accession_number`, `created`, `id_run`, `lane`, `tag_index`, `platform`,
`merged`, `manual_qc`, and nullable `deliverable`.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_availability.go`, `internal/mlwh/schema.go`
**Test file:** `internal/mlwh/tools_availability_test.go`

**Acceptance tests:**

1. Given table-driven sample, study, and run calls with all list options, when
   invoked, then each exact path carries `file_type`,
   `deliverables_only=true`, `order_by=created_desc`, `since`, `until`,
   `limit`, and `offset`; one HTTP request is recorded.
2. Given matching count calls, when invoked, then all filters except
   `order_by`, `limit`, and `offset` are sent to the correct `/count` path.
3. Given headers `total=23` and `next=20`, when returned, then each list wrapper
   has the semantic `irods_paths` array, `total=23`, and `next_offset=20`.
4. Given one ordinary and one merged row, when decoded, then every listed field
   is preserved; the merged row has `merged=true` and zero `id_run`, `lane`,
   and `tag_index`.
5. Given discriminator-backed true/false rows and a pass-through row, when
   decoded, then `deliverable` is true, false, and null, respectively.
6. Given `.CRAM`, a valid unmatched suffix, and each invalid suffix class, when
   forwarded, then upstream normalization yields the expected rows, empty
   result, or mapped 400 error without MCP-side suffix validation.
7. Descriptions define `created` as data-added time, deliverability as an
   approximation of target and not `is_spiked`, and direct callers to
   `mlwh_freshness` for cache as-of state.

### C3: Add latest-data study and sponsor tools

As an agent, I want newest-first data pages, so that I can answer what was
recently added to iRODS.

Add `mlwh_latest_data_for_study`, `mlwh_count_latest_data_for_study`,
`mlwh_latest_data_for_faculty_sponsor`, and
`mlwh_count_latest_data_for_faculty_sponsor`. Lists use upstream page helpers,
default to 10, reject limits above 1000, and return `latest_data`, `total`, and
`next_offset`. Faculty sponsor is a substring of the Study field, not a
study-users role. `file_type` is optional on all four tools. Call
`LatestDataForStudyPage`, `LatestDataForFacultySponsorPage`,
`CountLatestDataForStudy`, and `CountLatestDataForFacultySponsor` directly.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_availability.go`, `internal/mlwh/schema.go`
**Test file:** `internal/mlwh/tools_availability_test.go`

**Acceptance tests:**

1. Given a study call without pagination, when invoked, then the stub receives
   `/study/S1/latest-data?limit=10&offset=0` and the newest row is first.
2. Given a sponsor call with `file_type=cram`, limit 20, and offset 40, when
   invoked, then the stub receives
   `/latest-data/faculty-sponsor/Ada` with those exact query values.
3. Given list response headers, when returned, then the semantic wrapper
   preserves `total` and `next_offset`; each `RecentDataRow` exposes all exact
   upstream fields, including `created`, `irods_path`, coordinates, platform,
   and `merged`.
4. Given matching count calls, when invoked, then the exact study and sponsor
   `/count` paths receive the same `file_type` and return `{"count":N}`.
5. Descriptions say the result is a bounded page, not every row tied for the
   maximum `created` value; `created` is data-added time; and freshness comes
   from `mlwh_freshness`.

## D. Runs and Sequencing Aggregates

### D1: Add sample run and sample study counts

As an agent, I want runs for a sample and matching counts, so that I can size
and inspect its sequencing history.

Add `mlwh_runs_for_sample`, `mlwh_count_runs_for_sample`, and
`mlwh_count_studies_for_sample`. Keep `mlwh_studies_for_sample`. The run list
uses `RunsForSamplePage` and the normal bounded semantic wrapper.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_detail.go`, `internal/mlwh/schema.go`
**Test file:** `internal/mlwh/tools_detail_test.go`

**Acceptance tests:**

1. Given `sanger_name=S1`, limit 25, and offset 50, when runs are requested,
   then `/sample/S1/runs` receives those values and returns `runs`, `total`,
   and `next_offset` exactly.
2. Given the two count tools, when invoked, then
   `/sample/S1/runs/count` and `/sample/S1/studies/count` return their exact
   `wa.Count` objects.
3. Given not-found and never-synced responses, when returned, then existing
   structured error precedence remains unchanged.

### D2: Add global run listing and count

As an agent, I want a global keyset run page, so that I can drill into runs
across all platforms without offset drift.

`mlwh_runs` accepts optional `since`, `until`, platform array, `limit`, and
`cursor`; it wraps `RunListing` rows under `runs`. Default limit is 100 and
maximum is 1000. `mlwh_count_runs` accepts the same date/platform filters.
Each row is exact `wa.RunListingRow`.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_runs.go`, `internal/mlwh/schema.go`,
`internal/mlwh/provider.go`
**Test file:** `internal/mlwh/tools_runs_test.go`

**Acceptance tests:**

1. Given two platform filters, date bounds, limit 50, and cursor
   `pacbio:r2`, when invoked, then `/runs` receives repeated `platform`
   parameters and the exact other values.
2. Given a row, when returned, then `id`, `platform`, `native_id`,
   `manufacturer`, `run_date`, `date_basis`, and `cache_synced_at` are
   preserved under `runs`; no `total` or `next_offset` is invented.
3. Given the last row id `ont:e9`, when continuation guidance is read, then it
   says to pass that id as the next cursor.
4. Given the count tool and identical filters, when invoked, then
   `/runs/count` receives those filters and returns the exact count.
5. Descriptions define run grain by platform and label ONT warehouse load time
   as not a true sequencing date.

### D3: Add monthly and general aggregates

As an agent, I want server-side run and sequencing aggregates, so that grouping
does not require study fan-out.

`mlwh_monthly_run_counts` accepts optional run filters and returns
`monthly_run_counts`. `mlwh_sequencing_aggregate` requires `group_by` array and
`unit=runs|samples|products`, accepts optional run filters, and returns
`aggregates`. Call `MonthlyRunCounts` or `SequencingAggregate` exactly once.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_runs.go`, `internal/mlwh/schema.go`
**Test file:** `internal/mlwh/tools_runs_test.go`

**Acceptance tests:**

1. Given monthly bounds and platform filters, when invoked, then
   `/runs/monthly` receives exact repeated platform values and returns rows
   with `month`, `manufacturer`, `platform`, `count`, `date_basis`, and
   `cache_synced_at`.
2. Given `group_by=[month,programme,platform]`, `unit=products`, bounds, and
   platforms, when invoked, then `/sequencing/aggregate` receives repeated
   `group_by` and `platform` values plus the exact scalar options.
3. Given an aggregate row, when returned, then `group` contains only requested
   keys and `unit`, `count`, `date_basis`, and `cache_synced_at` are unchanged.
4. Given an invalid or missing `group_by` or `unit`, when invoked, then the
   result is an actionable tool error and no MCP-side aggregate is attempted.
5. Descriptions state runs use per-platform run dates, samples/products use
   iRODS `created`, and a run touching multiple study groups counts once in
   each group.

## E. Programme, Study Users, and Sample CRAMs

### E1: Add programme discovery and study listing

As an agent, I want programme vocabulary and exact programme membership, so
that I can use the same grouping values as sequencing aggregates.

Add `mlwh_studies_for_programme`, `mlwh_count_studies_for_programme`, and
`mlwh_programmes`. The study list is an exact programme match and a normal
page. Programmes are an unpaged
`{"programmes":[{"name":...,"study_count":...}]}` result. Existing
`mlwh_study_overview` exposes upstream `programme` through its OpenAPI schema.
Call `StudiesForProgrammePage`, `CountStudiesForProgramme`, and `Programmes`.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_people.go`, `internal/mlwh/tools_overview.go`,
`internal/mlwh/schema.go`
**Test file:** `internal/mlwh/tools_people_test.go`,
`internal/mlwh/tools_overview_test.go`

**Acceptance tests:**

1. Given programme `Cancer`, when the list and count are invoked, then the
   exact `/studies/programme/Cancer` paths receive pagination only on the list
   and return matching wrappers.
2. Given `/programmes` rows, when returned, then distinct non-empty names and
   their exact study counts appear under `programmes`.
3. Given a study overview with `programme=Cancer`, when returned, then the
   structured result and OpenAPI-backed schema both expose that field.
4. Descriptions direct callers to `mlwh_programmes` to discover exact values
   and to `mlwh_freshness` for cache state.

### E2: Add inverse study-user tools

As an agent, I want users and roles for one study, so that I can distinguish
membership from faculty sponsorship.

Add `mlwh_study_users` and `mlwh_count_study_users`. Both accept
`study_lims_id` and optional comma-separated `role`; the list adds pagination.
Omitted role means all roles. Allowed roles are `owner`, `manager`,
`data_access_contact`, `follower`, `slf_manager`, `lab_manager`, and
`administrator`; matching is an exact case-insensitive set. This differs from
person-to-studies, whose omitted role uses only owner, manager, and
data-access-contact membership. Call `StudyUsersPage` and `CountStudyUsers`.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_people.go`, `internal/mlwh/schema.go`
**Test file:** `internal/mlwh/tools_people_test.go`

**Acceptance tests:**

1. Given no role, when list and count run, then the query omits `role` and all
   stub role assignments are represented.
2. Given `role=owner,follower`, when list and count run, then the identical
   role value is sent and only those assignments are returned/counted.
3. Given a list page, when returned, then `users` contains exact
   `{role,name,login,email}` rows plus `total` and `next_offset`.
4. Given an invalid role, when returned upstream, then the mapped error is
   preserved.
5. Descriptions say faculty sponsor is a Study field and person-to-studies
   omission has a different three-role default.

### E3: Add merged-aware sample CRAM tools

As an agent, I want one selected CRAM per sample in a study, so that merged
multi-lane objects are not silently lost.

Add `mlwh_sample_crams_for_study` and
`mlwh_count_sample_crams_for_study`. The list is a normal page of exact
`wa.SampleCRAM` rows under `sample_crams`. Call `SampleCRAMsForStudyPage` and
`CountSampleCRAMsForStudy`.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_availability.go`, `internal/mlwh/schema.go`
**Test file:** `internal/mlwh/tools_availability_test.go`

**Acceptance tests:**

1. Given a study containing ordinary and merged sample CRAMs, when listed,
   then one row per selected sample is returned and a merged composite is
   preferred when present.
2. Given a row, when decoded, then its exact canonical fields are `name`,
   `accession_number`, `irods_path`, and `merged`; legacy alias names are not
   emitted by MCP.
3. Given list response headers, when returned, then `sample_crams`, `total`,
   and `next_offset` are exact.
4. Given the count tool, when invoked, then
   `/study/S1/sample-crams/count` returns the matching number of selected
   sample CRAM rows.
5. Descriptions warn that an empty product-level attachment does not prove a
   sample-level CRAM is absent and direct callers to freshness.

### E4: Preserve empty-study status arrays

As an agent, I want stable array types for empty studies, so that I do not need
null handling.

Ensure `mlwh_study_status_breakdown` returns `per_platform: []` whenever the
decoded upstream value is nil. Other status fields remain unchanged.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_overview.go`
**Test file:** `internal/mlwh/tools_overview_test.go`

**Acceptance tests:**

1. Given an empty-study breakdown with nil or empty `PerPlatform`, when the
   tool returns, then structured and text JSON both contain
   `"per_platform":[]`, never null.
2. Given a populated breakdown, when returned, then every upstream platform
   entry and all other aggregate fields are unchanged.

## F. Workflow, Hardening, and Regression Coverage

### F1: Route real-world questions to one curated call

As an agent, I want accurate workflow guidance, so that I choose the cheapest
correct tool.

Rewrite the prefix of `mlwh://workflow`, then append the live Registry
catalogue. Route product tables, actual files, merged-aware sample CRAMs,
literal sample prefixes, exact search filters, newest data, sample runs,
monthly runs, sequencing aggregates, programme membership, and study roles to
their dedicated tools. Use the generic call only as a fallback.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/workflow.go`
**Test file:** `internal/mlwh/workflow_test.go`

**Acceptance tests:**

1. Given each target question from the feature prompt, when workflow text is
   searched, then it names the matching curated tool and its critical option.
2. Product guidance says products without files remain rows and `file_type`
   only narrows an attachment; file guidance uses `children=irods`.
3. Sample CRAM guidance selects `mlwh_sample_crams_for_study`; search guidance
   says literal prefix is default and `words=true` is opt-in.
4. Recency guidance says "added to iRODS" and distinguishes `created` from
   source mutation and cache freshness.
5. Run and aggregate guidance preserves run grain, `date_basis`, grouping
   units, and the ONT caveat; people guidance distinguishes sponsor, programme,
   and study-user roles.
6. Workflow text never names a manifest tool or `mlwh_count_export`.

### F2: Migrate the public README to API 1.8.0

As a user, I want the public README to describe the current tools, so that I
do not call removed endpoints or miss cheaper curated workflows.

Update the introduction, prompt cookbook, interpretation notes, and exposed
tool catalogue. Remove manifest tool and endpoint guidance. Route chosen-column
product tables to `mlwh_export(children=products,parent_kind=study,...)`, state
that bounded export `Total` replaces an export-count tool, and describe the
literal-prefix sample-search default. List every tool added by this feature and
document the pagination and domain caveats needed to call them correctly.
Extend the existing README test without removing its shared-HTTP assertions.

**Package:** `cmd/mlwh-mcp-server/`
**File:** `README.md`
**Test file:** `cmd/mlwh-mcp-server/readme_test.go`

**Acceptance tests:**

1. Given the complete README in lowercase, when public workflow and catalogue
   text is inspected, then it contains no `mlwh_study_manifest`,
   `mlwh_count_study_manifest`, `/manifest`, manifest cookbook heading, or
   instruction to build or page a manifest.
2. Given version prose and the `--version` example, when read, then the MLWH API
   version is `1.8.0` and `1.7.0` is absent.
3. Given product-table guidance, when read, then it selects
   `mlwh_export` with `children=products` and `parent_kind=study`, states that
   products without iRODS objects remain rows, and says there is no
   `mlwh_count_export`; bounded `Total` sizes the export.
4. Given sample-search guidance, when read, then it says the default is a
   case-insensitive literal whole-value prefix and `words=true` is the opt-in
   separator-agnostic word-prefix mode, not a substring mode.
5. Given the exposed-tool catalogue, when inspected, then it names
   `mlwh_export`, `mlwh_latest_data_for_study`,
   `mlwh_count_latest_data_for_study`,
   `mlwh_latest_data_for_faculty_sponsor`,
   `mlwh_count_latest_data_for_faculty_sponsor`, `mlwh_runs_for_sample`,
   `mlwh_count_runs_for_sample`, `mlwh_count_studies_for_sample`, `mlwh_runs`,
   `mlwh_count_runs`, `mlwh_monthly_run_counts`,
   `mlwh_sequencing_aggregate`, `mlwh_studies_for_programme`,
   `mlwh_count_studies_for_programme`, `mlwh_programmes`, `mlwh_study_users`,
   `mlwh_count_study_users`, `mlwh_sample_crams_for_study`, and
   `mlwh_count_sample_crams_for_study`.
6. Given README pagination notes, when read, then they distinguish export's
   bounded 1000-row default and keyset `NextCursor` from offset-backed export,
   normal semantic pages' `total`/`next_offset`, latest-data's 10-row default,
   and global runs' last-row `id` cursor with no `total` or `next_offset`.
7. Given README caveat notes, when read, then they cover product `file_type` as
   attachment-only, merged-aware sample CRAMs, iRODS `created` as data-added
   time, platform `date_basis` including the ONT warehouse-load caveat, and
   `mlwh_freshness` as the cache as-of source.

### F3: Preserve bounded, cancellable, actionable behavior

As an operator, I want new tools to share existing safeguards, so that large
or failed calls remain manageable.

Keep the 1 MiB default `MLWH_MAX_TOOL_RESULT_BYTES` /
`--mlwh-max-tool-result-bytes` guard. Apply context to every HTTP request and
export drain. Preserve existing error mapping and header defaults. Update size
guidance for matrix and semantic pages. Verify
`mlwh_samples_with_data_for_study` and
`mlwh_count_samples_with_data_for_study` `since`/`until` behavior against
v0.8.0.

**Package:** `internal/mlwh/`, `internal/core/`
**File:** `internal/mlwh/provider.go`, `internal/mlwh/errmap.go`,
`internal/mlwh/tools_availability.go`, `internal/core/server.go`
**Test file:** `internal/mlwh/provider_test.go`,
`internal/mlwh/errmap_test.go`, `internal/mlwh/tools_availability_test.go`,
`internal/core/server_test.go`

**Acceptance tests:**

1. Given every new list tool with omitted, negative, maximum, and over-maximum
   pagination, when called, then its documented default/max applies and invalid
   values fail before HTTP where the MCP owns pagination.
2. Given request cancellation before or during a remote call, when observed,
   then no retry, fan-out, or successful partial result occurs.
3. Given upstream bad-request, not-found, ambiguity, unsupported identifier,
   impaired-upstream, and never-synced errors, when any new tool receives one,
   then the existing structured message and sentinel precedence are preserved.
4. Given an oversized typed or generic result, when guarded, then the error
   recommends a smaller page and the tool's actual continuation mechanism.
5. Given samples-with-data `since` and optional `until`, when list and count
   run, then exact RFC3339 values reach the existing paths, list/count match,
   and `until` without `since` remains an upstream bad request.
6. Given all provider tests, when executed, then all servers are
   `httptest.Server` instances and no real database or live `wa mlwh` server is
   contacted.

## Implementation Order

1. **Version and removal (A1, A2).** Update `wa`, remove manifest code, and
   restore Registry/OpenAPI compilation. Sequential foundation.
2. **Schema/page helpers and export (B1-B3).** Add generated input vocabulary,
   semantic wrappers, header parsing, bounded export, and stream
   materialization. Sequential after 1.
3. **Search and data-object reads (C1-C3).** Update optioned search/iRODS and
   add latest-data pairs. Search and availability files may proceed in
   parallel after phase 2 helpers.
4. **Runs and aggregates (D1-D3).** Add sample run counts and focused global
   run/aggregate tools. Parallel with phase 3 after shared schemas.
5. **Programme, users, CRAMs, status (E1-E4).** Add remaining curated reads and
   compatibility normalization. Parallel by production file after phase 2.
6. **Workflow, public docs, and hardening (F1-F3).** Update routing, README,
   size guidance, error and cancellation coverage, and full registration
   assertions. Sequential after all curated tools.
7. **Full verification.** Run `golangci-lint run --fix`, then
   `CGO_ENABLED=1 go test -tags netgo --count 1 ./...` without live services.

## Appendix: Key Decisions

- **Thin adapter.** All domain behavior remains in `wa v0.8.0`; MCP owns only
  tool ergonomics, semantic wrappers, header capture, stream materialization,
  and global result guarding.
- **One export, no export count.** A matrix avoids arbitrary row objects;
  bounded `Total` and relationship-specific continuation replace a synthetic
  count endpoint.
- **Pointer only where semantics require it.** Export deliverability is
  tri-state. Search and iRODS deliverability retain upstream plain-boolean
  behavior.
- **One-request pages.** `CallWithHeaders` prevents option loss and avoids a
  second sizing request for optioned search and iRODS pages.
- **Time vocabulary is explicit.** iRODS `created`, platform run dates, and
  cache sync state are separate concepts in every schema and description.
- **Freshness is not injected.** Only upstream types that carry
  `cache_synced_at` expose it. Other lists/counts direct callers to
  `mlwh_freshness`.
- **Error policy.** Upstream validation and sentinel errors remain
  authoritative; MCP adds no duplicate domain validator.

### Testing strategy

Every acceptance test maps to an independent GoConvey block. Extend the shared
stub to record repeated query values and response headers. Use canned v0.8.0
JSON, exact paths, context cancellation, and in-memory MCP transports. Follow
`go-implementor` for implementation and `go-reviewer` for review.
