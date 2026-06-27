# Phase 7: Resources + version surfacing + flag

Ref: [spec.md](spec.md) sections G1, G2, G4, G5, Implementation Order
item 7

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 4 per the spec's Implementation Order: G2/G3/G5 need
the core, and G1 needs the provider. (G3 itself was delivered in phase 2;
this phase covers the remaining version-surfacing stories.) This phase may
proceed once phase 4 is reviewed; it does not depend on phases 5-6.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)
- Shared conventions: `go-conventions`
  (/home/ubuntu/.claude/skills/go-conventions/SKILL.md)
- Shared testing: `testing-principles`
  (/home/ubuntu/.claude/skills/testing-principles/SKILL.md)

## Items

### Item 7.1: G1 - Workflow / endpoint-catalogue resource

spec.md section: G1

In `internal/mlwh/workflow.go`, register a resource whose body is
`mlwh.EndpointReference()` (the Registry-derived Markdown catalogue),
prefixed with a short note on common workflows (resolve -> detail ->
expand). URI `mlwh://workflow`, MIME type `text/markdown`; build by calling
`EndpointReference()` (not a copied doc). Covers all 3 G1 acceptance tests
(text contains `EndpointReference()` output, e.g. the heading and a
`/resolve/sample` entry; MIMEType `text/markdown`; guidance mentioning
"resolve" and "detail").

- [ ] implemented
- [ ] reviewed

### Item 7.2: G2 - Version MCP resource

spec.md section: G2

In `internal/core/version.go`, the core registers a resource at URI
`mcp-server://version`, MIME type `application/json`, whose body is
`VersionInfo` JSON-marshaled
(`{"server_version":"...","api_versions":{"mlwh":"1.6.0"}}`); the MLWH
targeted version is `mlwh.APIVersion`. Covers both G2 acceptance tests (JSON
parses to `server_version` "0.1.0" and `api_versions.mlwh` ==
`mlwh.APIVersion`; MIMEType `application/json`).

- [ ] implemented
- [ ] reviewed

### Item 7.3: G4 - --version flag

spec.md section: G4

In `cmd/mcp-server/main.go`, parse a `--version` flag that prints the server
version and the targeted MLWH API version (`mlwh.APIVersion`) to stdout and
exits 0 without opening the transport. The server version is a build-time
package variable in `internal/core` (default e.g. "dev", overridable via
`-ldflags -X`). Also complete the normal-run wiring that passes
`&mcp.StdioTransport{}` to `Run` (the stdio end of H1). Covers both G4
acceptance tests (stdout contains the server version and the
`mlwh.APIVersion` value with exit 0; the command returns promptly and does
not block on stdin / start serving), tested in `main_test.go`.

- [ ] implemented
- [ ] reviewed

### Item 7.4: G5 - Startup-log version line

spec.md section: G5

In `internal/core/server.go`, when `Run` begins serving, emit one startup
log line through `Options.Logger` (`*slog.Logger`) naming this server's
version (`ServerVersion`) AND each provider's targeted upstream API version
(for MLWH, `mlwh.APIVersion`), sourced from the same `VersionInfo` the core
assembles; fall back to a default logger if none is configured. Covers the
single G5 acceptance test (a buffer-backed logger receives a startup line
containing both "0.1.0" and `mlwh.APIVersion` when `Run` reaches the serving
phase over an in-memory transport).

- [ ] implemented
- [ ] reviewed
