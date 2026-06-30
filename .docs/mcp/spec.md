# MLWH MCP Server Specification

## Overview

A Model Context Protocol (MCP) server, written in Go, that lets LLM agents
(Claude Code, Codex, and later a web UI server-side agent) query the
Multi-LIMS Warehouse (MLWH) read API in natural, tool-driven ways:
search/count samples and studies, resolve identifiers, drill into detail and
fan-outs, and report data freshness. The MLWH read API is provided by the
separate upstream `wa` project (`github.com/wtsi-hgi/wa`); this server is a
thin, well-described bridge whose value is making those endpoints ergonomic and
correctly usable by an LLM, not re-implementing them. The MLWH provider imports
`github.com/wtsi-hgi/wa/mlwh` and reuses its typed `Queryer` client, response
types, `Registry`, and `OpenAPIDocument()` directly, so there is no type drift
and the server is compile-time-locked to the upstream API version.

The server is built to host multiple independent services through a
service-agnostic core and self-contained providers. MLWH is the first provider.
At least one further, unrelated service is expected later; it will not be a `wa`
service and may differ in domain, client, auth, and transport. Adding a service
must require only a new provider package plus its registration - no core change.

This round ships the MLWH provider over the stdio transport only. Streamable
HTTP is deliberately deferred but the transport is a clean seam so it can be
added later without core changes. Tests are hermetic: the MLWH provider is
exercised against an `httptest` stub MLWH server, never a live warehouse.

## Architecture

### Repository layout (root Go module)

The MCP server is a Go module rooted at the repository root. The pre-existing
Next.js + FastAPI web UI scaffold is relocated under `webui/` unchanged. This
mirrors the `wa` repo's own layout (root Go module + `frontend/` subdir).

```
go.mod                      module github.com/wtsi-hgi/llm-knowledge-base, go 1.25
go.sum
LICENSE                     unchanged, stays at root
README.md                   NEW: whole-project README (MCP-first; web UI future)
.gitignore                  UPDATED: Go + Node + Python
cmd/
  mcp-server/
    main.go                 CLI entrypoint: flag parsing, wiring only
internal/
  core/                     service-agnostic core (no MLWH/wa types)
    server.go               server build + provider registration + Run
    provider.go             Provider interface, Registrar, version info
    transport.go            transport seam (stdio only this round)
    version.go              build-time version vars + version resource
    errs.go                 generic tool-error helpers (text + IsError)
  mlwh/                     MLWH provider (imports wa/mlwh)
    provider.go             Provider impl, RemoteConfig wiring, registration
    tools_search.go         search/count tools (sample + study)
    tools_resolve.go        resolve/classify + find_samples tools
    tools_detail.go         detail & fan-out tools
    tools_freshness.go      freshness tool
    tools_call.go           generic Call-based escape-hatch tool
    schema.go               output schemas + enums sourced from wa/mlwh
    errmap.go               wa/mlwh Err* -> MCP tool-error mapping
    workflow.go             workflow resource from EndpointReference()
webui/
  frontend/                 moved from ./frontend (unchanged)
  backend/                  moved from ./backend (unchanged)
  run-dev.sh                moved from ./run-dev.sh (unchanged)
  .env.example              moved from ./.env.example (web .env, unchanged)
  README.md                 NEW: holds the former root web-oriented README text
```

The repo-root Go module is chosen (over a `server/` subfolder) because it makes
the MCP server the primary artefact (`go build ./cmd/mcp-server`,
`go test ./...` from root), matches the `wa` repo's convention, and keeps the
web UI cleanly quarantined under one subfolder. The move is purely
organisational: no web UI code is deleted or rewritten.

### Dependencies

- `github.com/wtsi-hgi/wa/mlwh` - MLWH typed client, types, Registry, OpenAPI,
  `APIVersion`, `EndpointReference()`, `IdentifierKinds()`, `Err*` sentinels.
- `github.com/modelcontextprotocol/go-sdk/mcp` pinned to `v1.6.1` - MCP server,
  typed `AddTool[In,Out]`, resources, `StdioTransport`.

### MCP SDK facts the implementation relies on (verified against v1.6.1)

- `mcp.NewServer(*mcp.Implementation, *mcp.ServerOptions) *mcp.Server`.
  `Implementation{Name, Title, Version string}`. `ServerOptions{Instructions
  string, Logger *slog.Logger, ...}`.
- `mcp.AddTool[In, Out any](s *mcp.Server, t *mcp.Tool, h
  mcp.ToolHandlerFor[In, Out])`. `In`/`Out` must be struct or map (or `any`).
- `ToolHandlerFor[In,Out] func(context.Context, *mcp.CallToolRequest, In)
  (*mcp.CallToolResult, Out, error)`.
- Input schema: inferred from `In` reflection; property descriptions come from
  the `jsonschema:"..."` struct tag. `doc:` tags are NOT read by the SDK.
- Output schema: if `Tool.OutputSchema` is left nil and `Out != any`, it is
  inferred from `Out` reflection (again, `doc:` tags ignored). If
  `Tool.OutputSchema` is PRE-SET (to a `map[string]any` or `*jsonschema.Schema`
  of type "object"), the SDK uses it verbatim (resolves, does not overwrite).
  This is how field-level `doc:` descriptions are preserved - see Story F1.
- For a successful typed handler the SDK auto-populates
  `CallToolResult.StructuredContent` from the marshaled `Out` value AND, when
  `Content` is unset, auto-fills `Content` with a `TextContent` block holding
  the same JSON. So returning a typed `Out` yields structured+text for free.
- If `Out` is `any`, the output schema is omitted and `StructuredContent`
  carries whatever the handler returns (used by the escape-hatch tool).
- A non-nil `error` from the handler becomes a tool error
  (`CallToolResult.IsError=true`, message in `Content`), not a protocol error.
- Resources: `s.AddResource(*mcp.Resource{URI, Name, Title, Description,
  MIMEType string}, mcp.ResourceHandler)`. `ResourceHandler
  func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult,
  error)`. `ReadResourceResult{Contents []*mcp.ResourceContents}`.
  `ResourceContents{URI, MIMEType, Text string}`.
- Transport: `mcp.StdioTransport{}` (a `mcp.Transport`); run via
  `s.Run(ctx context.Context, t mcp.Transport) error` (blocks until peer
  disconnects or ctx is cancelled).

### wa/mlwh facts the implementation relies on (verified against ~/wa/mlwh)

- `mlwh.NewRemoteClient(mlwh.RemoteConfig) (*mlwh.RemoteClient, error)`;
  `*mlwh.RemoteClient` implements `mlwh.Queryer` (40 methods).
