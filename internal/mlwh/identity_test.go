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
	"errors"
	"net/http"
	"os"
	"os/user"
	"regexp"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/smartystreets/goconvey/convey"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

func TestIdentityRemoteConfig(t *testing.T) {
	Convey("A2.4: identityRemoteConfig keeps other fields and replaces UserAgent and Identity", t, func() {
		got := identityRemoteConfig(wa.RemoteConfig{
			BaseURL:   "http://x",
			Timeout:   5 * time.Second,
			UserAgent: "other/1",
			Identity:  wa.ClientIdentity{ClientID: "c", Username: "mallory"},
		}, "0.4.0", "http")

		So(got.BaseURL, ShouldEqual, "http://x")
		So(got.Timeout, ShouldEqual, 5*time.Second)
		So(got.UserAgent, ShouldEqual, "mlwh-mcp-server/0.4.0")
		So(got.Identity, ShouldResemble, wa.ClientIdentity{MCPTransport: "http"})
	})

	Convey("A2.5: identityRemoteConfig for stdio adds the local username", t, func() {
		got := identityRemoteConfig(wa.RemoteConfig{BaseURL: "http://x"}, "dev", "stdio")

		So(got.UserAgent, ShouldEqual, "mlwh-mcp-server/dev")
		So(got.Identity, ShouldResemble, wa.ClientIdentity{
			MCPTransport: "stdio",
			Username:     localUsername(user.Current, os.Getenv),
		})
	})
}

func TestLocalUsername(t *testing.T) {
	Convey("A2.6: localUsername prefers the OS user, then $USER, then empty", t, func() {
		getenvBob := func(key string) string {
			if key == "USER" {
				return "bob"
			}

			return ""
		}
		getenvEmpty := func(string) string { return "" }
		currentErr := func() (*user.User, error) { return nil, errors.New("x") }

		Convey("a named current user wins over $USER", func() {
			current := func() (*user.User, error) { return &user.User{Username: "alice"}, nil }
			So(localUsername(current, getenvBob), ShouldEqual, "alice")
		})

		Convey("a current-user error falls back to $USER", func() {
			So(localUsername(currentErr, getenvBob), ShouldEqual, "bob")
		})

		Convey("an empty current username falls back to $USER", func() {
			current := func() (*user.User, error) { return &user.User{}, nil }
			So(localUsername(current, getenvBob), ShouldEqual, "bob")
		})

		Convey("an error and an empty $USER give empty", func() {
			So(localUsername(currentErr, getenvEmpty), ShouldEqual, "")
		})
	})
}

func TestSessionClientID(t *testing.T) {
	Convey("B1.1: sessionClientID hashes a session ID to a 32-hex SHA-256 prefix", t, func() {
		So(sessionClientID("abc"), ShouldEqual, "ba7816bf8f01cfea414140de5dae2223")
	})

	Convey("B1.2: sessionClientID of an empty ID is the stable per-process ID", t, func() {
		first := sessionClientID("")
		second := sessionClientID("")

		So(first, ShouldEqual, processClientID())
		So(second, ShouldEqual, processClientID())
		So(regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(processClientID()), ShouldBeTrue)
	})
}

func TestAgentIdentity(t *testing.T) {
	Convey("B1.3: agentIdentity prefers named clientInfo, then the HTTP User-Agent", t, func() {
		uaX := http.Header{"User-Agent": {"x/1"}}

		So(agentIdentity(&mcp.Implementation{Name: "claude-code", Version: "2.1.0"}, nil),
			ShouldEqual, "claude-code/2.1.0")
		So(agentIdentity(&mcp.Implementation{Name: "claude-code"}, nil), ShouldEqual, "claude-code")
		So(agentIdentity(&mcp.Implementation{Version: "1"}, uaX), ShouldEqual, "x/1")
		So(agentIdentity(nil, http.Header{"User-Agent": {"agent-cli/1.2"}}), ShouldEqual, "agent-cli/1.2")
		So(agentIdentity(&mcp.Implementation{Name: "claude-code", Version: "2.1.0"}, uaX),
			ShouldEqual, "claude-code/2.1.0")
		So(agentIdentity(nil, nil), ShouldEqual, "")
	})
}

