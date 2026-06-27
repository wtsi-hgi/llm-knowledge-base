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
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

// TestWorkflowResource exercises Story G1: the MLWH provider registers a
// workflow / endpoint-catalogue resource at mlwh://workflow whose body is the
// always-current wa.EndpointReference() catalogue, prefixed with short workflow
// guidance. It reads the resource end-to-end over a real in-memory MCP client.
func TestWorkflowResource(t *testing.T) {
	Convey("Given a core server hosting the MLWH provider pointed at a stub", t, func() {
		stub := newStubMLWH(t)

		clientSession, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		contents := readResource(t, clientSession, "mlwh://workflow")

		Convey("G1.1: the resource text contains wa.EndpointReference()'s output", func() {
			// The catalogue heading and a concrete endpoint entry prove the body is
			// the real, Registry-derived EndpointReference() output (not a copied
			// doc). Assert against substrings of EndpointReference()'s actual output.
			So(contents.Text, ShouldContainSubstring, "wa mlwh API endpoint reference")
			So(contents.Text, ShouldContainSubstring, "/resolve/sample")

			// Whatever EndpointReference() emits in full must be embedded verbatim.
			So(contents.Text, ShouldContainSubstring, wa.EndpointReference())
		})

		Convey("G1.2: the resource's MIMEType is text/markdown", func() {
			So(contents.MIMEType, ShouldEqual, "text/markdown")
		})

		Convey("G1.3: the resource text contains workflow guidance mentioning resolve and detail", func() {
			So(contents.Text, ShouldContainSubstring, "resolve")
			So(contents.Text, ShouldContainSubstring, "detail")
		})
	})
}

// readResource reads the resource with the given URI over the connected MCP
// client and returns its first contents block, failing the test on a protocol
// error or if the resource returns no contents. It lets a test assert the
// resource body and MIME type a client actually observes over real MCP.
func readResource(t *testing.T, cs *mcp.ClientSession, uri string) *mcp.ResourceContents {
	t.Helper()

	res, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource(%q) returned protocol error: %v", uri, err)
	}

	if len(res.Contents) == 0 {
		t.Fatalf("ReadResource(%q) returned no contents", uri)
	}

	return res.Contents[0]
}