- `RemoteConfig{BaseURL string; Timeout time.Duration; Token string; CACert
  string; CacheTTL time.Duration}`. `NewRemoteClient` reads only BaseURL,
  Timeout, Token, CACert. `CacheTTL` is INERT for the remote client - do not
  wire it expecting an effect.
- `(*mlwh.RemoteClient).Call(ctx context.Context, method string, pathParams
  []string, query url.Values) (any, error)` - generic dispatcher keyed by
  `Registry` `Method` name; returns a pointer to the endpoint's result type
  (e.g. `*mlwh.Match`, `*[]mlwh.Study`); surfaces the same errors as the typed
  methods, including unknown method and path-param arity mismatch.
- `mlwh.Registry []mlwh.Endpoint`; `Endpoint{Method, Verb, Path string;
  PathParams []string; Query []string; Paginated bool; NewResult func() any;
  Summary, Description string; QueryParams []QueryParam}`. All 40 entries are
  `GET`, carry non-empty Summary+Description; paginated entries declare
  limit/offset QueryParams. Paginated fan-out entries use
  `fetchAllPaginationParams()` (limit defaults to the fetch-all page server-side
  when ABSENT); the two search entries use `searchPaginationParams()` (default
  100, max 1000).
- `mlwh.OpenAPIDocument() map[string]any` - OpenAPI 3.1.0; `components.schemas`
  carry per-field descriptions from `doc:` tags (snake_case names from `json:`
  tags); each operation carries `x-queryer-method` = the Registry Method name.
  `info.version` derives from `mlwh.APIVersion`.
- `mlwh.APIVersion` (const string, currently `"1.6.0"`) - compile-time targeted
  MLWH API version; reading it never contacts a server.
- `mlwh.EndpointReference() string` - deterministic Markdown endpoint catalogue,
  Registry-derived (same source as api-reference.md).
- `mlwh.IdentifierKinds() []mlwh.IdentifierKind` - 15 kinds in stable order
  (sample_uuid, sample_lims_id, sanger_sample_name, sanger_sample_id,
  supplier_name, sample_accession, donor_id, study_uuid, study_lims_id,
  study_accession, study_name, run_id, library_type, library_id,
  id_library_lims).
- `FindSamplesBy*` Registry methods (Method prefix `FindSamplesBy`):
  FindSamplesBySangerID, FindSamplesByIDSampleLims,
  FindSamplesByAccessionNumber, FindSamplesBySupplierName,
  FindSamplesByLibraryType (5).
- Sentinels (`errors.Is`-checkable): `ErrNotFound`, `ErrCacheNeverSynced`,
  `ErrAmbiguous`, `ErrUnsupportedIdentifier`, `ErrUpstreamImpaired`. The remote
  client maps the six documented codes back to sentinels: 404->ErrNotFound,
  409->ErrAmbiguous, 422->ErrUnsupportedIdentifier, 502->ErrUpstreamImpaired,
  503->ErrCacheNeverSynced; 400 has no dedicated sentinel and surfaces as
  ErrUpstreamImpaired (the remote client wraps unrecognised codes as
  ErrUpstreamImpaired). For SLICE-returning endpoints a never-synced cache
  returns `errors.Join(ErrCacheNeverSynced, ErrNotFound)`, so error mapping MUST
  check `ErrCacheNeverSynced` BEFORE `ErrNotFound`.
- Result types reused directly as tool payloads: `Sample`, `Study`, `Run`,
  `Lane`, `IRODSPath`, `Library`, `Match`, `Count`, `Freshness`
  (`{tables: [{table, high_water, last_run, ever_synced}]}`), `SampleDetail`,
  `StudyDetail`, `RunDetail`, `LibraryDetail`, `EnrichmentResult`, `TaggedID`,
  `SearchValues`. `Count` is `{count int}` only - no floor flag; `count==10000`
  for sample-search counts means "at least 10000" (see Story A2).

### Search/pagination/count semantics (must be conveyed in tool descriptions)

- Sample search: case-insensitive WORD-PREFIX over name, supplier_name,
  common_name, donor_id; minimum term length 3. "mus" and "musculus" both match
  "Mus Musculus"; a mid-word substring does not.
- Study search: case-insensitive SUBSTRING over name, study_title, programme,
  faculty_sponsor; minimum term length 3.
- Search pagination: default page 100, maximum 1000. Over-max limit is REJECTED
  (400), not clamped.
- Fan-out pagination (the C2 list tools): no bounded default - omitting `limit`
  fetches ALL rows. The handler achieves this by sending `limit=1000000` (the
  upstream fetch-all sentinel) when the caller omits it; sending `limit=0` would
  return ZERO rows (the server treats an explicit 0 as `LIMIT 0`, substituting
  its fetch-all page only when the param is absent). See Story C2.
- Sample-search count: exact up to a cap of 10000; a count of 10000 is a FLOOR
  ("at least 10000") for very common terms.
- Freshness reads succeed even on a never-synced cache (every table reports
  ever_synced=false, empty timestamps), so answers can be caveated.

### Core provider abstraction (service-agnostic seam)

```go
// Package internal/core.

// Registrar is the subset of *mcp.Server a provider may use to register its
// MCP surface. It hides everything else about the core from providers.
type Registrar interface {
    Server() *mcp.Server // for mcp.AddTool[In,Out](r.Server(), ...)
    AddResource(r *mcp.Resource, h mcp.ResourceHandler)
}

// Provider is a self-contained backing service. It owns its config, client
// construction, and MCP tool/resource set. The core knows nothing of any
// provider's domain, client, transport, or auth.
type Provider interface {
    // Name is a short, stable provider identifier (e.g. "mlwh").
    Name() string
    // Register adds this provider's tools and resources via the Registrar.
    // ctx bounds any setup; it returns an error if the provider cannot start.
    Register(ctx context.Context, r Registrar) error
}

// VersionInfo is the server's own version and the per-provider targeted
// upstream API versions, surfaced to clients.
type VersionInfo struct {
    ServerVersion string            // this server's build version
    APIVersions   map[string]string // provider name -> targeted upstream version
}

// New builds a configured core server (implementation info, instructions,
// logger, version resource) with no providers registered yet.
func New(opts Options) (*Server, error)

// Options configures the core server.
type Options struct {
    ServerVersion string
    Logger        *slog.Logger
    Providers     []Provider
}

// Run registers every configured provider then serves over the transport until
// ctx is cancelled or the peer disconnects.
func (s *Server) Run(ctx context.Context, t mcp.Transport) error
```

