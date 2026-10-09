/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
 *
 * Author: Sendu Bala <sb10@sanger.ac.uk>
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be included
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 ******************************************************************************/

package mlwh

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

// feedbackDescription is the LLM-facing description for mlwh_provide_feedback.
// It tells the agent to report problems on its own initiative, at most once per
// problem, to tell the user afterwards, what each input field holds, what the
// server attaches, and what a success returns.
const feedbackDescription = "Report a problem with the current user request to the MLWH server admin. " +
	"Call this on your own initiative, without asking the user first, whenever one of these categories applies: " +
	"could_not_answer (you could not work out how to answer the request), " +
	"agent_mistake (you made a mistake while answering), " +
	"no_endpoint (no endpoint could possibly answer the question), " +
	"user_unhappy (the user was unhappy with the answer), " +
	"other (any other problem with the request). " +
	"Submit at most one report per distinct problem, and never resubmit a report that succeeded. " +
	"After it succeeds, tell the user briefly that you reported the problem to the server admin. " +
	"description is your own account of what went wrong. user_request is the user's original request " +
	"verbatim (optional). tools_tried lists the MCP tools you used (optional). " +
	"The server attaches its version, the MLWH API version, the transport, and the agent application's " +
	"name/version or User-Agent; wa records this MCP server's network address, not the user's. " +
	"Returns the report's id and created_at (UTC RFC3339). On failure, follow the error's next step."

const (
	feedbackUnavailableCode     = "feedback_unavailable"
	feedbackUnavailableNextStep = "the wa server could not be reached or failed; " +
		"tell the user it could not be reported; do not retry repeatedly"
)

// feedbackErrorCode pairs a wa feedback sentinel with the stable code and next
// step that mapFeedbackError reports for it.
type feedbackErrorCode struct {
	sentinel error
	code     string
	nextStep string
}

// feedbackErrorCodes lists the feedback sentinels in mapping-precedence order.
// Many feedback errors also wrap wa.ErrUpstreamImpaired, so the feedback
// sentinels are checked first and anything else is feedback_unavailable.
var feedbackErrorCodes = []feedbackErrorCode{
	{
		sentinel: wa.ErrFeedbackDisabled,
		code:     "feedback_disabled",
		nextStep: "feedback is disabled on the wa server; tell the user it could not be reported; do not retry",
	},
	{
		sentinel: wa.ErrFeedbackUnsupported,
		code:     "feedback_unsupported",
		nextStep: "the wa server has no feedback endpoint at this URL: it predates MLWH API 1.9.0 or is secured; " +
			"tell the user; do not retry",
	},
	{
		sentinel: wa.ErrFeedbackUnauthorized,
		code:     "feedback_unauthorized",
		nextStep: "the wa server requires authentication, which this MCP server does not support; " +
			"tell the user; do not retry",
	},
	{
		sentinel: wa.ErrFeedbackTooLarge,
		code:     "feedback_too_large",
		nextStep: "shorten description, user_request, or tools_tried and resubmit once",
	},
	{
		sentinel: wa.ErrFeedbackInvalid,
		code:     "feedback_invalid",
		nextStep: "fix category or description and resubmit once",
	},
}

// feedbackInputSchema describes the mlwh_provide_feedback input. The category
// enum and its per-value description come from wa.FeedbackCategories, and the
// field descriptions state wa's caps, so the SDK rejects an unknown category or
// a missing description before the handler runs.
func feedbackInputSchema() map[string]any {
	categories := wa.FeedbackCategories()
	values := make([]any, len(categories))
	meanings := make([]string, len(categories))

	for i, category := range categories {
		values[i] = string(category)
		meanings[i] = string(category) + ": " + category.Description()
	}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"category", "description"},
		"properties": map[string]any{
			"category": map[string]any{
				"type":        "string",
				"enum":        values,
				"description": "the problem category; one of " + strings.Join(meanings, "; "),
			},
			"description": map[string]any{
				"type": "string",
				"description": fmt.Sprintf("your own account of what went wrong (at most %d bytes)",
					wa.FeedbackMaxDescriptionBytes),
			},
			"user_request": map[string]any{
				"type": "string",
				"description": fmt.Sprintf("optional: the user's original request, verbatim in the user's own words "+
					"(at most %d bytes)", wa.FeedbackMaxUserRequestBytes),
			},
			"tools_tried": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
				"description": fmt.Sprintf("optional: the MCP tools you used (at most %d items of at most %d bytes each)",
					wa.FeedbackMaxToolsTried, wa.FeedbackMaxToolNameBytes),
			},
		},
	}
}

