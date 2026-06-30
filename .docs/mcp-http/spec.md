# MLWH MCP Streamable HTTP Specification

## Overview

Add an opt-in streamable-HTTP mode to `mlwh-mcp-server`. Stdio remains the
default and behaves as it does today. When `--http` or `MLWH_HTTP_ADDR` is set,
one admin-run process listens on the configured internal bind address and serves
the same MCP tools/resources to many network clients.

HTTP mode uses the MCP Go SDK streamable HTTP handler and is mounted on the
same `go-authserver` foundation as `wa mlwh serve`. It is unauthenticated plain
HTTP for this round; the operator owns internal-network placement. The design
must keep the auth/TLS seam local so a later change can use
`EnableAuthWithServerToken` and `AuthRouter()` without redesign.

## Architecture

Dependencies:

- Keep `github.com/modelcontextprotocol/go-sdk v1.6.1`.
- Add a direct requirement on `github.com/wtsi-hgi/go-authserver v1.6.0`.
- Use gin only through `go-authserver`'s router and `gin.WrapH`.

Core files:

- `internal/core/transport.go`: keep `Run(ctx, mcp.Transport)` for stdio.
- `internal/core/http.go`: new HTTP serving path, provider registration reuse,
  health route, SDK handler, injectable auth-server adapter and streamable
  handler factory for tests, graceful plain HTTP startup and shutdown.
- `internal/core/server.go`: share startup logging so HTTP logs transport and
  address while preserving existing version fields.
- `internal/core/http_test.go`: GoConvey tests with fake providers, real SDK
  HTTP client transport, health, concurrency, shutdown, and logging.

Command/docs files:

- `cmd/mlwh-mcp-server/main.go`: parse `--http`; resolve
  `MLWH_HTTP_ADDR`; select stdio vs HTTP; keep production signal wiring behind
  an injectable `signal.NotifyContext` wrapper and core server factory for
  command tests.
- `cmd/mlwh-mcp-server/main_test.go`: config precedence, stdio default,
  `--version`, and HTTP selection tests.
- `internal/mlwh/harness_test.go` and/or a new `internal/mlwh/http_test.go`:
  end-to-end MLWH HTTP tests against the existing stub warehouse.
- `README.md`: admin startup, Claude Code HTTP config, Codex HTTP config.
- `cmd/mlwh-mcp-server/readme_test.go`: GoConvey `TestREADMEHTTPDocs` reads
  `../../README.md` and verifies the HTTP docs.

Public core API:

```go
type HTTPOptions struct {
    Addr            string
    MCPPath         string
    HealthPath      string
    ShutdownTimeout time.Duration
    LogWriter       io.Writer
}

func (s *Server) RunHTTP(ctx context.Context, opts HTTPOptions) error
```

Defaults and validation:

- `Addr` is required by `RunHTTP`; empty returns
  `core: HTTP addr is required`.
- Empty `MCPPath` defaults to `/mcp`.
- Empty `HealthPath` defaults to `/health`.
- Zero `ShutdownTimeout` defaults to `5 * time.Second`.
- Nil `LogWriter` uses `io.Discard`; command wiring passes `os.Stderr`.

`RunHTTP` behaviour:

- Register providers through the same helper used by `Run`.
- Construct `gas.New(opts.LogWriter)`.
- Register `GET opts.HealthPath` on `authServer.Router()`.
- Register `GET`, `POST`, and `DELETE` for `opts.MCPPath` on
  `authServer.Router()` with no auth group, using `gin.WrapH`.
- Do not call `AuthRouter()`, `EnableAuthWithServerToken`, TLS `Start`, JWT
  middleware, or auth route groups in this phase.
- Build the handler with:

```go
mcp.NewStreamableHTTPHandler(
    func(*http.Request) *mcp.Server { return s.mcpServer },
    &mcp.StreamableHTTPOptions{Stateless: true, Logger: s.logger},
)
```

