# Feature: answer sequencing-data availability & recency for a study, cheaply

> This prompt is the feature description for the **`wa` MLWH REST API** (the
> `wa mlwh serve` command, code under `mlwh/`). It will be moved into the `wa`
> repo and fed to that repo's spec-writer workflow. All paths below are relative
> to the `wa` repo root. The requester maintains a downstream consumer — the
> **MLWH MCP server** (`mlwh-mcp-server`), a thin read-only bridge that turns
> these endpoints into agent tools — which can only ever be as good as the
> endpoints this API offers.
>
> **Scope rule for this spec: everything described here is in scope to build.**
> The "Design decisions" section settles *how* each item is implemented, never
> *whether*. There are no optional items.

## Summary

Make the most common class of user question about a study answerable **cheaply —
one request, small response**:

- **"How many samples in study X have sequencing data available, and how many do
  not?"**
- **"Is there any *new* sequencing data available for study X this week?"** —
  i.e. data **added to iRODS** within a recent window.
- "Which samples have data / which are still missing it?"
- "What's in study X?" / "How much data is there?"

Today the API cannot answer any of these without abuse, even though almost all the
needed facts are already cached. This feature adds the small, indexed aggregate +
recency + budget-safety surface to close that.

## Why this is needed (the motivating incident — read this)

An agent asked "how many of study 7607's 428 samples have sequencing data?" had
only bad options:

1. `GET /study/:id/samples/count` → `428`: counts samples, not samples with
   *data*.
2. `GET /study/:id/irods` → the per-study iRODS list. **Huge** (735 rows /
   ~170 KB here; far larger in production) — it blew the downstream MCP client's
   token budget and was spilled to a file — **and** each `IRODSPath` row carries
   **no sample identity** (`id_product`/`collection`/`data_object`/`irods_path`),
   so it cannot be aggregated back to "distinct samples with data" anyway.
3. `GET /study/:id/detail` → also huge (~600 KB) and carries **no** iRODS/lane
   info per sample despite its name.

The only thing that worked was enumerating the 428 sample names and calling
`GET /sample/:id/irods` **428 times** — N round-trips for one aggregate, and not
viable through MCP at all. And there is currently **no way whatsoever** to ask the
recency question ("new this week"). The data is cached; the API just never exposes
the aggregates.

## Three timestamps — do not conflate them

The recency question hinges on picking the right time. There are three, and only
one is the answer:

1. **When the data was added to iRODS** — the iRODS-location **creation** time,
   the source column **`seq_product_irods_locations.created`** (`datetime`,
   `DEFAULT CURRENT_TIMESTAMP`, set once at insert — verified against the live
   warehouse). This is the *only* thing "any new data this week?" is about.
2. **`last_updated`** — the MLWH row's last-*changed* time (the source column
   **`seq_product_irods_locations.last_changed`**, `datetime ... on update
   CURRENT_TIMESTAMP`, which the mirror stores as `last_updated`). This is what the
   cache syncs on (it is the `sync_state.high_water`; `mlwh/freshness.go:54`
   documents `HighWater` as "latest synced last_updated"). It is a **proxy that
   conflates newly-added data with later-modified data** (QC edits, re-loads,
   collection moves all bump it), so it is the **wrong** signal for "new" and must
   not be presented as such.
3. **`last_run`** — when **`wa` last synced** its cache from MLWH
   (`sync_state.last_run`, surfaced by `GET /freshness`). Users do **not** care
   about this as the answer; it only **bounds how complete** a recent-window
   answer can be (data added to iRODS after the last sync is not in the cache
   yet). It is the **freshness caveat**, never the answer.

Consequence: `seq_product_irods_locations.created` is **not currently mirrored** —
the sync source queries select only `spi.last_changed` (`mlwh/sync.go` ~572–592)
and the mirror carries only `last_updated`
(`mlwh/cache_schema/sqlite/seq_product_irods_locations_mirror.sql:9`). Answering the
recency question correctly therefore **requires a cache schema change**: carry
`created` into the mirror (see deliverable R).

