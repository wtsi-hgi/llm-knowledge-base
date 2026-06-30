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
	"strings"
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

		Convey("E3.1: the workflow guidance routes common questions to cheap tools first", func() {
			guidance := workflowGuidancePrefix(t, contents.Text)

			So(guidance, ShouldContainSubstring, "mlwh_study_overview")
			So(guidance, ShouldContainSubstring, "mlwh_count_samples_with_data_for_study")
			So(guidance, ShouldContainSubstring, "mlwh_study_status_breakdown")
			So(guidance, ShouldContainSubstring, "mlwh_sample_progress")
			So(guidance, ShouldContainSubstring, "mlwh_run_status")
			So(guidance, ShouldContainSubstring, "mlwh_study_manifest")
			So(guidance, ShouldContainSubstring, "file_type=cram")
			So(guidance, ShouldContainSubstring, "mlwh_resolve_person")
		})

		Convey("E3.2: the workflow guidance describes recency by iRODS created timestamps", func() {
			guidance := workflowGuidancePrefix(t, contents.Text)

			So(guidance, ShouldContainSubstring, "added_last_7_days")
			So(guidance, ShouldContainSubstring, "since")
			So(guidance, ShouldContainSubstring, "until")
			So(guidance, ShouldContainSubstring, "added to iRODS")
			So(guidance, ShouldContainSubstring, "iRODS `created`")
			So(guidance, ShouldNotContainSubstring, "last_changed is new data")
			So(guidance, ShouldNotContainSubstring, "last_updated is new data")
		})

		Convey("E3.3: the workflow guidance warns away from expensive detail and iRODS paging", func() {
			guidance := workflowGuidancePrefix(t, contents.Text)

			So(guidance, ShouldContainSubstring, "do not page iRODS")
			So(guidance, ShouldContainSubstring, "mlwh_study_detail")
			So(guidance, ShouldContainSubstring, "availability/count questions")
			So(guidance, ShouldContainSubstring, "data-access-group questions")
			So(guidance, ShouldContainSubstring, "do not use study detail")
		})

		Convey("E3.4: the workflow guidance keeps open phase elapsed time on the agent side", func() {
			guidance := workflowGuidancePrefix(t, contents.Text)

			So(guidance, ShouldContainSubstring, "open phase elapsed time")
			So(guidance, ShouldContainSubstring, "agent side")
			So(guidance, ShouldContainSubstring, "reached_at")
			So(guidance, ShouldContainSubstring, "entered_at")
		})

		Convey("E3.5: the Registry-derived catalogue is still appended with study overview", func() {
			So(contents.Text, ShouldContainSubstring, wa.EndpointReference())
			So(contents.Text, ShouldContainSubstring, "/study/:id/overview")
		})

		Convey("E3.6: the workflow guidance explains cache freshness caveats", func() {
			guidance := workflowGuidancePrefix(t, contents.Text)

			So(guidance, ShouldContainSubstring, "cache_synced_at")
			So(guidance, ShouldContainSubstring, "when present")
			So(guidance, ShouldContainSubstring, "mlwh_freshness")
			So(guidance, ShouldContainSubstring, "bare lists")
			So(guidance, ShouldContainSubstring, "counts")
			So(guidance, ShouldContainSubstring, "mlwh_run_status")
			So(guidance, ShouldContainSubstring, "mlwh_call_endpoint")
			So(guidance, ShouldContainSubstring, "without cache_synced_at")
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

// workflowGuidancePrefix returns the human-authored guidance portion before the
// Registry-derived endpoint catalogue in the public resource body.
func workflowGuidancePrefix(t *testing.T, body string) string {
	t.Helper()

	parts := strings.SplitN(body, "\n---\n\n", 2)
	if len(parts) != 2 {
		t.Fatalf("workflow resource body did not contain guidance/catalogue separator")
	}

	return parts[0]
}
