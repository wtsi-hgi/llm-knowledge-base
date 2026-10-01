# MLWH Agent Feedback Tool Specification

## Overview

Add the MCP tool `mlwh_provide_feedback`. Agents call it on their own
initiative, without asking the user, when they could not work out an answer,
made a mistake, found no endpoint that could answer, saw the user was
unhappy, or hit another problem. The tool forwards one report to
`wa mlwh serve`, which stores it for the server admin. Reports are read on
wa's `/feedback` admin page; this repo has no read path.

The tool takes typed agent input (category, description, optional verbatim
user request, optional tools tried). It automatically attaches this server's
version, the targeted wa API version, the transport, and the connecting
agent application's identity. It calls `wa.RemoteClient.SubmitFeedback` and
returns the new report's id and receive time. Failures are structured tool
errors. There is no local fallback.

Depends on wa `v0.10.0` (`wa.APIVersion` `1.9.0`). The wire contract is
owned by `/home/ubuntu/wa/.docs/feedback/spec.md` ("Wire contract", "Go
types", story C1). This spec restates only what the MCP side uses. If the
two disagree, the wa spec wins.

## Architecture

### Authority and dependency

- `go.mod` and `go.sum` require `github.com/wtsi-hgi/wa v0.10.0`, with no
  replace directive. `wa.APIVersion` is `1.9.0`.
- Release order: wa tags `v0.10.0` first, then this repo bumps.
- Upstream symbols used (package `github.com/wtsi-hgi/wa/mlwh`, imported as
  `wa`):

```go
func (*wa.RemoteClient) SubmitFeedback(
    context.Context, wa.FeedbackSubmission,
) (wa.FeedbackReceipt, error)

type wa.FeedbackCategory string
func wa.FeedbackCategories() []wa.FeedbackCategory
func (wa.FeedbackCategory) Description() string

const (
    wa.FeedbackMaxBodyBytes        = 65536
    wa.FeedbackMaxDescriptionBytes = 16384
    wa.FeedbackMaxUserRequestBytes = 16384
    wa.FeedbackMaxToolsTried       = 50
    wa.FeedbackMaxToolNameBytes    = 128
    wa.FeedbackMaxMetaBytes        = 256
)

var wa.ErrFeedbackDisabled, wa.ErrFeedbackInvalid, wa.ErrFeedbackTooLarge,
    wa.ErrFeedbackUnsupported, wa.ErrFeedbackUnauthorized error
```

- `wa.FeedbackSubmission` JSON fields: `category`, `description`,
  `user_request`, `tools_tried`, `mcp_server_version`, `wa_api_version`,
  `transport`, `client_name`, `client_version`, `client_user_agent`. Go
  fields: `Category`, `Description`, `UserRequest`, `ToolsTried`,
  `MCPServerVersion`, `WAAPIVersion`, `Transport`, `ClientName`,
  `ClientVersion`, `ClientUserAgent`.
- `wa.FeedbackReceipt` is `{ID int64 "id"; CreatedAt string "created_at"}`.
  `created_at` is UTC RFC3339.
- Categories: `could_not_answer`, `agent_mistake`, `no_endpoint`,
  `user_unhappy`, `other`.
- `SubmitFeedback` POSTs `<MLWH_BASE_URL>/feedback`. Upstream results:

| Upstream result                                                     | Sentinel                                          |
| ------------------------------------------------------------------- | ------------------------------------------------- |
| 201                                                                 | none (receipt)                                    |
| 400 with wa envelope code `bad_request`                             | `ErrFeedbackInvalid`                              |
| 400 without it (e.g. plain-text 400 from `http://` to a secured wa) | `ErrUpstreamImpaired`                             |
| 401 (secured wa, base URL ends `/rest/v1/auth`)                     | `ErrFeedbackUnauthorized`                         |
| 404 (wa older than API 1.9.0, or secured wa at its root URL)        | `ErrFeedbackUnsupported`                          |
| 413 (any body)                                                      | `ErrFeedbackTooLarge`                             |
| 503 `feedback_disabled` (no feedback DB)                            | `ErrFeedbackDisabled`                             |
| other status, transport failure, bad JSON                           | `ErrUpstreamImpaired` or unwrapped envelope error |

- The feedback route is not a `wa.Registry` entry. `mlwh_call_endpoint`, its
  method enum, and the Registry/OpenAPI parity tests are unchanged.
- This server never sets `RemoteConfig.Token`, so it reaches only a
  plain-mode wa, like every other tool. This spec documents that and does
  not fix it. Secured wa serves TLS only and mounts submit under
  `/rest/v1/auth`, and `RemoteClient` appends `/feedback` to the base URL
  verbatim. So the result depends on `MLWH_BASE_URL` (wa spec "Known
  limits"):
  - `http://host:port`: Go's TLS listener answers with a plain-text 400 and
    no wa envelope, so `feedback_unavailable`.
  - `https://` with an untrusted certificate (no `MLWH_CA_CERT` for a
    private CA): transport failure, so `feedback_unavailable`.
  - `https://host:port`: 404, so `feedback_unsupported`.
  - `https://host:port/rest/v1/auth`: 401, so `feedback_unauthorized`.
  - Every existing tool gets `ErrUpstreamImpaired` in all four cases.
- wa records the request's remote address, which is this MCP server's host,
  not the end user's machine.

### Files

```text
go.mod, go.sum                        wa v0.10.0
internal/mlwh/provider.go             Option, WithServerInfo, provider fields
internal/mlwh/tools_feedback.go       tool, input schema, metadata, errors
internal/mlwh/workflow.go             feedback guidance
internal/mlwh/harness_test.go         record method, headers, body
internal/mlwh/tools_feedback_test.go  tool tests
internal/mlwh/http_test.go            HTTP-mode metadata test
internal/mlwh/provider_test.go        surface list, version 1.9.0
internal/mlwh/workflow_test.go        guidance tests
cmd/mlwh-mcp-server/main.go           pass server info to provider
cmd/mlwh-mcp-server/main_test.go      wiring test, version 1.9.0
cmd/mlwh-mcp-server/readme_test.go    README assertions
README.md                             tool catalogue, version, limits
```

`internal/core/` is unchanged and stays service-agnostic.

### Provider options (`internal/mlwh/provider.go`)

```go
// Option configures the provider at construction.
type Option func(*provider)

// WithServerInfo sets the MCP server build version and transport name
// ("stdio" or "http") attached to feedback reports.
func WithServerInfo(serverVersion, transport string) Option

func New(cfg wa.RemoteConfig, opts ...Option) (core.Provider, error)

type provider struct {
    client        *wa.RemoteClient
    serverVersion string // default core.ServerVersion
    transport     string // default "stdio"
}
```

Existing `New(cfg)` call sites compile unchanged. `main.go` calls
`mlwh.New(remoteCfg, mlwh.WithServerInfo(core.ServerVersion,
string(cfg.Transport)))`.

### Tool contract (`internal/mlwh/tools_feedback.go`)

```go
type provideFeedbackInput struct {
    Category    string   `json:"category"`
    Description string   `json:"description"`
    UserRequest string   `json:"user_request,omitempty"`
    ToolsTried  []string `json:"tools_tried,omitempty"`
}
```

The output is `wa.FeedbackReceipt`, schema `outputSchemaFor("FeedbackReceipt")`.
The handler builds the upstream body with:

```go
// feedbackSubmission copies in and attaches the metadata below.
func (p *provider) feedbackSubmission(
    req *mcp.CallToolRequest, in provideFeedbackInput,
) wa.FeedbackSubmission
```

The tool is registered by `p.registerFeedbackTool(r)` from `Register`,
before `registerCallTool`.

The input schema is a hand-built map (`feedbackInputSchema()`):

```json
{"type":"object","additionalProperties":false,
 "required":["category","description"],
 "properties":{
  "category":{"type":"string","enum":["could_not_answer","agent_mistake",
    "no_endpoint","user_unhappy","other"],"description":"<per-value list>"},
  "description":{"type":"string","description":"..."},
  "user_request":{"type":"string","description":"..."},
  "tools_tried":{"type":"array","items":{"type":"string"},
    "description":"..."}}}
```

- The `enum` comes from `wa.FeedbackCategories()`. The category description
  lists each value as `<value>: <wa Description()>`.
- The other descriptions state the caps (16384 bytes, 16384 bytes, and 50
  items of at most 128 bytes each), and that `user_request` must be the
  user's words verbatim.

Attached metadata:

| Field                           | Source                                                                                    |
| ------------------------------- | ----------------------------------------------------------------------------------------- |
| `mcp_server_version`            | `p.serverVersion`                                                                         |
| `wa_api_version`                | `wa.APIVersion`                                                                           |
| `transport`                     | `p.transport`                                                                             |
| `client_name`, `client_version` | `req.Session.InitializeParams().ClientInfo` when non-nil (stdio handshake)                |
| `client_user_agent`             | `req.Extra.Header.Get("User-Agent")` when `req.Extra` and its `Header` are non-nil (HTTP) |

- HTTP mode stays stateless (`Stateless: true`), so `ClientInfo` is nil
  there and the User-Agent is the identity. A nil `req.Session` or nil
  params leaves the client fields empty.
- Each attached string is truncated to `wa.FeedbackMaxMetaBytes` bytes at a
  UTF-8 rune boundary, so attached metadata never causes a 413.
- Agent-supplied fields pass through unmodified. Beyond the SDK's schema
  check, wa is the only validator.

### Structured tool errors

Failures return `core.ToolError` with a message of the form
`<code>: <detail> (<next step>)`. The SDK packs it as `IsError: true` text
content. Codes are checked in this order with `errors.Is`:

| Sentinel                  | Code                    | Next step in message                                                                                                      |
| ------------------------- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `ErrFeedbackDisabled`     | `feedback_disabled`     | feedback is disabled on the wa server; tell the user it could not be reported; do not retry                               |
| `ErrFeedbackUnsupported`  | `feedback_unsupported`  | the wa server has no feedback endpoint at this URL: it predates MLWH API 1.9.0 or is secured; tell the user; do not retry |
| `ErrFeedbackUnauthorized` | `feedback_unauthorized` | the wa server requires authentication, which this MCP server does not support; tell the user; do not retry                |
| `ErrFeedbackTooLarge`     | `feedback_too_large`    | shorten description, user_request, or tools_tried and resubmit once                                                       |
| `ErrFeedbackInvalid`      | `feedback_invalid`      | fix category or description and resubmit once                                                                             |
| any other error           | `feedback_unavailable`  | the wa server could not be reached or failed; tell the user it could not be reported; do not retry repeatedly             |

`<detail>` is the upstream error text, which keeps wa's envelope message
such as `description exceeds 16384 bytes`.

### Tool description (`feedbackDescription`)

The description must say, in substance:

- Report a problem with the current user request to the MLWH server admin.
  Call it on your own initiative, without asking the user first, whenever a
  category applies (list the five with meanings).
- Submit at most one report per distinct problem. Never resubmit a report
  that succeeded.
- After it succeeds, tell the user briefly that you reported the problem to
  the server admin.
- `description` is your own account of what went wrong. `user_request` is
  the user's original request verbatim, optional. `tools_tried` lists the
  MCP tools you used, optional.
- The server attaches its version, the MLWH API version, the transport, and
  the agent application's name/version or User-Agent. wa records this MCP
  server's network address, not the user's.
- Returns `id` and `created_at`. On failure, follow the error's next step.

## A. Dependency and Provider Wiring

### A1: Bump wa to v0.10.0

As the maintainer, I want the provider compiled against MLWH API 1.9.0, so
that `SubmitFeedback` and the feedback types exist.

Update every assertion and doc that pins `1.8.1`:

- `TestWAAPI18Contract` (renamed `TestWAAPI19Contract`)
- `cmd/mlwh-mcp-server/main_test.go`
- `cmd/mlwh-mcp-server/readme_test.go`
- `README.md` (prose and `--version` example)

**Package:** `internal/mlwh/`, `cmd/mlwh-mcp-server/`
**File:** `go.mod`, `go.sum`, `README.md`
**Test file:** `internal/mlwh/provider_test.go`,
`cmd/mlwh-mcp-server/main_test.go`, `cmd/mlwh-mcp-server/readme_test.go`

**Acceptance tests:**

1. Given the module, then `wa.APIVersion == "1.9.0"` and
   `provider.APIVersion() == wa.APIVersion`.
2. Given `--version`, then output contains `MLWH API version 1.9.0`.
3. Given `wa.Registry`, then no entry has `Path == "/feedback"`, and the
   `mlwh_call_endpoint` method enum equals the Registry method set
   (existing parity tests pass unchanged).
4. Given README, then it contains `currently MLWH API 1.9.0` and
   `# MLWH API version 1.9.0`, and not `1.8.1`.

### A2: Server info option

As the feedback tool, I want the server version and transport injected at
construction, so that reports carry them without the core knowing MLWH.

**Package:** `internal/mlwh/`, `cmd/mlwh-mcp-server/`
**File:** `internal/mlwh/provider.go`, `cmd/mlwh-mcp-server/main.go`
**Test file:** `internal/mlwh/tools_feedback_test.go`,
`cmd/mlwh-mcp-server/main_test.go`

Signatures as in Architecture.

**Acceptance tests:**

1. Given `New(cfg)` with no options and `core.ServerVersion` `"dev"`, when
   the tool submits, then the stub body has `mcp_server_version` `"dev"` and
   `transport` `"stdio"`.
2. Given `New(cfg, WithServerInfo("0.4.0", "http"))`, then the body has
   `mcp_server_version` `"0.4.0"` and `transport` `"http"`.
3. Given `serve` run with `--http 127.0.0.1:0`, `MLWH_BASE_URL` set to a
   stub, and `newCoreServer` replaced by a capture, when the captured
   provider is run in a real `core.New` over in-memory transports and the
   tool is called, then the stub body has `transport` `"http"` and
   `mcp_server_version` equal to `core.ServerVersion`. Without `--http`,
   `transport` is `"stdio"`.

## B. Feedback Tool

### B1: Register the tool

As an agent, I want a typed, self-describing feedback tool, so that I know
when and how to report problems.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_feedback.go`, `internal/mlwh/provider.go`
**Test file:** `internal/mlwh/tools_feedback_test.go`,
`internal/mlwh/provider_test.go`

Add `"mlwh_provide_feedback"` to `fullSurfaceTools`.

**Acceptance tests:**

1. Given a tools listing, then `mlwh_provide_feedback` is present and every
   existing tool remains present.
2. Given its input schema, then `required` is `["category","description"]`,
   `additionalProperties` is false, properties are exactly `category`,
   `description`, `user_request`, `tools_tried`, and `category.enum` equals
   `wa.FeedbackCategories()` as strings, in order.
3. Given the category property description, then it contains each
   `"<value>: <Description()>"` pair.
4. Given its output schema, then it is an object with required `id`
   (integer) and `created_at` (string).
5. Given the tool description, then it contains `without asking the user`,
   `at most one report per distinct problem`, `tell the user briefly`,
   `server admin`, and all five category values.
6. Given a call with `category: "bogus"`, then the result has
   `IsError == true` and the stub received no request. The SDK validates
   input against the hand-built schema, so the handler has no local check.
7. Given a call missing `description`, then `IsError == true` and no request
   is sent.

### B2: Submit and return the receipt

As an agent, I want the report forwarded with automatic context and its id
returned, so that I can confirm it to the user.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_feedback.go`
**Test file:** `internal/mlwh/tools_feedback_test.go`,
`internal/mlwh/harness_test.go`, `internal/mlwh/http_test.go`

Extend `recordedRequest` with `Method string`, `Header http.Header`, and
`Body []byte`, captured for every request.

**Acceptance tests:**

1. Given the stub replying 201 `{"id":7,"created_at":"2026-10-01T12:00:00Z"}`
   on `/feedback`, when called over the in-memory harness with
   `{category:"no_endpoint", description:"No consent endpoint.",
   user_request:"Which samples withdrew consent?",
   tools_tried:["mlwh_search_samples"]}`, then the result is not an error.
   `StructuredContent` decodes to `{"id":7,"created_at":
   "2026-10-01T12:00:00Z"}`. The stub saw exactly one `POST /feedback` with
   `Content-Type: application/json`.
2. Given test 1, then the decoded request body has `category`,
   `description`, `user_request`, and `tools_tried` equal to the input,
   `wa_api_version == wa.APIVersion`, `transport == "stdio"`,
   `client_name == "test-client"`, `client_version == "0.0.0"` (harness
   client implementation), and no `client_user_agent` key.
3. Given only `category` and `description`, then the body has no
   `user_request` or `tools_tried` keys.
4. Given `runMLWHHTTPServer` and a client connected over a
   `StreamableClientTransport` whose `HTTPClient` has a `RoundTripper` that
   clones each request and sets `User-Agent: agent-cli/1.2` before calling
   `http.DefaultTransport`, then the body has
   `client_user_agent == "agent-cli/1.2"` and no `client_name` or
   `client_version` keys. `connectMLWHHTTPClient` sets no `HTTPClient`, so
   without this hook Go sends `Go-http-client/1.1`.
5. Given the test 4 setup with a `User-Agent` of 300 ASCII bytes, then
   `client_user_agent` is its first 256 bytes. Given 300 bytes made of
   3-byte runes, then it is 255 bytes and valid UTF-8.
6. Given a 20000-byte `description` and the stub configured to reply 413
   `{"code":"payload_too_large","message":"description exceeds 16384
   bytes"}`, then the stub body's `description` equals the input unmodified,
   and the result text starts with `feedback_too_large: ` (B3).
7. Given `New(cfg, WithServerInfo(v, "stdio"))` with `v` 300 ASCII bytes,
   when the tool submits, then the body's `mcp_server_version` is the first
   256 bytes of `v`.
8. Given `p.feedbackSubmission` called directly with
   `&mcp.CallToolRequest{}` (nil `Session`, nil `Extra`), and again with
   `&mcp.CallToolRequest{Session: &mcp.ServerSession{}}` (nil
   `InitializeParams()`), and input `{Category:"other", Description:"x"}`,
   then neither call panics, and each result has empty `ClientName`,
   `ClientVersion`, and `ClientUserAgent`, `Category == "other"`,
   `Description == "x"`, and `WAAPIVersion == wa.APIVersion`.

### B3: Structured errors

As an agent, I want each failure explained with a stable code and next step,
so that I neither loop nor stay silent.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/tools_feedback.go`
**Test file:** `internal/mlwh/tools_feedback_test.go`

`mapFeedbackError(err error) error` builds messages per the Architecture
table. It returns nil for nil.

**Acceptance tests:**

1. Given the stub replying 503 `{"code":"feedback_disabled","message":
   "feedback is disabled on this server"}`, then `IsError == true` and the
   text starts with `feedback_disabled: ` and contains
   `feedback is disabled on this server` and `do not retry`.
2. Given 404 with text body `404 page not found`, then the text starts with
   `feedback_unsupported: ` and contains `1.9.0`.
3. Given 401 with any body, then it starts with `feedback_unauthorized: `.
4. Given 413 `{"code":"payload_too_large","message":"description exceeds
   16384 bytes"}`, then it starts with `feedback_too_large: ` and contains
   `description exceeds 16384 bytes` and `resubmit once`.
5. Given 400 `{"code":"bad_request","message":"description is required"}`,
   then it starts with `feedback_invalid: ` and contains
   `description is required`. Given 400 with text body `Client sent an HTTP
   request to an HTTPS server.\n`, then it starts with
   `feedback_unavailable: `.
6. Given 500 `{"code":"internal_error","message":"could not store
   feedback"}`, or 503 `{"code":"cache_never_synced","message":"cache never
   synced"}` (no feedback sentinel, wa C1.8), or 201 with body `not json`
   (wa C1.10 maps it to `ErrUpstreamImpaired`), or a stub closed before the
   call, then it starts with `feedback_unavailable: `. The 503 text also
   contains `cache never synced`.
7. Given `mapFeedbackError(nil)`, then nil. Given an error wrapping both
   `ErrFeedbackDisabled` and `ErrUpstreamImpaired`, then the code is
   `feedback_disabled`.
8. Given a stub `/feedback` handler that signals it started and blocks on
   `request.Context().Done()`, when the client ctx is cancelled after the
   signal, then `CallTool` returns within 5s with a nil result and
   `errors.Is(err, context.Canceled)`, and the stub saw exactly one
   `POST /feedback` (no retry). Mirrors `TestAvailabilityToolsF3Cancellation`
   F3.2.

## C. Guidance and Docs

### C1: Workflow resource guidance

As an agent reading `mlwh://workflow`, I want feedback guidance alongside
the routes, so that I report problems consistently.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/workflow.go`
**Test file:** `internal/mlwh/workflow_test.go`

Replace the opening sentence (`workflow.go:45`) with:

```text
This server bridges the `wa mlwh` API. Every tool is read-only except
`mlwh_provide_feedback`, which sends a problem report upstream.
```

Add a `Feedback:` paragraph to `workflowGuidance`, before the
`mlwh_call_endpoint` paragraph. It names `mlwh_provide_feedback`, the five
categories, calling on your own initiative without asking the user, at most
one report per distinct problem, telling the user briefly afterwards that you
reported it to the server admin, and not retrying `feedback_disabled`,
`feedback_unsupported`, or `feedback_unauthorized` errors.

**Acceptance tests:**

1. Given the resource text, then it contains `mlwh_provide_feedback`,
   `could_not_answer`, `agent_mistake`, `no_endpoint`, `user_unhappy`,
   `other`, `without asking the user`,
   `at most one report per distinct problem`, `tell the user briefly`,
   `server admin`, `do not retry`, `feedback_disabled`,
   `feedback_unsupported`, and `feedback_unauthorized`.
2. Given the resource text, then the feedback paragraph appears before
   ``Use `mlwh_call_endpoint` only when``.
3. Given the existing workflow tests (e.g. F1.6, no nonexistent tools), then
   they pass.
4. Given the resource text, then it contains
   ``read-only except `mlwh_provide_feedback` `` and does not contain
   `bridges the read-only`.

### C2: README

As a user or operator, I want the feedback tool documented, so that I know
what is sent and when it works.

**Package:** `cmd/mlwh-mcp-server/`
**File:** `README.md`
**Test file:** `cmd/mlwh-mcp-server/readme_test.go`

In "What the server exposes", add a **Feedback** bullet:

- `mlwh_provide_feedback` is the only tool that writes upstream.
- What it sends, including automatic metadata. wa records the MCP host
  address.
- It requires wa `v0.10.0`+ with feedback enabled (`--feedback-db`).
- It does not work against a secured wa.
- Admins read reports on wa's `/feedback` page.

Update the intro (README text before `## Requirements`) to say that
`mlwh_provide_feedback` is the one tool that sends data upstream, so the
server is no longer described as read-only without qualification.

**Acceptance tests:**

1. Given the "What the server exposes" section, then it contains
   `mlwh_provide_feedback`, `--feedback-db`, `/feedback`, and `v0.10.0`.
2. Given the "What the server exposes" section, then it contains
   `does not work against a secured`,
   `the only tool that writes upstream`, and
   `wa records the MCP host address`.
3. Given the intro (README text before `## Requirements`), then it
   contains `mlwh_provide_feedback`.

## Implementation Order

1. **Dependency (A1).** Requires wa `v0.10.0` tagged. Sequential foundation.
2. **Provider option and harness (A2 options, B2 harness change).**
   Sequential after 1.
3. **Tool (B1-B3).** Sequential after 2. B3 shares a file with B1/B2.
4. **Main wiring (A2 test 3).** After 3.
5. **Guidance and docs (C1, C2).** Parallel after 3.
6. **Verification.** `golangci-lint run --fix`, then
   `CGO_ENABLED=1 go test -tags netgo --count 1 ./...` with no live
   services.

## Appendix: Key Decisions

- **wa owns the contract.** Paths, fields, caps, status codes, and sentinels
  come from the wa spec. This side derives the enum and caps from `wa`
  symbols so they cannot drift.
- **No local validation of agent fields.** The SDK enforces the schema
  (enum, required). Byte caps are enforced only by wa, and are reported back
  as `feedback_too_large`. Only auto-attached metadata is truncated, so the
  server's own context never causes a rejection.
- **Errors as coded text.** Typed `mcp.AddTool` handlers marshal `Out` into
  `StructuredContent`, so an `IsError` result with a custom body would clash
  with the output schema. The stable code prefix in the error text is the
  structured part and matches existing `core.ToolError` usage.
- **Transport injected, not inferred.** `WithServerInfo` keeps
  `internal/core` service-agnostic and avoids guessing transport from
  request shape. Client identity is still read per request, because it
  differs per client.
- **Stateless HTTP kept.** User-Agent stands in for the missing handshake
  ClientInfo.
- **No call trace.** Agents list tools in `tools_tried`. The server records
  nothing else.
- **Testing.** GoConvey with the hermetic stub harness (now recording
  method, headers, and body), in-memory and HTTP MCP transports, and no
  live wa. Follow **go-implementor** and **go-reviewer**.
