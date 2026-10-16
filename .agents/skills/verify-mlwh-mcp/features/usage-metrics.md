# Usage Identification

## What It Is

Every request the server makes to wa during a `tools/call` carries usage
identity, so wa's usage summary attributes MCP traffic: a User-Agent naming
`mlwh-mcp-server` and its version, plus `X-WA-MCP-Transport`,
`X-WA-Client-ID`, `X-WA-MCP-Agent`, `X-WA-MCP-Tool`, and, over stdio only,
`X-WA-Client-User` (the OS username). The agent is the stdio handshake's
client name/version, or the HTTP User-Agent in HTTP mode. wa records them
when run with `--usage-db`; admins read the aggregates on wa's `/usage` page
and admin API. The header table is in the repo README's `Usage metrics`
section. Contract: `.docs/usage/spec.md`.

## How To Reach It

- stdio: the agent host spawns `mlwh-mcp-server` and calls any tool.
- HTTP: the host calls any tool on `http://<addr>/mcp` of
  `mlwh-mcp-server --http`.
- Admin view: `GET $MLWH_URL/usage?window=24h` with
  `Authorization: Bearer $(cat "$STATE_HOME/.wa-mlwh-server.token")`
  (`STATE_HOME` from verify-wa's `stack.env`), or the wa frontend's account
  menu, `Usage`.

## How To Drive It

`"$SKILL/scripts/drive-usage.sh" "$RUN"`. Each run uses a fresh HTTP
User-Agent stamp, and every count is an after-minus-before delta, so reruns
on one stack pass.

1. Baseline `GET /usage?window=24h`. Evidence: `60-usage-before.json`.
2. stdio `mlwh_freshness {}` with client info `verify-mlwh-mcp-usage 3.4.5`:
   not `isError`. Evidence: `61-stdio-freshness.json`, server stderr
   `61-stdio-server.stderr`.
3. HTTP `mlwh_call_endpoint {"method":"Freshness"}` with
   `User-Agent: verify-mlwh-mcp-usage-agent/7.6 (<stamp>)`: not `isError`.
   Evidence: `62-http-call-endpoint.json`.
4. The same `GET /usage`. Evidence: `63-usage-after.json`. Expected deltas:
    - `categories` entry `mcp`: `requests` +2, `sessions` +2 (one client ID
      per stdio process, one per HTTP session).
    - `users` entry `$(id -un)`: `by_category.mcp` +1, from stdio only.
    - `mcp_tools`: `mlwh_freshness` +1, `mlwh_call_endpoint` +1.
    - `mcp_agents`: `verify-mlwh-mcp-usage/3.4.5` +1 and the full HTTP
      User-Agent +1.
    - `endpoints` entry `GET` `/freshness`: `by_category.mcp` +2.

For the admin page itself, follow verify-wa's `usage-admin-page.md` recipe
on the same stack (`$WA_RUN`).

## Gotchas

- The stack must be wa `v0.11.0`+ with usage on. verify-wa test mode
  enables it, and its doctor checks `GET /usage` is 401 without a token, not
  503.
- Counts are compared as before/after deltas because the wa launch keeps
  earlier rows: the doctor's `curl /freshness` (category `other`) and every
  earlier drive on the stack are already counted.
- wa caps `mcp_agents` at 25 entries. On a long-lived stack the recipe's
  agent can fall off the list after many feedback drives, each of which adds
  agents. Test mode uses a fresh usage DB per launch, so it normally does not
  bite.
- In HTTP mode the agent is the User-Agent (`-ua`), not the `-name`
  handshake. `mcpclient` makes one call per process, so per-process stdio
  client ID stability cannot be shown with it.
- The usage DB path is not in `stack.env`: it is the `--usage-db` argument of
  the running `mlwh serve`. wa deletes it on shutdown, so read it (read-only)
  before Cleanup.
