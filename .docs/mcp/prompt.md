# Feature: an MCP server for the MLWH read API, built to host multiple services

## Summary

Build a [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server
that lets LLM agents (e.g. Claude) query the **MLWH** (Multi-LIMS Warehouse) read
API in natural, tool-driven ways: search and find samples and studies, resolve
identifiers, drill into details and fan-outs, and report data freshness. That
read API is provided by a **separate upstream project, `wa`
(`github.com/wtsi-hgi/wa`)**, which ships the `wa mlwh serve` command. This
server is a thin, **well-described bridge** between MCP clients and that API —
its value is making those endpoints _ergonomic and correctly usable by an LLM_,
not re-implementing them.

Crucially, this server must be **built to host multiple, independent services**.
MLWH is only the first. At least one further service is expected later, and it
will **not** be part of `wa` — assume it is an entirely separate, unrelated
service with its own domain, client, auth and possibly transport. The
architecture must therefore not assume future services come from `wa`, resemble
MLWH, or share its types. Adding a service must be a matter of dropping in a new
provider module — never a rework of the core.

## Background: the MLWH API this server talks to (provided by the `wa` project)

`wa` (Go; module `github.com/wtsi-hgi/wa`) provides `wa mlwh serve`: a read-only
HTTP API over a synced cache of the warehouse. **The `wa` project is checked out
locally at `~/wa` (`/home/ubuntu/wa`)** — read its `mlwh/` package source and its
current `.docs/mcp/` references directly from that working copy when authoring the
spec; it is the authoritative material referred to throughout this prompt.

- It self-describes at `GET /openapi.json` (OpenAPI **3.1.0**; the API version is
  currently **1.6.0**) and has `GET /health` and `GET /freshness`.
- Endpoint families: word-prefix **sample search** + bounded **count**
  (`/search/sample/:term`, `/search/sample/:term/count`), **study** substring
  search + count, identifier **resolve** (`/resolve/...`), **find-by-field**
  (`/find/sample/...`), `classify` / `expand` / `enrich`, and **detail/fan-out**
  endpoints for samples, studies, runs and libraries.
- **Source of truth = the code + the live `GET /openapi.json`**, which the `wa`
  project generates at runtime from its endpoint `Registry` and served Go types,
  so it is always current — together with the `mlwh` Go package this server
  imports. Of the `wa` project's human docs under its `.docs/mcp/` directory,
  `api-reference.md` (generated from the same `Registry`, and guarded in `wa`'s
  CI by a no-drift golden test, so it cannot silently fall out of sync) and
  `glossary.md` are current for the external behaviour. **Do NOT trust
  `wa`'s `.docs/mcp/spec.md`, its `phase*.md`, or its `.docs/mcp/prompt.md` as
  current** — they are historical design docs that still contain superseded
  material from before later bug fixes (an FTS5 / MySQL-FULLTEXT ngram
  sample-search design and a `SupportsFullTextSearch` startup refusal that were
  _removed_ and replaced by a token-prefix index). Use them for background only
  and verify every claim against the code / OpenAPI.

Semantics the tools MUST convey to the agent so it queries correctly:

- Sample search is a **case-insensitive word-prefix** match over
  `name` / `supplier_name` / `common_name` / `donor_id`, **minimum term length 3**
  (so "mus" and "musculus" both match "Mus Musculus"; a mid-word substring does
  not). Study search is a case-insensitive **substring** match (over
  `name` / `study_title` / `programme` / `faculty_sponsor`) and shares the same
  **minimum term length 3**.
- Sample **counts are exact only up to a cap (10000)** and report the cap as a
  **floor** for very common terms (a `count` of 10000 means "at least 10000").
- Results are **paginated** (bounded default and maximum page sizes; over-max is
  rejected, not clamped).
- `/freshness` returns per-table `ever_synced` and high-water timestamps and
  succeeds even on a never-synced cache, so the agent can **caveat staleness** or
  detect "not synced yet".

## HARD REQUIREMENT 1 — Go, importing the `wa` project's `mlwh` package

Implement this server in **Go**, and have the MLWH provider **import
`github.com/wtsi-hgi/wa/mlwh`** rather than re-implement the HTTP/JSON client or
the data types. There is a concrete, substantial benefit to depending on that
package, so it is a requirement, not a preference:

- Make every MLWH call through `mlwh.NewRemoteClient(mlwh.RemoteConfig{BaseURL: ...})`,
  which returns a `*mlwh.RemoteClient` that implements the **`mlwh.Queryer`**
  interface — the full (~40-method) MLWH read API (`SearchSamples`,
  `CountSampleSearch`, `SearchStudies`, `ResolveSample`, `SampleDetail`,
  `Freshness`, …). Typed in, typed out, version-locked to the upstream server.
- Reuse the package's exported response types (`mlwh.Sample`, `mlwh.Study`,
  `mlwh.Count`, `mlwh.Freshness`, `mlwh.Match`, `mlwh.SampleDetail`, …) directly
  as MCP tool result payloads.
