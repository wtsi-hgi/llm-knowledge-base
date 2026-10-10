#!/usr/bin/env bash
# Non-mutating check that a stack started by launch.sh is worth driving.
# Usage: doctor.sh <run-dir>    Exit 0 only when every check passes.
set -uo pipefail

RUN="${1:?usage: doctor.sh <run-dir>}"
# shellcheck disable=SC1091
. "$RUN/stack.env"
# shellcheck disable=SC1091
. "$RUN/revision.txt"
fail=0

check() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    printf 'ok   %s\n' "$name"
  else
    printf 'FAIL %s\n' "$name"
    fail=1
  fi
}

printf -- '-- wa stack (verify-wa doctor)\n'
"$WA_SKILL/scripts/doctor.sh" "$WA_RUN" || fail=1

printf -- '-- mlwh-mcp-server\n'
check "built binary is this checkout's HEAD ($REPO)" test "$(git -C "$REPO" rev-parse HEAD)" = "$REVISION"
check "built binary reports version $SERVER_VERSION" bash -c "'$SERVER' --version | grep -qx 'mlwh-mcp-server version $SERVER_VERSION'"
check "HTTP server pid $(cat "$RUN/mcp-http.pid") alive" kill -0 "$(cat "$RUN/mcp-http.pid")"
check "HTTP /health is 200" test "$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$MCP_HEALTH_URL")" = 200
check "HTTP server targets $MLWH_URL" bash -c "tr '\\0' '\\n' </proc/$(cat "$RUN/mcp-http.pid")/environ | grep -qx 'MLWH_BASE_URL=$MLWH_URL'"

printf 'revision %s status [%s]\n' "$REVISION" "$STATUS_SHORT"
exit "$fail"