func TestCallIdentity(t *testing.T) {
	Convey("B1.4: callIdentity of an empty request gives only the process client ID", t, func() {
		var got wa.ClientIdentity

		So(func() { got = callIdentity(&mcp.CallToolRequest{}) }, ShouldNotPanic)
		So(got, ShouldResemble, wa.ClientIdentity{ClientID: processClientID()})
	})

	Convey("B1.5: callIdentity combines session, header agent, and tool name", t, func() {
		got := callIdentity(&mcp.CallToolRequest{
			Session: &mcp.ServerSession{},
			Params:  &mcp.CallToolParamsRaw{Name: "mlwh_freshness"},
			Extra:   &mcp.RequestExtra{Header: http.Header{"User-Agent": {"a/1"}}},
		})

		So(got, ShouldResemble, wa.ClientIdentity{
			ClientID: processClientID(),
			MCPAgent: "a/1",
			MCPTool:  "mlwh_freshness",
		})
	})
}

// TestClientLevelIdentity covers Story A2 through the real MCP path: the
// provider's wa client sends the configured product, version, transport, and
// (stdio only) user headers on its upstream request.
func TestClientLevelIdentity(t *testing.T) {
	Convey("Given a stub MLWH serving a synced /freshness", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSON("/freshness", 200, syncedFreshness())

		Convey("A2.1: New with no options sends the dev User-Agent and stdio transport", func() {
			So(core.ServerVersion, ShouldEqual, "dev")

			req := freshnessRequest(t, stub)

			So(req.Header.Get("User-Agent"), ShouldEqual, "mlwh-mcp-server/dev")
			So(req.Header.Get(wa.HeaderMCPTransport), ShouldEqual, "stdio")
		})

		Convey("A2.2: WithServerInfo(0.4.0, http) sends its version and transport and no user", func() {
			req := freshnessRequest(t, stub, WithServerInfo("0.4.0", "http"))

			So(req.Header.Get("User-Agent"), ShouldEqual, "mlwh-mcp-server/0.4.0")
			So(req.Header.Get(wa.HeaderMCPTransport), ShouldEqual, "http")
			So(req.Header.Values(wa.HeaderClientUser), ShouldBeEmpty)
		})

		Convey("A2.3: WithServerInfo(v1.2.3, stdio) sends its version and the local user", func() {
			u := localUsername(user.Current, os.Getenv)

			req := freshnessRequest(t, stub, WithServerInfo("v1.2.3", "stdio"))

			So(req.Header.Get("User-Agent"), ShouldEqual, "mlwh-mcp-server/v1.2.3")

			if u == "" {
				So(req.Header.Values(wa.HeaderClientUser), ShouldBeEmpty)
			} else {
				So(req.Header.Get(wa.HeaderClientUser), ShouldEqual, u)
			}
		})
	})
}