- Serve plain HTTP with a ctx-aware `StartHTTP` adapter modelled on
  `wa cmd/mlwh.go`: `net.Listen("tcp", addr)`, `http.Server{Handler:
  authServer.Router(), ReadHeaderTimeout: 30 * time.Second}`, and
  `Shutdown` with `ShutdownTimeout` when ctx is cancelled.
- Return nil for `http.ErrServerClosed`.
- Call `authServer.Stop()` on exit.

Health response:

```json
{"status":"ok"}
```

Startup logging:

- Existing stdio startup version log remains valid.
- HTTP startup log contains `server_version`, `api_versions`, `transport=http`,
  `addr=<resolved addr>`, `mcp_path=/mcp`, and `health_path=/health`.

Command config:

```go
const envHTTPAddr = "MLWH_HTTP_ADDR"
```

- Add flag `--http <addr>`.
- If `--http` is present, its exact value wins over env, including `""`.
- If `--http` is absent, use `MLWH_HTTP_ADDR`.
- Empty resolved value selects stdio.
- Non-empty resolved value selects HTTP and is passed as `HTTPOptions.Addr`.
- `--version` still short-circuits before config resolution or serving.

## Section A: Transport Configuration

### A1: Stdio remains default

As a local user, I want no-config transport behaviour to stay stdio, so that
existing Claude Code and Codex command-based setups keep working.

With no `--http` flag and no `MLWH_HTTP_ADDR`, `run` must build the MLWH
provider and call `srv.Run(ctx, &mcp.StdioTransport{})`. It must not open an
HTTP listener.

**Package:** `cmd/mlwh-mcp-server/`
**File:** `cmd/mlwh-mcp-server/main.go`
**Test file:** `cmd/mlwh-mcp-server/main_test.go`

**Acceptance tests:**

1. Given args `[]string{}` and env `MLWH_HTTP_ADDR=""`, when args are parsed,
   then the resolved HTTP address is `""` and transport mode is stdio.
2. Given valid MLWH config, no `--http`, no `MLWH_HTTP_ADDR`, and an injected
   core server factory returning a fake server, when `serve` runs with a
   cancellable context that stops the fake stdio run, then the fake records
   exactly one `Run(ctx, &mcp.StdioTransport{})` call and zero `RunHTTP` calls.
3. Given no HTTP config and invalid `MLWH_MAX_TOOL_RESULT_BYTES=bad`, when
   `serve` runs, then it returns an error containing
   `MLWH_MAX_TOOL_RESULT_BYTES` before any stdio serving begins.
4. Given `--version` and no `MLWH_BASE_URL`, when `run` executes, then stdout
   contains `core.ServerVersion` and `wa.APIVersion`, and it returns within
   3 seconds without opening stdio or HTTP.

### A2: HTTP config selects HTTP

As an admin, I want one flag/env setting to enable HTTP, so that deployment is
explicit and easy to script.

`--http` and `MLWH_HTTP_ADDR` carry the bind address, e.g. `:8080` or
`127.0.0.1:8080`. If `--http` is present, its exact value wins over env, even
when it is `""`. If `--http` is absent, env is used. Empty resolved value means
stdio; non-empty resolved value enables HTTP.

**Package:** `cmd/mlwh-mcp-server/`
**File:** `cmd/mlwh-mcp-server/main.go`
**Test file:** `cmd/mlwh-mcp-server/main_test.go`

**Acceptance tests:**

1. Given env `MLWH_HTTP_ADDR=":8080"` and no `--http`, when args are parsed,
   then the resolved HTTP address is `":8080"`.
2. Given env `MLWH_HTTP_ADDR=":8080"` and args `--http 127.0.0.1:9090`, when
   args are parsed, then the resolved HTTP address is `127.0.0.1:9090`.
3. Given env `MLWH_HTTP_ADDR=":8080"` and args `--http ""`, when args are
   parsed, then the resolved HTTP address is `""` and transport mode is stdio.
