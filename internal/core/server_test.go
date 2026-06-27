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

package core

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/smartystreets/goconvey/convey"
)

// TestServerImplementationAndInstructions exercises Story G3 (implementation
// info + instructions) and Story H1 acceptance test 1 (Run accepts any
// mcp.Transport). It is end-to-end: it builds a core server, runs it over an
// in-memory transport, connects a real MCP client, and inspects the advertised
// implementation, instructions, and tool list as the client observes them on
// connect.
func TestServerImplementationAndInstructions(t *testing.T) {
	Convey("Given a core server built with a server version and a provider", t, func() {
		provider := newTestProvider()

		clientSession, cleanup := runServerWithClient(t, Options{
			ServerVersion: "0.1.0",
			Providers:     []Provider{provider},
		})
		defer cleanup()

		init := clientSession.InitializeResult()

		Convey("G3.1: the advertised Implementation has the right name and version", func() {
			So(init.ServerInfo, ShouldNotBeNil)
			So(init.ServerInfo.Name, ShouldEqual, "mlwh-mcp-server")
			So(init.ServerInfo.Version, ShouldEqual, "0.1.0")
		})

		Convey("G3.2: the Instructions contain the server version and the provider's API version", func() {
			So(init.Instructions, ShouldContainSubstring, "0.1.0")
			So(init.Instructions, ShouldContainSubstring, provider.APIVersion())
		})

		Convey("H1.1: a connected client can list tools and sees the provider's tool", func() {
			res, err := clientSession.ListTools(context.Background(), &mcp.ListToolsParams{})
			So(err, ShouldBeNil)
			So(res, ShouldNotBeNil)

			names := make([]string, 0, len(res.Tools))
			for _, tool := range res.Tools {
				names = append(names, tool.Name)
			}

			So(names, ShouldContain, provider.toolName)
		})
	})

	Convey("Given a core server with no providers (G3.1 needs no provider)", t, func() {
		clientSession, cleanup := runServerWithClient(t, Options{ServerVersion: "2.3.4"})
		defer cleanup()

		init := clientSession.InitializeResult()

		Convey("G3.1: the Implementation name and version are still correct", func() {
			So(init.ServerInfo, ShouldNotBeNil)
			So(init.ServerInfo.Name, ShouldEqual, "mlwh-mcp-server")
			So(init.ServerInfo.Version, ShouldEqual, "2.3.4")
		})

		Convey("G3.2: the Instructions still contain the server version", func() {
			So(init.Instructions, ShouldContainSubstring, "2.3.4")
		})
	})

	Convey("Given a core server built from providers, the assembled VersionInfo "+
		"reflects the seam (the mechanism G2/G3/G5 rely on)", t, func() {
		first := newTestProvider()
		first.name = "alpha"
		first.apiVersion = "ALPHA 1.0.0"

		second := newTestProvider()
		second.name = "beta"
		second.apiVersion = "BETA 2.0.0"

		srv, err := New(Options{
			ServerVersion: "0.1.0",
			Providers:     []Provider{first, second},
		})
		So(err, ShouldBeNil)

		info := srv.VersionInfo()

		Convey("it carries the server version", func() {
			So(info.ServerVersion, ShouldEqual, "0.1.0")
		})

		Convey("it maps each provider name to its targeted upstream API version", func() {
			So(info.APIVersions["alpha"], ShouldEqual, "ALPHA 1.0.0")
			So(info.APIVersions["beta"], ShouldEqual, "BETA 2.0.0")
			So(len(info.APIVersions), ShouldEqual, 2)
		})
	})

	Convey("Given Instructions that mention multiple providers", t, func() {
		first := newTestProvider()
		first.name = "alpha"
		first.apiVersion = "ALPHA 1.0.0"
		first.toolName = "alpha_ping"
		first.resourceURI = "alpha://thing"

		second := newTestProvider()
		second.name = "beta"
		second.apiVersion = "BETA 2.0.0"
		second.toolName = "beta_ping"
		second.resourceURI = "beta://thing"

		clientSession, cleanup := runServerWithClient(t, Options{
			ServerVersion: "0.1.0",
			Providers:     []Provider{first, second},
		})
		defer cleanup()

		init := clientSession.InitializeResult()

		Convey("each provider's targeted API version appears in the Instructions", func() {
			So(init.Instructions, ShouldContainSubstring, first.APIVersion())
			So(init.Instructions, ShouldContainSubstring, second.APIVersion())
		})
	})
}
