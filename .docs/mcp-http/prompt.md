# Feature: a shared, admin-run MLWH MCP server over streamable HTTP

## Summary

Add a **streamable-HTTP transport** to the MLWH MCP server so it can run as a
shared service: an **admin starts one long-lived instance** on an internal host
and **many users connect to it over the network** from their agent CLIs (Claude
Code, Codex) by pointing at its URL — **with no copy of the binary on any user's
machine and nothing on their PATH**.

The spec-writer workflow should use this file as the feature-description source
of truth and write the resulting specification to `.docs/mcp-http/spec.md`, with
phase documents in `.docs/mcp-http/`.

Today the server (this repo's `mlwh-mcp-server`, specified in
[`../mcp/spec.md`](../mcp/spec.md)) ships the **stdio** transport only: each user
runs their own copy as a local subprocess that their agent CLI launches, so the
executable must be installed per user. That stays the default. This feature adds
an **opt-in HTTP mode** selected by configuration; the stdio behaviour is
unchanged.

The original design deliberately deferred HTTP but built for it: `internal/core`
exposes the transport as a clean seam — `Run(ctx, mcp.Transport)` accepts any
`mcp.Transport` — the MCP Go SDK dependency already ships a server-side
streamable-HTTP handler, and the spec's "Stdio only, HTTP-ready" decision states
streamable HTTP can be added later "with no core change" to the provider surface.
This feature realises that deferred mode.

## Background: what exists today (this code is authoritative — read it)

- **The core transport seam.** `internal/core/transport.go` defines
  `(*Server).Run(ctx context.Context, t mcp.Transport) error`: it registers every
  configured provider, emits one startup version log line, then serves over the
  given transport until ctx is cancelled or the peer disconnects.
  `internal/core/server.go` has `New(Options) (*Server, error)` (builds the
  `*mcp.Server` with Implementation `mlwh-mcp-server`, Instructions, the
  `mcp-server://version` resource, and a `*slog.Logger`), the `Registrar` (which
  exposes `Server() *mcp.Server`), and the `Provider` seam. The core is
  service-agnostic and imports no `wa`/MLWH types.
- **The entrypoint.** `cmd/mlwh-mcp-server/main.go` is the composition root: a
  `run(args, stdout)` function parses flags (`--version`, the `--mlwh-*` provider
  flags), and `serve(cfg)` builds the MLWH provider + the core and calls
  `srv.Run(ctx, &mcp.StdioTransport{})` under a `signal.NotifyContext`. `--version`
  short-circuits before any transport is opened.
- **One binary per service.** The repo uses a per-service-binary model: the shared
  `internal/core` library plus a thin `cmd/<service>-mcp-server` entrypoint that
  wires that service's `internal/<service>` provider. `mlwh` is the first such
  provider; a future, unrelated service is a new `cmd/` + `internal/` package with
  no core change.
- **The test harness.** `internal/mlwh/harness_test.go` stands up a stub MLWH
  `httptest.Server` and a helper that builds a core server with the MLWH provider
  and runs it over `mcp.NewInMemoryTransports()` with a connected MCP client.
  Tests are hermetic — never a live warehouse.
- **Version surfacing.** The server already surfaces its version four ways
  (`--version`, the `mcp-server://version` resource, the startup log line, and the
  Implementation/Instructions); HTTP mode must keep all of these working.

## What the HTTP "shared server" mode must do

- Serve the **same MCP tool/resource surface** as stdio (all the MLWH tools, the
  `mlwh://workflow` resource, the `mcp-server://version` resource) over streamable
  HTTP, to **many concurrent client sessions** from one process.
- Be selected by **configuration** (a flag/env), with **stdio remaining the
  default** when HTTP is not requested.
- Be a **long-lived process** an admin starts once (suitable to run under
  systemd / in a container), listening on a configurable bind address on the
  internal network.
- **Shut down gracefully** on SIGINT/SIGTERM (stop accepting new connections,
  drain in-flight requests, then exit).