4. Given resolved HTTP address `127.0.0.1:0`, when `serve` is invoked with a
   cancellable context/fake core server, then it calls `RunHTTP` with
   `HTTPOptions{Addr:"127.0.0.1:0", MCPPath:"/mcp", HealthPath:"/health"}` and
   never calls `Run` with `mcp.StdioTransport`.

## Section B: Core HTTP Server

### B1: Shared streamable HTTP handler

As a service owner, I want HTTP mode in `internal/core`, so that every
per-service binary can reuse it without provider changes.

`RunHTTP` registers configured providers, builds one shared `*mcp.Server`, and
serves it through `mcp.NewStreamableHTTPHandler` with `Stateless: true`.
The core must not import `github.com/wtsi-hgi/wa/...` or `internal/mlwh`.

**Package:** `internal/core/`
**File:** `internal/core/http.go`, `internal/core/transport.go`
**Test file:** `internal/core/http_test.go`

**Acceptance tests:**

1. Given a fake provider with tool `test_ping`, when a client connects through
   `mcp.StreamableClientTransport{Endpoint:<httptest URL>/mcp}`, then
   `ListTools` includes exactly one `test_ping` entry from that provider.
2. Given the same server, when the client reads `mcp-server://version`, then
   the JSON has `server_version:"0.1.0"` and
   `api_versions.testsvc:"TESTAPI 9.9.9"`.
3. Given two HTTP client sessions connected to the same `/mcp` endpoint, when
   both call `test_ping`, then both receive structured content
   `{"message":"pong"}` and both calls complete without protocol errors.
4. Given `RunHTTP` is called with empty `HTTPOptions.Addr`, when invoked,
   then it returns an error whose string is `core: HTTP addr is required`.
5. Given an injected streamable handler factory that records its
   `*mcp.StreamableHTTPOptions`, when `RunHTTP` builds the `/mcp` route, then
   the recorded options are non-nil, `Stateless` is `true`, and `Logger` is the
   core server logger.
6. Given the same injected factory records the `getServer` callback, when that
   callback is called with any request, then it returns the same shared
   `*mcp.Server` that registered provider `test_ping`.
7. Given a static import check over `internal/core`, when `go list` or a test
   reads imports, then no core file imports `github.com/wtsi-hgi/wa/` or
   `github.com/wtsi-hgi/llm-knowledge-base/internal/mlwh`.

### B2: go-authserver foundation and plain routes

As a future maintainer, I want the HTTP server mounted on `go-authserver`, so
that auth/TLS can be enabled later locally.

HTTP mode constructs `gas.New`, registers `/mcp` and `/health` on
`authServer.Router()` with no auth group, and keeps `AuthRouter()` and
`EnableAuthWithServerToken` reachable through the local adapter. It must not
turn on TLS, auth, tokens, or JWT checks.

**Package:** `internal/core/`
**File:** `internal/core/http.go`
**Test file:** `internal/core/http_test.go`

**Acceptance tests:**

1. Given a fake auth server adapter and a sentinel SDK `http.Handler`, when
   routes are built, then `/mcp` `GET`/`POST`/`DELETE` and `/health` `GET` are
   registered on `Router()` with no auth group.
2. Given the same fake adapter, when routes are built and serving starts, then
   `EnableAuthWithServerToken`, TLS `Start`, `AuthRouter()`, JWT middleware,
   and auth route-group methods are called zero times.
3. Given the sentinel MCP handler is mounted, when `POST /mcp` is served
   through the gin engine, then the sentinel body/header is returned, proving
   the MCP route uses the SDK `http.Handler` through `gin.WrapH`.
4. Given `GET /health`, when called before any MCP initialization, then the
   response status is 200, the JSON body is `{"status":"ok"}`, and no provider
   tool handler is invoked.
5. Given `GET /not-found`, when called, then the response status is 404.
6. Given the implementation imports, when inspected, then `internal/core`
   imports `github.com/wtsi-hgi/go-authserver` as `gas`.

