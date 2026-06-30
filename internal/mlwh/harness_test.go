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
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/smartystreets/goconvey/convey"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

// This file is the shared, hermetic stub-MLWH test harness reused by every
// phase-5/6 provider test. It stands up an httptest.Server that serves canned
// JSON for EXACT wa mlwh request paths (the shapes wa's *RemoteClient decodes,
// verified by round-trip through the typed client) and records the requests it
// receives, plus a helper that builds a core *mcp.Server with the MLWH provider
// pointed at the stub and drives it through a real in-memory MCP client. Tests
// are hermetic: the stub is the only "server"; a live warehouse is never used.

// recordedRequest captures one HTTP request the stub received, so a test can
// assert the exact path and query parameters wa's RemoteClient sent (e.g. that
// limit=5/offset=10 reached the stub) or assert that NO request was made.
type recordedRequest struct {
	// Path is the request URL path, e.g. "/search/sample/mus".
	Path string

	// Query holds the decoded query parameters, e.g. {"limit":["5"]}.
	Query url.Values
}

// stubResponse is the canned reply the stub returns for one exact path. A
// success carries a status in the 2xx range and a body that JSON-marshals to
// the shape wa's RemoteClient decodes for that endpoint (a JSON array for the
// slice endpoints, a {"count":N} object for the count endpoints). An error
// carries a non-2xx status and is rendered as wa's {"code","message"} envelope
// so the remote client maps it back to the matching Err* sentinel.
type stubResponse struct {
	status  int
	body    any
	headers http.Header
	isErr   bool
	code    string
	msg     string
}

// stubMLWH is the hermetic MLWH stub: an httptest.Server that serves canned
// JSON keyed by exact request path and records every request for later
// assertions. It is safe for the test goroutine to read its records while the
// server's handler goroutine appends to them.
type stubMLWH struct {
	t      *testing.T
	server *httptest.Server

	mu        sync.Mutex
	routes    map[string]stubResponse
	requests  []recordedRequest
	unmatched []string
}

// newStubMLWH starts a hermetic MLWH stub and registers it for shutdown when the
// test ends. No routes are configured yet; add them with respondJSON or
// respondError before driving a tool.
func newStubMLWH(t *testing.T) *stubMLWH {
	t.Helper()

	stub := &stubMLWH{
		t:      t,
		routes: map[string]stubResponse{},
	}

	stub.server = httptest.NewServer(http.HandlerFunc(stub.handle))
	t.Cleanup(stub.server.Close)

	return stub
}

// respondJSON makes the stub reply with the given 2xx status and JSON body for
// requests to exactly this path. body must marshal to the shape wa's
// RemoteClient decodes for that endpoint, or the typed method (and so the tool
// under test) will surface a decode error.
func (s *stubMLWH) respondJSON(path string, status int, body any) {
	s.respondJSONWithHeaders(path, status, body, nil)
}

// respondJSONWithHeaders makes the stub reply with the given 2xx status, JSON
// body, and HTTP headers for requests to exactly this path. It is used for
// header-aware wa RemoteClient page methods that read X-Total-Count and
// X-Next-Offset while still decoding the body as the endpoint's normal JSON.
func (s *stubMLWH) respondJSONWithHeaders(path string, status int, body any, headers http.Header) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.routes[path] = stubResponse{status: status, body: body, headers: headers.Clone()}
}

// respondError makes the stub reply with the given non-2xx status and wa error
// envelope ({"code","message"}) for requests to exactly this path, so the
// remote client maps the documented code (e.g. "not_found"->404->ErrNotFound)
// back to its sentinel and the provider's error mapping is exercised.
func (s *stubMLWH) respondError(path string, status int, code, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.routes[path] = stubResponse{status: status, isErr: true, code: code, msg: msg}
}

// handle serves a recorded request from the configured routes. An unconfigured
// path is recorded as unmatched and answered 404 with a wa-shaped envelope, so a
// test that wrongly expects a request still sees a clear failure rather than a
// hang.
func (s *stubMLWH) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, recordedRequest{Path: r.URL.Path, Query: r.URL.Query()})
	resp, ok := s.routes[r.URL.Path]
	if !ok {
		s.unmatched = append(s.unmatched, r.URL.Path)
	}
	s.mu.Unlock()

	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(httpErrorBody{Code: "not_found", Message: "stub: no route for " + r.URL.Path})

		return
	}

	copyHeaders(w.Header(), resp.headers)
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}

	if resp.isErr {
		w.WriteHeader(resp.status)
		_ = json.NewEncoder(w).Encode(httpErrorBody{Code: resp.code, Message: resp.msg})

		return
	}

	w.WriteHeader(resp.status)
	_ = json.NewEncoder(w).Encode(resp.body)
}

func copyHeaders(dst, src http.Header) {
	for name, values := range src {
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}

// httpErrorBody is the on-the-wire shape of wa's HTTP error envelope, which the
// remote client decodes to recover the error code. It mirrors wa's unexported
// httpErrorEnvelope so the stub can emit byte-identical error bodies.
type httpErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// requestCount returns how many requests the stub has received so far. A test
// asserts this is 0 to prove a cheap input guard short-circuited before any HTTP
// call (e.g. a too-short term or an over-max limit).
func (s *stubMLWH) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.requests)
}

