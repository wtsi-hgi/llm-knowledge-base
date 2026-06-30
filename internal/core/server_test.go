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
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/smartystreets/goconvey/convey"
)

type payloadProvider struct{}

func (payloadProvider) Name() string { return "payload" }

func (payloadProvider) APIVersion() string { return "payload 1.0.0" }

func (payloadProvider) Register(_ context.Context, r Registrar) error {
	type payloadInput struct{}

	type payloadOutput struct {
		Payload string `json:"payload"`
	}

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:        "test_payload",
		Description: "returns a fixed payload for result-size guard tests",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ payloadInput) (*mcp.CallToolResult, payloadOutput, error) {
		return nil, payloadOutput{Payload: "1234567890"}, nil
	})

	return nil
}

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

// TestResultSizeGuard exercises A2's public core behaviour through a real MCP
// client: oversized typed tool results are replaced with a structured tool
// error, while a disabled guard leaves the original result untouched.
func TestResultSizeGuard(t *testing.T) {
	Convey("Given a fake provider tool returning a ten-byte payload and a 20-byte guard", t, func() {
		clientSession, cleanup := runServerWithClient(t, Options{
			ServerVersion:          "0.1.0",
			Providers:              []Provider{payloadProvider{}},
			MaxToolResultBytes:     20,
			ToolResultSizeGuidance: "use an overview, count, or smaller page",
		})
		defer cleanup()

		res, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "test_payload",
			Arguments: map[string]any{},
		})
		So(err, ShouldBeNil)

		Convey("A2.1: the result is replaced with the structured size error", func() {
			obj := sizeErrorObject(res)
			So(obj["code"], ShouldEqual, "tool_result_too_large")
			So(jsonNumber(obj["limit_bytes"]), ShouldEqual, float64(20))
			So(jsonNumber(obj["actual_bytes"]), ShouldBeGreaterThan, float64(20))
			So(obj["guidance"], ShouldEqual, "use an overview, count, or smaller page")
		})

		Convey("the text content contains the same JSON error object", func() {
			So(len(res.Content), ShouldEqual, 1)

			text, ok := res.Content[0].(*mcp.TextContent)
			So(ok, ShouldBeTrue)

			var textObj map[string]any
			So(json.Unmarshal([]byte(text.Text), &textObj), ShouldBeNil)
			So(textObj["code"], ShouldEqual, "tool_result_too_large")
			So(jsonNumber(textObj["limit_bytes"]), ShouldEqual, float64(20))
			So(textObj["guidance"], ShouldEqual, "use an overview, count, or smaller page")
		})
	})

	Convey("Given the same fake provider tool and a disabled guard", t, func() {
		clientSession, cleanup := runServerWithClient(t, Options{
			ServerVersion:      "0.1.0",
			Providers:          []Provider{payloadProvider{}},
			MaxToolResultBytes: 0,
		})
		defer cleanup()

		res, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "test_payload",
			Arguments: map[string]any{},
		})
		So(err, ShouldBeNil)

		Convey("A2.2: the original payload is returned successfully", func() {
			So(res.IsError, ShouldBeFalse)

			obj, ok := res.StructuredContent.(map[string]any)
			So(ok, ShouldBeTrue)
			So(obj["payload"], ShouldEqual, "1234567890")
		})
	})
}

func sizeErrorObject(res *mcp.CallToolResult) map[string]any {
	So(res.IsError, ShouldBeTrue)

	obj, ok := res.StructuredContent.(map[string]any)
	So(ok, ShouldBeTrue)

	return obj
}

func jsonNumber(value any) float64 {
	switch n := value.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		number, _ := n.Float64()

		return number
	default:
		return -1
	}
}
