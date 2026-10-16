# Phase 6: Verify recipe

Ref: [spec.md](spec.md) sections C3

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phases 4 and 5 are both reviewed. It needs a wa
`v0.11.0` checkout (default `/home/ubuntu/wa`, or `WA_CHECKOUT`) so the
verify-wa test-mode stack runs with usage recording on.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Item 6.1: C3 - Verify recipe

spec.md section: C3

Add `.agents/skills/verify-mlwh-mcp/features/usage-metrics.md` in the
format of `feedback-tool.md`, a `Usage identification` row in
`features/README.md`, the usage summary in the `SKILL.md` description,
and `scripts/drive-usage.sh <run-dir>` following `drive-feedback.sh`
(`pass_if`, `PASS`/`FAIL` lines, exit 0 only when all pass, evidence
numbered `6x`). The `mcpclient` command lines must carry the full
transport flags as `drive-feedback.sh` does: `-stdio "$SERVER"` for the
stdio call and `-http "$MCP_HTTP_URL"` for the HTTP call, alongside the
spec's `-name`/`-version` and `-ua` arguments. Record the spec's
gotchas in `usage-metrics.md`, plus one more: wa caps `mcp_agents` at 25
entries, so on a long-lived stack the recipe's agent can fall off the list
after many feedback drives; test mode uses a fresh usage DB per launch, so
it normally does not bite. Prove the recipe by launching a stack, passing
Doctor, and running `drive-usage.sh` to a clean exit. Covers the 1
acceptance test from C3.

- [x] implemented
- [x] reviewed
