#!/usr/bin/env bash
# Stop everything a launch.sh run started, by recorded PID/process group only:
# the HTTP MCP server, any feedback-off wa serve left by an interrupted
# drive-feedback-disabled.sh, then the wa stack through verify-wa's cleanup.sh.
# Evidence under <run-dir>/evidence (and the wa run's evidence) is kept.
# Usage: cleanup.sh <run-dir>
set -uo pipefail

RUN="${1:?usage: cleanup.sh <run-dir>}"
leftover=0

for pidfile in "$RUN"/*.pid; do
  [[ -s "$pidfile" ]] || continue
  pid="$(cat "$pidfile")"
  # Each was started with setsid, so its pid is its process group.
  kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
  for _ in $(seq 1 20); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.25
  done
  if kill -0 "$pid" 2>/dev/null; then
    kill -KILL -- "-$pid" 2>/dev/null || kill -KILL "$pid" 2>/dev/null
  fi
  rm -f "$pidfile"
done

# The wa run dir, from stack.env or, after a failed launch, its launch output.
WA_RUN=""
WA_SKILL=""
if [[ -s "$RUN/stack.env" ]]; then
  # shellcheck disable=SC1091
  . "$RUN/stack.env"
elif [[ -s "$RUN/logs/wa-launch.out" ]]; then
  WA_RUN="$(sed -n 's/^RUN=//p' "$RUN/logs/wa-launch.out" | tail -n 1)"
  WA_SKILL="$(cd "${WA_CHECKOUT:-$(dirname "$(realpath "$0")")/../../../../../wa}" && pwd -P)/.agents/skills/verify-wa"
fi
if [[ -n "$WA_RUN" ]]; then
  "$WA_SKILL/scripts/cleanup.sh" "$WA_RUN" || leftover=1
else
  printf 'cleanup.sh: no wa run recorded; if verify-wa launch.sh printed a run dir, run its cleanup.sh on it\n' >&2
fi

if [[ -n "${MCP_HTTP_URL:-}" ]]; then
  port="$(sed -E 's#.*:([0-9]+)/mcp#\1#' <<<"$MCP_HTTP_URL")"
  if ss -ltn "( sport = :$port )" | grep -q LISTEN; then
    printf 'cleanup.sh: port %s still listening\n' "$port" >&2
    leftover=1
  fi
fi

printf 'evidence kept: %s/evidence (%s files)\n' "$RUN" "$(find "$RUN/evidence" -type f 2>/dev/null | wc -l)"
exit "$leftover"
