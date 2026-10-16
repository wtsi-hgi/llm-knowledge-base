# Usage metrics: MCP side

Spec the mlwh server storing usage metrics, broken down by CLI, MCP and other API usage, including things like (per time period) num unique clients, num queries per client/"session", num calls per endpoint. Give server admin simple access to understand the usage metrics. Suggest following a similar pattern on how feedback was implemented. Note that I don't really know what can be determined from callers, so take my suggestions prior with a pinch of salt, but at least in general I want admins to know the system is being used and which endpoints are most popular.

This spec (/home/ubuntu/llm-knowledge-base/.docs/usage/spec.md) covers the MCP server side in this repo (/home/ubuntu/llm-knowledge-base). The companion spec at /home/ubuntu/wa/.docs/usage/spec.md (requirements and settled decisions in /home/ubuntu/wa/.docs/usage/prompt.md) covers wa mlwh serve, the wa CLI, the wa web UI and the admin page, and it OWNS the wire contract (identification headers, User-Agent product names, size caps, `RemoteConfig.UserAgent`/`Identity`, `WithClientIdentity`). This spec consumes that contract and must agree with it exactly; it must not redefine it.

## Notes

### Decisions (settled in the wa clarification rounds)

- The MCP server identifies itself on every request it makes to wa: User-Agent `mlwh-mcp-server/<version>`, its transport (stdio or http), a per-session ID, the agent identity, the MCP tool name, and a username where it has one.
- Per-session ID: in HTTP mode (go-sdk stateless), a hash of the client-echoed go-sdk session ID (`req.Session.ID()`), stable per client session. In stdio mode `Session.ID()` is empty, so generate a random ID once per process.
- Agent identity: clientInfo name/version in stdio; in HTTP mode clientInfo is unavailable (stateless), so send the agent's HTTP User-Agent instead, as the feedback tool already does (`internal/mlwh/tools_feedback.go`).
- Username: stdio sends the OS username; HTTP mode has no auth so never sends a username. No opt-out; documented as self-declared metadata.
- The provider shares one `wa.RemoteClient` across all sessions, so per-call identity (tool, session, agent) travels per request via the context (`wa.WithClientIdentity`), while the User-Agent is client-level config.
- Every tool, including `mlwh_call_endpoint` and `mlwh_provide_feedback`, carries the identity, so wa can attribute calls, including unmatched/404 paths, to tool and agent.
- This needs wa v0.11.0 (which ships the contract); bump `github.com/wtsi-hgi/wa` to it.
- The version in the User-Agent is `core.ServerVersion` as-is; unstamped builds (plain `go build`/`go install ...@vX`) send `mlwh-mcp-server/dev`. No build-info fallback in this feature.
- Agents are not told about usage recording in LLM-facing text. The README gets a usage section (headers sent per transport; stdio sends the OS username, self-declared, no opt-out; HTTP never sends a username) and the README feedback bullet notes the stdio username caveat. `feedbackDescription` stays unchanged (it is accurate about the report itself).

### Findings

- No blocking mismatch with the wa contract. wa v0.11.0 is not yet tagged; phase 1 needs a "tag exists" precondition like `.docs/feedback/phase1.md`.
- `provider` holds one `*wa.RemoteClient` built in `New` (`internal/mlwh/provider.go`) before options are applied; `WithServerInfo` sets version/transport after. `New` must build `RemoteConfig.UserAgent` and `Identity` (transport, plus username in stdio) after options. Existing tests rely on `New(cfg)` defaulting to `"stdio"`/`core.ServerVersion`.
- ~80 tool handlers call `p.client`; editing each is error-prone. Inject per-call identity in one place: a provider-owned receiving middleware on `"tools/call"` (precedent `core.ResultSizeGuard`, `internal/core/errs.go`; wired via `AddReceivingMiddleware`), added from `Register` via `r.Server()`, keeping `internal/core` free of wa imports. The middleware sees `*mcp.CallToolRequest` (tool name `Params.Name`, `Session`, `Extra.Header`).
- `core.ServerVersion` defaults to `"dev"`, stamped by Makefile ldflags. Transport set in `cmd/mlwh-mcp-server/main.go` via `WithServerInfo`.
- HTTP mode: stateless, unauthenticated, one shared `*mcp.Server`; per-request User-Agent at `req.Extra.Header`. go-sdk v1.6.1: stateless requests without `Mcp-Session-Id` get a fresh ID returned only on initialize; compliant clients echo it, so `ServerSession.ID()` is stable per session; non-echoing clients (raw curl) get a new ID per call (Known limit). Stdio `ID()` is `""`; ClientInfo from `req.Session.InitializeParams().ClientInfo`.
- Protocol traffic (initialize, tools/list, resources) and tool calls rejected before an upstream call never reach wa, so they are not counted (Known limit). wa counts upstream requests, not tool invocations.
- Test stub already records request headers (`internal/mlwh/harness_test.go`).
- wa C2's `components.parameters` does not affect tool schemas (`schema.go` reads only `components.schemas`).
- Version pins to update for wa v0.11.0 / API 1.10.0: `go.mod`, `cmd/mlwh-mcp-server/main_test.go`, `internal/mlwh/provider_test.go` (`TestWAAPI19Contract`), `cmd/mlwh-mcp-server/readme_test.go`, `README.md`, `.agents/skills/verify-mlwh-mcp/features/feedback-tool.md`, `.agents/skills/verify-mlwh-mcp/SKILL.md`. Leave feedback's "API 1.9.0"/"wa v0.10.0+" history statements unchanged.
- Client ID format: mirror wa (`^[0-9a-f]{32}$`): HTTP = first 32 hex of SHA-256 of the go-sdk session ID; stdio = 16 `crypto/rand` bytes as hex once per process. Username source mirrors wa D2 (`os/user.Current().Username`, else `$USER`, else empty). Agent header `<name>/<version>` (name alone if version empty; omitted if ClientInfo nil); in HTTP the agent's User-Agent. wa's client sanitises/caps all header values, so this repo adds no truncation.
- Transport and username go in `RemoteConfig.Identity`; client ID, agent and tool go in context via `wa.WithClientIdentity` (context overrides config).
- The feedback PR added a verify-skill recipe (`.agents/skills/verify-mlwh-mcp/features/`); a usage recipe follows the same precedent.
- Every wa request originates inside a `tools/call` (schemas come from compiled-in `wa.OpenAPIDocument()`; the workflow resource and `internal/core` never touch `p.client`), and no tool handler spawns goroutines, so the middleware's context reaches every client call including `SubmitFeedback`.
- `New` must move client construction after the options loop; nothing else depends on the current order.
- `core.ServerVersion` may be `v`-prefixed or `dev`; wa stores the version verbatim and categorises on the product token only, so this is fine.
- `webui/` is an unused scaffold and does not call wa.
