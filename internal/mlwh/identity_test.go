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
	"os"
	"os/user"
	"testing"
	"time"

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