The core registers a version MCP resource (Story G2), sets the server's
`Implementation` info + `Instructions` (Story G3), and logs a startup version
line (Story G5) - all from `VersionInfo`, which it assembles by asking each
provider for its targeted API version. The MLWH provider supplies
`mlwh.APIVersion`. A provider must NOT need a Go-importable
package: the seam is `Provider` + `Registrar` only.

### Generic tool-error helper (core)

```go
// toolError returns a (*mcp.CallToolResult, zero Out, error) triple a typed
// handler can return directly; the SDK packs err into an IsError result.
// Providers map their own domain errors to a clear message before calling it.
```

## A. Sample & study search and count

### A1: Search samples by word prefix

As an agent, I want a `mlwh_search_samples` tool, so that I can find samples
whose words prefix-match a term.

Calls `(*RemoteClient).SearchSamples(ctx, term, limit, offset)`. Input struct:
`Term string` (min length 3), `Limit int` (default 100, max 1000),
`Offset int` (default 0). Output: `[]mlwh.Sample` (wrapped so output schema is
an object - see F2). The tool description states: case-insensitive word-prefix
over name/supplier_name/common_name/donor_id; min term 3; default page 100, max
1000 (over-max rejected). Short terms and over-max limits are rejected before
the call where cheaply detectable, otherwise via the mapped upstream error.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_search.go`
**Test file:** `internal/mlwh/tools_search_test.go`

**Acceptance tests:**

1. Given a stub MLWH server returning two samples for `/search/sample/mus`,
   when the tool is called with `{"term":"mus"}`, then the result's
   `StructuredContent` holds those two samples and `IsError` is false.
2. Given the same stub, when called with `{"term":"mus"}`, then `Content` has
   one text block whose JSON parses to the same two samples (structured+text).
3. Given any stub, when called with `{"term":"mu"}` (length 2), then the result
   is a tool error (`IsError=true`) whose message mentions the 3-character
   minimum, and no HTTP request is made to the stub.
4. Given any stub, when called with `{"term":"mus","limit":1001}`, then the
   result is a tool error mentioning the 1000 maximum, and no request is made.
5. Given a stub, when called with `{"term":"mus","limit":5,"offset":10}`, then
   the stub receives `limit=5` and `offset=10` as query parameters.
6. Given a stub, when called with `{"term":"mus"}` and no limit/offset, then the
   stub receives `limit=100` and `offset=0`.
7. The registered tool's name is `mlwh_search_samples` and its description
   contains "word-prefix" and "minimum" (3) and "1000".

### A2: Count samples matching a word prefix

As an agent, I want a `mlwh_count_samples` tool, so that I can size a sample
search without transferring rows, understanding the 10000 floor.

Calls `CountSampleSearch(ctx, term)`. Input: `Term string`. Output:
`mlwh.Count`. Description states the count is exact up to 10000 and that a
returned count of exactly 10000 means "at least 10000".

**File:** `internal/mlwh/tools_search.go`
**Test file:** `internal/mlwh/tools_search_test.go`

**Acceptance tests:**

1. Given a stub returning `{"count":42}` for `/search/sample/mus/count`, when
   called with `{"term":"mus"}`, then `StructuredContent` is `{"count":42}` and
   `IsError` is false.
2. Given a stub returning `{"count":10000}`, when called with `{"term":"a"}`
   (after the min-length guard - term "abc"), then `StructuredContent` is
   `{"count":10000}` and the tool description explains 10000 is a floor.
3. Given any stub, when called with `{"term":"ab"}`, then the result is a tool
   error mentioning the 3-character minimum and no request is made.
4. The tool name is `mlwh_count_samples` and its description contains "10000"
   and "at least".

### A3: Search studies by substring

As an agent, I want a `mlwh_search_studies` tool, so that I can find studies
whose fields contain a substring.

Calls `SearchStudies(ctx, term, limit, offset)`. Input as A1 (`Term`, `Limit`
default 100 max 1000, `Offset`). Output: `[]mlwh.Study`. Description states
case-insensitive substring over name/study_title/programme/faculty_sponsor; min
term 3; default 100, max 1000.

**File:** `internal/mlwh/tools_search.go`
**Test file:** `internal/mlwh/tools_search_test.go`

**Acceptance tests:**

1. Given a stub returning three studies for `/search/study/cancer`, when called
   with `{"term":"cancer"}`, then `StructuredContent` holds those three studies.
2. Given any stub, when called with `{"term":"ab"}`, then the result is a tool
   error mentioning the 3-character minimum and no request is made.
3. Given any stub, when called with `{"term":"x","limit":1001}` (term "xyz"),
   then the result is a tool error mentioning the 1000 maximum.
4. The tool name is `mlwh_search_studies` and its description contains
   "substring".

### A4: Count studies matching a substring; count all studies

As an agent, I want `mlwh_count_studies_search` and `mlwh_count_studies` tools,
so that I can size study searches and the whole study set.

`mlwh_count_studies_search` calls `CountStudySearch(ctx, term)` (input
`Term string`, min 3). `mlwh_count_studies` calls `CountStudies(ctx)` (no
input - input schema `{"type":"object"}`). Both output `mlwh.Count`.

**File:** `internal/mlwh/tools_search.go`
**Test file:** `internal/mlwh/tools_search_test.go`

**Acceptance tests:**

1. Given a stub returning `{"count":7}` for `/search/study/abc/count`, when
   `mlwh_count_studies_search` is called with `{"term":"abc"}`, then
   `StructuredContent` is `{"count":7}`.
2. Given any stub, when `mlwh_count_studies_search` is called with
   `{"term":"ab"}`, then the result is a tool error mentioning the minimum.
3. Given a stub returning `{"count":1234}` for `/studies/count`, when
   `mlwh_count_studies` is called with `{}`, then `StructuredContent` is
   `{"count":1234}`.

## B. Resolve, classify, and find

### B1: Resolve and classify identifiers

As an agent, I want resolve/classify tools, so that I can turn a raw identifier
into a canonical `Match` and learn its kind.

Tools (one per resolver, each input `Identifier string`, output `mlwh.Match`):
`mlwh_classify_identifier` (ClassifyIdentifier), `mlwh_resolve_sample`
(ResolveSample), `mlwh_resolve_sample_name` (ResolveSampleName),
`mlwh_resolve_study` (ResolveStudy), `mlwh_resolve_run` (ResolveRun),
`mlwh_resolve_library` (ResolveLibrary), `mlwh_resolve_library_identifier`
(ResolveLibraryIdentifier). Descriptions derive from the matching Registry
entry's Summary/Description.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_resolve.go`
**Test file:** `internal/mlwh/tools_resolve_test.go`

