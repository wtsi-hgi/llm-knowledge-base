# MLWH Usage Identification Specification

## Overview

`wa mlwh serve` v0.11.0 can record one usage row per request it receives
and show aggregates on its admin `/usage` page. It tells callers apart by
optional identification headers. This spec makes `mlwh-mcp-server` send
them on every request it makes to wa, so admins see MCP usage by tool,
agent, session, transport, and (stdio only) user.

The User-Agent, transport, and stdio username are client-level
`wa.RemoteConfig` fields set once in `New`. The provider shares one
`wa.RemoteClient` across all MCP sessions, so the per-call client ID, agent,
and tool travel in the request context: one provider-owned receiving
middleware on `"tools/call"` wraps the context with `wa.WithClientIdentity`
before any tool handler runs. No tool handler changes. This repo stores
nothing and has no read path; wa counts and reports.

Depends on wa `v0.11.0` (`wa.APIVersion` `1.10.0`). The wire contract is
owned by `/home/ubuntu/wa/.docs/usage/spec.md` ("Wire contract:
identification headers", stories A1 and C1). This spec restates only what
the MCP side uses. If the two disagree, the wa spec wins.

## Architecture

### Authority and dependency

- `go.mod` and `go.sum` require `github.com/wtsi-hgi/wa v0.11.0`, with no
  replace directive. `wa.APIVersion` is `1.10.0`.
- Release order: wa tags `v0.11.0` first, then this repo bumps.
- Upstream symbols used (package `github.com/wtsi-hgi/wa/mlwh`, imported as
  `wa`), exactly as the wa spec defines them:

```go
const wa.UserAgentProductMCP = "mlwh-mcp-server"

const (
    wa.HeaderClientID     = "X-WA-Client-ID"
    wa.HeaderClientUser   = "X-WA-Client-User"
    wa.HeaderMCPTransport = "X-WA-MCP-Transport"
    wa.HeaderMCPAgent     = "X-WA-MCP-Agent"
    wa.HeaderMCPTool      = "X-WA-MCP-Tool"
)

// ClientIdentity is optional caller identification. Empty fields send no
// header.
type wa.ClientIdentity struct {
    ClientID     string
    Username     string
    MCPTransport string
    MCPAgent     string
    MCPTool      string
}

// WithClientIdentity returns ctx carrying id. RemoteClient requests made
// with it send id's non-empty fields, each overriding the same field of
// RemoteConfig.Identity. A nested call replaces the outer identity.
func wa.WithClientIdentity(ctx context.Context, id wa.ClientIdentity) context.Context

// wa.RemoteConfig gains:
//   UserAgent string            // sent on every request; "" keeps Go's default
//   Identity  wa.ClientIdentity // sent on every request
```

- `RemoteClient` sets these headers on every request, including
  `SubmitFeedback`. It sanitises each value (bytes outside 0x20-0x7E become
  `?`) and cuts it to its cap: User-Agent 512, client ID 128, user 128,
  transport 32, agent 256, tool 128 bytes. This repo adds no sanitising or
  truncation.
- wa categorises a request as `mcp` when the User-Agent product token (text
  before the first `/` or space) is exactly `mlwh-mcp-server`. The version
  after `/` is stored verbatim, so `dev` and `v`-prefixed values are fine.
- wa servers older than API 1.10.0 ignore the headers.

### Headers this server sends

| Header               | Set via | Value                                                                                    |
| -------------------- | ------- | ---------------------------------------------------------------------------------------- |
| `User-Agent`         | config  | `wa.UserAgentProductMCP + "/" + p.serverVersion`, e.g. `mlwh-mcp-server/dev`             |
| `X-WA-MCP-Transport` | config  | `p.transport` (`stdio` or `http`)                                                        |
| `X-WA-Client-User`   | config  | only when `p.transport == "stdio"`: `localUsername(user.Current, os.Getenv)`             |
| `X-WA-Client-ID`     | context | `sessionClientID(req.Session.ID())`: SHA-256 prefix in HTTP, per-process random in stdio |
| `X-WA-MCP-Agent`     | context | `agentIdentity(ClientInfo, req.Extra.Header)`: stdio clientInfo, HTTP User-Agent         |
| `X-WA-MCP-Tool`      | context | `req.Params.Name`                                                                        |

- `p.serverVersion` defaults to `core.ServerVersion`, and `main.go` already
  passes `core.ServerVersion` through `WithServerInfo`. Unstamped builds
  (`go build`, `go install ...@vX`) send `mlwh-mcp-server/dev`. There is no
  build-info fallback.
- HTTP mode never sends a username: it has no auth. Stdio sends the OS
  username with no opt-out. It is self-declared metadata.
- Client ID format mirrors wa's CLI run ID, `^[0-9a-f]{32}$`.
- HTTP mode stays stateless (`Stateless: true`). go-sdk v1.6.1 assigns a
  session ID on initialize, compliant clients echo it as `Mcp-Session-Id`,
  and `ServerSession.ID()` returns it on each request. In stdio and
  in-memory transports `ID()` is `""`.

### Files

```text
go.mod, go.sum                        wa v0.11.0
internal/mlwh/identity.go             config identity, per-call identity,
                                      tools/call middleware
internal/mlwh/identity_test.go        identity unit and harness tests
internal/mlwh/provider.go             New order, Register wiring, doc text
internal/mlwh/http_test.go            runMLWHHTTPServer options; HTTP tests
internal/mlwh/provider_test.go        TestWAAPI110Contract
internal/mlwh/workflow_test.go        agent-facing text guard
cmd/mlwh-mcp-server/main_test.go      version 1.10.0; header wiring test
cmd/mlwh-mcp-server/readme_test.go    version pins; usage docs
README.md                             version, usage section, feedback note
.agents/skills/verify-mlwh-mcp/       usage recipe, version pins
```

`cmd/mlwh-mcp-server/main.go`, `internal/core/`, every `tools_*.go`
handler, `workflow.go`, and `feedbackDescription` are unchanged.
`internal/core/` stays free of wa imports.

### Identity functions (`internal/mlwh/identity.go`)

```go
// identityRemoteConfig returns cfg with UserAgent set to
// wa.UserAgentProductMCP+"/"+serverVersion and Identity replaced by
// wa.ClientIdentity{MCPTransport: transport}, plus Username from
// localUsername(user.Current, os.Getenv) when transport is "stdio". Any
// UserAgent or Identity already in cfg is discarded. Other fields are kept.
func identityRemoteConfig(cfg wa.RemoteConfig, serverVersion, transport string) wa.RemoteConfig

// localUsername returns current().Username when current returns no error
// and a non-empty name, else getenv("USER"), else "".
func localUsername(current func() (*user.User, error), getenv func(string) string) string

// processClientID is 16 crypto/rand bytes as lowercase hex, made once per
// process.
var processClientID = sync.OnceValue(func() string { ... })

// sessionClientID returns the first 32 lowercase hex characters of the
// SHA-256 of sessionID, or processClientID() when sessionID is "".
func sessionClientID(sessionID string) string

// agentIdentity returns "<name>/<version>" from info when info is non-nil
// and info.Name is non-empty ("<name>" alone when Version is ""), else
// header.Get("User-Agent") when header is non-nil, else "".
func agentIdentity(info *mcp.Implementation, header http.Header) string

// callIdentity builds the per-call identity for a tools/call request:
// ClientID from sessionClientID (Session.ID(), or "" when Session is nil),
// MCPAgent from agentIdentity (InitializeParams().ClientInfo when Session
// and params are non-nil; Extra.Header when Extra is non-nil), and MCPTool
// from Params.Name when Params is non-nil. It never panics on nil fields.
func callIdentity(req *mcp.CallToolRequest) wa.ClientIdentity

// clientIdentityMiddleware returns receiving middleware that, for method
// "tools/call" with a *mcp.CallToolRequest, calls next with
// wa.WithClientIdentity(ctx, callIdentity(req)). Other methods and request
// types pass through unchanged.
func clientIdentityMiddleware() mcp.Middleware
```

- `New` validates `BaseURL`, builds `p` with defaults, applies options,
  then calls `wa.NewRemoteClient(identityRemoteConfig(cfg, p.serverVersion,
  p.transport))` and sets `p.client`. Nothing else depends on the old order.
- `Register` first calls
  `r.Server().AddReceivingMiddleware(clientIdentityMiddleware())`, then the
  existing `register*` calls unchanged.
- Every wa request starts inside a `tools/call`. Schemas come from the
  compiled-in `wa.OpenAPIDocument()`; the workflow resource and
  `internal/core` never touch `p.client`; and no handler spawns goroutines,
  so the middleware's context reaches every client call, including
  `mlwh_call_endpoint` and `SubmitFeedback`.
- `WithServerInfo`'s doc comment becomes: sets the MCP server build version
  and transport name ("stdio" or "http") attached to feedback reports and
  sent to wa as the User-Agent version and transport header.

### Known limits (documented, not fixed)

- wa counts upstream requests, not tool invocations. A tool making three
  wa calls counts three. Protocol traffic (`initialize`, `tools/list`,
  resources) and tool calls rejected before any upstream call (schema
  validation, unknown tool) never reach wa and are not counted.
- HTTP clients that do not echo `Mcp-Session-Id` (e.g. raw `curl`) get a
  new go-sdk session ID per request, so each call counts as a new session.
- Usernames are self-declared and spoofable; stdio users cannot opt out.
- All HTTP-mode users share the MCP host's IP in wa's records.
- In stdio, one process is one client ID. A restarted server is a new
  session.

## A. Dependency and Provider Wiring

### A1: Bump wa to v0.11.0

As the maintainer, I want the provider compiled against MLWH API 1.10.0, so
that `RemoteConfig.UserAgent`, `Identity`, and `WithClientIdentity` exist.

Update the pins of the current API version:

- `TestWAAPI19Contract` (renamed `TestWAAPI110Contract`; its A1.1 title
  says `1.10.0`).
- `TestRunVersionFlag` in `cmd/mlwh-mcp-server/main_test.go`.
- `TestREADMEPublicAPIDocs` (title `MLWH API 1.10.0`, F2.2 assertions).
- `README.md`: `currently MLWH API 1.10.0` and the `--version` example
  `# MLWH API version 1.10.0`.
- `.agents/skills/verify-mlwh-mcp/features/feedback-tool.md`: stored
  `wa_api_version` 1.10.0.
- `.agents/skills/verify-mlwh-mcp/SKILL.md` Requirements: a wa checkout at
  `v0.11.0`+ (agent feedback and usage metrics).

Leave history statements unchanged: feedback's `MLWH API 1.9.0` next step
and its tests, README `wa v0.10.0`+ for feedback, and `pre-1.9.0` /
`Since wa v0.10.1` in `feedback-tool.md`.

**Package:** `internal/mlwh/`, `cmd/mlwh-mcp-server/`
**File:** `go.mod`, `go.sum`, `README.md`,
`.agents/skills/verify-mlwh-mcp/SKILL.md`,
`.agents/skills/verify-mlwh-mcp/features/feedback-tool.md`
**Test file:** `internal/mlwh/provider_test.go`,
`cmd/mlwh-mcp-server/main_test.go`, `cmd/mlwh-mcp-server/readme_test.go`

**Acceptance tests:**

1. Given the module, then `wa.APIVersion == "1.10.0"` and
   `provider.APIVersion() == wa.APIVersion`.
2. Given `--version`, then output contains `MLWH API version 1.10.0`.
3. Given README (lowercased, single-spaced), then it contains
   `currently mlwh api 1.10.0` and `# mlwh api version 1.10.0`, and not
   `currently mlwh api 1.9.0`, `1.8.1`, or `1.7.0`.
4. Given the existing Registry/OpenAPI parity, schema, and
   `mlwh_call_endpoint` method-enum tests, then they pass unchanged (wa C2
   adds only `components.parameters`, which `schema.go` does not read).

### A2: Client-level identity in New

As the server admin, I want every wa request from this server to name its
product, version, transport, and (stdio) user, so that wa categorises it as
MCP.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/identity.go`, `internal/mlwh/provider.go`
**Test file:** `internal/mlwh/identity_test.go`

Signatures as in Architecture. Harness tests stub `/freshness` with a
synced response and call `mlwh_freshness` with `{}` through
`runMLWHServerWithClientOptions`.

**Acceptance tests:**

1. Given `New(cfg)` with no options and `core.ServerVersion` `"dev"`, when
   `mlwh_freshness` runs, then the stub's one request has
   `User-Agent: mlwh-mcp-server/dev` and `X-WA-MCP-Transport: stdio`.
2. Given `WithServerInfo("0.4.0", "http")`, then the request has
   `User-Agent: mlwh-mcp-server/0.4.0`, `X-WA-MCP-Transport: http`, and
   `Header.Values("X-WA-Client-User")` is empty.
3. Given `WithServerInfo("v1.2.3", "stdio")` and
   `u := localUsername(user.Current, os.Getenv)`, then `User-Agent` is
   `mlwh-mcp-server/v1.2.3`, and `X-WA-Client-User` equals `u` when `u` is
   non-empty, else is absent.
4. Given `identityRemoteConfig(wa.RemoteConfig{BaseURL: "http://x",
   Timeout: 5 * time.Second, UserAgent: "other/1", Identity:
   wa.ClientIdentity{ClientID: "c", Username: "mallory"}}, "0.4.0",
   "http")`, then `BaseURL` is `http://x`, `Timeout` is 5 s, `UserAgent` is
   `mlwh-mcp-server/0.4.0`, and `Identity` equals
   `wa.ClientIdentity{MCPTransport: "http"}`.
5. Given `identityRemoteConfig(wa.RemoteConfig{BaseURL: "http://x"},
   "dev", "stdio")`, then `UserAgent` is `mlwh-mcp-server/dev` and
   `Identity` equals `wa.ClientIdentity{MCPTransport: "stdio", Username:
   localUsername(user.Current, os.Getenv)}`.
6. Given `localUsername` with `current` returning
   `&user.User{Username: "alice"}, nil` and `getenv` returning `"bob"` for
   `USER`, then `"alice"`. Given `current` returning `nil, errors.New("x")`,
   then `"bob"`. Given `current` returning `&user.User{}, nil`, then `"bob"`.
   Given an error and `getenv` returning `""`, then `""`.
7. Given `New(wa.RemoteConfig{}, WithServerInfo("0.4.0", "http"))`, then
   `ErrBaseURLRequired` and a nil provider (existing test passes).

### A3: Main wiring

As the maintainer, I want the binary's transport choice to reach the wa
headers, so that production traffic is labelled correctly.

`main.go` already calls `mlwh.New(remoteCfg,
mlwh.WithServerInfo(core.ServerVersion, string(cfg.Transport)))`; this
story only extends `TestServeWiresServerInfo`. Its stub also captures the
`/feedback` request headers.

**Package:** `cmd/mlwh-mcp-server/`
**File:** none (test only)
**Test file:** `cmd/mlwh-mcp-server/main_test.go`

**Acceptance tests:**

1. Given `core.ServerVersion` `"9.9.9-test"` and `serve` run with
   `--http 127.0.0.1:0`, when the captured provider submits feedback, then
   the request has `User-Agent: mlwh-mcp-server/9.9.9-test`,
   `X-WA-MCP-Transport: http`, `X-WA-MCP-Tool: mlwh_provide_feedback`, and
   no `X-WA-Client-User`.
2. Given the same without `--http`, then `X-WA-MCP-Transport: stdio` and
   the same `User-Agent`.

## B. Per-Call Identity

### B1: Identity helpers

As the server admin, I want each call's session, agent, and tool derived
consistently, so that wa can count sessions and top tools and agents.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/identity.go`
**Test file:** `internal/mlwh/identity_test.go`

**Acceptance tests:**

1. Given `sessionClientID("abc")`, then
   `"ba7816bf8f01cfea414140de5dae2223"`.
2. Given `sessionClientID("")` called twice, then both equal
   `processClientID()`, which matches `^[0-9a-f]{32}$`.
3. Given `agentIdentity` with:
   - `&mcp.Implementation{Name: "claude-code", Version: "2.1.0"}`, nil:
     `"claude-code/2.1.0"`.
   - `&mcp.Implementation{Name: "claude-code"}`, nil: `"claude-code"`.
   - `&mcp.Implementation{Version: "1"}` and a header with
     `User-Agent: x/1`: `"x/1"`.
   - nil and a header with `User-Agent: agent-cli/1.2`: `"agent-cli/1.2"`.
   - `&mcp.Implementation{Name: "claude-code", Version: "2.1.0"}` and a
     header with `User-Agent: x/1`: `"claude-code/2.1.0"`.
   - nil, nil: `""`.
4. Given `callIdentity(&mcp.CallToolRequest{})`, then no panic and the
   result equals `wa.ClientIdentity{ClientID: processClientID()}`.
5. Given `callIdentity` with `Session: &mcp.ServerSession{}`,
   `Params: &mcp.CallToolParamsRaw{Name: "mlwh_freshness"}`, and
   `Extra: &mcp.RequestExtra{Header: http.Header{"User-Agent": {"a/1"}}}`,
   then the result equals `wa.ClientIdentity{ClientID: processClientID(),
   MCPAgent: "a/1", MCPTool: "mlwh_freshness"}`.

### B2: tools/call middleware

As the server admin, I want every tool call's wa requests to carry its
identity, so that calls are attributed to tool and agent, including
unmatched and failed upstream paths.

Harness tests use `runMLWHServerWithClientOptions` (client
`test-client/0.0.0`) or `runMLWHServerWithClientImplementation`, with
`WithServerInfo("0.4.0", "stdio")`, and stub `/freshness` with a synced
response.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/identity.go`, `internal/mlwh/provider.go`
**Test file:** `internal/mlwh/identity_test.go`

**Acceptance tests:**

1. Given `mlwh_freshness` called with `{}`, then the stub's one request has
   `X-WA-MCP-Tool: mlwh_freshness`, `X-WA-MCP-Agent: test-client/0.0.0`,
   `X-WA-Client-ID` equal to `processClientID()`, and
   `X-WA-MCP-Transport: stdio`.
2. Given one session calling `mlwh_freshness` then `mlwh_call_endpoint`
   with `{"method": "Freshness"}`, then the stub saw two requests to
   `/freshness`, with `X-WA-MCP-Tool` `mlwh_freshness` then
   `mlwh_call_endpoint`, and equal `X-WA-Client-ID` values.
3. Given two separate harness servers in one test, each calling
   `mlwh_freshness`, then both requests carry the same `X-WA-Client-ID`.
4. Given the stub replying 201
   `{"id":7,"created_at": "2026-10-01T12:00:00Z"}` on `/feedback` and
   `mlwh_provide_feedback` called with `{category:"other", description:"x"}`,
   then the `POST /feedback` has `X-WA-MCP-Tool: mlwh_provide_feedback`,
   `X-WA-MCP-Agent: test-client/0.0.0`, `X-WA-Client-ID` equal to
   `processClientID()`, and its body still has `client_name`
   `"test-client"` and `client_version` `"0.0.0"`.
5. Given client implementation `{Name: "claude-code", Version: ""}`, then
   `X-WA-MCP-Agent: claude-code`.
6. Given no `/freshness` response configured (the stub answers with its 404
   wa envelope), when `mlwh_freshness` runs, then the result has
   `IsError == true` and the failed request still carries
   `X-WA-MCP-Tool: mlwh_freshness`.
7. Given a connected session that lists tools and reads `mlwh://workflow`
   and `mcp-server://version`, then the stub received 0 requests.
8. Given a `mlwh_provide_feedback` call with `category: "bogus"`, then
   `IsError == true` and the stub received 0 requests.

### B3: HTTP mode identity

As the server admin, I want shared HTTP sessions told apart and never
carrying a username, so that session counts are meaningful and HTTP users
are not misattributed.

`runMLWHHTTPServer` gains `providerOpts ...Option`, passed to `New`;
existing callers compile unchanged. Tests use `WithServerInfo("0.5.0",
"http")`, stub `/freshness` with a synced response, and connect with
`connectMLWHHTTPClientWith` (`userAgentRoundTripper` for a custom
User-Agent).

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/http_test.go`
**Test file:** `internal/mlwh/identity_test.go`

**Acceptance tests:**

1. Given one session with User-Agent `agent-cli/1.2` calling
   `mlwh_freshness` twice, then `cs.ID()` is non-empty and both stub
   requests have `User-Agent: mlwh-mcp-server/0.5.0`,
   `X-WA-MCP-Transport: http`, `X-WA-MCP-Agent: agent-cli/1.2`,
   `X-WA-MCP-Tool: mlwh_freshness`, `X-WA-Client-ID` equal to
   `sessionClientID(cs.ID())`, and no `X-WA-Client-User`.
2. Given two sessions each calling `mlwh_freshness`, then their
   `X-WA-Client-ID` values differ and each matches `^[0-9a-f]{32}$`.
3. Given a session with no `HTTPClient` (`connectMLWHHTTPClient`), then
   `X-WA-MCP-Agent` is `Go-http-client/1.1`.
4. Given sessions with User-Agents `agent-a/1` and `agent-b/1` calling
   `mlwh_freshness` concurrently, then the stub saw 2 requests, and the
   request whose `X-WA-Client-ID` is `sessionClientID` of the `agent-a/1`
   session has `X-WA-MCP-Agent: agent-a/1`, and likewise for `agent-b/1`.
5. Given the stub storing feedback and a session with User-Agent
   `agent-cli/1.2` calling `mlwh_provide_feedback`, then the
   `POST /feedback` has `X-WA-MCP-Tool: mlwh_provide_feedback`,
   `X-WA-MCP-Agent: agent-cli/1.2`, and no `X-WA-Client-User`, and
   `TestMLWHHTTPFeedbackUserAgent` passes unchanged.

## C. Guidance and Docs

### C1: Agent-facing text unchanged

As the maintainer, I want agents not told about usage recording, so that
LLM-facing text stays about the task.

Checks cover only text this repo owns: the `mlwh://workflow` body
appends upstream `wa.EndpointReference()`, whose intro already says
"usage".

**Package:** `internal/mlwh/`
**File:** none (test only)
**Test file:** `internal/mlwh/workflow_test.go`

**Acceptance tests:**

1. Given the `workflowGuidance` constant, the server `Instructions` from
   the client's `InitializeResult()`, and the `mlwh_provide_feedback` tool
   description, then none contains `X-WA-` or, case-insensitively,
   `usage`.

### C2: README

As a user or operator, I want the identification documented, so that I
know what this server sends to wa.

Add `## Usage metrics` after the "What the server exposes" section's
closing paragraph and before `## Development`. It says:

- wa `v0.11.0`+ run with `--usage-db` records each request; admins read
  the aggregates on wa's `/usage` page. Older wa ignores the headers.
- Every request carries User-Agent `mlwh-mcp-server/<version>`;
  unstamped builds send `mlwh-mcp-server/dev`.
- A table of `X-WA-MCP-Transport`, `X-WA-Client-ID`, `X-WA-MCP-Agent`,
  `X-WA-MCP-Tool`, and `X-WA-Client-User` with their stdio and HTTP values
  per "Headers this server sends".
- Stdio sends the OS username. It is self-declared, with no opt-out. HTTP
  mode never sends a username.
- wa counts its own requests, not tool calls; protocol messages are not
  counted. All HTTP-mode users share the MCP host's IP.

In the intro, reword the sentence calling `mlwh_provide_feedback` "the
one tool that sends data upstream": it is the one tool that sends a report
upstream, and every request carries identifying metadata (see [Usage
metrics](#usage-metrics)).

In the **Feedback** bullet, add that in stdio mode the report request,
like every request to wa, carries the OS username (self-declared; see
Usage metrics).

**Package:** `cmd/mlwh-mcp-server/`
**File:** `README.md`
**Test file:** `cmd/mlwh-mcp-server/readme_test.go`

Tests read
`singleSpaced(readmeSection(readme, "## Usage metrics", "## Development"))`,
`singleSpaced(readmeSection(readme, "- **Feedback**", "- **Escape hatch**"))`,
and
`singleSpaced(readmeSection(readme, "# MLWH MCP Server", "## Requirements"))`
(the intro), case preserved.

**Acceptance tests:**

1. Given the usage section, then it contains `mlwh-mcp-server/<version>`,
   `mlwh-mcp-server/dev`, `X-WA-MCP-Transport`, `X-WA-Client-ID`,
   `X-WA-MCP-Agent`, `X-WA-MCP-Tool`, and `X-WA-Client-User`.
2. Given the usage section, then it contains `OS username`,
   `self-declared`, `no opt-out`, `never sends a username`, `v0.11.0`,
   `--usage-db`, and `/usage`.
3. Given the Feedback bullet, then it contains `OS username` and
   `self-declared`.
4. Given the intro, then it contains `sends a report upstream` and
   `#usage-metrics`, and does not contain `sends data upstream`.
5. Given `TestREADMEFeedbackDocs` and `TestREADMEHTTPDocs`, then they pass
   unchanged.

### C3: Verify recipe

As a verifier, I want a recipe that proves real MCP traffic reaches wa's
usage summary, so that the feature is checked on the user path.

Add `features/usage-metrics.md` in the format of `feedback-tool.md` (what
it is, how to reach it, how to drive it, gotchas), a row in
`features/README.md` (`Usage identification`, entry points "every
`tools/call` over stdio and HTTP", helper `drive-usage.sh`), and the usage
summary in the `SKILL.md` description. `scripts/drive-usage.sh <run-dir>`
follows `drive-feedback.sh` (`pass_if`, `PASS`/`FAIL` lines, exit 0 only
when all pass, evidence numbered `6x`):

1. `GET $MLWH_URL/usage?window=24h` with
   `Authorization: Bearer $(cat "$STATE_HOME/.wa-mlwh-server.token")`, saved as
   `60-usage-before.json`.
2. stdio:
   `-name verify-mlwh-mcp-usage -version 3.4.5 call mlwh_freshness '{}'`,
   saved as `61-stdio-freshness.json`.
3. HTTP: `-ua "verify-mlwh-mcp-usage-agent/7.6 ($STAMP)" call
   mlwh_call_endpoint '{"method":"Freshness"}'`, saved as
   `62-http-call-endpoint.json`.
4. The same `GET /usage`, saved as `63-usage-after.json`.

Gotchas to record: the stack must be wa `v0.11.0`+ with usage on (verify-wa
test mode enables it); counts are compared as before/after deltas because
the wa launch keeps earlier rows; the admin page view is verify-wa's
`usage-admin-page.md` recipe.

**Package:** none (skill files)
**File:** `.agents/skills/verify-mlwh-mcp/features/usage-metrics.md`,
`.agents/skills/verify-mlwh-mcp/scripts/drive-usage.sh`,
`.agents/skills/verify-mlwh-mcp/features/README.md`,
`.agents/skills/verify-mlwh-mcp/SKILL.md`
**Test file:** none (proved by running the drive)

**Acceptance tests:**

1. Given a launched stack and a passing Doctor, when `drive-usage.sh` runs,
   then it prints `PASS` for each check below and exits 0:
   - both calls returned without `.result.isError`;
   - the `mcp` entry of `categories` grew by 2 `requests` and 2
     `sessions`;
   - `users` entry `$(id -un)` grew by 1 in `by_category.mcp` (stdio only,
     so HTTP sent no username);
   - `mcp_tools` `mlwh_freshness` and `mlwh_call_endpoint` each grew by 1;
   - `mcp_agents` gained 1 for `verify-mlwh-mcp-usage/3.4.5` and 1 for
     the HTTP User-Agent;
   - `endpoints` entry `GET` `/freshness` grew by 2 in `by_category.mcp`.

## Implementation Order

1. **Dependency (A1).** Precondition:
   `timeout 60 go list -m github.com/wtsi-hgi/wa@v0.11.0` succeeds; if not,
   stop and report that wa `v0.11.0` is not tagged. Sequential foundation.
2. **Client-level identity (A2).** Sequential after 1. Creates
   `identity.go`.
3. **Per-call identity (B1, B2).** Sequential after 2; same files.
4. **HTTP identity and main wiring (B3, A3).** Parallel after 3.
5. **Guidance and docs (C1, C2).** Parallel after 3.
6. **Verify recipe (C3).** After 4 and 5; needs a wa `v0.11.0` checkout.
7. **Verification.** `golangci-lint run --fix`, then
   `CGO_ENABLED=1 go test -tags netgo --count 1 ./...` with no live
   services, then `drive-usage.sh` and `drive-feedback.sh` on a launched
   stack.

## Appendix: Key Decisions

- **wa owns the contract.** Header names, caps, sanitising, and the
  product token come from wa symbols, so they cannot drift. This side only
  chooses values.
- **One middleware, not ~80 handler edits.** A receiving middleware on
  `"tools/call"` (precedent `core.ResultSizeGuard`) covers every tool,
  including future ones. It lives in the provider and is added from
  `Register` via `r.Server()`, so `internal/core` stays service-agnostic.
- **Config vs context.** Values fixed per process (User-Agent, transport,
  stdio username) are `RemoteConfig`; values per call (client ID, agent,
  tool) are context, because one client serves all sessions.
- **Provider owns identity.** `identityRemoteConfig` replaces any caller
  `UserAgent`/`Identity`; `Config.Resolve` never sets them.
- **User-Agent version from `p.serverVersion`.** It equals
  `core.ServerVersion` in the binary and lets tests vary it through
  `WithServerInfo`, as feedback does.
- **Presence-based agent rule.** `agentIdentity` prefers handshake
  clientInfo and falls back to the HTTP User-Agent. Stateless HTTP has no
  clientInfo, so this yields the transport rule without branching on
  transport, and matches `feedbackSubmission`.
- **Client ID from session presence.** A non-empty go-sdk session ID is
  hashed (never sent raw); otherwise the per-process ID is used. Stdio and
  in-memory transports have no session ID.
- **Username by transport.** Only `p.transport == "stdio"` sends one: HTTP
  mode is shared and unauthenticated, so the server's OS user is not the
  caller.
- **No agent disclosure.** Tool descriptions, the Instructions, and
  `workflowGuidance` do not mention usage recording; the README does. The
  appended upstream endpoint reference is wa's text and is not checked.
- **Testing.** GoConvey with the hermetic stub harness (which already
  records headers), in-memory and HTTP MCP transports, injected
  `user.Current`/`getenv` for `localUsername`, and no live wa. Follow
  **go-implementor** and **go-reviewer**.
