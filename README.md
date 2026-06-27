# MLWH MCP Server

A [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server, written
in Go, that lets LLM agents (Claude Code, Codex, and later a web UI server-side
agent) query the Multi-LIMS Warehouse (MLWH) read API in natural, tool-driven
ways: search and count samples and studies, resolve identifiers, drill into
detail and fan-outs, and report data freshness.

The MLWH read API is provided by the separate upstream
[`wa`](https://github.com/wtsi-hgi/wa) project (its `wa mlwh serve` command). This
server is a thin, well-described bridge whose value is making those endpoints
ergonomic and correctly usable by an LLM, not re-implementing them: it imports
`github.com/wtsi-hgi/wa/mlwh` and reuses its typed client, response types,
registry, and OpenAPI document directly, so there is no type drift and the server
is compile-time-locked to the upstream API version (currently MLWH API 1.6.0).

This first round ships the MLWH provider over the **stdio** transport only, so it
runs as a local subprocess launched per user by an agent CLI. Streamable HTTP —
which would let an admin run one shared instance everyone connects to over the
network — is deliberately deferred, but the transport is a clean seam so it can be
added later without any core change.

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
# MLWH API version 1.6.0
```

## Configuration

The server needs to know where your MLWH API lives. Configure it with environment
variables, or the equivalent command-line flags (a flag overrides its env var):

| Env var         | Flag               | Required | Meaning                                                                          |
| --------------- | ------------------ | -------- | -------------------------------------------------------------------------------- |
| `MLWH_BASE_URL` | `--mlwh-base-url`  | **yes**  | Base URL of the `wa mlwh serve` HTTP API, e.g. `http://mlwh.internal:8080`.      |
| `MLWH_CA_CERT`  | `--mlwh-ca-cert`   | no       | Path to a PEM CA-certificate file, if the API is served over TLS with a private CA. |
| `MLWH_TIMEOUT`  | `--mlwh-timeout`   | no       | Per-request HTTP timeout as a Go duration (e.g. `15s`, `1m`).                     |

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

## What the server exposes

A curated, LLM-ergonomic tool surface generated from the MLWH endpoint registry
(so it stays in lockstep with the upstream API), grouped by task:

- **Search & count** — `mlwh_search_samples`, `mlwh_count_samples`,
  `mlwh_search_studies`, `mlwh_count_studies_search`, `mlwh_count_studies`. The
  descriptions carry the semantics an agent needs: sample search is a word-prefix
  match, study search a substring match, both with a 3-character minimum; counts
  are exact up to a floor of 10000.
- **Resolve, find & expand** — `mlwh_classify_identifier`, seven `mlwh_resolve_*`
  tools, the unified `mlwh_find_samples` (a `field` enum over the exact-match
  endpoints), and the `mlwh_expand_*` tools.
- **Detail & fan-out** — `mlwh_sample_detail` / `_study_detail` / `_run_detail` /
  `_library_detail`, plus list/fan-out tools (`mlwh_all_studies`,
  `mlwh_samples_for_study`, `mlwh_irods_paths_for_sample`, …). Omitting `limit` on
  a fan-out fetches every row.
- **Freshness** — `mlwh_freshness` reports per-table sync state so answers can be
  caveated; it succeeds even against a never-synced cache.
- **Escape hatch** — `mlwh_call_endpoint` dispatches any registry method by name,
  so no endpoint is unreachable even if it lacks a curated tool.

Plus two MCP resources: `mlwh://workflow` (a Markdown endpoint catalogue with
guidance on composing calls — resolve → detail → expand) and `mcp-server://version`
(this server's version and the targeted MLWH API version). Upstream errors are
mapped to clear, actionable tool errors (not found; ambiguous — disambiguate;
unsupported identifier; cache not synced yet; …).

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

See [`.docs/mcp/spec.md`](.docs/mcp/spec.md) for the full specification and
[`.docs/mcp/`](.docs/mcp/) for the phased implementation plan.

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
