#!/usr/bin/env bash
# Start the verify-mlwh-mcp stack: a wa test stack through the wa checkout's
# own verify-wa skill (real `wa mlwh serve` with feedback on, fixture cache,
# admin /feedback page), this repo's mlwh-mcp-server and MCP client built into
# the run dir, and one long-running `mlwh-mcp-server --http` pointed at wa.
#
# Usage: launch.sh
# Env:   VERIFY_MLWH_MCP_OUT  (required) evidence root, outside this repo
#        WA_CHECKOUT          wa checkout with .agents/skills/verify-wa
#                             (default: <this repo>/../wa)
#        WA_VERIFY_PORT_BASE  (default 47690) wa uses +1..+4 (+4 is the
#                             spare port for the feedback-off serve), MCP HTTP +5
# Prints RUN=<run dir> as its last line.
set -euo pipefail

REPO="$(cd "$(dirname "$(realpath "$0")")/../../../.." && pwd -P)"
SKILL="$REPO/.agents/skills/verify-mlwh-mcp"
OUT="${VERIFY_MLWH_MCP_OUT:?set VERIFY_MLWH_MCP_OUT to a directory outside the repo (e.g. your scratchpad)}"
WA_CHECKOUT="$(cd "${WA_CHECKOUT:-$REPO/../wa}" && pwd -P)"
WA_SKILL="$WA_CHECKOUT/.agents/skills/verify-wa"
BASE="${WA_VERIFY_PORT_BASE:-47690}"
MCP_HTTP_PORT=$((BASE + 5))

if [[ ! -x "$WA_SKILL/scripts/launch.sh" ]]; then
  printf 'launch.sh: %s has no verify-wa skill; set WA_CHECKOUT to a wa v0.11.0+ checkout that contains .agents/skills/verify-wa\n' "$WA_CHECKOUT" >&2
  exit 1
fi

if ss -ltn "( sport = :$MCP_HTTP_PORT )" | grep -q LISTEN; then
  printf 'launch.sh: port %s is in use; set WA_VERIFY_PORT_BASE to a free range.\n' "$MCP_HTTP_PORT" >&2
  exit 1
fi

RUN="$OUT/verify-mlwh-mcp/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$RUN/evidence" "$RUN/logs" "$RUN/bin"
printf 'launch.sh: run dir %s (pass it to cleanup.sh if launch fails)\n' "$RUN" >&2
cd "$REPO"

# Same version stamping as `make build`.
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
go build -ldflags "-X github.com/wtsi-hgi/llm-knowledge-base/internal/core.ServerVersion=$VERSION" \
  -o "$RUN/bin/mlwh-mcp-server" ./cmd/mlwh-mcp-server
go build -o "$RUN/bin/mcpclient" "$SKILL/scripts/mcpclient.go"

{
  printf 'REVISION=%s\n' "$(git rev-parse HEAD)"
  printf 'STATUS_SHORT=%q\n' "$(git status --short | tr '\n' ';')"
} >"$RUN/revision.txt"

# wa stack. Its scripts act on (and write runtime files under) WA_CHECKOUT/.tmp.
if ! WA_VERIFY_PORT_BASE="$BASE" "$WA_SKILL/scripts/launch.sh" >"$RUN/logs/wa-launch.out" 2>&1; then
  tail -n 30 "$RUN/logs/wa-launch.out" >&2
  printf 'launch.sh: verify-wa launch failed; run cleanup.sh %s\n' "$RUN" >&2
  exit 1
fi
WA_RUN="$(sed -n 's/^RUN=//p' "$RUN/logs/wa-launch.out" | tail -n 1)"
MLWH_URL="$(sed -n 's/^MLWH_URL=//p' "$WA_RUN/stack.env")"

{
  printf 'REPO=%s\n' "$REPO"
  printf 'SKILL=%s\n' "$SKILL"
  printf 'SERVER_VERSION=%s\n' "$VERSION"
  printf 'SERVER=%s\n' "$RUN/bin/mlwh-mcp-server"
  printf 'CLIENT=%s\n' "$RUN/bin/mcpclient"
  printf 'WA_CHECKOUT=%s\n' "$WA_CHECKOUT"
  printf 'WA_SKILL=%s\n' "$WA_SKILL"
  printf 'WA_RUN=%s\n' "$WA_RUN"
  printf 'MLWH_URL=%s\n' "$MLWH_URL"
  printf 'MCP_HTTP_URL=http://127.0.0.1:%s/mcp\n' "$MCP_HTTP_PORT"
  printf 'MCP_HEALTH_URL=http://127.0.0.1:%s/health\n' "$MCP_HTTP_PORT"
} >"$RUN/stack.env"

# Long-running HTTP MCP server, in its own session so cleanup can kill its group.
MLWH_BASE_URL="$MLWH_URL" setsid "$RUN/bin/mlwh-mcp-server" --http "127.0.0.1:$MCP_HTTP_PORT" \
  >"$RUN/logs/mcp-http.log" 2>&1 </dev/null &
printf '%s\n' "$!" >"$RUN/mcp-http.pid"

for _ in $(seq 1 60); do
  if curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$MCP_HTTP_PORT/health"; then
    printf 'RUN=%s\n' "$RUN"
    exit 0
  fi
  if ! kill -0 "$(cat "$RUN/mcp-http.pid")" 2>/dev/null; then
    printf 'launch.sh: mlwh-mcp-server --http exited; see %s\n' "$RUN/logs/mcp-http.log" >&2
    exit 1
  fi
  sleep 0.5
done

printf 'launch.sh: mlwh-mcp-server --http not healthy after 30s; run cleanup.sh %s\n' "$RUN" >&2
exit 1
