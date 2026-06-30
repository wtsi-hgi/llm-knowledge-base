# MLWH MCP Server

A [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server, written
in Go, that lets LLM agents (Claude Code, Codex, and later a web UI server-side
agent) query the Multi-LIMS Warehouse (MLWH) read API in natural, tool-driven
ways: search and count samples and studies, resolve identifiers, answer cheap
overview/status/availability questions, page through manifests and iRODS paths,
route people and sponsor questions, drill into detail and fan-outs, and report
data freshness.

The MLWH read API is provided by the separate upstream
[`wa`](https://github.com/wtsi-hgi/wa) project (its `wa mlwh serve` command). This
server is a thin, well-described bridge whose value is making those endpoints
ergonomic and correctly usable by an LLM, not re-implementing them: it imports
`github.com/wtsi-hgi/wa/mlwh` and reuses its typed client, response types,
registry, and OpenAPI document directly, so there is no type drift and the server
is compile-time-locked to the upstream API version (currently MLWH API 1.7.0).

The MLWH provider currently ships over the **stdio** transport only, so it runs
as a local subprocess launched per user by an agent CLI. Streamable HTTP, which
would let an admin run one shared instance everyone connects to over the network,
is deliberately deferred, but the transport is a clean seam so it can be added
later without any core change.

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
# MLWH API version 1.7.0
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
  - "What study id matches the name `cancer`?"
  - "List study candidates for `rare disease`, enough to disambiguate the id."
- **Resolve or classify an identifier**
  - "What kind of MLWH identifier is `5901`?"
  - "Resolve sample `S1` to its canonical identifiers."
  - "Resolve run `52553`."
  - "Expand this study id into related search values."
- **Exact sample lookup**
  - "Find samples where accession is exactly `ERS123`."
  - "Count samples with supplier name exactly `Bob`."
  - "Find samples for library type `WGS`."
- **Study overview and metadata**
  - "Give me a quick overview of study `S1`."
  - "What data access group is study `S1` in?"
  - "How many samples in `S1` have data, have no data, or were sequenced but have no data?"
- **Availability and recency**
  - "How many samples in study `S1` have data added since `2026-06-21T00:00:00Z`?"
  - "List the first page of samples in `S1` with data between these two timestamps."
  - "Which samples in `S1` still have no sequencing data?"
  - "How much new data was added to iRODS for `S1` in the last 7 days?"
- **QC and status**
  - "Break down study `S1` by received, sequenced, not sequenced, and manual QC state."
  - "How many samples in `S1` passed, failed, or are pending QC?"
  - "Show status by platform, preserving ONT and not-tracked values."
- **Sample and run progress**
  - "What is happening with sample `S1` right now?"
  - "Show sample `ONT1` progress, including not-tracked QC if present."
  - "Summarize run `52553`."
  - "Show the status timeline for run `52553`."
- **iRODS paths and CRAMs**
  - "How many CRAM paths are there for sample `S1`?"
  - "List CRAM iRODS paths for study `S1`, first page only."
  - "Show iRODS paths for run `52553` with `file_type=cram`."
  - "Are there any VCF paths for `S1`?"
- **Study manifest**
  - "Build a manifest for study `S1`."
  - "Build a study `S1` manifest with an iRODS CRAM column."
  - "How many manifest rows would study `S1` return?"
- **People, sponsors, and users**
  - "Which studies have faculty sponsor `Carl`?"
  - "How many studies is `cwa` associated with as a user?"
  - "List studies where `cwa` has role `Follower`."
  - "Resolve the person name `Carl` across sponsors and study users."
- **Detail and fan-outs**
  - "Show sample detail for `S1`."
  - "Show lean study detail for `S1`, page size 100."
  - "List runs for study `S1`."
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
  - "Use a smaller page if the result is too large."
- **Advanced registry access**
  - "Call the MLWH Registry method `AllStudies` with `limit=100` and `offset=0`."
  - "Use the generic endpoint caller for a Registry method that does not have a curated tool yet."

Notes for interpreting answers:

- Paged list and paged detail tools default to `limit=100`, reject `limit > 1000`,
  and return `total` plus `next_offset`; `next_offset=-1` means there is no next
  page.
- Count responses and bare list responses do not carry `cache_synced_at`; ask the
  agent to use `mlwh_freshness` when you need a cache as-of caveat.
- Responses that include `cache_synced_at` should use that field as the as-of
  timestamp. Data "added to iRODS" means the upstream iRODS `created` timestamp,
  not sync `last_run` or generic row-change fields.
- If a response would be too large, the server returns a structured
  `tool_result_too_large` error with guidance instead of flooding the chat.

## What the server exposes

A curated, LLM-ergonomic tool surface generated from the MLWH endpoint registry
(so it stays in lockstep with the upstream API), grouped by task:

- **Search and study lookup**: `mlwh_search_samples`, `mlwh_count_samples`,
  `mlwh_search_studies`, `mlwh_count_studies_search`, `mlwh_count_studies`.
  Sample search is a case-insensitive word-prefix match; study search is a
  case-insensitive substring match across `name`, `study_title`, `programme`,
  and `faculty_sponsor`.
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
- **Availability, iRODS, and manifests**:
  `mlwh_count_samples_with_data_for_study`,
  `mlwh_samples_with_data_for_study`,
  `mlwh_samples_without_data_for_study`,
  `mlwh_irods_paths_for_sample`, `mlwh_count_irods_paths_for_sample`,
  `mlwh_irods_paths_for_study`, `mlwh_count_irods_paths_for_study`,
  `mlwh_irods_paths_for_run`, `mlwh_count_irods_paths_for_run`,
  `mlwh_study_manifest`, `mlwh_count_study_manifest`. iRODS tools support
  upstream `file_type` suffix filtering, such as `cram`.
- **Detail, fan-out, and count counterparts**: `mlwh_sample_detail`,
  `mlwh_study_detail`, `mlwh_run_detail`, `mlwh_library_detail`,
  `mlwh_all_studies`, `mlwh_samples_for_study`, `mlwh_count_samples_for_study`,
  `mlwh_samples_for_run`, `mlwh_count_samples_for_run`,
  `mlwh_libraries_for_study`, `mlwh_count_libraries_for_study`,
  `mlwh_runs_for_study`, `mlwh_count_runs_for_study`,
  `mlwh_lanes_for_sample`, `mlwh_count_lanes_for_sample`,
  `mlwh_studies_for_sample`, `mlwh_count_samples_for_library`,
  `mlwh_count_samples_for_library_id`,
  `mlwh_count_samples_for_library_lims_id`,
  `mlwh_count_samples_for_library_type`.
- **People, sponsors, and study users**:
  `mlwh_studies_for_faculty_sponsor`,
  `mlwh_count_studies_for_faculty_sponsor`, `mlwh_studies_for_user`,
  `mlwh_count_studies_for_user`, `mlwh_resolve_person`,
  `mlwh_count_resolve_person`. Sponsor tools read `faculty_sponsor`; user tools
  read `study_users` membership and optionally filter by role.
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
specification, and [`.docs/realworld/spec.md`](.docs/realworld/spec.md) for the
RealWorld MLWH tool expansion reviewed here.

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