**Acceptance tests:**

1. Given a stub returning a sample `Match` (kind `sanger_sample_name`) for
   `/resolve/sample/ABC123`, when `mlwh_resolve_sample` is called with
   `{"identifier":"ABC123"}`, then `StructuredContent` is that Match with kind
   `sanger_sample_name`.
2. Given a stub returning 404 `{"code":"not_found",...}`, when
   `mlwh_resolve_sample` is called, then the result is a tool error
   (`IsError=true`) whose message indicates the identifier was not found.
3. Given a stub returning 409 `{"code":"ambiguous",...}`, when
   `mlwh_resolve_sample` is called, then the tool error message indicates the
   identifier matched multiple records and suggests disambiguating.
4. Given a stub returning 422 `{"code":"unsupported_identifier",...}`, when
   `mlwh_resolve_sample` is called, then the tool error indicates the
   identifier form is unsupported.
5. All seven resolve/classify tools are registered with the names listed above.

### B2: Find samples by exact field (unified, enum-driven)

As an agent, I want a single `mlwh_find_samples` tool with a `field` enum, so
that I can find samples by an exact field value without choosing among five
flat tools.

One tool unifies the five `FindSamplesBy*` endpoints. Input: `Field string`
(constrained enum), `Value string`. The enum values are derived from
`mlwh.Registry` by filtering for the `FindSamplesBy` Method prefix, then mapped
to clean field names: `sanger_id` (FindSamplesBySangerID), `lims_id`
(FindSamplesByIDSampleLims), `accession` (FindSamplesByAccessionNumber),
`supplier_name` (FindSamplesBySupplierName), `library_type`
(FindSamplesByLibraryType). The handler maps the chosen field to the
corresponding `FindSamplesBy*` method. Output: `[]mlwh.Sample`. The enum is
sourced from code (not hand-maintained); the input schema's `field` property
sets `enum` to the derived field names. The mapping (field name <-> Registry
Method) lives in one table in `schema.go`.

**File:** `internal/mlwh/tools_resolve.go`
**Test file:** `internal/mlwh/tools_resolve_test.go`

**Acceptance tests:**

1. Given a stub returning one sample for `/find/sample/accession/SAMEA1`, when
   `mlwh_find_samples` is called with `{"field":"accession","value":"SAMEA1"}`,
   then `StructuredContent` holds that one sample.
2. Given a stub, when called with `{"field":"sanger_id","value":"S1"}`, then the
   stub receives a request to `/find/sample/sanger-id/S1`.
3. Given any stub, when called with `{"field":"nonsense","value":"x"}`, then the
   result is a tool error (schema validation rejects the invalid enum) and no
   request is made.
4. The registered tool's input schema `field` property has an `enum` of exactly
   `["sanger_id","lims_id","accession","supplier_name","library_type"]` (order
   follows Registry order), each value derived from a `FindSamplesBy*` Registry
   method.

### B3: Expand identifiers

As an agent, I want expand tools, so that I can turn a canonical identifier into
related identifiers or downstream search values.

Tools: `mlwh_expand_identifier` (ExpandIdentifier -> `[]mlwh.TaggedID`),
`mlwh_expand_search_values` (ExpandSearchValues -> `mlwh.SearchValues`),
`mlwh_expand_sample_search_values` (ExpandSampleSearchValues ->
`[]string`). Input: `Kind string` (enum from `mlwh.IdentifierKinds()`),
`Canonical string`. The `kind` enum is sourced from `IdentifierKinds()`.

**File:** `internal/mlwh/tools_resolve.go`
**Test file:** `internal/mlwh/tools_resolve_test.go`

**Acceptance tests:**

1. Given a stub returning two `TaggedID`s for `/expand/study_lims_id/5901`, when
   `mlwh_expand_identifier` is called with
   `{"kind":"study_lims_id","canonical":"5901"}`, then `StructuredContent` holds
   those two TaggedIDs.
2. Given any stub, when called with `{"kind":"bogus_kind","canonical":"x"}`,
   then the result is a tool error (enum rejected) and no request is made.
3. The `kind` enum equals `mlwh.IdentifierKinds()` mapped to their string
   values, in the same order (15 values, first `sample_uuid`, last
   `id_library_lims`).

## C. Detail and fan-out

### C1: Detail tools (sample/study/run/library)

As an agent, I want grouped detail tools, so that I can fetch a fully assembled
view of a sample, study, run, or library.

