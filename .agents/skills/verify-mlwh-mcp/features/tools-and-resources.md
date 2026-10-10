# Tools And Resources

## What It Is

What an agent host loads on connect: `initialize` (server name
`mlwh-mcp-server` and its stamped version), the curated `mlwh_*` tool list,
and two resources, `mlwh://workflow` (cheap-first guidance plus the endpoint
catalogue) and `mcp-server://version` (server and MLWH API versions).

## How To Reach It

Any MCP session, stdio or HTTP. `mcpclient … list` lists tools and reads
every resource.

## How To Drive It

`"$SKILL/scripts/drive-basics.sh" "$RUN"` runs `list` over both transports.
Expect: `.server.version` = `SERVER_VERSION`; tools include
`mlwh_provide_feedback`, `mlwh_freshness`, `mlwh_classify_identifier`,
`mlwh_call_endpoint`; `mlwh://workflow` non-empty and naming
`mlwh_freshness`; `mcp-server://version` naming the stamped version; the
stdio server's stderr carries the startup version line. Evidence:
`21-stdio-list.json`, `21-http-list.json`, `21-stdio-list.server.stderr`.

## Gotchas

- The tool list comes from wa's Registry at build time, so its size changes
  with the wa module version in `go.mod`. Assert on named tools, not counts.
- The stamped version is `git describe --tags --always --dirty`; untracked
  files do not make it dirty.
