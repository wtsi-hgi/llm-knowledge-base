# MLWH MCP Server

A [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server,
written in Go, that lets LLM agents (Claude Code, Codex, and later a web UI
server-side agent) query the Multi-LIMS Warehouse (MLWH) read API in natural,
tool-driven ways: search and count samples and studies, resolve identifiers,
drill into detail and fan-outs, and report data freshness.

The MLWH read API is provided by the separate upstream
[`wa`](https://github.com/wtsi-hgi/wa) project. This server is a thin,
well-described bridge whose value is making those endpoints ergonomic and
correctly usable by an LLM, not re-implementing them: it imports
`github.com/wtsi-hgi/wa/mlwh` and reuses its typed client, response types,
registry, and OpenAPI document directly, so there is no type drift and the
server is compile-time-locked to the upstream API version.

## Status

The Go MCP server is the primary artefact of this repository. It is under
active development; see [`.docs/mcp/spec.md`](.docs/mcp/spec.md) for the full
specification and [`.docs/mcp/`](.docs/mcp/) for the phased implementation plan.

This first round ships the MLWH provider over the stdio transport only.
Streamable HTTP is deliberately deferred, but the transport is a clean seam so
it can be added later without core changes.

## Architecture

The server is built to host multiple independent services through a
service-agnostic core and self-contained providers. MLWH is the first provider;
adding another service requires only a new provider package plus its
registration, with no core change.

```
go.mod                  module github.com/wtsi-hgi/llm-knowledge-base
cmd/mcp-server/         CLI entrypoint (flag parsing, wiring only)
internal/core/          service-agnostic core (provider seam, transport, version)
internal/mlwh/          MLWH provider (imports wa/mlwh)
webui/                  Next.js + FastAPI web UI scaffold (future component)
```

## Building

Requires Go 1.25+.

```bash
go build ./cmd/mcp-server
go test ./...
```

The build depends on the private `github.com/wtsi-hgi/wa` module. It is not
published on the public module proxy, so a local checkout is wired in via a
`replace` directive in `go.mod` pointing at the checkout's path. Adjust that
directive to match the location of your `wa` checkout.

## Web UI

A Next.js + FastAPI web UI scaffold lives under [`webui/`](webui/) as a future
component. It is currently a standalone scaffold and is not yet wired to the MCP
server; see [`webui/README.md`](webui/README.md) for its own setup and
developer instructions.

## Licence

See [LICENSE](LICENSE).