## Background: what exists today (this code is authoritative — read it)

- **The endpoint registry.** `mlwh/registry.go` — the `Registry` slice
  (`Endpoint`: `Method`, `Verb`, `Path`, `PathParams`, `Paginated`, `NewResult`,
  `Summary`, `Description`, `QueryParams`) generates `/openapi.json` and the
  endpoint reference, so it cannot drift. Mirror these entries:
  `CountSamplesForStudy` (~463–471), `SamplesForStudy` (~138–148),
  `IRODSPathsForStudy` (~258–268), `StudyDetail` (~380–388),
  `LanesForSample` (~234–244), plus `fetchAllPaginationParams()` and
  `newSliceResult`/`newResult`.
- **The count template.** `mlwh/count.go` — `CountSamplesForStudy` (~70–117) and
  the reusable `queryCount` helper (~120–133). Its SQL
  (`countSamplesForStudyCacheSQL`, ~42) joins `library_samples` → `sample_mirror`
  on `id_study_lims`, `COUNT(DISTINCT id_sample_tmp)`, and handles
  synced-with-rows / synced-empty / never-synced (`ErrCacheNeverSynced`).
- **The iRODS query + its dropped/absent columns.** `mlwh/hierarchy.go` —
  `IRODSPathsForStudy` (~1215–1248), `IRODSPathsForSample` (~1179–1212),
  `queryIRODSPaths` (~1298–1325). The study query already reads
  `seq_product_irods_locations_mirror WHERE id_study_lims = ?`; that table carries
  `id_sample_tmp` and `last_updated` but the `SELECT` and the `IRODSPath` struct
  (`mlwh/types.go` ~101–106) project neither.
- **The lanes query.** `mlwh/hierarchy.go` `LanesForSample` (~1124–1176) over
  `iseq_product_metrics_mirror` (also carrying `id_sample_tmp`, `id_study_lims`,
  `id_run`, `position`, `tag_index`, `last_updated`).
- **The cache schema (what's mirrored).**
  `mlwh/cache_schema/{sqlite,mysql}/seq_product_irods_locations_mirror.sql`:
  columns `id_iseq_product`, `irods_root_collection`,
  `irods_data_relative_path`, `irods_collection`, `irods_file_name`,
  **`id_sample_tmp`**, **`id_study_lims`**, **`last_updated`**; indexes
  `seq_product_irods_locations_mirror_id_sample_tmp_idx` and
  `spi_mirror_study_lims_sample_tmp_idx (id_study_lims, id_sample_tmp)`. Note there
  is **no creation-time column** and **no `(id_study_lims, last_updated)` /
  `(id_study_lims, created)` index** yet. `study_mirror` (key `id_study_lims`,
  has `last_updated` + index), `sample_mirror` (key `name`, has `last_updated` +
  `sample_mirror_last_updated_idx`), and `library_samples` complete the graph.
- **Incremental sync + freshness.** `sync_state` (`table_name`, `high_water`,
  `last_run`, `resume_cursor`, `indexes_dropped`) drives incremental sync keyed on
  `last_changed`; `mlwh/freshness.go` (`Freshness`, `TableFreshness{HighWater,
  LastRun, EverSynced}`, ~50–93) reports per-table `high_water` and `last_run`.
- **The iRODS sync source queries.** `mlwh/sync.go` ~560–592 holds the
  `seq_product_irods_locations` source SELECTs (initial / resume / incremental, in
  two join variants). They select
  `spi.id_seq_product_irods_locations_tmp, spi.id_product,
  spi.irods_root_collection, spi.irods_data_relative_path, ifc.id_sample_tmp,
  study.id_study_lims, spi.last_changed` — keying the incremental window on
  `spi.last_changed` and storing it as the mirror's `last_updated` (the row struct
  ~2542, batch insert ~2586, column list ~137). **`spi.created` is not selected**;
  the verified source columns are `created` (set at insert) and `last_changed`
  (bumped on update). Deliverable (R) adds `spi.created` to every one of these
  variants. The incremental window stays keyed on `last_changed`; `created` rides
  along (a new row has `created == last_changed`, so it is captured the first time
  it crosses the high-water mark).
