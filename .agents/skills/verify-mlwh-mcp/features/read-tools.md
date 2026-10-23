# Read Tools

## What It Is

The bulk of the server: read-only `mlwh_*` tools over wa's MLWH API. Two
cheap representatives: `mlwh_freshness` (per-table sync state, used to
caveat answers; works even on a never-synced cache) and
`mlwh_classify_identifier` (what kind of identifier a string is, plus the
matched entity).

## How To Reach It

`tools/call` from any MCP session. Fixture data (wa test mode): studies
`6568` and `7607`, sample `WTSI_wEMB10524782`; five tables synced
(`study`, `sample`, `iseq_flowcell`, `iseq_product_metrics`,
`seq_product_irods_locations`), the rest never synced.

## How To Drive It

`drive-basics.sh`, per transport:

- `mlwh_freshness {}`: not `isError`, and `structuredContent.tables` equals
  wa's own `GET /freshness` tables (`20-wa-freshness-direct.json`).
  Evidence: `22-<transport>-freshness.json`.
- `mlwh_classify_identifier {"identifier":"6568"}`: `kind` is
  `study_lims_id`, study accession `EGAS00001006568`.
  Evidence: `23-<transport>-classify.json`.

Other tools: `"$CLIENT" … call <tool> '<json>'`. `list` prints tool names
only; read a tool's input schema from its `internal/mlwh/tools_*.go`. See
verify-wa's `mlwh-query-api.md` for what the fixture cache holds.

## Gotchas

- Only five tables are synced in the fixture cache. A tool that depends on
  another table reflects that state; check `mlwh_freshness` before calling
  an empty answer a bug.
- The fixture has no value shared by several samples. To check
  `mlwh_find_samples` paging (`total`, `next_offset`), ambiguity errors that
  name candidate samples, or `multi_sample`, copy the `--mlwh-cache` file of
  the running `wa mlwh serve` (`ps`) into the scratchpad, insert
  `sample_mirror`, `donor_samples`, and `sample_search_token` rows sharing a
  `supplier_name`, and serve the copy with `$WA_RUN/wa mlwh serve --port
  <spare> --url 127.0.0.1:<spare> --mlwh-cache <copy>`, plus an
  `mlwh-mcp-server --http` against it on another free port. Stop both
  yourself; Cleanup does not know them.
- The fixture has too few matches to page a search. To walk a search by
  cursor, seed the same scratch copy with 25 `sample_mirror` rows named
  `cursorwalk_NN` (plus their `sample_search_token` rows for `words=true`)
  and a few `study_mirror` rows named `Cursorwalk study N`, then walk
  `mlwh_search_samples term=cursorwalk limit=10` (and `mlwh_search_studies
  limit=2`) by passing each `next_cursor` back as `cursor` until it is
  empty: expect 3 pages, distinct rows, `next_offset=-1` after page 1. To
  see `stale=true`, update the copy's `sync_state.last_run` for `sample`
  between two pages.
- Large results hit `MLWH_MAX_TOOL_RESULT_BYTES` (default 1 MiB) and return
  a structured size error.