// lastRequest returns the most recently recorded request, or false if none was
// made. A test reads it to assert the exact path and query parameters wa's
// RemoteClient sent.
func (s *stubMLWH) lastRequest() (recordedRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.requests) == 0 {
		return recordedRequest{}, false
	}

	return s.requests[len(s.requests)-1], true
}

// runMLWHServerWithClient builds the MLWH provider pointed at the stub, wraps it
// in a core server, runs that over one half of an in-memory transport pair
// (proving Run accepts any mcp.Transport), connects a real MCP client over the
// other half, and returns the connected session so a test can list and call
// tools and read resources end-to-end through real MCP. The returned cleanup
// cancels Run and waits for it to stop; it must be deferred.
func runMLWHServerWithClient(t *testing.T, stub *stubMLWH) (*mcp.ClientSession, func()) {
	t.Helper()

	return runMLWHServerWithClientOptions(t, stub, core.Options{})
}

func runMLWHServerWithClientOptions(
	t *testing.T,
	stub *stubMLWH,
	opts core.Options,
) (*mcp.ClientSession, func()) {
	t.Helper()

	provider, err := New(wa.RemoteConfig{BaseURL: stub.server.URL})
	if err != nil {
		t.Fatalf("mlwh.New() returned error: %v", err)
	}

	opts.ServerVersion = firstNonEmpty(opts.ServerVersion, "test")
	opts.Providers = []core.Provider{provider}

	srv, err := core.New(opts)
	if err != nil {
		t.Fatalf("core.New() returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	runErr := make(chan error, 1)

	go func() {
		runErr <- srv.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()

		t.Fatalf("client Connect() returned error: %v", err)
	}

	cleanup := func() {
		_ = clientSession.Close()

		cancel()

		select {
		case <-runErr:
		case <-time.After(5 * time.Second):
			t.Errorf("Run did not return after context cancellation")
		}
	}

	return clientSession, cleanup
}

func TestStubMLWHHeaderAwareResponses(t *testing.T) {
	Convey("F3.1: Given respondJSONWithHeaders is configured with X-Total-Count: 2 and an MCP tool calls that route", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSONWithHeaders("/studies", http.StatusOK, []wa.Study{
			{IDStudyLims: "S1", Name: "Alpha"},
			{IDStudyLims: "S2", Name: "Beta"},
		}, http.Header{
			"X-Total-Count": {"2"},
			"X-Next-Offset": {"-1"},
		})

		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		res := callTool(t, cs, "mlwh_call_endpoint", map[string]any{
			"method":       "AllStudies",
			"query_params": map[string]any{"limit": "100", "offset": "0"},
		})

		obj := structuredObject(res)

		So(obj["total"], ShouldEqual, 2)
		So(obj["next_offset"], ShouldEqual, -1)
		result, ok := obj["result"].([]any)
		So(ok, ShouldBeTrue)
		So(len(result), ShouldEqual, 2)

		req, ok := stub.lastRequest()
		So(ok, ShouldBeTrue)
		So(req.Path, ShouldEqual, "/studies")
		So(req.Query.Get("limit"), ShouldEqual, "100")
		So(req.Query.Get("offset"), ShouldEqual, "0")
	})
}

func TestStubMLWHUnmatchedRouteReturnsWAEnvelope(t *testing.T) {
	Convey("F3.2: Given an unmatched route, when the remote client calls it, then the stub preserves wa-shaped 404 errors", t, func() {
		stub := newStubMLWH(t)

		client, err := wa.NewRemoteClient(wa.RemoteConfig{BaseURL: stub.server.URL})
		So(err, ShouldBeNil)
		defer func() {
			So(client.Close(), ShouldBeNil)
		}()

		_, err = client.ResolveStudy(context.Background(), "missing-study")

		So(err, ShouldNotBeNil)
		So(errors.Is(err, wa.ErrNotFound), ShouldBeTrue)

		req, ok := stub.lastRequest()
		So(ok, ShouldBeTrue)
		So(req.Path, ShouldEqual, "/resolve/study/missing-study")
	})
}

// callTool calls the named tool over the connected MCP client with the given
// arguments and returns the result, failing the test on a protocol error (a
// tool error is a normal result with IsError set, not a protocol error, so it is
// returned for the caller to assert on).
func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%q) returned protocol error: %v", name, err)
	}

	return res
}

// toolByName lists the server's tools over the MCP client and returns the one
// with the given name, or false if it is not registered. It lets a test assert
// observable tool metadata (name, description, input schema) as a client sees
// it.
func toolByName(t *testing.T, cs *mcp.ClientSession, name string) (*mcp.Tool, bool) {
	t.Helper()

	res, err := cs.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools() returned error: %v", err)
	}

	for _, tool := range res.Tools {
		if tool.Name == name {
			return tool, true
		}
	}

	return nil, false
}

// firstTextContent returns the text of the first text content block in a tool
// result, or "" if there is none. The typed handlers leave Content unset, so the
// SDK auto-fills one text block holding the JSON of the structured output; a
// test parses it to prove the structured+text duo agree.
func firstTextContent(res *mcp.CallToolResult) string {
	for _, content := range res.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			return text.Text
		}
	}

	return ""
}