- **Handler wiring & invariants.** `mlwh/server.go` — one `case` per
  `Registry.Method` (~373–396); `RegisterRoutes` (~79–96). Every query bakes in
  `id_lims = 'SQSCP'`; keep it.
- **The add-a-query recipe** (`mlwh/registry.go` package docstring, ~26–30):
  (1) schema columns/indices in **both** dialects; (2) one `Client` method;
  (3) one `Queryer` member (`mlwh/queryer.go` ~31); (4) one `Registry` entry; plus
  a `server.go` handler case.
- **Generated docs.** After changing the `Registry`, run
  `WA_REFRESH_DOCS=1 go test ./mlwh -run TestWriteEndpointReference` (writes
  `.docs/mcp/api-reference.md`); drift guards
  (`TestEndpointReferenceAndOpenAPICoverSamePathsG1`) fail CI otherwise. Update
  `.docs/mcp/glossary.md` for new terms ("sequencing data available", "added to
  iRODS").
- **Hermetic tests.** GoConvey over an ephemeral SQLite cache
  (`openSQLiteSyncTestCache`), seeded via helpers in `mlwh/count_test.go` /
  `mlwh/hierarchy_test.go` (`seedHierarchyStudy`, `seedSampleMirrorSearchRow`,
  `seedLibrarySample`, `seedSyncStateRun`, the iRODS/product-metrics seeders).
  Never a live warehouse. Existing count tests cross-check the count against the
  length of the matching list — do the same.

## What the feature must deliver

### Availability

- **(S) A study sequencing-availability summary** — one GET, small fixed-size
  response, e.g. `GET /study/:id/sequencing-summary →
  { samples_total, samples_with_data, samples_without_data, data_objects, runs,
    newest_data_added, added_last_7_days, cache_synced_at }`. It directly answers
  "how many have data / how many don't / how much / anything new", and carries the
  freshness caveat (see F). The exact field set is settled in Design decisions, but
  it includes at least the sample-with/without-data counts, a "how much" figure,
  and the recency fields.
- **(C) A bare count** of samples-with-data, e.g.
  `GET /study/:id/samples-with-data/count → Count`, built on `queryCount` over
  `library_samples → sample_mirror → seq_product_irods_locations_mirror` scoped by
  `id_study_lims`.
- **(E) Enumerate which samples have / lack data.** Provide **both**:
  - list endpoints `GET /study/:id/samples-with-data` and
    `.../samples-without-data` returning `Sample`s, paginated like the other study
    fan-outs; and
  - **sample identity on the per-study iRODS rows** — add the sample's
    `id_sample_tmp` and Sanger `name` to the `IRODSPath` rows returned by
    `/study/:id/irods` (additive fields), so that list is aggregatable by sample
    standalone.

### Recency ("new data this week")

- **(R) Mirror the iRODS-location creation timestamp.** Carry the verified source
  column **`seq_product_irods_locations.created`** into the mirror: add a
  creation-time column to `seq_product_irods_locations_mirror` in **both** dialects
  (sqlite + mysql) plus a supporting index `(id_study_lims, <created column>)`; add
  `spi.created` to **all** the source SELECT variants in `mlwh/sync.go` ~560–592;
  and extend the sync row struct (~2542) and batch insert (~2586) to scan/store it.
  Keep the incremental window keyed on `last_changed` (no high-water change). This
  is the only new mirrored source data the feature adds, and it is what makes
  "added to iRODS since X" answerable precisely rather than via the `last_updated`
  proxy. Note re-syncing to backfill `created` for existing rows.
- **(T) Date-windowed availability**, filtering on the creation timestamp from (R):
  - a count, e.g. `GET /study/:id/samples-with-data/count?since=<RFC3339>` (and/or
    a dedicated "new since" count), returning distinct samples whose data was added
    to iRODS in the window; and
  - a list of the new data / newly-covered samples in the window.
  The window is expressed as explicit `since` (and optional `until`) RFC3339
  parameters — the API stays date-explicit; callers translate "this week" into a
  date. Both are single indexed range queries over the new column/index.

### Overviews that displace the giant aggregates

- **(O1) A cheap study overview** — small fixed-size superset of (S) answering
  "what's in study X?": sample / library / run counts, samples-with-data &
  data-object counts, the library types present, and the sequencing date range —
  all cheap aggregates over indexed columns. (May be the same endpoint as (S);
  settle in Design decisions.)
- **(O2) A cheap run overview** — the run-level analogue (how many samples /
  studies / data objects on a run) so "what's on this run / how much" needs
  neither `/run/:id/detail` nor per-sample calls.

### Budget-safety surface completion

- **(N) A `/count` counterpart for every paginated list endpoint** so any list can
  be sized before transfer: `/study/:id/irods/count`, `/sample/:id/irods/count`,
  `/study/:id/runs/count`, `/study/:id/libraries/count`, `/sample/:id/lanes/count`,
  `/run/:id/samples/count`, and the `library*/samples` + `find/sample/*` lists.
  Each is the same `queryCount` + four-step recipe.
- **(M) Sizing metadata on list responses** — return the total matching count and
  the next offset alongside each page (an envelope such as
  `{items, total, next_offset}`, or response headers; settle the exact shape),
  so one page reveals how much remains.
- **(L) Bounded / lean detail aggregates** — give `/study/:id/detail` and
  `/run/:id/detail` pagination of their nested collections, a `fields`/`lean`
  projection that drops heavy nested objects, and **de-duplication** of repeated
  nested entities (return each study/library once in a lookup table instead of
  re-embedding it under every sample). (See `StudyDetail`/`RunDetail` in
  `mlwh/types.go` and their builders in `mlwh/hierarchy.go`.)

### Freshness, woven through

- **(F) Every availability/recency response must let the caller honestly caveat
  recency** by surfacing the relevant table's `last_run` (when `wa` last synced the
  iRODS data) — e.g. a `cache_synced_at` field on the summary/overview and on the
  windowed responses — kept **clearly distinct** from any data-added timestamp.
  Reuse `mlwh/freshness.go`. A recent-window answer is only complete up to
  `last_run`.