- Expose a cheap **liveness/health endpoint** for operations (matching
  `wa mlwh serve`'s `GET /health`), so a load balancer / monitor can probe it
  without speaking MCP.
- Emit a clear **startup log line** naming the transport and bind address
  alongside the existing server + targeted MLWH API versions.
- Let users connect with **no local binary**: their agent CLI is pointed at the
  server's URL (the HTTP transport), not at a command to launch.

## HARD REQUIREMENTS

1. **Build on the existing seam + the official SDK.** Use the MCP Go SDK
   (`github.com/modelcontextprotocol/go-sdk`, already pinned at v1.6.1) server-side
   streamable-HTTP handler (`mcp.NewStreamableHTTPHandler` /
   `mcp.StreamableHTTPOptions`). Do not re-implement MCP-over-HTTP and do not
   redesign the core's transport seam. The streamable-HTTP serving is handler-based
   (one shared `*mcp.Server`, many client sessions) rather than a single
   `mcp.Transport`, so the core needs a small HTTP-serving path that reuses the
   same provider registration `Run` performs — keep that change localized and the
   provider/tool code untouched.
2. **Stdio stays the default and unchanged.** HTTP is opt-in via configuration.
   With no HTTP configuration set, the binary behaves exactly as today (stdio).
3. **Same surface, service-agnostic core.** HTTP mode must expose the identical
   tool/resource surface as stdio with no changes to the MLWH provider or tools.
   The HTTP serving belongs in the shared `internal/core` (so every per-service
   binary gets it) plus the per-binary `cmd/` wiring; the core must stay free of
   `wa`/MLWH types.
4. **Internal-network posture, built on `go-authserver` like `wa`.** The MLWH data
   and the upstream `wa mlwh serve` API are internal and read-only, and this MCP
   server is a thin read-only bridge to them, so it runs **unauthenticated over
   plain HTTP right now**. But — exactly like `wa mlwh serve` — it must be built on
   the same auth-capable foundation, **`github.com/wtsi-hgi/go-authserver`**
   (imported as `gas`), so that **turning auth on later is a small, localized change
   rather than a redesign**. Mirror `wa`'s wiring: construct the auth server
   (`gas.New`), register the MCP route(s) on its router with **no auth group** for
   now — the unauthenticated path, like `wa`'s
   `server.RegisterRoutes(authServer.Router(), nil)` — and serve plain HTTP. Do NOT
   implement TLS or authentication in this round, but DO leave the
   `go-authserver`-provided seam (TLS + server token via `EnableAuthWithServerToken`,
   and the authenticated route group from `AuthRouter()`) ready to switch on. The
   operator remains responsible for internal-network placement.

## Configuration & deployment

- A single binary that runs **either** stdio (default) **or** HTTP, chosen by a
  flag/env consistent with the existing `--mlwh-*` flag + `MLWH_*` env convention
  (e.g. an `--http <addr>` / `MLWH_HTTP_ADDR` that, when set, enables HTTP and
  gives the bind address such as `:8080` or `127.0.0.1:8080`). The spec settles the
  exact names and the precise stdio-vs-HTTP selection rule.
- The bind address is configurable so the operator controls the interface
  (internal-only). Reuse the existing MLWH provider configuration unchanged
  (`MLWH_BASE_URL`, etc.) — HTTP mode only adds the transport/listener config.
- Graceful shutdown on signal, via the auth server's lifecycle (`wa` uses a
  ctx-aware `StartHTTP` plus `Stop()`), with a bounded drain timeout.
- A static health route (e.g. `GET /health`) registered on the same
  `go-authserver` router as the MCP endpoint but separate from the MCP path, so a
  probe needs no MCP handshake (as `wa mlwh serve` exposes `GET /health`).
- Operational logs (including the startup line) continue to go to stderr via the
  core's `slog.Logger`; in HTTP mode they no longer need to avoid stdout (stdout is
  only protocol-sensitive for stdio), but keeping logs on stderr is fine.

## Client usage (must be covered in the docs the feature ships)

Document, in the repo README, how end users connect to a shared instance with **no
local binary** — pointing their agent CLI at the URL:

- **Claude Code:** the HTTP transport form, e.g.
  `claude mcp add --transport http mlwh http://mlwh-mcp.internal:8080/<mcp-path>`
  (and the equivalent `.mcp.json` entry with the URL/transport).
- **Codex CLI:** the current documented HTTP MCP server form in
  `~/.codex/config.toml`; verify the exact syntax against the installed CLI or
  official docs during spec authoring. The important requirement is that it is a
  URL-based server, not a launched command.

The admin-run instructions (how to start the one shared instance, suitable
systemd/container invocation) also belong in the README.

## Non-functional / engineering conventions

