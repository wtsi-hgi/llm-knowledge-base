# Agent Feedback Tool

## What It Is

`mlwh_provide_feedback` lets an agent report a problem with the current user
request (category `could_not_answer`, `agent_mistake`, `no_endpoint`,
`user_unhappy`, `other`, plus description, optional `user_request` and
`tools_tried`). The server attaches its version, the MLWH API version, the
transport, and either the stdio handshake's client name/version or the HTTP
User-Agent, then makes one unauthenticated `POST <MLWH_BASE_URL>/feedback`.
It returns wa's receipt `{id, created_at}`. Errors come back as tool errors
`<code>: <wa detail> (<next step>)`. The wa admin reads reports on the
frontend's `/feedback` page. Contract: `.docs/feedback/spec.md`.

## How To Reach It

- stdio: the agent host spawns `mlwh-mcp-server` with `MLWH_BASE_URL` set.
- HTTP: the host connects to `http://<addr>/mcp` of `mlwh-mcp-server --http`.
- Admin view: wa frontend, account menu, `Feedback`.

## How To Drive It

1. Success on both transports, admin page, stored rows:
   `"$SKILL/scripts/drive-feedback.sh" "$RUN"`. Each run uses fresh markers.
    - stdio call with client info `verify-mlwh-mcp-stdio 2.3.4`: receipt
      with integer id and `created_at` ending `Z`.
      Evidence: `31-stdio-feedback.json`, server stderr `30-*`.
    - HTTP call with `User-Agent: verify-mlwh-mcp-agent/9.8 (...)` and
      handshake name `verify-mlwh-mcp-http-handshake`: same receipt shape.
      Evidence: `32-http-feedback.json`.
    - Admin API and the feedback SQLite file (read-only) each hold one row per
      receipt id, equal `created_at`, every agent field verbatim,
      `mcp_server_version` = stamped version, `wa_api_version` 1.12.0,
      `remote_addr` 127.0.0.1. stdio: `transport stdio`, client name/version
      set, no UA. HTTP: `transport http`, UA set, no client name/version.
      Evidence: `33-admin-api-list.json`, `34-feedback-db-rows.json`,
      `34-feedback-db-path.txt`.
    - Browser, through `admin-feedback.mjs`: log in, account menu, `Feedback`,
      then `article[aria-label="Feedback #<id>"]` per report shows the
      marker, category label (`Could not answer`, `No endpoint`), user
      request, tools, and the line `Client … · MCP server … · wa API … ·
      Transport … · From 127.0.0.1`. The HTTP card shows the UA as Client
      and not the handshake name. Evidence: `40-feedback-page.png`,
      `41-stdio-report.{png,txt}`, `41-http-report.{png,txt}`,
      `admin-feedback-summary.json`.
2. Feedback off: `"$SKILL/scripts/drive-feedback-disabled.sh" "$RUN"`
   starts a feedback-off `wa mlwh serve` on wa's spare port and calls the
   tool over stdio against it. Expect `isError`, text matching
   `feedback_disabled: …feedback is disabled on this server… (feedback is
   disabled on the wa server; tell the user it could not be reported; do not
   retry)`, and exactly one `POST /feedback … STATUS=503` in wa's access
   log. Evidence: `50-wa-serve-feedback-off.log`,
   `52-disabled-feedback.json`, `52-disabled-error.txt`,
   `53-mcp-upstream-posts.log`.

Other error codes (`feedback_unsupported`, `feedback_unauthorized`,
`feedback_unavailable`) need a pre-1.9.0 or secured wa, or no wa at all: point
`MLWH_BASE_URL` at it and use `mcpclient call` the same way. A closed port
gives `feedback_unavailable` with no wa needed.

## Gotchas

- wa's `/feedback` admin API and page need the run-dev stack's admin token
  (`$STATE_HOME/.wa-mlwh-server.token` in verify-wa's `stack.env`).
- Ids keep counting within one wa launch. Find reports by receipt id, never
  by position.
- The error detail comes from the wa module's `RemoteClient`, not this
  server. Since wa v0.10.1 it states `feedback is disabled on this server`
  once and no feedback error mentions `upstream database impaired`; a
  repeated sentinel means an older wa module is required in `go.mod`.
- `remote_addr` is the MCP server's address, so always 127.0.0.1 here.
- A feedback-off serve that dies silently: see verify-wa's
  `mlwh-feedback-submit.md` gotchas (exit 1 with empty stderr).