## HARD REQUIREMENTS

1. **One request, small response** for every count/summary/overview question;
   response size independent of study/run size. No client should ever page the full
   iRODS list or call a per-sample endpoint N times to answer availability or
   recency.
2. **Single indexed query per aggregate.** Counts/summaries are SQL
   (`COUNT(DISTINCT ...)`, range scans on the new creation-time index), never an
   in-process scan of a fetched list. Add only the indices the new queries need.
3. **Correct recency signal.** "New / added to iRODS since X" filters on the
   mirrored **creation** timestamp from (R), never on `last_updated`. Never present
   `last_updated` or `last_run` as "when data was added".
4. **Reuse existing infrastructure & invariants.** `queryCount`, the four-step
   recipe, one-`case`-per-`Method` handlers, `id_lims = 'SQSCP'` in every query,
   and the never-synced / empty / unknown-study behaviour consistent with
   `CountSamplesForStudy`.
5. **Self-describing metadata.** Each new endpoint gets a clear `Summary` and
   `Description` (the downstream MCP surfaces `Description` verbatim as the agent's
   tool help): state the precise definition of "available", the recency semantics
   and window parameters, the study-scoping rule, and the freshness caveat.
6. **Regenerate generated docs; keep drift guards green.** Add `Registry` entries,
   refresh `.docs/mcp/api-reference.md`, update the glossary; OpenAPI must cover the
   new paths.
7. **Hermetic GoConvey tests.** Seed a study with samples — some with iRODS rows
   and some without, across ≥2 runs/tags, with iRODS rows of **differing creation
   times** (inside and outside the window), and at least one sample shared with
   another study (to exercise scoping). Assert the counts / summary / overview /
   windowed results, cross-check count against list length, and cover
   never-synced / empty-study and the freshness fields. Test both schema dialects'
   new column/index.