Tools: `mlwh_sample_detail` (SampleDetail, input `SangerName string`, output
`mlwh.SampleDetail`), `mlwh_study_detail` (StudyDetail, input `StudyLimsID
string`, output `mlwh.StudyDetail`), `mlwh_run_detail` (RunDetail, input `IDRun
string`, output `mlwh.RunDetail`), `mlwh_library_detail` (LibraryDetail, input
`PipelineIDLims string` + `StudyLimsID string`, output `mlwh.LibraryDetail`).
Descriptions explain each detail aggregate and its inputs.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_detail.go`
**Test file:** `internal/mlwh/tools_detail_test.go`

**Acceptance tests:**

1. Given a stub returning a `SampleDetail` for `/sample/S1/detail`, when
   `mlwh_sample_detail` is called with `{"sanger_name":"S1"}`, then
   `StructuredContent` is that SampleDetail (sample + lanes + libraries).
2. Given a stub, when `mlwh_library_detail` is called with
   `{"pipeline_id_lims":"P1","study_lims_id":"5901"}`, then the stub receives a
   request to `/library/P1/study/5901/detail`.
3. Given a stub returning 503 `{"code":"cache_never_synced",...}` for
   `/study/X/detail`, when `mlwh_study_detail` is called, then the result is a
   tool error indicating the warehouse cache is not synced yet.
4. The four detail tools are registered with the names listed above.

### C2: Fan-out enumeration tools

As an agent, I want fan-out tools, so that I can list the entities linked to a
study, run, sample, or library.

Paginated tools (input adds `Limit int`, `Offset int`; output the listed
slice). FETCH-ALL DEFAULT: when the caller omits `limit`, the handler MUST pass
`limit=1000000` (the upstream fetch-all sentinel `mlwhServerFetchAllLimit`),
NOT 0. The typed `[]T` Queryer methods take `limit, offset int` and the remote
client serialises both literally (`remotePagination` -> `strconv.Itoa`), so an
explicit `limit=0` reaches the server as `limit=0`; the server only substitutes
its fetch-all page when the `limit` query param is ABSENT, so `limit=0` becomes
`LIMIT 0` and returns ZERO rows. Because the int methods cannot express
"absent", the handler defaults Limit to 1000000 itself. So a fetch-all call
(`{}`) yields the upstream request `limit=1000000&offset=0` and returns every
row; `offset` defaults to 0. Tool descriptions state this fetch-all default.
The tools: `mlwh_all_studies` (AllStudies, no path param),
`mlwh_samples_for_study` (SamplesForStudy), `mlwh_samples_for_run`
(SamplesForRun), `mlwh_libraries_for_study` (LibrariesForStudy),
`mlwh_runs_for_study` (RunsForStudy), `mlwh_lanes_for_sample` (LanesForSample),
`mlwh_irods_paths_for_sample` (IRODSPathsForSample),
`mlwh_irods_paths_for_study` (IRODSPathsForStudy). Non-paginated:
`mlwh_studies_for_sample` (StudiesForSample), `mlwh_count_samples_for_study`
(CountSamplesForStudy -> `mlwh.Count`). The remaining sample fan-outs by library
(SamplesForLibrary/ID/LimsID/Type) are reachable via the escape-hatch tool (E1)
and need not each have a curated tool; if added they follow the same pattern.

**File:** `internal/mlwh/tools_detail.go`
**Test file:** `internal/mlwh/tools_detail_test.go`

**Acceptance tests:**

1. Given a stub returning two studies for `/studies`, when `mlwh_all_studies`
   is called with `{}` (no limit/offset), then the stub received
   `limit=1000000` and `offset=0` (the fetch-all sentinel, NOT `limit=0`) and
   `StructuredContent` holds those two studies. (A real server returns every
   row for `limit=1000000` but ZERO rows for `limit=0`.)
2. Given a stub, when `mlwh_samples_for_study` is called with
   `{"study_lims_id":"5901","limit":50,"offset":50}` (deliberate paging), then
   the stub receives a request to `/study/5901/samples` with `limit=50` and
   `offset=50` (an explicit limit is passed through unchanged).
3. Given a stub returning `{"count":300}` for `/study/5901/samples/count`, when
   `mlwh_count_samples_for_study` is called with `{"study_lims_id":"5901"}`,
   then `StructuredContent` is `{"count":300}`.
4. Given a stub returning an empty array for `/sample/S1/studies`, when
   `mlwh_studies_for_sample` is called with `{"sanger_name":"S1"}`, then
   `StructuredContent` is `[]` and `IsError` is false.

## D. Freshness

### D1: Report cache freshness

As an agent, I want a `mlwh_freshness` tool, so that I can caveat answers about
data staleness and detect a never-synced cache.

Calls `Freshness(ctx)` (no input). Output `mlwh.Freshness`. Description states
it reports per-table high-water + last-run timestamps and ever_synced, and
succeeds even on a never-synced cache.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_freshness.go`
**Test file:** `internal/mlwh/tools_freshness_test.go`

**Acceptance tests:**

1. Given a stub returning a `Freshness` with five tables (each ever_synced=true,
   non-empty high_water) for `/freshness`, when `mlwh_freshness` is called with
   `{}`, then `StructuredContent` holds those five table entries.
2. Given a stub returning a `Freshness` whose tables all have ever_synced=false
   and empty high_water/last_run, when called, then the result is NOT an error
   (`IsError=false`) and `StructuredContent` reflects the never-synced state.
3. The tool name is `mlwh_freshness` and it accepts empty input `{}`.

## E. Generic escape-hatch tool

### E1: Call any MLWH endpoint

As an agent, I want a generic `mlwh_call_endpoint` tool, so that no Registry
endpoint is unreachable even if it lacks a curated tool.

Input: `Method string` (Registry Method name, e.g. `SampleDetail`),
`PathParams []string` (path parameters in declaration order), `QueryParams
map[string]string` (query parameters, including `limit`/`offset` for paginated
endpoints). It dispatches through `(*mlwh.RemoteClient).Call(ctx, method,
pathParams, query)`: the handler need not look the Method up in `mlwh.Registry`
first, because `Call` validates the method and path-param arity itself,
rejecting unknown methods and arity mismatches. The handler converts
`QueryParams` to `url.Values`, calls `Call`, and returns the decoded value.
Output is `any` (UNTYPED JSON passthrough): no per-endpoint output schema; the
SDK places the decoded value in `StructuredContent` and the JSON text in
`Content`. The description lists how to find Method names (the workflow
resource, Story G1) and notes it is an escape hatch; prefer the curated tools.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_call.go`
**Test file:** `internal/mlwh/tools_call_test.go`

The handler signature uses `Out = any` so the SDK omits the output schema:

```go
func callEndpoint(ctx context.Context, _ *mcp.CallToolRequest, in CallInput)
    (*mcp.CallToolResult, any, error)
```

**Acceptance tests:**

1. Given a stub returning a `Match` for `/resolve/study/5901`, when
   `mlwh_call_endpoint` is called with
   `{"method":"ResolveStudy","path_params":["5901"]}`, then `StructuredContent`
   is the decoded Match value and `IsError` is false.
2. Given a stub returning two studies for `/studies?limit=2&offset=0`, when
   called with
   `{"method":"AllStudies","query_params":{"limit":"2","offset":"0"}}`, then the
   stub receives `limit=2`/`offset=0` and `StructuredContent` holds the two
   studies.
3. Given any stub, when called with `{"method":"NoSuchMethod"}`, then the result
   is a tool error (from `Call`'s unknown-method error) and the message names
   the method.
4. Given any stub, when called with `{"method":"SampleDetail"}` (no path
   params), then the result is a tool error indicating a path-param arity
   mismatch.
5. The registered tool has NO output schema (`Tool.OutputSchema` is nil because
   `Out` is `any`).

## F. Output schemas and result shapes

### F1: Output schemas preserve doc-tag field descriptions

As an agent, I want each typed tool's output schema to carry the upstream
field-level descriptions, so that I understand result fields.

Because the MCP SDK's reflection reads `jsonschema:` tags (not `doc:` tags), and
the `mlwh` result types document fields only via `doc:` tags, the output schema
for each typed tool is sourced from `mlwh.OpenAPIDocument()`'s
`components.schemas` (which DO carry the `doc:` descriptions). `schema.go`
provides:

```go
// outputSchemaFor returns an MCP-ready output schema (a map[string]any of
// type "object") for the named OpenAPI component schema, resolved from
// mlwh.OpenAPIDocument(). For a tool returning a slice, the slice is wrapped
// in an object with a single named array property (see F2) so the schema's
// top-level type is "object", as MCP requires.
func outputSchemaFor(componentName string) (map[string]any, error)
```

The schema is built once and pre-set on `Tool.OutputSchema` before
`mcp.AddTool`, so the SDK uses it verbatim instead of reflecting over the Go
type. `$ref`s within the component schemas are inlined or resolved so the schema
is self-contained.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/schema.go`
**Test file:** `internal/mlwh/schema_test.go`