- Generate the MLWH tool set from **`mlwh.Registry`** (`[]mlwh.Endpoint`) and
  `mlwh.OpenAPIDocument()`. Each `Endpoint` carries `Method`, `Verb`, `Path`,
  path/query params, pagination info, and a required non-empty `Summary` and
  `Description` — ideal raw material for MCP tool names, descriptions and input
  schemas that stay in lockstep with the upstream API version.
- Net effect: no serialization/type drift, compile-time coupling to the upstream
  API version, and new MLWH endpoints surface in this server with little or no
  extra code.

Go is therefore this server's implementation language. Note the import
requirement is specific to the **MLWH provider**: a future, unrelated service is
integrated by its own provider module using whatever client suits it — the core
stays Go and provider-agnostic, making no assumption that other services expose a
Go package to import.

The `wa` module is imported like any other Go module dependency — a standard
`require` on `github.com/wtsi-hgi/wa` (for MLWH, that imported package _is_ the
contract). Supporting prose can be pulled from `wa`'s
`.docs/mcp/api-reference.md` / `glossary.md`, but the package API and the live
`/openapi.json` are authoritative.

## HARD REQUIREMENT 2 — multi-service architecture (future services are not `wa` services)

Architect around a **service-provider abstraction**:

- The **core** is service-agnostic: MCP transport, tool/resource registration,
  configuration, logging, health, and error mapping.
- Each backing service is a self-contained **provider** that registers its own
  MCP tools/resources and supplies its own typed client. The MLWH provider (which
  imports `wa/mlwh`) is the first.
- Assume at least one more service is coming that is **unrelated to `wa`** and may
  differ completely — different domain, client library, authentication, and even
  transport. The seam must not leak MLWH- or `wa`-specific assumptions into the
  core, and must not require an importable Go package the way the MLWH provider
  happens to have. **Adding that service must require only a new provider package
  plus its registration — no changes to the core.**
- A provider owns: its config (e.g. base URL/credentials), client construction,
  tool/resource set and descriptions, and the mapping of its domain results to
  MCP outputs. Cross-cutting concerns stay in the core.
- Only the MLWH provider needs to be implemented in this round; the others just
  need a clean, proven place to plug in.

## HARD REQUIREMENT 3 — repository layout: coexist cleanly with the pre-existing web UI

This is **not a greenfield repo**. It already contains a working "hello world"
full-stack scaffold — a **Next.js 16 (App Router) frontend** (`frontend/`) and a
**FastAPI backend** (`backend/`), tied together by a root `run-dev.sh`,
`README.md` and `.env.example`. That scaffold exists for **possible future use as
a web UI** onto an LLM chat that will itself be a client of the MCP server(s)
described here (MLWH first, others later). Priority and scope:

- **The Go MCP server is the first and primary deliverable**, and must be
  immediately usable over **stdio** by local agent CLIs — **Claude Code** and
  **Codex** are the reference clients — with no web UI in the loop.
- The existing web UI scaffold is **out of scope to build on or change
  functionally** in this round. No web UI code should be **deleted** — treat it
  as a placeholder to be grown later.
- **Leave room for easy future integration**: the eventual web UI's server-side
  agent is just another MCP client. This may well have **zero impact** on the MCP
  server's own design — if so, do not distort the design to accommodate it. (At
  most it is a further reason to keep a streamable-HTTP transport option open;
  stdio stays the baseline — see Configuration & deployment.)

The concrete requirement here: **the spec must settle a clean top-level repository
layout** so the Go server and the web UI scaffold are not confusingly
intermingled. Lean:

- Relocate the existing web UI — `frontend/`, `backend/`, and its web-specific
  root files (`run-dev.sh`, the web `.env.example`, and the web-oriented
  `README.md` content) — under a dedicated **`webui/`** subfolder, and give the
  Go MCP server a clear home of its own (a repo-root Go module, or a sibling
  subfolder such as `server/` — the spec picks one and says why).
- Keep the repo coherent after the move: a root `README.md` describing the whole
  project (MCP-server-first, web UI as a future component), a `.gitignore`
  covering Go as well as Node/Python, and `LICENSE` left at the root. The move is
  purely organisational — relocation, not rewrites.

## Functional requirements (MLWH provider)

- Expose the MLWH read operations as MCP tools with LLM-ergonomic names,
  descriptions and input schemas, derived from / consistent with `mlwh.Registry`
  + OpenAPI. Cover at least: sample search + count, study search + count,
  identifier resolve/find, sample/study/run/library detail & fan-out, and
  freshness.
