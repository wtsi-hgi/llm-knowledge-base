#!/usr/bin/env bash
# Drive mlwh_provide_feedback end to end against a stack from launch.sh:
#   1. stdio: spawn the built server through the MCP client, tools/call.
#   2. HTTP: tools/call on the running --http server with a custom User-Agent.
#   3. Side effect: wa's admin API and the feedback SQLite DB hold both rows,
#      ids equal to the receipts, metadata as the server attached it.
#   4. Browser: the wa admin /feedback page shows both cards (admin-feedback.mjs).
# Usage: drive-feedback.sh <run-dir>    Exit 0 only when every check passes.
set -uo pipefail

RUN="${1:?usage: drive-feedback.sh <run-dir>}"
# shellcheck disable=SC1091
. "$RUN/stack.env"
# shellcheck disable=SC1091
. "$WA_RUN/stack.env" # STATE_HOME, PGID, FRONTEND_URL (overrides REPO: use WA paths below only)
EV="$RUN/evidence"
fail=0

pass_if() { # <name> <ok-command...>
  local name="$1"
  shift
  if "$@" >/dev/null; then printf 'PASS %s\n' "$name"; else printf 'FAIL %s\n' "$name"; fail=1; fi
}

STAMP="$(date -u +%Y%m%dT%H%M%SZ)-$RANDOM"
STDIO_MARK="verify-mlwh-mcp stdio $STAMP"
HTTP_MARK="verify-mlwh-mcp http $STAMP"
HTTP_UA="verify-mlwh-mcp-agent/9.8 (http check $STAMP)"
WA_API_VERSION="$("$SERVER" --version | sed -n 's/^MLWH API version //p')"

stdio_args="$(jq -nc --arg d "$STDIO_MARK: no tool reports per-lane yield for a study" '{
  category: "could_not_answer", description: $d,
  user_request: "How many gigabases did study 6568 produce per lane?",
  tools_tried: ["mlwh_study_overview", "mlwh_runs_for_study"]}')"
http_args="$(jq -nc --arg d "$HTTP_MARK: there is no endpoint for library prep kit lot numbers" '{
  category: "no_endpoint", description: $d,
  user_request: "Which library prep kit lot was used for study 6568?",
  tools_tried: ["mlwh_libraries_for_study"]}')"

# 1. stdio. The client spawns the server; the server inherits MLWH_BASE_URL.
MLWH_BASE_URL="$MLWH_URL" "$CLIENT" -stdio "$SERVER" -stderr "$EV/30-stdio-server.stderr" \
  -name verify-mlwh-mcp-stdio -version 2.3.4 \
  call mlwh_provide_feedback "$stdio_args" >"$EV/31-stdio-feedback.json" 2>"$EV/31-stdio-client.stderr"
pass_if "stdio: MCP exchange completed" test -s "$EV/31-stdio-feedback.json"
STDIO_ID="$(jq -r '.result.structuredContent.id // empty' "$EV/31-stdio-feedback.json" 2>/dev/null)"
pass_if "stdio: receipt has integer id and created_at, not isError" \
  jq -e '(.result.isError|not) and (.result.structuredContent.id|type=="number") and (.result.structuredContent.created_at|test("Z$"))' "$EV/31-stdio-feedback.json"

# 2. HTTP, with a User-Agent and a handshake name that must NOT be recorded
# (HTTP mode is stateless, so the User-Agent is the identity).
"$CLIENT" -http "$MCP_HTTP_URL" -ua "$HTTP_UA" -name verify-mlwh-mcp-http-handshake -version 7.7.7 \
  call mlwh_provide_feedback "$http_args" >"$EV/32-http-feedback.json" 2>"$EV/32-http-client.stderr"
pass_if "http: MCP exchange completed" test -s "$EV/32-http-feedback.json"
HTTP_ID="$(jq -r '.result.structuredContent.id // empty' "$EV/32-http-feedback.json" 2>/dev/null)"
pass_if "http: receipt has integer id and created_at, not isError" \
  jq -e '(.result.isError|not) and (.result.structuredContent.id|type=="number") and (.result.structuredContent.created_at|test("Z$"))' "$EV/32-http-feedback.json"

# 3a. Admin API (what the /feedback page reads).
curl -s -H "Authorization: Bearer $(cat "$STATE_HOME/.wa-mlwh-server.token")" \
  "$MLWH_URL/feedback?limit=200" >"$EV/33-admin-api-list.json"