**Acceptance tests:**

1. Given `mlwh.OpenAPIDocument()`, when `outputSchemaFor("Sample")` is called,
   then the returned schema's `properties.supplier_name.description` equals the
   `doc:` text "name the sample supplier gave the sample".
2. Given the built `mlwh_resolve_sample` tool, when its `Tool.OutputSchema` is
   inspected, then it is non-nil, has top-level `type` "object", and contains
   the `Match` field descriptions sourced from the component schema (e.g.
   `kind` description "kind of the resolved identifier").
3. Given `outputSchemaFor("Sample")`, when the result is JSON-marshaled, then it
   marshals to a valid JSON object (no unresolved `$ref` that would make
   `mcp.AddTool` reject it).

### F2: Slice results wrapped as an object

As an agent, I want list-returning tools to produce a structured object, so that
the result satisfies MCP's object-typed output-schema requirement.

MCP output schemas and `StructuredContent` must be JSON objects, but several
tools return Go slices (`[]Sample`, `[]Study`, ...). Each such tool's `Out` type
is a wrapper struct with one slice field, e.g.:

```go
type samplesResult struct {
    Samples []mlwh.Sample `json:"samples"`
}
```

The handler wraps the slice; the output schema (F1) describes the same wrapper.
The wrapper field name is consistent per element type (samples, studies, runs,
lanes, irods_paths, libraries, tagged_ids, values).

**File:** `internal/mlwh/schema.go` (wrappers may live beside the tools)
**Test file:** `internal/mlwh/tools_search_test.go` (covered by A1.1)

**Acceptance tests:**

1. Given a stub returning two samples, when `mlwh_search_samples` is called,
   then `StructuredContent` is an object `{"samples":[...]}` with two entries
   (not a bare array).
2. Given the wrapper-based `mlwh_search_samples` tool, when its output schema is
   inspected, then top-level `type` is "object" with a `samples` property of
   type "array".

## G. Workflow and version surfacing

### G1: Workflow / endpoint-catalogue resource

As an agent, I want an MCP resource describing how the MLWH endpoints compose,
so that I can plan multi-step workflows (resolve -> detail -> expand).

The MLWH provider registers a resource whose body is `mlwh.EndpointReference()`
(the always-current Registry-derived Markdown catalogue), prefixed with a short
note on common workflows (resolve an identifier, then fetch its detail, then
expand). URI `mlwh://workflow`, MIME type `text/markdown`. Built by calling
`EndpointReference()` (not a copied doc).

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/workflow.go`
**Test file:** `internal/mlwh/workflow_test.go`

**Acceptance tests:**

1. Given the registered resources, when the `mlwh://workflow` resource is read,
   then its text contains the output of `mlwh.EndpointReference()` (e.g. the
   heading "wa mlwh API endpoint reference" and an entry for `/resolve/sample`).
2. The resource's MIMEType is `text/markdown`.
3. The resource text additionally contains workflow guidance mentioning
   "resolve" and "detail".

### G2: Version MCP resource

As an agent, I want a version resource, so that I can read this server's version
and the targeted MLWH API version at runtime and caveat answers.

The core registers a resource at URI `mcp-server://version`, MIME type
`application/json`, whose body is `VersionInfo` JSON-marshaled
(`{"server_version":"...","api_versions":{"mlwh":"1.6.0"}}`). The MLWH targeted
version is `mlwh.APIVersion`.

**Package:** `internal/core/`
**File:** `internal/core/version.go`
**Test file:** `internal/core/version_test.go`

**Acceptance tests:**

1. Given a core server built with `ServerVersion:"0.1.0"` and an MLWH provider,
   when the `mcp-server://version` resource is read, then its JSON parses to
   `server_version` "0.1.0" and `api_versions.mlwh` equal to `mlwh.APIVersion`.
2. The resource's MIMEType is `application/json`.

### G3: Server implementation info and instructions

As an agent connecting to the server, I want the server's advertised name,
version, and instructions to state both versions, so that I see them on connect.

The core sets `mcp.Implementation{Name:"mlwh-mcp-server", Version:
ServerVersion}` and `ServerOptions.Instructions` to a string that includes the
server version and each provider's targeted API version (e.g. "MLWH API
1.6.0"). Instructions also briefly point at the workflow and version resources.

**File:** `internal/core/server.go`
**Test file:** `internal/core/server_test.go`

**Acceptance tests:**

1. Given a core server built with `ServerVersion:"0.1.0"` and an MLWH provider,
   when the server's `Implementation` is inspected, then `Name` is
   "mlwh-mcp-server" and `Version` is "0.1.0".
2. Given the same, when the configured `Instructions` are inspected, then they
   contain "0.1.0" and the MLWH API version (`mlwh.APIVersion`).

### G4: --version flag

As an operator, I want `mcp-server --version`, so that I can see both versions
without starting the server.

`cmd/mcp-server` parses a `--version` flag; when set it prints the server
version and the targeted MLWH API version (`mlwh.APIVersion`) to stdout and
exits 0 without opening the transport. The server version is a build-time
package variable in `internal/core` (default e.g. "dev", overridable via
`-ldflags -X`).

**Package:** `cmd/mcp-server/`
**File:** `cmd/mcp-server/main.go`
**Test file:** `main_test.go`

**Acceptance tests:**

1. Given the built binary, when run with `--version`, then stdout contains the
   server version and the string `mlwh.APIVersion` value, and the exit code is
   0.
2. Given the binary run with `--version`, then it does not block on stdin / does
   not start serving (the command returns promptly).

### G5: Startup-log version line

As an operator, I want the server to log both its own version and each
provider's targeted upstream API version at startup, so that the running
server's versions are visible in the logs.

When `Run` begins serving, the core emits one startup log line through the
configured `Options.Logger` (`*slog.Logger`) that names this server's version
(`ServerVersion`) AND each provider's targeted upstream API version (for MLWH,
`mlwh.APIVersion`, sourced from the same `VersionInfo` the core assembles). If
no logger is configured the core uses a default, so a configured logger always
receives the line.

**Package:** `internal/core/`
**File:** `internal/core/server.go`
**Test file:** `internal/core/server_test.go`

**Acceptance tests:**

