# Phase 2: Core seam

Ref: [spec.md](spec.md) sections G3, H1, Core provider abstraction,
Generic tool-error helper, Implementation Order item 2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 1 (needs the bootstrapped module). Per the spec's
Implementation Order, phase 3 (Schema + error foundations) is
parallelisable with this phase once the module builds; the two share no
code, so they may proceed concurrently.

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

### Item 2.1: Service-agnostic core abstraction and server build

spec.md section: Core provider abstraction, Generic tool-error helper,
Implementation Order item 2 (I-prereqs)

Implement `internal/core`: the `Provider` and `Registrar` interfaces,
`VersionInfo`, `Options`, `New(opts Options) (*Server, error)`, the
build-time version vars, and the generic `toolError` helper (in `errs.go`).
The core must stay free of MLWH/wa types. This is the seam prerequisite for
stories I1/I2; those assertions land in later phases. See spec.md for the
exact interface and struct definitions.

This item underpins the phase's acceptance tests (no story acceptance tests
are dedicated to the I-prereqs themselves; they are exercised via G3 and
H1 below).

- [ ] implemented
- [ ] reviewed

### Item 2.2: H1 - Stdio transport seam (Run takes any mcp.Transport)

spec.md section: H1

Implement the transport seam in `internal/core/transport.go` and
`(*Server).Run(ctx, mcp.Transport)` so it accepts any `mcp.Transport`. Add
no HTTP transport code, config, or listeners anywhere in the module. Covers
both H1 acceptance tests: (1) a core server run over
`mcp.NewInMemoryTransports()` lets a connected test client list the MLWH
tools (proving `Run` takes any transport - note the MLWH tools become
listable only once a provider is wired, so this assertion is realised end
to end in phase 8; this phase proves the in-memory transport path with the
core alone); (2) inspecting the module confirms no HTTP transport, config
flag, or listener exists. The `cmd/mcp-server` wiring of
`&mcp.StdioTransport{}` is completed in phase 7's flag work.

- [ ] implemented
- [ ] reviewed

### Item 2.3: G3 - Server implementation info and instructions

spec.md section: G3

In `internal/core/server.go`, set
`mcp.Implementation{Name:"mlwh-mcp-server", Version: ServerVersion}` and
`ServerOptions.Instructions` to a string that includes the server version
and each provider's targeted upstream API version (e.g. "MLWH API 1.6.0"),
briefly pointing at the workflow and version resources, assembled from
`VersionInfo`. Fully covers G3 acceptance test 1 (Implementation
`Name`/`Version`), which needs no provider. For G3 acceptance test 2
(Instructions contain the server version and a provider's targeted API
version) this phase proves the core mechanism with a test-double provider
supplying a version, since the core stays provider-agnostic; the literal
`mlwh.APIVersion` value is realised once the MLWH provider is wired (it
supplies `mlwh.APIVersion` to `VersionInfo` in phase 4).

- [ ] implemented
- [ ] reviewed