// TestToolsCallIdentity covers Story B2 through the real MCP path: every
// upstream wa request made inside a tools/call carries that call's tool,
// agent, and client ID, while protocol traffic and pre-upstream rejections
// reach wa not at all.
func TestToolsCallIdentity(t *testing.T) {
	Convey("Given a stub MLWH serving a synced /freshness", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSON("/freshness", http.StatusOK, syncedFreshness())

		stdio := WithServerInfo("0.4.0", "stdio")

		Convey("B2.1: mlwh_freshness sends its tool, clientInfo agent, process client ID, and transport", func() {
			req := freshnessRequest(t, stub, stdio)

			So(req.Header.Get(wa.HeaderMCPTool), ShouldEqual, "mlwh_freshness")
			So(req.Header.Get(wa.HeaderMCPAgent), ShouldEqual, "test-client/0.0.0")
			So(req.Header.Get(wa.HeaderClientID), ShouldEqual, processClientID())
			So(req.Header.Get(wa.HeaderMCPTransport), ShouldEqual, "stdio")
		})

		Convey("B2.2: two tools in one session send their own tool names and one client ID", func() {
			cs, cleanup := runMLWHServerWithClientOptions(t, stub, core.Options{}, stdio)
			defer cleanup()

			So(callTool(t, cs, "mlwh_freshness", map[string]any{}).IsError, ShouldBeFalse)
			So(callTool(t, cs, "mlwh_call_endpoint", map[string]any{"method": "Freshness"}).IsError, ShouldBeFalse)

			reqs := stub.recordedRequests()
			So(reqs, ShouldHaveLength, 2)
			So(reqs[0].Path, ShouldEqual, "/freshness")
			So(reqs[1].Path, ShouldEqual, "/freshness")
			So(reqs[0].Header.Get(wa.HeaderMCPTool), ShouldEqual, "mlwh_freshness")
			So(reqs[1].Header.Get(wa.HeaderMCPTool), ShouldEqual, "mlwh_call_endpoint")
			So(reqs[0].Header.Get(wa.HeaderClientID), ShouldNotBeEmpty)
			So(reqs[1].Header.Get(wa.HeaderClientID), ShouldEqual, reqs[0].Header.Get(wa.HeaderClientID))
		})

		Convey("B2.3: two harness servers in one process share the stdio client ID", func() {
			first := freshnessRequest(t, stub, stdio)

			other := newStubMLWH(t)
			other.respondJSON("/freshness", http.StatusOK, syncedFreshness())
			second := freshnessRequest(t, other, stdio)

			So(first.Header.Get(wa.HeaderClientID), ShouldNotBeEmpty)
			So(second.Header.Get(wa.HeaderClientID), ShouldEqual, first.Header.Get(wa.HeaderClientID))
		})

		Convey("B2.4: mlwh_provide_feedback sends identity headers and keeps its client body fields", func() {
			respondFeedbackReceipt(stub)

			cs, cleanup := runMLWHServerWithClientOptions(t, stub, core.Options{}, stdio)
			defer cleanup()

			submitMinimalFeedback(t, cs)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Method, ShouldEqual, http.MethodPost)
			So(req.Path, ShouldEqual, feedbackPath)
			So(req.Header.Get(wa.HeaderMCPTool), ShouldEqual, feedbackToolName)
			So(req.Header.Get(wa.HeaderMCPAgent), ShouldEqual, "test-client/0.0.0")
			So(req.Header.Get(wa.HeaderClientID), ShouldEqual, processClientID())

			body := lastFeedbackBody(t, stub)
			So(body["client_name"], ShouldEqual, "test-client")
			So(body["client_version"], ShouldEqual, "0.0.0")
		})

		Convey("B2.5: a client with no version is identified by its name alone", func() {
			cs, cleanup := runMLWHServerWithClientImplementation(t, stub,
				&mcp.Implementation{Name: "claude-code", Version: ""}, core.Options{}, stdio)
			defer cleanup()

			So(callTool(t, cs, "mlwh_freshness", map[string]any{}).IsError, ShouldBeFalse)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Header.Get(wa.HeaderMCPAgent), ShouldEqual, "claude-code")
		})
	})

	Convey("Given a stub MLWH with no routes", t, func() {
		stub := newStubMLWH(t)

		cs, cleanup := runMLWHServerWithClientOptions(t, stub, core.Options{}, WithServerInfo("0.4.0", "stdio"))
		defer cleanup()

		Convey("B2.6: a failed upstream request still carries the tool name", func() {
			res := callTool(t, cs, "mlwh_freshness", map[string]any{})
			So(res.IsError, ShouldBeTrue)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/freshness")
			So(req.Header.Get(wa.HeaderMCPTool), ShouldEqual, "mlwh_freshness")
		})

		Convey("B2.7: listing tools and reading resources makes no upstream request", func() {
			_, found := toolByName(t, cs, "mlwh_freshness")
			So(found, ShouldBeTrue)

			readResource(t, cs, "mlwh://workflow")
			readResource(t, cs, "mcp-server://version")

			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("B2.8: a feedback call rejected before upstream makes no request", func() {
			res := callTool(t, cs, feedbackToolName, map[string]any{"category": "bogus", "description": "x"})

			So(res.IsError, ShouldBeTrue)
			So(stub.requestCount(), ShouldEqual, 0)
		})
	})
}

// freshnessRequest runs mlwh_freshness with {} against stub through a provider
// built with providerOpts, and returns the stub's one recorded request.
func freshnessRequest(t *testing.T, stub *stubMLWH, providerOpts ...Option) recordedRequest {
	t.Helper()

	cs, cleanup := runMLWHServerWithClientOptions(t, stub, core.Options{}, providerOpts...)
	defer cleanup()

	res := callTool(t, cs, "mlwh_freshness", map[string]any{})
	So(res.IsError, ShouldBeFalse)
	So(stub.requestCount(), ShouldEqual, 1)

	req, ok := stub.lastRequest()
	So(ok, ShouldBeTrue)
	So(req.Path, ShouldEqual, "/freshness")

	return req
}