1. Given a core server built with `ServerVersion:"0.1.0"`, an MLWH provider, and
   an `Options.Logger` whose `slog.Handler` writes to a `bytes.Buffer`, when the
   server is run over an in-memory transport (a connected client lets `Run`
   reach the serving phase), then the buffer contains a startup log line that
   includes both "0.1.0" and the MLWH API version (`mlwh.APIVersion`).

## H. Transport and configuration

### H1: Stdio transport with a clean seam

As an operator, I want the server to run over stdio, so that local agent CLIs
(Claude Code, Codex) can launch it.

`internal/core` exposes the transport as a seam: `Run(ctx, mcp.Transport)`
accepts any `mcp.Transport`; the binary passes `&mcp.StdioTransport{}`. No HTTP
transport code, config, or tests this round. The seam is `mcp.Transport` so a
streamable-HTTP transport can be supplied later with no core change.

**Package:** `internal/core/` + `cmd/mcp-server/`
**File:** `internal/core/transport.go`, `cmd/mcp-server/main.go`
**Test file:** `internal/core/server_test.go`, `main_test.go`

**Acceptance tests:**

1. Given a core server and an in-memory transport
   (`mcp.NewInMemoryTransports()` for the test, proving `Run` takes any
   `mcp.Transport`), when a test MCP client connects and lists tools, then it
   sees the MLWH tools (e.g. `mlwh_search_samples`).
2. Given `cmd/mcp-server`'s wiring, when inspected, then `Run` is invoked with
   `&mcp.StdioTransport{}` and there is no HTTP transport, config flag, or
   listener anywhere in the module.

### H2: MLWH provider configuration

As an operator, I want to configure the MLWH base URL (and optional TLS CA cert
and timeout), so that the provider can reach the internal, no-auth MLWH API.

The MLWH provider reads config from flags/env, generalising per-provider so
future providers can have their own settings. Recognised:
`MLWH_BASE_URL`/`--mlwh-base-url` (required), `MLWH_CA_CERT`/`--mlwh-ca-cert`
(optional), `MLWH_TIMEOUT`/`--mlwh-timeout` (optional duration). These populate
`mlwh.RemoteConfig{BaseURL, CACert, Timeout}`. `CacheTTL` is NOT wired (inert
for the remote client). `Token` is not exposed (the API is no-auth) but may be
left settable internally. A missing base URL is a clear startup error.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/provider.go`
**Test file:** `internal/mlwh/provider_test.go`

**Acceptance tests:**

1. Given env `MLWH_BASE_URL=http://stub.example`, when the MLWH provider's
   config is loaded, then the resulting `mlwh.RemoteConfig.BaseURL` is
   `http://stub.example`.
2. Given no base URL set anywhere, when the provider is constructed, then it
   returns a non-nil error whose message mentions the base URL is required.
3. Given env `MLWH_TIMEOUT=5s`, when config is loaded, then
   `mlwh.RemoteConfig.Timeout` is 5 seconds and `CacheTTL` is left zero.

## I. Provider seam proof and registration

### I1: MLWH provider registers its full tool/resource surface

As a developer, I want the MLWH provider to register through the core
`Provider`/`Registrar` seam, so that all its tools and resources appear on the
server.

`mlwh.New(cfg) (core.Provider, error)` builds the provider from config; its
`Register(ctx, r)` adds every tool in stories A-E and the workflow resource
(G1) via the `Registrar`. The shipped binary registers only this provider.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/provider.go`
**Test file:** `internal/mlwh/provider_test.go`

**Acceptance tests:**

1. Given a core server with only the MLWH provider (pointed at a stub), when an
   in-memory client lists tools, then the list includes `mlwh_search_samples`,
   `mlwh_count_samples`, `mlwh_search_studies`, `mlwh_resolve_sample`,
   `mlwh_find_samples`, `mlwh_sample_detail`, `mlwh_freshness`, and
   `mlwh_call_endpoint`.
2. Given the same, when the client lists resources, then it includes
   `mlwh://workflow` and `mcp-server://version`.
3. The MLWH provider's `Name()` returns "mlwh".

### I2: Multi-service seam proof (test-only fake provider)

As a developer, I want a fake provider defined in test code to register through
the same `Provider`/`Registrar` interface, so that I prove adding a service
needs no core change.

A `fakeProvider` (test code only) implements `core.Provider`, registering one
trivial tool (e.g. `fake_ping`) and one resource. A test builds a core server
with BOTH the MLWH provider and the fake provider and asserts both surfaces
appear. No production core file is modified to add the fake. This proves the
seam without the fake resembling MLWH or `wa`.

**Package:** `internal/core/`
**File:** (production) none new
**Test file:** `internal/core/provider_seam_test.go`

**Acceptance tests:**

1. Given a `fakeProvider` registering a `fake_ping` tool, when a core server is
   built with `[mlwhProvider, fakeProvider]` and a client lists tools, then the
   list contains both `fake_ping` and `mlwh_search_samples`.
2. Given the fake provider's tool is called, when invoked, then it returns its
   trivial result with `IsError=false`, proving the fake registered through the
   same handler path.
3. The fakeProvider is defined only in a `_test.go` file (no production core
   file references it).

## J. Error mapping

### J1: Map wa/mlwh sentinels to clear MCP tool errors

As an agent, I want upstream errors turned into clear, actionable tool errors,
so that I can react (disambiguate, retry later, fix input).

`errmap.go` maps a returned error to a human message, preferring `errors.Is`
against the `mlwh.Err*` sentinels over parsing HTTP status. Order matters:
check `ErrCacheNeverSynced` BEFORE `ErrNotFound` (slice endpoints return
`errors.Join(ErrCacheNeverSynced, ErrNotFound)`). Mapping:

| Sentinel                         | Code (HTTP) | Message gist                          |
|----------------------------------|-------------|---------------------------------------|
| ErrCacheNeverSynced              | 503         | warehouse cache not synced yet        |
| ErrNotFound                      | 404         | identifier not found                  |
| ErrAmbiguous                     | 409         | matches multiple records; disambiguate|
| ErrUnsupportedIdentifier         | 422         | identifier form not supported         |
| ErrUpstreamImpaired              | 502/400     | upstream impaired / bad request       |

A bad_request (400) has no dedicated sentinel and arrives as
`ErrUpstreamImpaired`; its message is preserved from the upstream envelope where
present, so a 400 "term too short" still reads sensibly. The mapped error is
returned from the handler so the SDK packs it into an `IsError` result.