row_ok() { # <file> <jq items path> <id> <json-args> <transport> <client_name> <client_version> <ua> <created_at>
  jq -e --argjson id "$3" --argjson in "$4" --arg t "$5" --arg cn "$6" --arg cv "$7" --arg ua "$8" \
    --arg sv "$SERVER_VERSION" --arg wv "$WA_API_VERSION" --arg ca "$9" "
    [$2[] | select(.id == \$id)] | length == 1 and (.[0] |
      .category == \$in.category and .description == \$in.description and
      .user_request == \$in.user_request and .tools_tried == \$in.tools_tried and
      .mcp_server_version == \$sv and .wa_api_version == \$wv and .transport == \$t and
      .client_name == \$cn and .client_version == \$cv and .client_user_agent == \$ua and
      .remote_addr == \"127.0.0.1\" and .created_at == \$ca)" "$1" >/dev/null
}
STDIO_AT="$(jq -r '.result.structuredContent.created_at // empty' "$EV/31-stdio-feedback.json" 2>/dev/null)"
HTTP_AT="$(jq -r '.result.structuredContent.created_at // empty' "$EV/32-http-feedback.json" 2>/dev/null)"
pass_if "admin API: stdio row #${STDIO_ID:-?} matches receipt and metadata (transport stdio, client verify-mlwh-mcp-stdio 2.3.4, no UA)" \
  row_ok "$EV/33-admin-api-list.json" .items "${STDIO_ID:-0}" "$stdio_args" stdio verify-mlwh-mcp-stdio 2.3.4 "" "$STDIO_AT"
pass_if "admin API: http row #${HTTP_ID:-?} matches receipt and metadata (transport http, UA set, no client name)" \
  row_ok "$EV/33-admin-api-list.json" .items "${HTTP_ID:-0}" "$http_args" http "" "" "$HTTP_UA" "$HTTP_AT"

# 3b. The feedback SQLite DB itself, read-only. Path from the running serve's args.
DB="$(ps -o args= -g "$PGID" | grep -o -- '--feedback-db [^ ]*' | head -n 1 | cut -d' ' -f2)"
printf '%s\n' "$DB" >"$EV/34-feedback-db-path.txt"
python3 -I - "$DB" "${STDIO_ID:-0}" "${HTTP_ID:-0}" >"$EV/34-feedback-db-rows.json" <<'EOF'
import json, sqlite3, sys
db, ids = sys.argv[1], [int(x) for x in sys.argv[2:]]
con = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
con.row_factory = sqlite3.Row
rows = [dict(r) for r in con.execute(
    f"SELECT * FROM feedback WHERE id IN ({','.join('?' * len(ids))}) ORDER BY id", ids)]
for r in rows:
    r["tools_tried"] = json.loads(r["tools_tried"])
print(json.dumps({"items": rows}, indent=2))
EOF
pass_if "feedback DB: stdio row matches receipt and metadata" \
  row_ok "$EV/34-feedback-db-rows.json" .items "${STDIO_ID:-0}" "$stdio_args" stdio verify-mlwh-mcp-stdio 2.3.4 "" "$STDIO_AT"
pass_if "feedback DB: http row matches receipt and metadata" \
  row_ok "$EV/34-feedback-db-rows.json" .items "${HTTP_ID:-0}" "$http_args" http "" "" "$HTTP_UA" "$HTTP_AT"

# 4. Browser: the admin sees both cards.
jq -n --argjson s "${STDIO_ID:-0}" --argjson h "${HTTP_ID:-0}" \
  --arg sm "$STDIO_MARK" --arg hm "$HTTP_MARK" --arg ua "$HTTP_UA" \
  --arg sv "$SERVER_VERSION" --arg wv "$WA_API_VERSION" '[
  {id: $s, name: "stdio", contains: [$sm, "Could not answer",
    "How many gigabases did study 6568 produce per lane?", "mlwh_study_overview, mlwh_runs_for_study",
    "Client verify-mlwh-mcp-stdio 2.3.4", "MCP server \($sv)", "wa API \($wv)", "Transport stdio", "From 127.0.0.1"]},
  {id: $h, name: "http", contains: [$hm, "No endpoint",
    "Which library prep kit lot was used for study 6568?", "mlwh_libraries_for_study",
    "Client \($ua)", "MCP server \($sv)", "wa API \($wv)", "Transport http", "From 127.0.0.1"],
    absent: ["verify-mlwh-mcp-http-handshake"]}]' >"$EV/40-expect.json"
node "$SKILL/scripts/admin-feedback.mjs" "$RUN" "$EV/40-expect.json" || fail=1

printf 'stdio id %s, http id %s, server %s, wa API %s\n' "${STDIO_ID:-none}" "${HTTP_ID:-none}" "$SERVER_VERSION" "$WA_API_VERSION"
exit "$fail"
