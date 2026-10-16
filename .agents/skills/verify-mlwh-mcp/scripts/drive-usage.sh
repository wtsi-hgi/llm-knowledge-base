#!/usr/bin/env bash
# Drive usage identification end to end against a stack from launch.sh:
#   1. wa's admin GET /usage?window=24h as the baseline.
#   2. stdio: mlwh_freshness with client info verify-mlwh-mcp-usage 3.4.5.
#   3. HTTP: mlwh_call_endpoint Freshness with a custom User-Agent.
#   4. The same GET /usage; every count is checked as an after-before delta,
#      because the wa launch keeps earlier rows.
# Usage: drive-usage.sh <run-dir>    Exit 0 only when every check passes.
set -uo pipefail

RUN="${1:?usage: drive-usage.sh <run-dir>}"
# shellcheck disable=SC1091
. "$RUN/stack.env"
# shellcheck disable=SC1091
. "$WA_RUN/stack.env" # STATE_HOME (overrides REPO: use WA paths below only)
EV="$RUN/evidence"
fail=0

pass_if() { # <name> <ok-command...>
  local name="$1"
  shift
  if "$@" >/dev/null; then printf 'PASS %s\n' "$name"; else printf 'FAIL %s\n' "$name"; fail=1; fi
}

STAMP="$(date -u +%Y%m%dT%H%M%SZ)-$RANDOM"
STDIO_AGENT="verify-mlwh-mcp-usage/3.4.5"
HTTP_UA="verify-mlwh-mcp-usage-agent/7.6 ($STAMP)"
OS_USER="$(id -un)"

usage() { # <evidence file>
  curl -s -H "Authorization: Bearer $(cat "$STATE_HOME/.wa-mlwh-server.token")" \
    "$MLWH_URL/usage?window=24h" >"$1"
}

# grew_by <n> <jq expr on one summary>: after minus before equals n. A missing
# entry counts as 0, so a value new in this run still compares.
grew_by() {
  jq -e -n --argjson n "$1" --arg u "$OS_USER" --arg sa "$STDIO_AGENT" --arg ua "$HTTP_UA" \
    --slurpfile b "$EV/60-usage-before.json" --slurpfile a "$EV/63-usage-after.json" \
    "def v: ($2) // 0; (\$a[0] | v) - (\$b[0] | v) == \$n"
}

# 1. Baseline.
usage "$EV/60-usage-before.json"
pass_if "usage: baseline summary read with the admin token" jq -e '.categories | type == "array"' "$EV/60-usage-before.json"

# 2. stdio. The client spawns the server; the server inherits MLWH_BASE_URL.
MLWH_BASE_URL="$MLWH_URL" "$CLIENT" -stdio "$SERVER" -stderr "$EV/61-stdio-server.stderr" \
  -name verify-mlwh-mcp-usage -version 3.4.5 \
  call mlwh_freshness '{}' >"$EV/61-stdio-freshness.json" 2>"$EV/61-stdio-client.stderr"
pass_if "stdio: mlwh_freshness returned without isError" \
  jq -e '.result and (.result.isError | not)' "$EV/61-stdio-freshness.json"

# 3. HTTP. Stateless HTTP has no handshake identity, so the User-Agent is the agent.
"$CLIENT" -http "$MCP_HTTP_URL" -ua "$HTTP_UA" \
  call mlwh_call_endpoint '{"method":"Freshness"}' >"$EV/62-http-call-endpoint.json" 2>"$EV/62-http-client.stderr"
pass_if "http: mlwh_call_endpoint Freshness returned without isError" \
  jq -e '.result and (.result.isError | not)' "$EV/62-http-call-endpoint.json"

# 4. After, then deltas.
usage "$EV/63-usage-after.json"
pass_if "usage: after summary read with the admin token" jq -e '.categories | type == "array"' "$EV/63-usage-after.json"

pass_if "usage: categories mcp requests +2" grew_by 2 '.categories[] | select(.category == "mcp") | .requests'
pass_if "usage: categories mcp sessions +2" grew_by 2 '.categories[] | select(.category == "mcp") | .sessions'
pass_if "usage: users $OS_USER by_category.mcp +1 (stdio only; HTTP sends no username)" \
  grew_by 1 '[.users[] | select(.name == $u and (.by_ip | not)) | .by_category.mcp // 0] | add'
pass_if "usage: mcp_tools mlwh_freshness +1" grew_by 1 '.mcp_tools[] | select(.value == "mlwh_freshness") | .requests'
pass_if "usage: mcp_tools mlwh_call_endpoint +1" grew_by 1 '.mcp_tools[] | select(.value == "mlwh_call_endpoint") | .requests'
pass_if "usage: mcp_agents $STDIO_AGENT +1" grew_by 1 '.mcp_agents[] | select(.value == $sa) | .requests'
pass_if "usage: mcp_agents $HTTP_UA +1" grew_by 1 '.mcp_agents[] | select(.value == $ua) | .requests'
pass_if "usage: endpoints GET /freshness by_category.mcp +2" \
  grew_by 2 '.endpoints[] | select(.method == "GET" and .route == "/freshness") | .by_category.mcp'

printf 'server %s, user %s, http agent %s\n' "$SERVER_VERSION" "$OS_USER" "$HTTP_UA"
exit "$fail"
