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

// This is an EXTERNAL test package (core_test) on purpose. Story I2 proves a
// second, unrelated service plugs into the core through the Provider/Registrar
// seam with no core change, by building a core with BOTH the real MLWH provider
// and a test-only fakeProvider. Because internal/mlwh imports internal/core, an
// in-package (package core) test could not import internal/mlwh without an
// import cycle, so this file lives in core_test, which may import internal/core,
// internal/mlwh, and wa together. The fakeProvider is deliberately defined ONLY
// here: no production core file references it, which is the whole point - adding
// a provider requires only a new Provider implementation plus its registration,
// not a core change. It reuses runCoreWithClient and newMLWHProvider from the
// other core_test files (same test package).
package core_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"

	. "github.com/smartystreets/goconvey/convey"
)

// fakeProvider must satisfy the core.Provider seam exactly; this fails to
// compile if the interface and the fake drift, so the seam proof stays honest.
var _ core.Provider = (*fakeProvider)(nil)

// fakeResourceURI is the URI of the fake provider's single resource, registered
// through the Registrar to prove resources (not just tools) flow through the
// seam.
const fakeResourceURI = "fake://info"

// fakePingInput is the (empty) input for fake_ping: it takes no arguments, so
// its empty struct infers the {"type":"object"} input schema MCP requires.
type fakePingInput struct{}

// fakePingOutput is the trivial typed result of fake_ping. Returning a typed Out
// makes the SDK populate StructuredContent and a text Content block for free, so
// the call succeeds with IsError=false through the ordinary handler path.
type fakePingOutput struct {
	Message string `json:"message" jsonschema:"a fixed acknowledgement that the fake provider handled the call"`
}

// fakeProvider is a minimal, test-only core.Provider for the multi-service seam
// proof (Story I2). It deliberately resembles neither MLWH nor wa: it owns no
// client and no domain types, registering a single trivial tool (fake_ping) and
// a single resource through the SAME core.Registrar seam the MLWH provider uses.
// Its existence only in this _test.go file proves a second, unrelated service
// plugs in with no production core change.
type fakeProvider struct{}

// Name returns the fake provider's stable identifier, distinct from "mlwh".
func (fakeProvider) Name() string { return "fake" }

// APIVersion returns a placeholder upstream version for the fake service. The
// core surfaces it like any provider's version without knowing the domain.
func (fakeProvider) APIVersion() string { return "fake 1.0.0" }

// Register adds the fake provider's one tool and one resource through the
// core.Registrar seam - mcp.AddTool[In,Out](r.Server(), ...) for fake_ping and
// r.AddResource(...) for the resource - exactly as the MLWH provider does. This
// is the only wiring the fake needs: no production core code changes to host it.
func (fakeProvider) Register(_ context.Context, r core.Registrar) error {
	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:        "fake_ping",
		Description: "A trivial no-op tool proving a second provider registers through the same core seam; returns a fixed acknowledgement.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ fakePingInput) (*mcp.CallToolResult, fakePingOutput, error) {
		return nil, fakePingOutput{Message: "pong"}, nil
	})

	r.AddResource(&mcp.Resource{
		URI:         fakeResourceURI,
		Name:        "fake-info",
		Title:       "Fake provider info",
		Description: "A trivial resource proving a second provider's resources also flow through the core Registrar seam.",
		MIMEType:    "text/plain",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: "text/plain",
				Text:     "fake provider",
			}},
		}, nil
	})

	return nil
}

// TestProviderSeamMultiService exercises Story I2: a core server built with BOTH
// the real MLWH provider and a test-only fakeProvider exposes both surfaces, and
// the fake's tool is callable, proving a second unrelated service plugs into the
// core through the Provider/Registrar seam with no core change. The MLWH
// provider is built hermetically (a dummy base URL); listing tools and calling
// fake_ping contact no server.
func TestProviderSeamMultiService(t *testing.T) {
	Convey("Given a core server built with both the MLWH provider and a test-only "+
		"fakeProvider", t, func() {
		mlwhProvider := newMLWHProvider(t)

		clientSession, cleanup := runCoreWithClient(t, core.Options{
			ServerVersion: "0.1.0",
			Providers:     []core.Provider{mlwhProvider, fakeProvider{}},
		})
		defer cleanup()

		Convey("I2.1: a tools listing contains both fake_ping and mlwh_search_samples", func() {
			res, err := clientSession.ListTools(context.Background(), &mcp.ListToolsParams{})
			So(err, ShouldBeNil)

			names := make(map[string]struct{}, len(res.Tools))
			for _, tool := range res.Tools {
				names[tool.Name] = struct{}{}
			}

			So(names, ShouldContainKey, "fake_ping")
			So(names, ShouldContainKey, "mlwh_search_samples")
		})

		Convey("I2.2: calling fake_ping returns its trivial result with IsError=false", func() {
			res, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "fake_ping",
				Arguments: map[string]any{},
			})
			So(err, ShouldBeNil)
			So(res.IsError, ShouldBeFalse)

			structured, ok := res.StructuredContent.(map[string]any)
			So(ok, ShouldBeTrue)
			So(structured["message"], ShouldEqual, "pong")
		})
	})
}