### B3: Graceful shutdown

As an operator, I want SIGINT/SIGTERM to drain HTTP requests, so that systemd or
containers can stop the shared service cleanly.

Command wiring continues to use `signal.NotifyContext`. In HTTP mode,
context cancellation stops accepting new connections, calls `Shutdown` with a
5-second default timeout, calls `authServer.Stop()`, and returns nil for
normal server shutdown. If `ShutdownTimeout` expires before in-flight handlers
finish, `RunHTTP` returns an error matching
`errors.Is(err, context.DeadlineExceeded)`.

**Package:** `internal/core/` + `cmd/mlwh-mcp-server/`
**File:** `internal/core/http.go`, `cmd/mlwh-mcp-server/main.go`
**Test file:** `internal/core/http_test.go`, `cmd/mlwh-mcp-server/main_test.go`

**Acceptance tests:**

1. Given `RunHTTP` is serving on a local listener and `ctx` is cancelled, when
   no requests are in flight, then `RunHTTP` returns nil within 3 seconds.
2. Given a request is in flight and `ctx` is cancelled, when the handler
   completes before `ShutdownTimeout`, then the client receives status 200 and
   `RunHTTP` returns nil.
3. Given `RunHTTP` has `ShutdownTimeout=50*time.Millisecond` and an injected
   MCP handler blocks until released, when one client starts `POST /mcp`,
   `ctx` is cancelled, and a new client with keep-alives disabled sends
   `GET /health`, then the new request returns a client-side error because the
   listener stopped accepting new connections, `RunHTTP` returns within
   500 milliseconds without releasing the blocked handler, and
   `errors.Is(err, context.DeadlineExceeded)` is true.
4. Given the fake auth server records calls, when HTTP serving exits, then
   `Stop()` has been called exactly once.
5. Given HTTP mode command wiring uses an injected `signal.NotifyContext`
   wrapper returning a cancellable context, and an injected fake core server
   whose `RunHTTP` records the context and blocks on `<-ctx.Done()`, when the
   injected context is cancelled to simulate SIGTERM, then `RunHTTP` received
   that same context, `serve` returns nil within 3 seconds, and the injected
   stop function was called exactly once.

## Section C: MLWH HTTP Surface

### C1: HTTP exposes the same MLWH surface

As an agent user, I want the shared HTTP server to expose the same MLWH tools
and resources as stdio, so that prompts and workflows do not change.

Use the existing stub MLWH warehouse. Start the core over streamable HTTP on a
local test endpoint and connect with the SDK client transport. Do not contact a
live warehouse. For surface comparisons, normalization may only sort tools by
`Name` and resources by `URI`; it must preserve all SDK metadata fields.

**Package:** `internal/mlwh/`
**File:** `internal/mlwh/harness_test.go`
**Test file:** `internal/mlwh/http_test.go`

**Acceptance tests:**

1. Given the same stub warehouse is used by the existing stdio/in-memory MLWH
   harness and the HTTP MLWH harness, when both clients call `ListTools`, then
   their normalized tool metadata slices, sorted by name, are `ShouldResemble`
   equal and the computed `missing_from_http` and `extra_in_http` name lists are
   both `[]string{}`.
2. Given the same two clients, when both call `ListResources`, then their
   normalized resource metadata slices, sorted by URI, are `ShouldResemble`
   equal and the computed `missing_from_http` and `extra_in_http` URI lists are
   both `[]string{}`.
3. Given the same two clients, when both read `mlwh://workflow`, then the first
   resource content from each response is `ShouldResemble` equal, has
   `MIMEType:"text/markdown"`, and its text contains `# MLWH workflows`,
   `wa mlwh API endpoint reference`, `/resolve/sample`, and
   `/study/:id/overview`.
