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
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/smartystreets/goconvey/convey"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

const (
	feedbackToolName  = "mlwh_provide_feedback"
	feedbackPath      = "/feedback"
	feedbackCreatedAt = "2026-10-01T12:00:00Z"

	// feedbackPaddedDescription and feedbackPaddedUserRequest carry leading and
	// trailing whitespace that must reach wa unmodified.
	feedbackPaddedDescription = "  No consent endpoint.\n"
	feedbackPaddedUserRequest = "\tWhich samples withdrew consent?  "
)

func TestFeedbackToolB1Registration(t *testing.T) {
	Convey("Given a core server with the MLWH provider over an in-memory client", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		tool, ok := toolByName(t, cs, feedbackToolName)
		So(ok, ShouldBeTrue)

		Convey("B1.1: a tools listing contains mlwh_provide_feedback and every existing tool", func() {
			registered := listToolNames(t, cs)
			So(registered, ShouldContainKey, feedbackToolName)

			var missing []string

			for _, name := range fullSurfaceTools {
				if _, ok := registered[name]; !ok {
					missing = append(missing, name)
				}
			}

			So(missing, ShouldBeEmpty)
		})

		Convey("B1.2: the input schema requires category and description, is closed, enumerates wa's categories in order, and states wa's caps", func() {
			schema, ok := tool.InputSchema.(map[string]any)
			So(ok, ShouldBeTrue)
			So(schema["type"], ShouldEqual, "object")
			So(schema["required"], ShouldResemble, []any{"category", "description"})
			So(schema["additionalProperties"], ShouldEqual, false)

			properties, ok := schema["properties"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(sortedKeys(properties), ShouldResemble, []string{"category", "description", "tools_tried", "user_request"})

			category, ok := properties["category"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(category["enum"], ShouldResemble, feedbackCategoryStrings())

			So(propertyDescription(properties, "description"), ShouldContainSubstring,
				fmt.Sprintf("at most %d bytes", wa.FeedbackMaxDescriptionBytes))

			userRequest := propertyDescription(properties, "user_request")
			So(userRequest, ShouldContainSubstring, "verbatim")
			So(userRequest, ShouldContainSubstring, fmt.Sprintf("at most %d bytes", wa.FeedbackMaxUserRequestBytes))

			So(propertyDescription(properties, "tools_tried"), ShouldContainSubstring,
				fmt.Sprintf("at most %d items of at most %d bytes each",
					wa.FeedbackMaxToolsTried, wa.FeedbackMaxToolNameBytes))
		})

		Convey("B1.3: the category description lists each value with wa's description", func() {
			properties := tool.InputSchema.(map[string]any)["properties"].(map[string]any)
			description, ok := properties["category"].(map[string]any)["description"].(string)
			So(ok, ShouldBeTrue)

			for _, category := range wa.FeedbackCategories() {
				So(description, ShouldContainSubstring, string(category)+": "+category.Description())
			}
		})

		Convey("B1.4: the output schema is an object requiring integer id and string created_at", func() {
			schema, ok := tool.OutputSchema.(map[string]any)
			So(ok, ShouldBeTrue)
			So(schema["type"], ShouldEqual, "object")
			So(schema["required"], ShouldContain, "id")
			So(schema["required"], ShouldContain, "created_at")

			properties, ok := schema["properties"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(properties["id"].(map[string]any)["type"], ShouldEqual, "integer")
			So(properties["created_at"].(map[string]any)["type"], ShouldEqual, "string")
		})

		Convey("B1.5: the tool description gives the when, how often, and after-success guidance", func() {
			So(tool.Description, ShouldContainSubstring, "without asking the user")
			So(tool.Description, ShouldContainSubstring, "at most one report per distinct problem")
			So(tool.Description, ShouldContainSubstring, "never resubmit a report that succeeded")
			So(tool.Description, ShouldContainSubstring, "tell the user briefly")
			So(tool.Description, ShouldContainSubstring, "server admin")

			for _, category := range wa.FeedbackCategories() {
				So(tool.Description, ShouldContainSubstring, string(category))
			}
		})

		Convey("B1.6: a bogus category is a tool error and sends no request", func() {
			res := callTool(t, cs, feedbackToolName, map[string]any{
				"category":    "bogus",
				"description": "could not find the run",
			})

			So(res.IsError, ShouldBeTrue)
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("B1.7: a call missing description is a tool error and sends no request", func() {
			res := callTool(t, cs, feedbackToolName, map[string]any{
				"category": string(wa.FeedbackCategoryOther),
			})

			So(res.IsError, ShouldBeTrue)
			So(stub.requestCount(), ShouldEqual, 0)
		})
	})
}

func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

func feedbackCategoryStrings() []any {
	categories := wa.FeedbackCategories()
	values := make([]any, len(categories))

	for i, category := range categories {
		values[i] = string(category)
	}

	return values
}

// propertyDescription returns the description of the named input schema
// property, or "" if it has none.
func propertyDescription(properties map[string]any, name string) string {
	property, _ := properties[name].(map[string]any)
	description, _ := property["description"].(string)

	return description
}

func TestFeedbackToolB2Submission(t *testing.T) {
	Convey("Given a stub that stores feedback and a core server over an in-memory client", t, func() {
		stub := newStubMLWH(t)
		respondFeedbackReceipt(stub)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("When the tool is called with every agent field", func() {
			res := callTool(t, cs, feedbackToolName, map[string]any{
				"category":     "no_endpoint",
				"description":  feedbackPaddedDescription,
				"user_request": feedbackPaddedUserRequest,
				"tools_tried":  []any{"mlwh_search_samples"},
			})

			Convey("B2.1: it returns the receipt after exactly one JSON POST /feedback", func() {
				So(res.IsError, ShouldBeFalse)

				structured, err := json.Marshal(res.StructuredContent)
				So(err, ShouldBeNil)

				var receipt map[string]any
				So(json.Unmarshal(structured, &receipt), ShouldBeNil)
				So(receipt, ShouldResemble, map[string]any{"id": float64(7), "created_at": feedbackCreatedAt})

				So(stub.requestCount(), ShouldEqual, 1)
				req, ok := stub.lastRequest()
				So(ok, ShouldBeTrue)
				So(req.Method, ShouldEqual, http.MethodPost)
				So(req.Path, ShouldEqual, feedbackPath)
				So(req.Header.Get("Content-Type"), ShouldEqual, "application/json")
			})

			Convey("B2.2: the body carries the agent fields byte for byte and the stdio session metadata", func() {
				body := lastFeedbackBody(t, stub)

				So(body["category"], ShouldEqual, "no_endpoint")
				So(body["description"], ShouldEqual, feedbackPaddedDescription)
				So(body["user_request"], ShouldEqual, feedbackPaddedUserRequest)
				So(body["tools_tried"], ShouldResemble, []any{"mlwh_search_samples"})
				So(body["wa_api_version"], ShouldEqual, wa.APIVersion)
				So(body["transport"], ShouldEqual, "stdio")
				So(body["client_name"], ShouldEqual, "test-client")
				So(body["client_version"], ShouldEqual, "0.0.0")
				So(body, ShouldNotContainKey, "client_user_agent")
			})
		})

		Convey("B2.3: a call with only category and description sends no optional agent keys", func() {
			res := callTool(t, cs, feedbackToolName, map[string]any{
				"category":    "other",
				"description": "Something else went wrong.",
			})
			So(res.IsError, ShouldBeFalse)

			body := lastFeedbackBody(t, stub)
			So(body["category"], ShouldEqual, "other")
			So(body["description"], ShouldEqual, "Something else went wrong.")
			So(body, ShouldNotContainKey, "user_request")
			So(body, ShouldNotContainKey, "tools_tried")
		})

		Convey("B2.6: an oversized description reaches wa unmodified", func() {
			stub.respondError(feedbackPath, http.StatusRequestEntityTooLarge,
				"payload_too_large", "description exceeds 16384 bytes")

			description := strings.Repeat("d", 20000)
			res := callTool(t, cs, feedbackToolName, map[string]any{
				"category":    "agent_mistake",
				"description": description,
			})
			So(res.IsError, ShouldBeTrue)
			So(firstTextContent(res), ShouldStartWith, "feedback_too_large: ")

			body := lastFeedbackBody(t, stub)
			So(body["description"], ShouldEqual, description)
		})

		Convey("B2.6: an oversized user_request and tools_tried reach wa unmodified", func() {
			stub.respondError(feedbackPath, http.StatusRequestEntityTooLarge,
				"payload_too_large", "user_request exceeds 16384 bytes")

			userRequest := strings.Repeat("u", 20000)
			toolsTried := make([]any, wa.FeedbackMaxToolsTried+1)

			for i := range toolsTried {
				toolsTried[i] = fmt.Sprintf("mlwh_tool_%d", i)
			}

			toolsTried[1] = strings.Repeat("t", 200)

			res := callTool(t, cs, feedbackToolName, map[string]any{
				"category":     "agent_mistake",
				"description":  "x",
				"user_request": userRequest,
				"tools_tried":  toolsTried,
			})
			So(res.IsError, ShouldBeTrue)
			So(firstTextContent(res), ShouldStartWith, "feedback_too_large: ")

			body := lastFeedbackBody(t, stub)
			So(body["user_request"], ShouldEqual, userRequest)
			So(body["tools_tried"], ShouldResemble, toolsTried)
		})
	})

	Convey("B2.7: a 300-byte server version is truncated to its first 256 bytes", t, func() {
		stub := newStubMLWH(t)
		respondFeedbackReceipt(stub)

		version := strings.Repeat("v", 300)
		cs, cleanup := runMLWHServerWithClientOptions(t, stub, core.Options{}, WithServerInfo(version, "stdio"))
		defer cleanup()

		submitMinimalFeedback(t, cs)

		So(lastFeedbackBody(t, stub)["mcp_server_version"], ShouldEqual, version[:wa.FeedbackMaxMetaBytes])
	})

	Convey("B2.7: a server version of exactly 256 bytes passes through unchanged", t, func() {
		stub := newStubMLWH(t)
		respondFeedbackReceipt(stub)

		version := strings.Repeat("v", wa.FeedbackMaxMetaBytes)
		cs, cleanup := runMLWHServerWithClientOptions(t, stub, core.Options{}, WithServerInfo(version, "stdio"))
		defer cleanup()

		submitMinimalFeedback(t, cs)

		So(lastFeedbackBody(t, stub)["mcp_server_version"], ShouldEqual, version)
	})

	Convey("B2.7: a 300-byte transport, client name, and client version are each cut to 256 bytes", t, func() {
		stub := newStubMLWH(t)
		respondFeedbackReceipt(stub)

		transport := strings.Repeat("t", 300)
		name := strings.Repeat("n", 300)
		version := strings.Repeat("c", 300)
		cs, cleanup := runMLWHServerWithClientImplementation(t, stub,
			&mcp.Implementation{Name: name, Version: version}, core.Options{}, WithServerInfo("v", transport))
		defer cleanup()

		submitMinimalFeedback(t, cs)

		body := lastFeedbackBody(t, stub)
		So(body["transport"], ShouldEqual, transport[:wa.FeedbackMaxMetaBytes])
		So(body["client_name"], ShouldEqual, name[:wa.FeedbackMaxMetaBytes])
		So(body["client_version"], ShouldEqual, version[:wa.FeedbackMaxMetaBytes])
	})

	Convey("B2.8: feedbackSubmission tolerates a request without session or HTTP extras", t, func() {
		built, err := New(wa.RemoteConfig{BaseURL: "http://stub.example"})
		So(err, ShouldBeNil)

		p, ok := built.(*provider)
		So(ok, ShouldBeTrue)

		in := provideFeedbackInput{Category: "other", Description: "x"}

		for _, req := range []*mcp.CallToolRequest{{}, {Session: &mcp.ServerSession{}}} {
			var sub wa.FeedbackSubmission

			So(func() { sub = p.feedbackSubmission(req, in) }, ShouldNotPanic)
			So(sub.ClientName, ShouldBeEmpty)
			So(sub.ClientVersion, ShouldBeEmpty)
			So(sub.ClientUserAgent, ShouldBeEmpty)
			So(sub.Category, ShouldEqual, wa.FeedbackCategory("other"))
			So(sub.Description, ShouldEqual, "x")
			So(sub.WAAPIVersion, ShouldEqual, wa.APIVersion)
		}
	})
}

func TestFeedbackToolA2ServerInfo(t *testing.T) {
	Convey("Given a stub that stores feedback", t, func() {
		stub := newStubMLWH(t)
		respondFeedbackReceipt(stub)

		Convey("A2.1: New(cfg) with no options reports core.ServerVersion \"dev\" over stdio", func() {
			original := core.ServerVersion
			core.ServerVersion = "dev"
			defer func() { core.ServerVersion = original }()

			cs, cleanup := runMLWHServerWithClient(t, stub)
			defer cleanup()

			submitMinimalFeedback(t, cs)

			body := lastFeedbackBody(t, stub)
			So(body["mcp_server_version"], ShouldEqual, "dev")
			So(body["transport"], ShouldEqual, "stdio")
		})

		Convey("A2.1: New(cfg) with no options reports a non-default core.ServerVersion", func() {
			original := core.ServerVersion
			core.ServerVersion = "9.9.9-test"
			defer func() { core.ServerVersion = original }()

			cs, cleanup := runMLWHServerWithClient(t, stub)
			defer cleanup()

			submitMinimalFeedback(t, cs)

			body := lastFeedbackBody(t, stub)
			So(body["mcp_server_version"], ShouldEqual, "9.9.9-test")
			So(body["transport"], ShouldEqual, "stdio")
		})

		Convey("A2.2: WithServerInfo sets the reported server version and transport", func() {
			cs, cleanup := runMLWHServerWithClientOptions(t, stub, core.Options{}, WithServerInfo("0.4.0", "http"))
			defer cleanup()

			submitMinimalFeedback(t, cs)

			body := lastFeedbackBody(t, stub)
			So(body["mcp_server_version"], ShouldEqual, "0.4.0")
			So(body["transport"], ShouldEqual, "http")
		})
	})
}

// respondFeedbackReceipt makes the stub store feedback as report 7.
func respondFeedbackReceipt(stub *stubMLWH) {
	stub.respondJSON(feedbackPath, http.StatusCreated, wa.FeedbackReceipt{ID: 7, CreatedAt: feedbackCreatedAt})
}

func TestFeedbackToolB3StructuredErrors(t *testing.T) {
	Convey("Given a stub /feedback route and a core server over an in-memory client", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("B3.1: a 503 feedback_disabled envelope is feedback_disabled with do not retry", func() {
			stub.respondError(feedbackPath, http.StatusServiceUnavailable,
				"feedback_disabled", "feedback is disabled on this server")

			res := callMinimalFeedback(t, cs)
			So(res.IsError, ShouldBeTrue)

			text := firstTextContent(res)
			So(text, ShouldStartWith, "feedback_disabled: ")
			So(text, ShouldContainSubstring, "feedback is disabled on this server")
			So(text, ShouldEndWith, " (feedback is disabled on the wa server; "+
				"tell the user it could not be reported; do not retry)")
			So(stub.requestCount(), ShouldEqual, 1)
		})

		Convey("B3.2: a 404 text body is feedback_unsupported naming MLWH API 1.9.0", func() {
			respondFeedbackText(stub, http.StatusNotFound, "404 page not found")

			text := feedbackErrorText(t, cs)
			So(text, ShouldStartWith, "feedback_unsupported: ")
			So(text, ShouldEndWith, " (the wa server has no feedback endpoint at this URL: "+
				"it predates MLWH API 1.9.0 or is secured; tell the user; do not retry)")
			So(stub.requestCount(), ShouldEqual, 1)
		})

		Convey("B3.3: a 401 is feedback_unauthorized ending with its parenthesised do not retry next step", func() {
			respondFeedbackText(stub, http.StatusUnauthorized, "unauthorized")

			text := feedbackErrorText(t, cs)
			So(text, ShouldStartWith, "feedback_unauthorized: ")
			So(text, ShouldEndWith, " (the wa server requires authentication, which this MCP server does not support; "+
				"tell the user; do not retry)")
			So(stub.requestCount(), ShouldEqual, 1)
		})

		Convey("B3.4: a 413 is feedback_too_large keeping wa's message and asking to resubmit once", func() {
			stub.respondError(feedbackPath, http.StatusRequestEntityTooLarge,
				"payload_too_large", "description exceeds 16384 bytes")

			text := feedbackErrorText(t, cs)
			So(text, ShouldStartWith, "feedback_too_large: ")
			So(text, ShouldContainSubstring, "description exceeds 16384 bytes")
			So(text, ShouldEndWith, " (shorten description, user_request, or tools_tried and resubmit once)")
			So(stub.requestCount(), ShouldEqual, 1)
		})

		Convey("B3.5: a 400 bad_request envelope is feedback_invalid keeping wa's message and asking to resubmit once", func() {
			stub.respondError(feedbackPath, http.StatusBadRequest, "bad_request", "description is required")

			text := feedbackErrorText(t, cs)
			So(text, ShouldStartWith, "feedback_invalid: ")
			So(text, ShouldContainSubstring, "description is required")
			So(text, ShouldEndWith, " (fix category or description and resubmit once)")
			So(stub.requestCount(), ShouldEqual, 1)
		})

		Convey("B3.5: a 400 without a wa envelope is feedback_unavailable", func() {
			respondFeedbackText(stub, http.StatusBadRequest, "Client sent an HTTP request to an HTTPS server.\n")

			So(feedbackErrorText(t, cs), ShouldStartWith, "feedback_unavailable: ")
			So(stub.requestCount(), ShouldEqual, 1)
		})

		Convey("B3.6: a 500 internal_error envelope is feedback_unavailable with do not retry repeatedly", func() {
			stub.respondError(feedbackPath, http.StatusInternalServerError, "internal_error", "could not store feedback")

			text := feedbackErrorText(t, cs)
			So(text, ShouldStartWith, "feedback_unavailable: ")
			So(text, ShouldContainSubstring, "do not retry repeatedly")
			So(stub.requestCount(), ShouldEqual, 1)
		})

		Convey("B3.6: a 503 cache_never_synced envelope is feedback_unavailable keeping wa's message", func() {
			stub.respondError(feedbackPath, http.StatusServiceUnavailable, "cache_never_synced", "cache never synced")

			text := feedbackErrorText(t, cs)
			So(text, ShouldStartWith, "feedback_unavailable: ")
			So(text, ShouldContainSubstring, "cache never synced")
			So(stub.requestCount(), ShouldEqual, 1)
		})

		Convey("B3.6: a 201 whose body is not JSON is feedback_unavailable", func() {
			respondFeedbackText(stub, http.StatusCreated, "not json")

			So(feedbackErrorText(t, cs), ShouldStartWith, "feedback_unavailable: ")
			So(stub.requestCount(), ShouldEqual, 1)
		})

		Convey("B3.6: an unreachable wa server is feedback_unavailable", func() {
			stub.server.Close()

			So(feedbackErrorText(t, cs), ShouldStartWith, "feedback_unavailable: ")
		})
	})

	Convey("B3.7: mapFeedbackError maps nil to nil", t, func() {
		So(mapFeedbackError(nil), ShouldBeNil)
	})

	Convey("B3.7: an error wrapping both ErrFeedbackDisabled and ErrUpstreamImpaired is feedback_disabled", t, func() {
		err := mapFeedbackError(fmt.Errorf("%w: %w", wa.ErrUpstreamImpaired, wa.ErrFeedbackDisabled))

		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldStartWith, "feedback_disabled: ")
	})

	Convey("B3.8: cancellation during the feedback call cancels its HTTP request without retry or a result", t, func() {
		stub := newStubMLWH(t)
		requestStarted := stub.respondBlockingFirst(feedbackPath)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		stopServer := sync.OnceFunc(cleanup)
		defer stopServer()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		type callOutcome struct {
			result *mcp.CallToolResult
			err    error
		}

		callDone := make(chan callOutcome, 1)

		go func() {
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{
				Name: feedbackToolName, Arguments: map[string]any{"category": "other", "description": "x"},
			})
			callDone <- callOutcome{result: result, err: err}
		}()

		select {
		case <-requestStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("feedback request did not reach the stub")
		}

		cancel()

		var outcome callOutcome
		select {
		case outcome = <-callDone:
		case <-time.After(5 * time.Second):
			t.Fatal("cancelled feedback request did not return")
		}

		So(outcome.result, ShouldBeNil)
		So(errors.Is(outcome.err, context.Canceled), ShouldBeTrue)

		// CallTool returns as soon as the client is cancelled, while the tool
		// handler may still be running. Stopping the server waits for in-flight
		// handlers to return, so any retry has reached the stub before counting.
		stopServer()
		So(stub.requestCount(), ShouldEqual, 1)

		req, ok := stub.lastRequest()
		So(ok, ShouldBeTrue)
		So(req.Method, ShouldEqual, http.MethodPost)
		So(req.Path, ShouldEqual, feedbackPath)
	})
}

// submitMinimalFeedback calls the feedback tool with only the required fields
// and fails the test if the call does not succeed.
func submitMinimalFeedback(t *testing.T, cs *mcp.ClientSession) {
	t.Helper()

	res := callMinimalFeedback(t, cs)
	if res.IsError {
		t.Fatalf("feedback call failed: %s", firstTextContent(res))
	}
}

// lastFeedbackBody decodes the JSON body of the stub's most recent request,
// which must be a POST /feedback.
func lastFeedbackBody(t *testing.T, stub *stubMLWH) map[string]any {
	t.Helper()

	req, ok := stub.lastRequest()
	if !ok || req.Method != http.MethodPost || req.Path != feedbackPath {
		t.Fatalf("stub's last request is not POST %s: %+v", feedbackPath, req)
	}

	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("decoding feedback body %q: %v", req.Body, err)
	}

	return body
}

// respondFeedbackText makes the stub reply to /feedback with status and a
// plain-text body, as a proxy, a pre-1.9.0 router, or a broken server would.
func respondFeedbackText(stub *stubMLWH, status int, body string) {
	stub.respondHandler(feedbackPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

// feedbackErrorText calls the feedback tool with only the required fields and
// returns the error result's text, failing the test if the call succeeded.
func feedbackErrorText(t *testing.T, cs *mcp.ClientSession) string {
	t.Helper()

	res := callMinimalFeedback(t, cs)
	if !res.IsError {
		t.Fatalf("feedback call succeeded, want a tool error: %+v", res.StructuredContent)
	}

	return firstTextContent(res)
}

// callMinimalFeedback calls the feedback tool with only the required fields.
func callMinimalFeedback(t *testing.T, cs *mcp.ClientSession) *mcp.CallToolResult {
	t.Helper()

	return callTool(t, cs, feedbackToolName, map[string]any{"category": "other", "description": "x"})
}
