# Phase 1: Repo layout + module bootstrap

Ref: [spec.md](spec.md) sections Architecture (Repository layout,
Dependencies), Implementation Order item 1

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

This is the first phase; it is sequential and has no predecessors. All
later phases build on the module bootstrapped here.

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

### Item 1.1: Relocate web UI scaffold under webui/

spec.md section: Architecture (Repository layout), Implementation Order
item 1

Move the pre-existing web UI scaffold unchanged under `webui/`: `frontend/`
-> `webui/frontend/`, `backend/` -> `webui/backend/`, `run-dev.sh` ->
`webui/run-dev.sh`, root `.env.example` -> `webui/.env.example`. Move the
former web-oriented root README text into `webui/README.md`. Keep `LICENSE`
at the repo root. The move is purely organisational: no web UI code is
deleted or rewritten. See spec.md for the full target layout.

This item has no acceptance tests of its own; it is verified by item 1.3's
verifiable outcome (web UI files exist only under `webui/`).

- [x] implemented
- [x] reviewed

### Item 1.2: Add root Go module, dependencies, README, and .gitignore

spec.md section: Architecture (Repository layout, Dependencies),
Implementation Order item 1

Add the repo-root `go.mod` for module
`github.com/wtsi-hgi/llm-knowledge-base` targeting `go 1.25`, requiring
`github.com/wtsi-hgi/wa/mlwh` and `github.com/modelcontextprotocol/go-sdk/mcp`
(the latter pinned to `v1.6.1`; record resolved checksums in `go.sum`). Add
the new MCP-first root `README.md`. Update `.gitignore` to cover Go + Node +
Python. Add a minimal `cmd/mcp-server/main.go` entrypoint (no logic this
phase).

This item has no acceptance tests of its own; it is verified by item 1.3.

- [x] implemented
- [x] reviewed

### Item 1.3: Verify the bootstrapped module builds

spec.md section: Implementation Order item 1 (Verifiable clause)

Confirm the bootstrap satisfies the spec's stated verifiable outcome:
`go build ./...` succeeds on an empty `cmd/mcp-server/main.go`, and web UI
files exist only under `webui/` (none remain at the repo root). No Go logic
beyond the empty entrypoint is added in this phase.

This is the phase's only verifiable acceptance criterion (the spec defines
it as a build/layout check, not a numbered story acceptance test).

- [x] implemented
- [x] reviewed
