# Phase 3: Feedback tool

Ref: [spec.md](spec.md) sections B1, B2, B3, A2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

Sequential after phase 2 is reviewed. The items share
`internal/mlwh/tools_feedback.go` and `internal/mlwh/tools_feedback_test.go`,
so they run sequentially in order.

### Skills

The orchestrator and its subagents must read and follow these skills by
name and at these absolute paths:

- Implementor: `go-implementor`
  (/home/ubuntu/.claude/skills/go-implementor/SKILL.md)
- Reviewer: `go-reviewer`
  (/home/ubuntu/.claude/skills/go-reviewer/SKILL.md)

## Items

### Item 3.1: B1 - Register the tool

spec.md section: B1

Create `internal/mlwh/tools_feedback.go` with `provideFeedbackInput`, the
hand-built `feedbackInputSchema()` (enum and category description derived
from `wa.FeedbackCategories()` and `Description()`, caps from the `wa`
constants), the `outputSchemaFor("FeedbackReceipt")` output schema,
`feedbackDescription`, and `p.registerFeedbackTool(r)`, called from
`Register` before `registerCallTool`. Add `"mlwh_provide_feedback"` to
`fullSurfaceTools` in `internal/mlwh/provider_test.go`. Covers all 7
acceptance tests from B1.

- [ ] implemented
- [ ] reviewed

### Item 3.2: B2 - Submit and return the receipt

spec.md section: B2

Implement the handler and `p.feedbackSubmission(req, in)`, which copies the
agent fields unmodified and attaches `mcp_server_version`, `wa_api_version`,
`transport`, `client_name`, `client_version`, and `client_user_agent`, each
truncated to `wa.FeedbackMaxMetaBytes` at a UTF-8 rune boundary. Call
`wa.RemoteClient.SubmitFeedback` and return the `wa.FeedbackReceipt`. Add the
HTTP-mode User-Agent tests to `internal/mlwh/http_test.go` and the rest to
`internal/mlwh/tools_feedback_test.go`, using the harness fields from item
2.2. Covers B2 acceptance tests 1-5, 7, and 8, the unmodified-body assertion
of test 6, and A2 acceptance tests 1 and 2 (default and `WithServerInfo`
metadata in the stub body). The `feedback_too_large: ` assertion of B2 test 6
needs `mapFeedbackError`, so it is written in item 3.3. Depends on items 2.1,
2.2, and 3.1.

- [ ] implemented
- [ ] reviewed

### Item 3.3: B3 - Structured errors

spec.md section: B3

Add `mapFeedbackError(err error) error`, returning nil for nil and otherwise
an error with the message `<code>: <detail> (<next step>)`, checking
sentinels with `errors.Is` in the order of the spec's Structured tool errors
table. Wire it into the handler from item 3.2, which returns the mapped error
through `core.ToolError`. Covers all 8 acceptance tests from B3, including the
cancellation test that mirrors `TestAvailabilityToolsF3Cancellation` F3.2,
plus the `feedback_too_large: ` assertion of B2 test 6.

- [ ] implemented
- [ ] reviewed
