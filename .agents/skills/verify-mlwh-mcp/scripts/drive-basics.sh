#!/usr/bin/env bash
# Drive the read surface an agent host sees first, over both transports:
# initialize + tools/list + resources/list + resources/read, then the read
# tools mlwh_freshness and mlwh_classify_identifier against wa's fixture cache.
# Usage: drive-basics.sh <run-dir>    Exit 0 only when every check passes.
set -uo pipefail

RUN="${1:?usage: drive-basics.sh <run-dir>}"
# shellcheck disable=SC1091
. "$RUN/stack.env"
EV="$RUN/evidence"
fail=0

pass_if() {
  local name="$1"
  shift
  if "$@" >/dev/null; then printf 'PASS %s\n' "$name"; else printf 'FAIL %s\n' "$name"; fail=1; fi
}

stdio() { # <evidence-name> <client args...>
  local name="$1"
  shift
  MLWH_BASE_URL="$MLWH_URL" "$CLIENT" -stdio "$SERVER" -stderr "$EV/$name.server.stderr" "$@" \
    >"$EV/$name.json" 2>"$EV/$name.client.stderr"
}
http() {
  local name="$1"
  shift
  "$CLIENT" -http "$MCP_HTTP_URL" "$@" >"$EV/$name.json" 2>"$EV/$name.client.stderr"
}

curl -s "$MLWH_URL/freshness" >"$EV/20-wa-freshness-direct.json"

for t in stdio http; do
  "$t" "21-$t-list" list
  pass_if "$t: initialize names server mlwh-mcp-server $SERVER_VERSION" \
    jq -e --arg v "$SERVER_VERSION" '.server.version == $v' "$EV/21-$t-list.json"
  pass_if "$t: tools/list has mlwh_provide_feedback, mlwh_freshness, mlwh_classify_identifier, mlwh_call_endpoint" \
    jq -e '.tools as $t | ["mlwh_provide_feedback","mlwh_freshness","mlwh_classify_identifier","mlwh_call_endpoint"] | all(. as $n | $t | index($n))' "$EV/21-$t-list.json"
  pass_if "$t: mlwh://workflow resource is non-empty and lists mlwh_freshness" \
    jq -e '.resources["mlwh://workflow"] | length > 0 and contains("mlwh_freshness")' "$EV/21-$t-list.json"
  pass_if "$t: mcp-server://version resource names $SERVER_VERSION" \
    jq -e --arg v "$SERVER_VERSION" '.resources["mcp-server://version"] | contains($v)' "$EV/21-$t-list.json"

  "$t" "22-$t-freshness" call mlwh_freshness '{}'
  pass_if "$t: mlwh_freshness succeeds and its tables equal wa's own /freshness" \
    jq -e --slurpfile wa "$EV/20-wa-freshness-direct.json" \
    '(.result.isError|not) and (.result.structuredContent.tables | length > 0) and (.result.structuredContent.tables | sort_by(.table)) == ($wa[0].tables | sort_by(.table))' "$EV/22-$t-freshness.json"

  "$t" "23-$t-classify" call mlwh_classify_identifier '{"identifier":"6568"}'
  pass_if "$t: mlwh_classify_identifier 6568 is a study_lims_id" \
    jq -e '(.result.isError|not) and (.result.structuredContent.kind == "study_lims_id")' "$EV/23-$t-classify.json"
done

pass_if "stdio server logged its startup version line" grep -q "$SERVER_VERSION" "$EV/21-stdio-list.server.stderr"
exit "$fail"