```go
// mapToolError converts a wa/mlwh client error into the error a typed handler
// returns (nil for no error). It preserves the upstream message and appends a
// short actionable hint per sentinel.
func mapToolError(err error) error
```

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/errmap.go`
**Test file:** `internal/mlwh/errmap_test.go`

**Acceptance tests:**

1. Given `errors.Join(mlwh.ErrCacheNeverSynced, mlwh.ErrNotFound)` (the slice
   never-synced case), when mapped, then the message mentions the cache is not
   synced (503 semantics), NOT "not found".
2. Given `fmt.Errorf("x: %w", mlwh.ErrNotFound)`, when mapped, then the message
   mentions the identifier was not found.
3. Given `fmt.Errorf("x: %w", mlwh.ErrAmbiguous)`, when mapped, then the message
   mentions multiple records and disambiguating.
4. Given `fmt.Errorf("x: %w", mlwh.ErrUnsupportedIdentifier)`, when mapped, then
   the message mentions the identifier form is unsupported.
5. Given `fmt.Errorf("term too short: %w", mlwh.ErrUpstreamImpaired)` (a 400),
   when mapped, then the message preserves "term too short".
6. Given `nil`, when mapped, then the result is `nil` (no error).

## Implementation Order

Each phase builds on tested foundations from the prior phases.

1. **Repo layout + module bootstrap.** Relocate `frontend/`, `backend/`,
   `run-dev.sh`, root `.env.example`, and the web-oriented README text under
   `webui/`. Add root `go.mod` (`github.com/wtsi-hgi/llm-knowledge-base`, go
   1.25) requiring `wa` and the MCP SDK v1.6.1. Add the new root `README.md`
   (MCP-first) and update `.gitignore` (Go + Node + Python). Keep `LICENSE` at
   root. No Go logic yet. (Verifiable: `go build ./...` succeeds on an empty
   `cmd/mcp-server/main.go`; web UI files exist only under `webui/`.)
2. **Core seam (Stories I-prereqs, G3, H1).** `internal/core`: `Provider`,
   `Registrar`, `Options`, `New`, `Run(ctx, mcp.Transport)`, Implementation +
   Instructions, version vars. Prove with the in-memory transport. Sequential
   after 1.
3. **Schema + error foundations (F1, F2, J1).** `internal/mlwh/schema.go`
   (`outputSchemaFor`, slice wrappers, field/enum derivation) and `errmap.go`.
   These have no network dependency and underpin every tool. Parallelisable
   with phase 2 once the module builds.
4. **MLWH provider config + registration shell (H2, I1 scaffolding).**
   `internal/mlwh/provider.go`: config load, `New`, `Register` (initially
   empty), `RemoteClient` construction. Sequential after 2-3.
5. **Search/count + resolve/find/expand tools (A, B).** Add tools to the
   provider, with the stub MLWH server test harness. Sequential after 4.
6. **Detail/fan-out + freshness + escape hatch (C, D, E).** Sequential after 5
   (reuses the harness and wrappers).
7. **Resources + version surfacing + flag (G1, G2, G4, G5).** Workflow + version
   resources, `--version` flag, startup-log version line. Sequential after 4
   (G2/G3/G5 need core; G1 needs provider).
8. **Seam proof + full registration assertions (I1, I2).** Fake provider test;
   assert the full MLWH surface registers. Sequential after 5-7.

## Appendix: Key Decisions

- **Hybrid tool generation with escape hatch.** Tools are derived from
  `mlwh.Registry` as the source of truth (names, descriptions, params,
  enums) but curated for LLM ergonomics: related endpoints are grouped
  (`mlwh_find_samples` unifies the five FindSamplesBy* endpoints behind a
  `field` enum; detail tools are grouped by entity). A single
  `mlwh_call_endpoint` escape hatch dispatches any Registry Method via
  `(*RemoteClient).Call`, so no endpoint is unreachable. ~40 flat low-level
  tools are deliberately avoided to protect agent usability.
- **Structured + text for free.** Typed `AddTool[In,Out]` auto-populates
  `StructuredContent` from the typed `Out` and fills `Content` with the same
  JSON as text, so structured-aware and text-only clients both work without
  per-tool serialisation code.
- **Output schemas sourced from OpenAPI, not reflection.** The MCP SDK reflects
  `jsonschema:` tags; the `mlwh` types document fields via `doc:` tags. To
  preserve field descriptions, output schemas are built from
  `mlwh.OpenAPIDocument()`'s `components.schemas` and pre-set on
  `Tool.OutputSchema` (the SDK honours a pre-set schema verbatim). Slice results
  are wrapped in a one-field object so the top-level schema type is "object".
- **Enums sourced from code.** The `find_samples` `field` enum is derived by
  filtering `mlwh.Registry` for the `FindSamplesBy` Method prefix; the
  identifier `kind` enum is `mlwh.IdentifierKinds()`. Invalid values are
  rejected at the schema layer.
- **Version is compile-time.** The targeted MLWH API version is the imported
  `mlwh.APIVersion` constant; reading it never contacts a server. It is surfaced
  four ways: `--version` flag (G4), `mcp-server://version` resource (G2),
  startup logs via the core logger (G5), and the server's Implementation info +
  Instructions (G3).
- **Provider seam is client-agnostic.** A provider implements
  `Provider`/`Registrar` only - no requirement for an importable Go package. The
  MLWH provider importing `wa/mlwh` is incidental. A test-only fake provider
  proves a second, unrelated service plugs in with no core change. The shipped
  binary registers only MLWH.
- **Stdio only, HTTP-ready.** Only the stdio transport is implemented; `Run`
  takes any `mcp.Transport` so streamable HTTP can be added later with no core
  change. No HTTP code/config/tests this round.
- **Error policy.** Map `mlwh.Err*` sentinels (not HTTP status) to tool errors;
  check `ErrCacheNeverSynced` before `ErrNotFound` because slice endpoints join
  the two; cover all six documented codes (400/404/409/422/502/503). Cheap input
  guards (min term length 3, max limit 1000) reject before the network call so
  the agent gets an immediate, clear error.

### Testing strategy

- Hermetic: every provider test runs against an `httptest.Server` stub that
  serves canned JSON for the exact `wa mlwh` paths, or against a
  `*mlwh.RemoteClient` pointed at that stub. Never a live warehouse.
- A shared test helper builds a core `*mcp.Server` with the MLWH provider
  pointed at a stub and an `mcp.NewInMemoryTransports()` client/server pair, so
  tool/resource registration and round-trips are exercised end-to-end.
- Every spec acceptance test maps to a GoConvey test using `So(...)`
  assertions, per **go-conventions**. Implementors follow **go-implementor**;
  reviewers follow **go-reviewer**. No stubs, hardcoded results, swallowed
  failures, or build-tag exclusions for acceptance tests.
</content>
</invoke>