// truncateFeedbackMeta returns s as valid UTF-8 cut to at most
// wa.FeedbackMaxMetaBytes bytes. Each run of invalid bytes (such as Latin-1
// obs-text in an HTTP User-Agent) first becomes one U+FFFD, as JSON encoding
// would otherwise expand every invalid byte to a 3-byte U+FFFD after the cut.
// The cut then backs off to the start of a rune so a multi-byte rune is never
// split, so the string wa decodes is never over its cap.
func truncateFeedbackMeta(s string) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	if len(s) <= wa.FeedbackMaxMetaBytes {
		return s
	}

	end := wa.FeedbackMaxMetaBytes
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}

	return s[:end]
}

// mapFeedbackError converts a SubmitFeedback error into the tool error
// "<code>: <detail> (<next step>)", where detail is the upstream error text
// and code and next step come from the first matching feedback sentinel, or
// feedback_unavailable when none matches. It returns nil for nil. The result
// still wraps err, so errors.Is sees the original sentinels and context errors.
func mapFeedbackError(err error) error {
	if err == nil {
		return nil
	}

	code, nextStep := feedbackUnavailableCode, feedbackUnavailableNextStep

	for _, mapping := range feedbackErrorCodes {
		if errors.Is(err, mapping.sentinel) {
			code, nextStep = mapping.code, mapping.nextStep

			break
		}
	}

	return fmt.Errorf("%s: %w (%s)", code, err, nextStep)
}

// registerFeedbackTool adds the mlwh_provide_feedback tool (Story B1) to the
// server through the Registrar, with the hand-built input schema and the
// OpenAPI-sourced FeedbackReceipt output schema. Building the output schema
// fails only on a programming error, so such a failure is a registration error.
func (p *provider) registerFeedbackTool(r core.Registrar) error {
	outputSchema, err := outputSchemaFor("FeedbackReceipt")
	if err != nil {
		return fmt.Errorf("mlwh: build feedback output schema: %w", err)
	}

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_provide_feedback",
		Description:  feedbackDescription,
		InputSchema:  feedbackInputSchema(),
		OutputSchema: outputSchema,
	}, p.provideFeedback)

	return nil
}

// provideFeedbackInput is the typed agent input for mlwh_provide_feedback. Its
// schema is hand-built by feedbackInputSchema so the category enum and caps
// come from wa.
type provideFeedbackInput struct {
	Category    string   `json:"category"`
	Description string   `json:"description"`
	UserRequest string   `json:"user_request,omitempty"`
	ToolsTried  []string `json:"tools_tried,omitempty"`
}

// provideFeedback is the mlwh_provide_feedback handler (Story B2). It forwards
// one report, with the server's metadata attached, to wa's /feedback route and
// returns wa's receipt. It makes exactly one upstream call and never retries.
func (p *provider) provideFeedback(
	ctx context.Context,
	req *mcp.CallToolRequest,
	in provideFeedbackInput,
) (*mcp.CallToolResult, wa.FeedbackReceipt, error) {
	receipt, err := p.client.SubmitFeedback(ctx, p.feedbackSubmission(req, in))
	if err != nil {
		return core.ToolError[wa.FeedbackReceipt](mapFeedbackError(err))
	}

	return nil, receipt, nil
}

// feedbackSubmission copies in and attaches the server's metadata.
//
// The agent fields pass through unmodified, so wa alone enforces their caps.
// The attached server version, MLWH API version, transport, handshake client
// name/version (stdio), and HTTP User-Agent are each truncated to
// wa.FeedbackMaxMetaBytes after invalid UTF-8 is replaced, so the server's own
// context never causes a 413. A request without a session, initialize params,
// client info, or HTTP extras leaves the matching client fields empty.
func (p *provider) feedbackSubmission(req *mcp.CallToolRequest, in provideFeedbackInput) wa.FeedbackSubmission {
	sub := wa.FeedbackSubmission{
		Category:         wa.FeedbackCategory(in.Category),
		Description:      in.Description,
		UserRequest:      in.UserRequest,
		ToolsTried:       in.ToolsTried,
		MCPServerVersion: truncateFeedbackMeta(p.serverVersion),
		WAAPIVersion:     truncateFeedbackMeta(wa.APIVersion),
		Transport:        truncateFeedbackMeta(p.transport),
	}

	if req.Session != nil {
		if params := req.Session.InitializeParams(); params != nil && params.ClientInfo != nil {
			sub.ClientName = truncateFeedbackMeta(params.ClientInfo.Name)
			sub.ClientVersion = truncateFeedbackMeta(params.ClientInfo.Version)
		}
	}

	if req.Extra != nil && req.Extra.Header != nil {
		sub.ClientUserAgent = truncateFeedbackMeta(req.Extra.Header.Get("User-Agent"))
	}

	return sub
}
