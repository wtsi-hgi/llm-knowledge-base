# Phase 5: README and gates

Ref: [spec.md](spec.md) sections D1, D2, E1

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 3 and parallel with phase 4. This phase updates docs,
adds README assertions, and finishes the dependency/test/lint gates. E1's
direct dependency assertion is handled in phase 1.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.agents/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.agents/skills/go-reviewer/SKILL.md)

## Items

### Item 5.1: D1 - Admin-run shared service docs

spec.md section: D1

Update `README.md` with admin HTTP mode docs showing stdio as the default,
startup with `--http` or `MLWH_HTTP_ADDR`, MCP at `/mcp`, health at
`/health`, unauthenticated plain HTTP for internal-network deployment,
`MLWH_BASE_URL=http://mlwh.internal:8080`,
`mlwh-mcp-server --http 127.0.0.1:8081`, and a minimal systemd or container
example that sets `MLWH_BASE_URL` and `MLWH_HTTP_ADDR`. Add GoConvey
assertions in `cmd/mlwh-mcp-server/readme_test.go`, covering all 4 acceptance
tests from D1.

- [ ] implemented
- [ ] reviewed

### Item 5.2: D2 - URL-based client docs

spec.md section: D2

Extend `README.md` and `cmd/mlwh-mcp-server/readme_test.go` with
`claude mcp add --transport http mlwh http://mlwh-mcp.internal:8080/mcp`,
Claude Code JSON containing `"type": "http"` and
`"url": "http://mlwh-mcp.internal:8080/mcp"`,
`codex mcp add mlwh --url http://mlwh-mcp.internal:8080/mcp`, Codex TOML
using `url = "http://mlwh-mcp.internal:8080/mcp"`, and text stating users do
not install or run a local `mlwh-mcp-server` binary for the shared HTTP client
setup. Also assert HTTP client examples do not include local command forms.
Cover all 6 acceptance tests from D2.

- [ ] implemented
- [ ] reviewed

### Item 5.3: E1 - Dependency and test gates

spec.md section: E1

Finish the repository gates after the implementation and docs are present:
run the targeted `CGO_ENABLED=1 go test -tags netgo --count 1 -v` command for
`./internal/core ./internal/mlwh ./cmd/mlwh-mcp-server`, run
`golangci-lint run --fix`, and inspect CI/Makefile shape so no new service
or binary target is added. Cover the remaining 3 acceptance tests from E1.

- [ ] implemented
- [ ] reviewed
