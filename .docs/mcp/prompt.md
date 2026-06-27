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
HTTP API over a synced cache of the warehouse.

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
  not).
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

The spec should state how the `wa` module is made available as a dependency (a Go
module dependency is the key part — for MLWH, the imported package _is_ the
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
  prompt that explains how the endpoints compose.
- Handle pagination, validate inputs, and map API errors into clear MCP tool
  errors the agent can act on (e.g. 503 never-synced → "warehouse cache not
  synced yet"; 400 → "term too short / bad input"; 404 → "not found").
- Surface freshness so answers can be caveated.

## Configuration & deployment

- Configurable MLWH server base URL (that API is internal, no-auth HTTP; optional
  CA cert for TLS — `mlwh.RemoteConfig` already supports timeout + CA cert).
  Per-service configuration should generalise cleanly to future providers, each
  of which may need quite different settings. Sensible env-var / flag based
  config.
- MCP transport: **stdio** as the baseline (local agents such as Claude Code /
  desktop); consider streamable HTTP if remote/shared hosting is wanted. The spec
  should settle this.

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
   that code at runtime).
2. **Current external-behaviour references** (cross-check against 1): the `wa`
   project's `.docs/mcp/api-reference.md` (generated, CI no-drift guarded) and
   `.docs/mcp/glossary.md`; its `.docs/mcp/security-posture.md` for the
   no-auth/internal posture.
3. **Historical only — contains superseded/incorrect material, do NOT treat as
   the contract:** the `wa` project's `.docs/mcp/spec.md` (mixes the old
   FTS5/FULLTEXT design with later word-prefix reconciliation notes), its
   `.docs/mcp/phase*.md` build plans, and its `.docs/mcp/prompt.md`. Background
   context at most.