4. Given the stub returns one study for `/studies` with
   `X-Total-Count: 1` and `X-Next-Offset: -1`, when the client calls
   `mlwh_call_endpoint` with `method:"AllStudies"` and query params
   `limit:"100", offset:"0"`, then the structured result has
   `total:1`, `next_offset:-1`, and `result` length 1.
5. Given the previous call, then the stub recorded path `/studies` and query
   parameters `limit=100` and `offset=0`.

### C2: Version surfaces still work over HTTP

As an operator, I want HTTP mode to preserve all version channels, so that
clients and logs remain auditable.

`--version` is unchanged. The HTTP MCP initialization result, `Instructions`,
`mcp-server://version`, and startup log all expose the server version and
targeted MLWH API version.

**Package:** `internal/mlwh/`, `internal/core/`,
`cmd/mlwh-mcp-server/`
**File:** `internal/core/server.go`, `internal/core/http.go`,
`cmd/mlwh-mcp-server/main.go`
**Test file:** `internal/mlwh/http_test.go`,
`internal/core/http_test.go`, `cmd/mlwh-mcp-server/main_test.go`

**Acceptance tests:**

1. Given HTTP mode with server version `0.1.0`, when a client connects, then
   `InitializeResult().ServerInfo.Name` is `mlwh-mcp-server` and
   `Version` is `0.1.0`.
2. Given the same connection, then the `Instructions` field contains `0.1.0`,
   `wa.APIVersion`, `mlwh://workflow`, and `mcp-server://version`.
3. Given the client reads `mcp-server://version`, then the JSON contains
   `server_version:"0.1.0"` and `api_versions.mlwh` equal to
   `wa.APIVersion`.
4. Given a buffer-backed slog logger and HTTP mode, when serving starts, then
   the log contains `0.1.0`, `wa.APIVersion`, `transport=http`,
   `addr=127.0.0.1:0`, `mcp_path=/mcp`, and `health_path=/health`.

## Section D: Documentation

### D1: Admin-run shared service docs

As an admin, I want README instructions for starting one shared instance, so
that users do not need local binaries.

Add a README section for HTTP mode. Show that stdio remains the default, and
that HTTP starts with `--http` or `MLWH_HTTP_ADDR`.

**Package:** repo docs
**File:** `README.md`
**Test file:** `cmd/mlwh-mcp-server/readme_test.go`

**Acceptance tests:**

1. Given `TestREADMEHTTPDocs` reads `../../README.md`, then the admin HTTP docs
   include `MLWH_BASE_URL=http://mlwh.internal:8080` and
   `mlwh-mcp-server --http 127.0.0.1:8081`.
2. Given the same README contents, then they state MCP is served at `/mcp`,
   health at `/health`, and the mode is unauthenticated plain HTTP for
   internal-network deployment.
3. Given the same README contents, then they include a minimal systemd or
   container example that sets `MLWH_BASE_URL` and `MLWH_HTTP_ADDR`.
4. Given the same GoConvey test, then it uses `So()` assertions and is run by
   `go test ./cmd/mlwh-mcp-server`, with no non-Go test target.

### D2: URL-based client docs

As an agent user, I want Claude Code and Codex HTTP config examples, so that I
can connect to the shared service without installing `mlwh-mcp-server`.

**Package:** repo docs
**File:** `README.md`
**Test file:** `cmd/mlwh-mcp-server/readme_test.go`

**Acceptance tests:**

1. Given `TestREADMEHTTPDocs` reads `../../README.md`, then the Claude Code
   HTTP docs include:
   `claude mcp add --transport http mlwh http://mlwh-mcp.internal:8080/mcp`.
2. Given the same README contents, then the Claude Code JSON docs include a
   `mcpServers.mlwh` entry with `"type": "http"` and
   `"url": "http://mlwh-mcp.internal:8080/mcp"`.
3. Given the same README contents, then the Codex CLI docs include:
   `codex mcp add mlwh --url http://mlwh-mcp.internal:8080/mcp`.