## Design decisions for the spec to settle (HOW, not WHETHER)

Each item below **will be built**; settle only the implementation:

- **Definition of "sequencing data available".** Use: ≥1 row in
  `seq_product_irods_locations_mirror` for the study (real data files in iRODS).
  Decide whether the summary *also* reports "sequenced but not yet in iRODS"
  (samples with `iseq_product_metrics_mirror` rows but no iRODS rows) as a separate
  figure. State the choice in every `Description`.
- **Study scoping of shared samples.** Scope "data for *this* study" by
  `seq_product_irods_locations_mirror.id_study_lims = :id` (as `/study/:id/irods`
  already does), not "data the sample has anywhere". This is the source of a real
  discrepancy seen in the incident (735 study-scoped objects vs 647 summed across
  un-scoped per-sample lists). Pick this rule and state it.
- **The mirror column for (R)** — the source column is `created` (settled); choose
  the mirror column name (e.g. `created` vs `irods_created`), its stored format
  (TEXT RFC3339, as `last_updated` is stored), and the exact index shape
  `(id_study_lims, <created column>)`.
- **Window semantics & parameters** — `since`/`until` (RFC3339), half-open vs
  closed intervals; and the precise meaning of "added" given that a creation
  timestamp records first registration.
- **Endpoint shapes & names** — one combined `sequencing-summary`/`overview`
  endpoint vs separate; the `samples-with-data[/count]`, run-overview, and
  `/count` counterpart paths; the summary/overview response structs. Keep
  consistent with the existing `/study/:id/...`, `Count`, and `*Detail`
  conventions.
- **Sizing-metadata shape (M)** — envelope vs headers, and whether it is always on
  or opt-in, reconciled with the current bare-slice contract.
- **Lean/de-dup detail shape (L)** — projection mechanism and the lookup-table
  layout for de-duplicated nested entities.

## Out of scope

- **New mirrored *tables*.** The only new mirrored source data is the
  iRODS-location creation-time **column** in deliverable (R) (plus supporting
  indices); everything else reuses already-mirrored data.
- Authentication / TLS changes (keep the current posture); mutating endpoints.
- Fuzzy relative-time parsing in the API ("this week"): the API takes explicit
  dates; callers compute the window.
- The downstream MCP server's tool surface (a separate, dependent spec).

## Pointers / prior art (in order of authority)

1. **This repo's code**: `mlwh/count.go` (`CountSamplesForStudy`, `queryCount`);
   `mlwh/hierarchy.go` (`IRODSPathsForStudy`, `LanesForSample`, `queryIRODSPaths`,
   the detail builders); `mlwh/freshness.go` (the `last_run`/`high_water` caveat
   source); `mlwh/cache_schema/{sqlite,mysql}/seq_product_irods_locations_mirror.sql`
   + `iseq_product_metrics_mirror.sql` + `sync_state.sql` (the linkage, the
   `last_updated` signal, and where the new creation column/index go);
   `mlwh/sync.go` (~560–592, the `seq_product_irods_locations` source SELECTs to
   extend with `spi.created`; row struct ~2542, batch insert ~2586);
   `mlwh/registry.go` (entry pattern + recipe); `mlwh/queryer.go`; `mlwh/server.go`;
   `mlwh/types.go` (`Count`, `IRODSPath`, `Sample`, `Lane`, `StudyDetail`,
   `RunDetail`).
2. **Generated docs + tests**: `.docs/mcp/api-reference.md`, `.docs/mcp/glossary.md`,
   the `WA_REFRESH_DOCS=1 go test ./mlwh -run TestWriteEndpointReference` flow, and
   the GoConvey hermetic-cache helpers in `mlwh/count_test.go` /
   `mlwh/hierarchy_test.go` / `mlwh/freshness_test.go`.
3. **The downstream consumer** (why descriptions matter): the MLWH MCP server turns
   each `Registry` entry into an agent tool and shows the `Description` as its help.
