---
name: verify-mlwh-mcp
description: "Verify user-visible mlwh-mcp-server changes end to end: launch a real wa mlwh serve test stack, drive the server over stdio and streamable HTTP with a real MCP client, and capture evidence for mlwh_provide_feedback (through to wa's admin /feedback page), tool and resource listing, and read tools. Use when proving an MCP server change works or reproducing an MCP server bug against a real wa."
---

# Verify mlwh-mcp-server

An agent host (Claude Code, Codex) either spawns `mlwh-mcp-server` over stdio
or connects to a shared `mlwh-mcp-server --http <addr>` at `/mcp`. Both talk
to `wa mlwh serve` at `MLWH_BASE_URL`. This skill drives exactly that: a
go-sdk MCP client (`scripts/mcpclient.go`) against the binary built from this
checkout, with a real wa behind it.

The wa side comes from the wa repo's own **verify-wa** skill: `run-dev.sh
--mode test` with a real `wa mlwh serve` on a seeded fixture cache, a
throwaway feedback DB, and the Next.js frontend with the admin `/feedback`
page. It needs no MLWH database, LDAP, or network.

Pick features from [features/README.md](features/README.md).

## Requirements

- A wa checkout at v0.11.0+ (agent feedback and usage metrics) that contains
  `.agents/skills/verify-wa/`. Pass it as `WA_CHECKOUT`; the default is
  `<this repo>/../wa`. verify-wa's scripts act on, and write runtime files
  under, that checkout's gitignored `.tmp/`. Read its `SKILL.md` for the wa
  stack's login, fixtures, and gotchas.
- Go, node, pnpm, `jq`, `python3`, `curl`, `ss`. verify-wa installs
  `frontend/node_modules` (Playwright included) in the wa checkout if missing.

## Setup For Every Command

Shell variables do not persist between tool calls. Start each command with:

```bash
SKILL=<this repo>/.agents/skills/verify-mlwh-mcp
RUN=<run dir printed by launch.sh>
```

## Launch

```bash
VERIFY_MLWH_MCP_OUT=<scratchpad dir> WA_CHECKOUT=<wa checkout> "$SKILL/scripts/launch.sh"
# last line: RUN=<VERIFY_MLWH_MCP_OUT>/verify-mlwh-mcp/<UTC stamp>
```

- `VERIFY_MLWH_MCP_OUT` is required. This repo does not gitignore `.tmp/`, so
  evidence lives outside the repo: point it at the harness scratchpad.
- Ports: `WA_VERIFY_PORT_BASE` (default 47690). wa takes +1 frontend,
  +2 results, +3 MLWH serve, +4 spare (the feedback-off serve); the HTTP MCP
  server takes +5. Both launch scripts refuse a busy port; choose another
  base rather than stopping someone else's server. One wa stack per wa
  checkout (test mode shares its `.tmp/state-test`).
- It builds `$RUN/bin/mlwh-mcp-server` with `make build`'s version stamping
  (`git describe --tags --always --dirty`) and `$RUN/bin/mcpclient`, records
  `REVISION` and `STATUS_SHORT` (`git status --short`) in
  `$RUN/revision.txt`, launches verify-wa (about a minute on first run), then
  starts `mlwh-mcp-server --http 127.0.0.1:<base+5>` against wa's MLWH serve.
- Ready means exit 0 and the `RUN=` line. On failure it prints its run dir
  first; run Cleanup on it, fix, relaunch.
- Everything else is in `$RUN/stack.env` (`SERVER`, `CLIENT`, `MLWH_URL`,
  `MCP_HTTP_URL`, `WA_RUN`, ...). Logs: `$RUN/logs/`.

## Doctor

```bash
"$SKILL/scripts/doctor.sh" "$RUN"   # every line "ok", exit 0
```

It runs verify-wa's doctor (feedback on, frontend up, tokens present), then
checks the built binary is this checkout's HEAD and reports the stamped
version, and the HTTP server is alive, healthy, and targeting `MLWH_URL`. Run
it before the first drive and after any failed drive.

## Drive

Each feature file has its recipe. All drive scripts print `PASS`/`FAIL` per
check and exit 0 only when every check passes.

The MCP client, for ad hoc calls (one JSON object on stdout; exit 1 only on
transport failure, so check `.result.isError`):

```bash
. "$RUN/stack.env"
MLWH_BASE_URL="$MLWH_URL" "$CLIENT" -stdio "$SERVER" -stderr "$RUN/evidence/x.stderr" list
"$CLIENT" -http "$MCP_HTTP_URL" -ua "my-agent/1.0" call mlwh_freshness '{}'
```

`-name`/`-version` set the initialize client info (stdio records it on
feedback). `mcpclient.go` is `//go:build ignore` under a dot directory, so
`go build ./...`, `go vet ./...`, and golangci-lint never see it; build it
only by file path from the repo root, as launch.sh does, so it shares this
repo's go-sdk version.

## Evidence

Everything lands in `$RUN/evidence/`, numbered by drive (`2x` basics,
`3x`/`4x` feedback, `5x` feedback-off). It survives Cleanup; leave it
uncommitted. wa's test-mode feedback DB is deleted on shutdown, so the drive
scripts copy rows out before Cleanup.

Record per check: expected, observed, tested revision (`$RUN/revision.txt`),
verdict, and evidence paths.

## Cleanup

```bash
"$SKILL/scripts/cleanup.sh" "$RUN"  # exit 0 = ports free, groups gone
```

It TERMs each recorded `$RUN/*.pid` process group (each was started with
`setsid`), then runs verify-wa's cleanup on `WA_RUN`. Run it after every
failed launch or drive too. Pre-existing `mlwh-mcp-server` processes from
agent hosts on this machine are not yours; leave them alone.

## Proof Standards

- Drive the user path: the real binary, a real MCP client, real wa routes,
  and the admin page through the account menu. In-memory transports, stubs,
  and direct DB writes prove fixtures, not features.
- Check side effects beside responses: the stored feedback row (admin API and
  the SQLite file, read-only), wa's access log for request counts.
- Verdict is VERIFIED, NOT VERIFIED (product failure), or INCONCLUSIVE
  (environment or access). Inconclusive is not a pass.

## Other Harnesses

`make test` runs the hermetic Go suite with stub upstreams, and is the
regression gate, not user-path proof. `webui/` is an unrelated Next.js +
FastAPI greeting scaffold with no MLWH or MCP behaviour; it is not mapped.