4. Given the same README contents, then the Codex TOML docs include:

```toml
[mcp_servers.mlwh]
url = "http://mlwh-mcp.internal:8080/mcp"
```

5. Given the same README contents, then the shared HTTP client docs state users
   do not install or run a local `mlwh-mcp-server` binary.
6. Given the same README contents, then no HTTP client example contains
   `"command": "mlwh-mcp-server"`, `command = "mlwh-mcp-server"`, or
   `-- mlwh-mcp-server`.

## Section E: Dependency and Gates

### E1: Dependency and test gates

As a maintainer, I want the dependency and CI shape explicit, so that the new
transport is reproducible.

Add `go-authserver` as a direct dependency. Keep tests hermetic and
GoConvey-based. No live MLWH warehouse is allowed.

**Package:** repo
**File:** `go.mod`, `go.sum`, `.github/workflows/ci.yml`, `Makefile`
**Test file:** existing Go test files

**Acceptance tests:**

1. Given `go.mod`, when inspected, then
   `github.com/wtsi-hgi/go-authserver v1.6.0` is a direct requirement.
2. Given the repository, when this command runs, then it passes without
   contacting a live MLWH service:

```bash
CGO_ENABLED=1 go test -tags netgo --count 1 \
  ./internal/core ./internal/mlwh ./cmd/mlwh-mcp-server -v
```

3. Given the repository, when `golangci-lint run --fix` runs, then it exits
   successfully.
4. Given CI/Makefile inspection, then no new service or binary target is added;
   the existing `mlwh-mcp-server` binary supports both modes.

## Implementation Order

1. **Core HTTP foundation (B1, B2, E1 partial).** Add direct dependency,
   `HTTPOptions`, provider-registration helper, streamable handler, health
   route, gas adapter, and core HTTP tests. Sequential foundation.
2. **Shutdown and logging (B3, C2 partial).** Add ctx-aware plain HTTP serving,
   shutdown timeout, `authServer.Stop()`, and HTTP startup log attrs. Sequential
   after 1.
3. **Command config (A1, A2).** Add `--http`/`MLWH_HTTP_ADDR`, mode selection,
   `RunHTTP` wiring, and preserve `--version`. Sequential after 1.
4. **MLWH HTTP E2E (C1, C2).** Extend the stub harness with SDK
   `StreamableClientTransport` tests for tools, resources, calls, concurrency,
   and versions. May start after 1; parallel with 5 after 3.
5. **README and gates (D1, D2, E1).** Update admin/user docs and run targeted
   tests/lint. Parallel with 4 after 3 for command examples.

## Appendix: Key Decisions

- **Selection rule:** a present `--http` wins over env even when empty; absent
  `--http` uses `MLWH_HTTP_ADDR`; empty resolved value means stdio; non-empty
  resolved value means HTTP.
- **Paths:** MCP is `/mcp`; health is `/health`; both share one listener.
- **Codex HTTP syntax:** the installed `codex-cli 0.141.0` exposes
  `codex mcp add <name> --url <URL>` for streamable HTTP, with TOML key
  `url` under `[mcp_servers.<name>]`.
- **Stateless:** use `StreamableHTTPOptions.Stateless=true`. The server exposes
  independent tools/resources and does not issue server-to-client requests, so
  no server-side session store is needed.
- **Security posture:** unauthenticated plain HTTP matches `wa mlwh serve`'s
  default. The access boundary is the internal network and any fronting layer.
- **Auth-ready seam:** mounting on `go-authserver` keeps `AuthRouter()` and
  `EnableAuthWithServerToken` local for a later TLS/token phase.
- **Testing:** follow `go-implementor`, `go-reviewer`, and `testing-principles`.
  Tests use GoConvey, `httptest`/local listeners, SDK streamable client
  transport, and the existing stub MLWH server only.
- **Provider scope:** no MLWH provider/tool/resource behaviour changes. Core
  remains service-agnostic.