- Idiomatic Go, building the HTTP server on `github.com/wtsi-hgi/go-authserver`
  (the gin-based server `wa` uses) with the SDK's streamable-HTTP `http.Handler`
  mounted onto its router (e.g. via gin's `WrapH`). Adding `go-authserver` as a
  direct dependency is expected (gin is already an indirect dependency via
  `wa/mlwh`).
- TDD with the repo's behaviour-focused testing discipline and the lint/test gates;
  wire any new `make`/CI needs into the existing `Makefile` + `.github/workflows`.
- **Hermetic tests**: exercise the HTTP path end-to-end without a live warehouse —
  serve the core server (with the existing stub MLWH provider) over streamable HTTP
  on an ephemeral local port (or an `httptest.Server` wrapping the handler), connect
  a real MCP client via the SDK's client-side streamable transport
  (`mcp.StreamableClientTransport`), and assert the same round-trips the stdio
  harness does (tools list, a tool call). Add coverage for the health endpoint and
  graceful shutdown.
- Keep all four version-surfacing channels working in HTTP mode.

## Out of scope

- *Enabling* TLS/HTTPS, authentication, authorization, or tokens in this round
  (the server runs unauthenticated plain HTTP, matching `wa mlwh serve`'s default
  posture). Out of scope to turn on now — but the design MUST keep it easy to add
  later by building on `go-authserver`; do not preclude it.
- Any change to the MLWH tool/resource surface or the provider's behaviour.
- Replacing or changing the stdio transport (it stays the default).
- Multi-tenant / per-user identity, rate limiting, quotas.
- The future non-MLWH service's own tools — HTTP mode should generalise to any
  per-service binary via the shared core, but only the MLWH binary is in scope now.
- A web UI (the `webui/` scaffold is unrelated to this feature).

## Key design decisions for the spec to settle

- The exact stdio-vs-HTTP **selection mechanism** and flag/env names (lean: an
  `--http <addr>`/`MLWH_HTTP_ADDR` that enables HTTP when set; absent ⇒ stdio).
- The **URL path** the MCP endpoint is served at (e.g. `/mcp`) vs the root, and how
  the health route coexists on the same listener.
- **Session statefulness**: the SDK's `StreamableHTTPOptions.Stateless`. This server
  only exposes tools/resources and initiates no server→client requests, and each
  tool call is an independent upstream request, so the **stateless** mode is the
  natural, simpler fit (no server-side session store, trivially restartable);
  confirm and justify the choice.
- The **shape of the core HTTP-serving API** (e.g. a `(*Server).RunHTTP(ctx, addr)`
  / serve method that reuses `Run`'s provider registration, vs. exposing the
  registered `*mcp.Server`) — keeping the transport seam clean and the provider code
  untouched.
- How the SDK's streamable-HTTP `http.Handler` mounts onto the `go-authserver`
  (gin) router, registered with no auth group now while leaving the authenticated
  group (`AuthRouter()`) wireable later — mirroring `wa`.
- Graceful-shutdown drain timeout and the health response body/shape.

## Pointers / prior art (in order of authority)

1. **This repo's existing code** (authoritative for the seam to build on):
   `internal/core/transport.go` (`Run`), `internal/core/server.go` (`New`,
   provider registration, startup log, Implementation/Instructions),
   `cmd/mlwh-mcp-server/main.go` (the `run`/`serve` flag wiring),
   `internal/mlwh/harness_test.go` (the hermetic stub + in-memory client harness to
   extend for an HTTP client test), and the existing spec
   [`../mcp/spec.md`](../mcp/spec.md) — especially Story **H1** (the transport
   seam), the "Stdio only, HTTP-ready" appendix decision, the version-surfacing
   stories **G2–G5**, and the **Testing strategy**.
2. **The MCP Go SDK v1.6.1** (authoritative for the HTTP wire protocol): the
   server-side `mcp.StreamableHTTPHandler` / `mcp.NewStreamableHTTPHandler(getServer
   func(*http.Request) *mcp.Server, opts *mcp.StreamableHTTPOptions)` with its
   `Stateless` option, and the client-side `mcp.StreamableClientTransport` for
   tests. Read the SDK source in the module cache; it is the contract.
3. **`wa mlwh serve` + `github.com/wtsi-hgi/go-authserver` v1.6.0** — the
   auth-ready internal-HTTP foundation to reuse. Read `~/wa/cmd/mlwh.go` for the
   wiring to mirror: `gas.New(...)`, `authServer.Router()` / `AuthRouter()`,
   `EnableAuthWithServerToken(...)`, the ctx-aware `StartHTTP` / TLS `Start` /
   `Stop` lifecycle, and the unauthenticated path
   `server.RegisterRoutes(authServer.Router(), nil)`. Also
   `~/wa/.docs/mcp/security-posture.md` for why unauthenticated-plain-HTTP is the
   deliberate default, and `~/wa/mlwh/server.go`'s static `GET /health` route.
4. **The `go-authserver` package source** (`gas.New(logWriter) *Server`,
   `(*Server).Router() *gin.Engine`, the auth/TLS methods) as the authority for the
   server foundation.

## Notes

Decisions already settled with the requester. These refine — and take precedence
over — any looser phrasing above:

- **Unauthenticated now, auth-ready via `go-authserver`.** Run as unauthenticated
  plain HTTP for now, but build the HTTP server on
  `github.com/wtsi-hgi/go-authserver` (as `wa` does) so authentication/TLS can be
  switched on later with a small change, not a redesign. Do not implement auth/TLS
  in this round; do register the MCP route with no auth group now (`wa`'s
  `RegisterRoutes(router, nil)` pattern). The operator handles internal-network
  placement.
- **Stdio stays the default.** HTTP is an opt-in mode chosen by configuration; with
  no HTTP config the binary is unchanged.
- **Same binary, both modes.** `mlwh-mcp-server` runs stdio or HTTP depending on
  config — not a separate executable. Users in HTTP mode need no local binary; they
  point their agent CLI at the URL.
- **HTTP serving lives in the shared core + cmd wiring**, exposing the identical
  tool/resource surface, so the per-service-binary model carries it to future
  services with no core change. The core stays service-agnostic (no `wa`/MLWH
  imports).
- **Reuse the SDK's streamable-HTTP handler**; do not hand-roll MCP-over-HTTP.
- **Tests stay hermetic** (stub MLWH + the SDK's streamable client transport over a
  local listener); never a live warehouse.
