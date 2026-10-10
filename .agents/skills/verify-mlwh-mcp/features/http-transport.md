# Streamable HTTP Transport

## What It Is

An admin runs one shared `mlwh-mcp-server --http <addr>` (or
`MLWH_HTTP_ADDR`); agent hosts connect by URL to `/mcp`. `/health` answers
for monitors. The listener is unauthenticated plain HTTP and stateless: no
session handshake persists between requests, so feedback identifies the
agent by User-Agent instead of client info.

## How To Reach It

launch.sh starts it on `127.0.0.1:<base+5>`; `MCP_HTTP_URL` and
`MCP_HEALTH_URL` are in `stack.env`. Its log is `$RUN/logs/mcp-http.log`.

## How To Drive It

- Doctor checks `/health` is 200 and the process's `MLWH_BASE_URL`.
- `drive-basics.sh` repeats every list and read-tool check over HTTP
  (`21-http-*`, `22-http-*`, `23-http-*`).
- `drive-feedback.sh` step 2 proves the HTTP identity path: the stored row
  and admin card carry the custom User-Agent and `transport http`, and not
  the handshake client name.
- Ad hoc: `"$CLIENT" -http "$MCP_HTTP_URL" -ua <ua> call <tool> '<json>'`.

## Gotchas

- Without `-ua`, Go's client sends `Go-http-client/1.1`, and that is what
  gets recorded.
- The server is started with `setsid`; stop it with Cleanup, which kills its
  recorded process group.