- Bake the search/count/pagination semantics above into tool descriptions so the
  agent uses them correctly (word-prefix, min length 3, count-floor at 10000,
  default/max page sizes).
- Help the agent with **multi-step workflows** (e.g. resolve an identifier →
  fetch detail → expand) via clear tool descriptions and/or an MCP resource or
  prompt that explains how the endpoints compose. The exported
  `mlwh.EndpointReference()` generates the always-current Markdown endpoint
  catalogue (Registry-derived — the same source as `api-reference.md`) and is
  ideal raw material for such a resource; prefer calling it over shipping a
  copied doc.
- Handle pagination, validate inputs, and map API errors into clear MCP tool
  errors the agent can act on. Prefer matching the exported, `errors.Is`-checkable
  `mlwh.Err*` sentinels the typed client returns (`ErrNotFound`,
  `ErrCacheNeverSynced`, `ErrAmbiguous`, `ErrUnsupportedIdentifier`, …) over
  parsing HTTP status, and cover all six stable error codes the API documents:
  400 bad_request ("term too short / bad input"), 404 not_found, 409 ambiguous
  ("matches multiple records — disambiguate"), 422 unsupported_identifier, 502
  upstream_impaired, and 503 cache_never_synced ("warehouse cache not synced
  yet").
- Surface freshness so answers can be caveated.

## Configuration & deployment

- Configurable MLWH server base URL (that API is internal, no-auth HTTP; optional
  CA cert for TLS — `mlwh.RemoteConfig` already supports timeout + CA cert).
  Per-service configuration should generalise cleanly to future providers, each
  of which may need quite different settings. Sensible env-var / flag based
  config.
- MCP transport: **stdio** as the baseline (local agents such as Claude Code /
  Codex / desktop); consider streamable HTTP if remote/shared hosting is wanted —
  e.g. the future web UI's server-side agent would be such a shared consumer. The
  spec should settle this, keeping stdio as the default.

## Non-functional / engineering conventions

- Idiomatic Go with a **TDD / behaviour-focused testing discipline** and the
  repo's lint and test gates. Tests must be **hermetic** — exercise the MLWH
  provider against a stub/fake MLWH server (or `RemoteClient` against
  `httptest`), never a live warehouse.
- Report both this server's own version and the MLWH API version it targets.

## Out of scope (for now)

- Any **writes** to the warehouse (the API is read-only).
- Re-implementing the MLWH HTTP API itself (it lives in the `wa` project).
- The future service's specific tools — only the **extensibility seam** is in
  scope now.

## Key design decisions for the spec to settle

- **Tool granularity / generation strategy:** auto-generate one MCP tool per
  `mlwh.Registry` endpoint vs. a curated, LLM-friendly subset (and/or grouped
  tools) vs. a hybrid (generated from the Registry as source of truth, with
  curated descriptions/grouping and possibly a generic "call endpoint" escape
  hatch). Lean: Registry-driven for correctness/lockstep, but do not blindly
  expose ~40 flat low-level tools if that harms agent usability.
- The exact **provider interface** — what a provider must implement to register
  tools/resources, config, and a client with the core — kept general enough that
  a non-`wa`, non-Go-importable service fits it cleanly.
- The **MCP Go SDK** to build on (recommendation: the official
  `github.com/modelcontextprotocol/go-sdk`; confirm during spec authoring).
- How `mlwh.Registry` / OpenAPI metadata maps to MCP tool input schemas and
  result shapes.

## Pointers / prior art (in order of authority)

1. **Authoritative, always current:** the `wa` project's `mlwh` Go package
   (`github.com/wtsi-hgi/wa`) — `Queryer`, `RemoteClient` + `RemoteConfig`,
   `Registry` + `Endpoint`, `OpenAPIDocument()`, and the response types — and a
   running `wa mlwh serve`'s `GET /openapi.json` (OpenAPI 3.1.0, generated from
   that code at runtime). All of this is readable locally under `~/wa/mlwh/`.
2. **Current external-behaviour references** (cross-check against 1), all under
   `~/wa/.docs/mcp/`: `api-reference.md` (generated, CI no-drift guarded) and
   `glossary.md`; `security-posture.md` for the no-auth/internal posture.
3. **Historical only — do NOT treat as the contract:** in the same
   `~/wa/.docs/mcp/` directory, `spec.md`, the `phase*.md` build plans, and that
   project's own `prompt.md` (see the superseded-material warning under
   "Background: the MLWH API this server talks to" above). Background context at
   most.

## Notes

Decisions settled during requirements clarification. These refine — and take
precedence over — any looser phrasing above:

- **Tool generation strategy (hybrid, with escape hatch):** generate the MLWH
  tool set from `mlwh.Registry` as the source of truth, but expose a curated,
  LLM-ergonomic surface — clear tool names/descriptions, with related endpoints
  grouped (e.g. unify the `FindSamplesBy*` endpoints into a single `find_samples`
  tool with a field enum; group the sample/study/run/library detail endpoints).
  Additionally provide one generic, Registry-driven "call any MLWH endpoint"
  escape-hatch tool so no endpoint is unreachable.
- **Result shape (structured + text):** every tool returns both typed
  `structuredContent` (output schemas generated from the `mlwh` Go result types)
  and a JSON text rendering of the same data, so structured-aware and text-only
  clients both work. The result types carry their field-level documentation in
  custom `doc:"…"` struct tags that feed `mlwh.OpenAPIDocument()`'s
  `components.schemas` — **not** the `json` / `jsonschema` tags the MCP SDK's
  struct reflection reads — so to preserve those field descriptions, source the
  output schemas (or at least their field docs) from the OpenAPI component
  schemas rather than from naive struct reflection.
- **MCP SDK:** build on the official `github.com/modelcontextprotocol/go-sdk`,
  pinned to **v1.6.1**.
- **Repository layout (root Go module):** the MCP server is a Go module rooted at
  the repository root (`go.mod` at root; entrypoint under `cmd/`, with internal
  packages alongside). Relocate the existing web UI — `frontend/`, `backend/`,
  and the web-specific root files (`run-dev.sh`, the web `.env.example`, and the
  web-oriented `README.md` content) — under a `webui/` subfolder. Keep `LICENSE`
  at the root, add a root `README.md` describing the whole project (MCP-first,
  web UI as a future component), and a `.gitignore` covering Go as well as
  Node/Python. This mirrors the `wa` repo's own layout (root Go module +
  `frontend/` subdir).
- **MLWH API version (build-time/static):** read the targeted MLWH API version
  from the exported **`mlwh.APIVersion`** constant — a compiled-in string from
  the imported `wa` package, so reporting it is a typed, compile-time lookup that
  never contacts a live MLWH server. (`mlwh.OpenAPIDocument()`'s `info.version`
  derives from that same constant, so the two cannot drift.)
- **Version surfacing (all of these):** surface both this server's own version
  and the targeted MLWH API version via (1) a `--version` CLI flag that prints
  them and exits, (2) an MCP resource the agent can read at runtime to caveat
  answers, (3) the startup logs, and (4) the MCP server's advertised
  implementation info / instructions seen by clients on connect.
- **Transport scope (stdio only this round):** implement the **stdio** transport
  only. Design the transport as a clean seam so streamable-HTTP can be added
  later with no core changes, but write no HTTP transport code, config, or tests
  in this round.
- **Multi-service seam proof (test-only fake provider):** prove the provider seam
  with a **fake provider defined in test code** that registers through the same
  core provider interface as MLWH. A test must assert that the fake's tools
  register and appear alongside MLWH's and that adding it requires no changes to
  the core. The shipped binary registers only the MLWH provider.
- **Escape-hatch tool contract:** the generic "call any MLWH endpoint" tool takes
  the target endpoint as a `mlwh.Registry` **`Method` name** (e.g. `SampleDetail`)
  plus `path_params` and `query_params` maps (the latter including `limit` /
  `offset`), and returns an **untyped JSON passthrough** (the raw decoded result
  as `structuredContent`, with no per-endpoint output schema). It dispatches
  through the exported **`(*mlwh.RemoteClient).Call(ctx, method, pathParams,
  query)`** generic dispatcher — the same typed client every other tool uses — so
  the escape hatch is a thin pass-through (look the `Method` up in
  `mlwh.Registry`, forward the params to `Call`, serialise the returned value),
  with no reflection and no per-endpoint switch.
- **Constrained enums for fixed value-sets:** where a tool input is a fixed set —
  identifier kinds from the exported `mlwh.IdentifierKinds()`, and the
  `FindSamplesBy*` field set behind the unified `find_samples` tool (its members
  derivable by filtering `mlwh.Registry` for the `FindSamplesBy` `Method` prefix,
  then mapped to clean field names) — generate a constrained **enum** in the
  input schema (values sourced from the code, not hand-maintained) so invalid
  kinds/fields are rejected at the schema layer and the agent sees the allowed
  values.

Factual grounding (from reading the `wa/mlwh` code — authoritative over prose
above): the live `mlwh.Registry` currently exposes 40 endpoints (all `GET`, one
per `Queryer` method), and
`mlwh.RemoteConfig` exposes `BaseURL`, `Timeout`, `CACert`, a bearer `Token`, and
`CacheTTL` — but `NewRemoteClient` reads only the first four; `CacheTTL` is inert
for the remote client (it configures the local cache-backed client), so don't
wire it expecting an effect. Treat the code and the live `/openapi.json` as the source of
truth for the exact endpoint set, the result types, and the config fields.
