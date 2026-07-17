# MLWH MCP Server

A [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server, written
in Go, that lets LLM agents (Claude Code, Codex, and later a web UI server-side
agent) query the Multi-LIMS Warehouse (MLWH) read API in natural, tool-driven
ways: search and count samples and studies, resolve identifiers, answer cheap
overview/status/availability questions, export chosen-column relationship
tables, page through iRODS paths and global runs, query sequencing aggregates,
discover programmes and study users, select merged-aware sample CRAMs, and
report data freshness.

The MLWH read API is provided by the separate upstream
[`wa`](https://github.com/wtsi-hgi/wa) project (its `wa mlwh serve` command). This
server is a thin, well-described bridge whose value is making those endpoints
ergonomic and correctly usable by an LLM, not re-implementing them: it imports
`github.com/wtsi-hgi/wa/mlwh` and reuses its typed client, response types,
registry, and OpenAPI document directly, so there is no type drift and the server
is compile-time-locked to the upstream API version (currently MLWH API 1.8.1).

The MLWH provider serves over the **stdio** transport by default, so it can run
as a local subprocess launched per user by an agent CLI. It can also run in
opt-in streamable HTTP mode for admins who want to operate one shared service on
an internal network.

## Requirements

- **Go 1.25+** to build and install.
- Network access to a running **`wa mlwh serve`** instance — the MLWH read API
  this server bridges to. You point the server at its base URL (see
  [Configuration](#configuration)).

## Install

```bash
# Install the `mlwh-mcp-server` binary into your Go bin ($(go env GOPATH)/bin):
make install

# ...or just build it to ./mlwh-mcp-server in the repo:
make build
```

`make install` puts `mlwh-mcp-server` on your `PATH` if `$(go env GOPATH)/bin`
(usually `~/go/bin`) is on it; otherwise use the binary's full path in the configs
below. Check the build and the versions it targets:

```bash
mlwh-mcp-server --version
# mlwh-mcp-server version <build version>
# MLWH API version 1.8.1
```

## Configuration

The server needs to know where your MLWH API lives. Configure it with environment
variables, or the equivalent command-line flags (a flag overrides its env var):

| Env var                      | Flag                          | Required | Meaning                                                                          |
| ---------------------------- | ----------------------------- | -------- | -------------------------------------------------------------------------------- |
| `MLWH_BASE_URL`              | `--mlwh-base-url`             | **yes**  | Base URL of the `wa mlwh serve` HTTP API, e.g. `http://mlwh.internal:8080`.      |
| `MLWH_CA_CERT`               | `--mlwh-ca-cert`              | no       | Path to a PEM CA-certificate file, if the API is served over TLS with a private CA. |
| `MLWH_TIMEOUT`               | `--mlwh-timeout`              | no       | Per-request HTTP timeout as a Go duration (e.g. `15s`, `1m`).                     |
| `MLWH_MAX_TOOL_RESULT_BYTES` | `--mlwh-max-tool-result-bytes` | no       | Maximum marshaled MCP tool-result size before a structured size error is returned; defaults to `1048576`, and values `<=0` disable the guard. |
| `MLWH_HTTP_ADDR`             | `--http`                      | no       | Address for opt-in streamable HTTP mode. Leave unset for the default stdio transport. |

The MLWH API is internal and unauthenticated, so there is no token to set. A
missing base URL is a clear startup error. `mlwh-mcp-server --version` needs no
configuration (it prints the versions and exits without opening a transport or
touching the network).

In normal use you set these in your **MCP client's** server config (see below),
and the client passes them to the binary it launches. For local testing you can
instead keep them in a `.env` file:

```bash
make config        # creates .env from .env.example
$EDITOR .env       # set MLWH_BASE_URL to your wa mlwh serve instance
make start         # loads .env and serves over stdio (Ctrl-C to stop)
```

`make start` speaks the MCP protocol over stdio: it reads JSON-RPC on stdin and
writes it on stdout, with operational logs on stderr. On its own it just waits for
a client, so it is mainly a smoke test — in real use an agent CLI launches it for
you, as described next.

## Run a shared HTTP service

Stdio remains the default. To run one admin-managed shared instance, start
streamable HTTP mode with either `--http` or `MLWH_HTTP_ADDR`.

```bash
MLWH_BASE_URL=http://mlwh.internal:8080 mlwh-mcp-server --http 127.0.0.1:8081
```

In HTTP mode, MCP is served at `/mcp` and health checks are served at `/health`.
The listener is unauthenticated plain HTTP for internal-network deployment, so
bind it to an internal interface or place it behind your existing trusted
network controls.

A minimal systemd unit can set both the upstream MLWH API and the listener
address:

```ini
[Unit]
Description=MLWH MCP Server
After=network-online.target

[Service]
Environment=MLWH_BASE_URL=http://mlwh.internal:8080
Environment=MLWH_HTTP_ADDR=127.0.0.1:8081
ExecStart=/usr/local/bin/mlwh-mcp-server
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## Use the shared HTTP service from agent CLIs

When an admin operates the shared HTTP service, point each MCP client at the
`/mcp` URL they provide. Users do not install or run a local
`mlwh-mcp-server` binary for shared HTTP client setup. Claude Code and Codex
connect directly to the shared HTTP endpoint.

Claude Code can register the shared service by URL:

```bash
claude mcp add --transport http mlwh http://mlwh-mcp.internal:8080/mcp
```

The equivalent Claude Code JSON config is a `mcpServers.mlwh` HTTP entry:

```json
{
  "mcpServers": {
    "mlwh": {
      "type": "http",
      "url": "http://mlwh-mcp.internal:8080/mcp"
    }
  }
}
```

Codex can register the same shared service by URL:

```bash
codex mcp add mlwh --url http://mlwh-mcp.internal:8080/mcp
```

The equivalent Codex TOML is:

```toml
[mcp_servers.mlwh]
url = "http://mlwh-mcp.internal:8080/mcp"
```

## Use it with Claude Code

Register the server once (here at user scope, so it is available in every
project):

```bash
claude mcp add --env MLWH_BASE_URL=http://mlwh.internal:8080 --scope user mlwh -- mlwh-mcp-server
```

- `mlwh` is the name the server appears under; everything after `--` is the
  command Claude Code runs to launch it (use the binary's full path if it is not
  on your `PATH`).
- Repeat `--env KEY=VALUE` for each setting (e.g. add `--env MLWH_TIMEOUT=30s`).
- `--scope` is one of `local` (this project only — the default), `project`
  (shared with your team via a checked-in `.mcp.json`), or `user` (all your
  projects).

The equivalent manual config is a `mcpServers` entry — in a project's `.mcp.json`,
or in `~/.claude.json` for user scope:

```json
{
  "mcpServers": {
    "mlwh": {
      "type": "stdio",
      "command": "mlwh-mcp-server",
      "args": [],
      "env": { "MLWH_BASE_URL": "http://mlwh.internal:8080" }
    }
  }
}
```

Then, in a Claude Code session, run `/mcp` and confirm `mlwh` shows as
**connected**. Claude calls the tools automatically when they are relevant (you
can also nudge it, e.g. _"use the mlwh tools to find samples matching 'mus'"_).
The tools are namespaced under the server, with names like `mlwh_search_samples`,
`mlwh_resolve_sample`, and `mlwh_sample_detail`. The server also publishes a
workflow-guide resource (`mlwh://workflow`) and a version resource
(`mcp-server://version`) that the agent can read.

## Use it with Codex CLI

Add the server with the CLI:

```bash
codex mcp add mlwh --env MLWH_BASE_URL=http://mlwh.internal:8080 -- mlwh-mcp-server
```

...or edit `~/.codex/config.toml` (or a project-scoped `.codex/config.toml`)
directly:

```toml
[mcp_servers.mlwh]
command = "mlwh-mcp-server"
args = []

[mcp_servers.mlwh.env]
MLWH_BASE_URL = "http://mlwh.internal:8080"
# MLWH_TIMEOUT = "30s"
```

Use the full path to `mlwh-mcp-server` if it is not on your `PATH`. Codex
discovers the tools on startup and calls them as needed during a session.

## What can I ask?

Once the MCP server is connected, talk to your agent normally. Include the sample
name, study id, run id, library id, person, or file type you care about; the
agent should choose the cheap overview/count/status tools first and only page
through lists when rows are actually needed.

Prompt cookbook:

- **Find samples or studies**
  - "Find samples matching `mus`."
  - "How many samples match `mus`?"
  - "Find deliverable WGS mouse samples whose names start with `mus`."
  - "Search sample names by word prefix instead of literal whole-value prefix."
  - "What study id matches the name `cancer`?"
  - "List study candidates for `rare disease`, enough to disambiguate the id."
  - Sample search defaults to a case-insensitive literal whole-value prefix.
    `words=true` is the opt-in separator-agnostic word-prefix mode, not a
    substring mode.
- **Resolve or classify an identifier**
  - "What kind of MLWH identifier is `5901`?"
  - "Resolve sample `S1` to its canonical identifiers."
  - "Resolve run `52553`."
  - "Expand this study id into related search values."
- **Exact sample lookup**
  - "Find samples where accession is exactly `ERS123`."
  - "Count samples with supplier name exactly `Bob`."
  - "Find samples for library type `WGS`."
- **Product tables and relationship exports**
  - "Use `mlwh_export` with `children=products`, `parent_kind=study`, and
    `parent_id=S1` to return the chosen product columns."
  - "Export the sample CRAM matrix for study `S1`."
  - "Export users and their roles for study `S1`."
  - "Export lanes for sample `S1`, or studies for programme `Cancer`."
  - "Export deliverable CRAM paths for sample `S1`, newest first."
  - Chosen-column product tables route to
    `mlwh_export(children=products,parent_kind=study,...)`; products without
    iRODS objects remain rows. There is no `mlwh_count_export`; bounded `Total`
    sizes the export.
- **Study overview and metadata**
  - "Give me a quick overview of study `S1`."
  - "What data access group is study `S1` in?"
  - "How many samples in `S1` have data, have no data, or were sequenced but have no data?"
- **Availability and recency**
  - "How many samples in study `S1` have data added since `2026-06-21T00:00:00Z`?"
  - "List the first page of samples in `S1` with data between these two timestamps."
  - "Which samples in `S1` still have no sequencing data?"
  - "How much new data was added to iRODS for `S1` in the last 7 days?"
  - "List the latest CRAM data rows for study `S1`, 10 at a time."
  - "Show the latest data page for faculty sponsor `Carl`, filtered to CRAM."
  - "Count latest CRAM data rows for faculty sponsor `Carl`."
- **QC and status**
  - "Break down study `S1` by received, sequenced, not sequenced, and manual QC state."
  - "How many samples in `S1` passed, failed, or are pending QC?"
  - "Show status by platform, preserving ONT and not-tracked values."
- **Sample and run progress**
  - "What is happening with sample `S1` right now?"
  - "Show sample `ONT1` progress, including not-tracked QC if present."
  - "Summarize run `52553`."
  - "Show the status timeline for run `52553`."
- **Global runs and sequencing aggregates**
  - "List PacBio and ONT runs in this half-open date window."
  - "Count runs using the same platform and date filters."
  - "Show monthly run counts with each platform's `date_basis`."
  - "Group sequencing products by month, programme, and platform."
- **iRODS paths and CRAMs**
  - "How many CRAM paths are there for sample `S1`?"
  - "List CRAM iRODS paths for study `S1`, first page only."
  - "Show iRODS paths for run `52553` with `file_type=cram`."
  - "List deliverable CRAM paths for study `S1` added between these timestamps,
    newest first."
  - "Count those same filtered iRODS paths without paging."
  - "Are there any VCF paths for `S1`?"
  - "Select one merged-aware sample CRAM row per sample in study `S1`."
  - "How many selected sample CRAMs are available for study `S1`?"
- **People, sponsors, and users**
  - "Which studies have faculty sponsor `Carl`?"
  - "How many studies is `cwa` associated with as a user?"
  - "List studies where `cwa` has role `Follower`."
  - "Resolve the person name `Carl` across sponsors and study users."
  - "Discover exact programme values, then list studies in programme `Cancer`."
  - "How many studies are in programme `Cancer`?"
  - "List every study-user role assignment for study `S1`."
  - "How many owners and followers are assigned to study `S1`?"
- **Detail and fan-outs**
  - "Show sample detail for `S1`."
  - "Show lean study detail for `S1`, page size 100."
  - "List runs for study `S1`."
  - "List and count runs for sample `S1`."
  - "Count studies linked to sample `S1`."
  - "List libraries for study `S1`."
  - "How many lanes does sample `S1` have?"
  - "How many samples are linked to library id `LIB123`?"
- **Freshness and caveats**
  - "How fresh is the MLWH cache?"
  - "Before answering, check whether the cache has synced recently."
  - "This list has no `cache_synced_at`; use `mlwh_freshness` for the as-of caveat."
- **Large answers and paging**
  - "Count first, then list the first 100 rows."
  - "Continue from `next_offset=100`."
  - "Continue this export using its returned cursor or offset model."
  - "Continue global runs from the last row's composite id."
  - "Use a smaller page if the result is too large."
- **Advanced registry access**
  - "Call the MLWH Registry method `AllStudies` with `limit=100` and `offset=0`."
  - "Call `RunListing` generically with `platform` repeated for PacBio and ONT."
  - "Use the generic endpoint caller for a Registry method that does not have a curated tool yet."

Notes for interpreting answers:

### Pagination and continuation

- Normal semantic pages default to 100 rows, reject limits above 1000, and
  return `total` and `next_offset`; `next_offset=-1` means there is no next page.
- Latest-data pages default to 10 rows. They are bounded pages, not every row
  tied for the maximum `created` value.
- Bounded `mlwh_export` calls default to 1000 rows. Keyset-backed product pages
  and unsorted iRODS export pages return `NextCursor`; pass it back as `cursor`.
  Offset-backed export relationships do not accept a cursor and continue with
  `offset + len(Rows)`. The matrix `Total` sizes a bounded export.
- `mlwh_runs` is a separate keyset page: pass the last row's `id` as the cursor.
  It returns neither `total` nor `next_offset`.
- If a response would be too large, the server returns a structured
  `tool_result_too_large` error with continuation guidance instead of flooding
  the chat.

### Domain caveats

- Sample search defaults to a case-insensitive literal whole-value prefix;
  `words=true` is the opt-in separator-agnostic word-prefix mode, not a
  substring mode.
- Product `file_type` is attachment-only: it filters iRODS objects attached to
  each product, not the product rows. Products without iRODS objects remain
  rows. Deliverability approximates iRODS target and `not is_spiked` where the
  upstream discriminator exists.
- Use `mlwh_sample_crams_for_study` for merged-aware sample CRAMs. An empty
  product-level attachment does not prove that a sample-level CRAM is absent;
  this tool prefers the merged composite when one exists.
- iRODS `created` means data was added to iRODS, not source mutation, row-change,
  or cache-sync time. Date windows are half-open: `since` is inclusive and
  `until` is exclusive.
- Global run and run-aggregate rows label their per-platform `date_basis`:
  Illumina and Elembio use run complete, Ultimagen uses run archived, PacBio
  uses `run_complete`, and ONT warehouse load time is not a true sequencing
  date. Sample/product aggregate windows instead use iRODS `created`.
- Faculty sponsorship, programme membership, and `study_users` roles are
  distinct. `mlwh_study_users` without `role` returns all roles for a study;
  `mlwh_studies_for_user` without `role` uses the upstream owner, manager, and
  `data_access_contact` defaults. Use `mlwh_programmes` to discover the exact
  programme values accepted by programme filters.
- `mlwh_freshness` is the cache as-of source. Count and bare-list responses have
  no `cache_synced_at`; rows that do carry it are only complete through that
  synchronization state. Use per-table `last_run` (the oldest relevant across
  tables) or an endpoint's `cache_synced_at` for cache currency and "as of"
  caveats; `high_water` is only wa's source-progress watermark that may stay old
  when the source data is unchanged, so never read it as "synced through".

## What the server exposes

A curated, LLM-ergonomic tool surface generated from the MLWH endpoint registry
(so it stays in lockstep with the upstream API), grouped by task:

- **Chosen-column relationship export**: `mlwh_export`. It returns the upstream
  `Columns`/`Rows` matrix and its bounded or complete continuation metadata.
- **Search and study lookup**: `mlwh_search_samples`, `mlwh_count_samples`,
  `mlwh_search_studies`, `mlwh_count_studies_search`, `mlwh_count_studies`.
  Sample search defaults to the literal-prefix semantics above; study search is
  a case-insensitive substring across `name`, `study_title`, `programme`, and
  `faculty_sponsor`.
- **Resolve, classify, exact find, and expand**: `mlwh_classify_identifier`,
  `mlwh_resolve_sample`, `mlwh_resolve_sample_name`, `mlwh_resolve_study`,
  `mlwh_resolve_run`, `mlwh_resolve_library`,
  `mlwh_resolve_library_identifier`, `mlwh_find_samples`,
  `mlwh_count_find_samples`, `mlwh_expand_identifier`,
  `mlwh_expand_search_values`, `mlwh_expand_sample_search_values`.
- **Overview, status, and progress**: `mlwh_study_overview`,
  `mlwh_study_status_breakdown`, `mlwh_run_overview`, `mlwh_run_status`,
  `mlwh_sample_progress`. These are the preferred tools for common availability,
  QC, run, and "what is happening?" questions.
- **Availability and iRODS**:
  `mlwh_count_samples_with_data_for_study`,
  `mlwh_samples_with_data_for_study`,
  `mlwh_samples_without_data_for_study`,
  `mlwh_latest_data_for_study`, `mlwh_count_latest_data_for_study`,
  `mlwh_latest_data_for_faculty_sponsor`,
  `mlwh_count_latest_data_for_faculty_sponsor`,
  `mlwh_irods_paths_for_sample`, `mlwh_count_irods_paths_for_sample`,
  `mlwh_irods_paths_for_study`, `mlwh_count_irods_paths_for_study`,
  `mlwh_irods_paths_for_run`, `mlwh_count_irods_paths_for_run`,
  `mlwh_sample_crams_for_study`, `mlwh_count_sample_crams_for_study`.
- **Global runs and aggregates**: `mlwh_runs`, `mlwh_count_runs`,
  `mlwh_monthly_run_counts`, `mlwh_sequencing_aggregate`.
- **Detail, fan-out, and count counterparts**: `mlwh_sample_detail`,
  `mlwh_study_detail`, `mlwh_run_detail`, `mlwh_library_detail`,
  `mlwh_all_studies`, `mlwh_samples_for_study`, `mlwh_count_samples_for_study`,
  `mlwh_samples_for_run`, `mlwh_count_samples_for_run`,
  `mlwh_libraries_for_study`, `mlwh_count_libraries_for_study`,
  `mlwh_runs_for_study`, `mlwh_count_runs_for_study`,
  `mlwh_runs_for_sample`, `mlwh_count_runs_for_sample`,
  `mlwh_lanes_for_sample`, `mlwh_count_lanes_for_sample`,
  `mlwh_studies_for_sample`, `mlwh_count_studies_for_sample`,
  `mlwh_count_samples_for_library`,
  `mlwh_count_samples_for_library_id`,
  `mlwh_count_samples_for_library_lims_id`,
  `mlwh_count_samples_for_library_type`.
- **Programmes, people, sponsors, and study users**:
  `mlwh_studies_for_programme`, `mlwh_count_studies_for_programme`,
  `mlwh_programmes`, `mlwh_study_users`, `mlwh_count_study_users`,
  `mlwh_studies_for_faculty_sponsor`,
  `mlwh_count_studies_for_faculty_sponsor`, `mlwh_studies_for_user`,
  `mlwh_count_studies_for_user`, `mlwh_resolve_person`,
  `mlwh_count_resolve_person`.
- **Freshness**: `mlwh_freshness` reports per-table sync state so answers can be
  caveated; it succeeds even against a never-synced cache.
- **Escape hatch**: `mlwh_call_endpoint` dispatches any upstream Registry method
  by name. If the upstream response includes pagination headers, the tool returns
  `result`, `total`, and `next_offset`; otherwise it returns the decoded result
  directly.

The server also publishes two MCP resources:

- `mlwh://workflow`: cheap-first guidance plus the always-current
  Registry-derived endpoint catalogue.
- `mcp-server://version`: this server's version and the targeted MLWH API
  version.

Upstream errors are mapped to clear, actionable tool errors: bad input, not
found, ambiguous identifiers, unsupported identifiers, never-synced cache state,
and impaired upstream/cache failures all keep the upstream context and add a
caller-oriented next step.

## Development

```bash
make test      # hermetic test suite (stubs the MLWH API; never hits a live warehouse)
make lint      # golangci-lint over all packages
make format    # gofmt (and cleanorder, if installed)
make help      # list all targets
```

CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs `make lint` and
`make build` + `make test` on pushes and pull requests to `main` / `develop`,
using the same Makefile targets.

See [`.docs/mcp/spec.md`](.docs/mcp/spec.md) for the original MCP server
specification, and [`.docs/realworld3/spec.md`](.docs/realworld3/spec.md) for
the MLWH API 1.8.1 expansion reviewed here.

## Architecture

The server is built to host multiple independent services through a
service-agnostic core (`internal/core`) and self-contained providers. MLWH is the
first. Each service is its own binary: a thin `cmd/<service>-mcp-server`
entrypoint wires that service's `internal/<service>` provider into the shared
core. Adding a service is therefore a new `cmd/` + `internal/` package plus its
registration — with no core change — and each service keeps its own configuration,
auth, and (in future) transport.

```
go.mod                  module github.com/wtsi-hgi/llm-knowledge-base
Makefile                build / install / lint / test / config / start
cmd/mlwh-mcp-server/    MLWH server entrypoint (flag parsing, wiring only)
internal/core/          service-agnostic core (provider seam, transport, version)
internal/mlwh/          MLWH provider (imports wa/mlwh)
webui/                  Next.js + FastAPI web UI scaffold (future component)
```

## Web UI

A Next.js + FastAPI web UI scaffold lives under [`webui/`](webui/) as a future
component. It is currently a standalone scaffold, not yet wired to the MCP server;
see [`webui/README.md`](webui/README.md) for its own setup and developer
instructions.

## Licence

See [LICENSE](LICENSE).
