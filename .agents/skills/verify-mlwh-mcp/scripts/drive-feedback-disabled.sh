#!/usr/bin/env bash
# Error path: mlwh_provide_feedback against a wa serve with feedback off.
# Starts the wa run's own binary as `wa mlwh serve` on a fresh never-synced
# cache with no --feedback-db and no WA_MLWH_FEEDBACK_PATH, on wa's SPARE_PORT,
# then calls the tool over stdio with MLWH_BASE_URL pointing at it. Expects one
# tool error "feedback_disabled: <detail> (<next step ... do not retry>)" and,
# in wa's own access log, exactly one POST /feedback.
# Usage: drive-feedback-disabled.sh <run-dir>   Exit 0 only when every check passes.
set -uo pipefail

RUN="${1:?usage: drive-feedback-disabled.sh <run-dir>}"
# shellcheck disable=SC1091
. "$RUN/stack.env"
SPARE_PORT="$(sed -n 's/^SPARE_PORT=//p' "$WA_RUN/stack.env")"
URL="http://127.0.0.1:$SPARE_PORT"
D="$RUN/disabled"
EV="$RUN/evidence"
LOG="$EV/50-wa-serve-feedback-off.log"
mkdir -p "$D/state"
rm -f "$D/cache.sqlite"* # fresh never-synced cache on every run
fail=0

pass_if() {
  local name="$1"
  shift
  if "$@" >/dev/null; then printf 'PASS %s\n' "$name"; else printf 'FAIL %s\n' "$name"; fail=1; fi
}

serve_ready() {
  for _ in $(seq 1 60); do
    curl -s -o /dev/null --max-time 2 "$URL/freshness" && return 0
    kill -0 "$(cat "$RUN/disabled-serve.pid")" 2>/dev/null || return 1
    sleep 0.5
  done
  return 1
}

env -u WA_MLWH_FEEDBACK_PATH -u WA_MLWH_SERVER_TOKEN -u WA_ENV XDG_STATE_HOME="$D/state" \
  setsid "$WA_RUN/wa" mlwh serve --mlwh-cache "$D/cache.sqlite" --port "$SPARE_PORT" \
  >"$LOG" 2>&1 </dev/null &
printf '%s\n' "$!" >"$RUN/disabled-serve.pid"
pass_if "feedback-off wa serve answers on :$SPARE_PORT" serve_ready
pass_if "wa confirms feedback off: POST /feedback is 503 feedback_disabled" \
  bash -c "curl -s -X POST -H 'content-type: application/json' -d '{\"category\":\"other\",\"description\":\"probe\"}' '$URL/feedback' | grep -q '\"code\":\"feedback_disabled\"'"
before="$(grep -c 'POST /feedback ' "$LOG")"

MLWH_BASE_URL="$URL" "$CLIENT" -stdio "$SERVER" -stderr "$EV/52-stdio-server.stderr" \
  call mlwh_provide_feedback '{"category":"other","description":"verify-mlwh-mcp feedback-off probe"}' \
  >"$EV/52-disabled-feedback.json" 2>"$EV/52-client.stderr"
sleep 1 # let any (unexpected) retry land before counting

TEXT="$(jq -r '.result.content[0].text // empty' "$EV/52-disabled-feedback.json" 2>/dev/null)"
printf '%s\n' "$TEXT" >"$EV/52-disabled-error.txt"
grep 'POST /feedback ' "$LOG" | tail -n +"$((before + 1))" >"$EV/53-mcp-upstream-posts.log"
pass_if "tool result isError true" jq -e '.result.isError == true' "$EV/52-disabled-feedback.json"
pass_if "error text is 'feedback_disabled: <wa detail> (<next step ending do not retry>)'" \
  grep -Eq '^feedback_disabled: .*feedback is disabled on this server.* \(feedback is disabled on the wa server; tell the user it could not be reported; do not retry\)$' "$EV/52-disabled-error.txt"
pass_if "MCP server sent exactly one POST /feedback, answered 503 (no retry)" \
  test "$(wc -l <"$EV/53-mcp-upstream-posts.log")" = 1 -a "$(grep -c 'STATUS=503' "$EV/53-mcp-upstream-posts.log")" = 1
pass_if "feedback-off serve created no admin token" test ! -e "$D/state/.wa-mlwh-server.token"

kill -TERM -- "-$(cat "$RUN/disabled-serve.pid")" 2>/dev/null
rm -f "$RUN/disabled-serve.pid"

printf 'error text: %s\n' "$TEXT"
exit "$fail"
