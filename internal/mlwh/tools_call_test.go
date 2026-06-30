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
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"

	. "github.com/smartystreets/goconvey/convey"
)

// TestCallTool covers Story E2: the generic mlwh_call_endpoint escape-hatch tool.
// Every assertion drives the tool over the real in-memory MCP client against the
// hermetic stub, so dispatch through (*RemoteClient).CallWithHeaders, the
// untyped passthrough output, the QueryParams->url.Values conversion, and the
// unknown-method and path-param arity errors surfaced by CallWithHeaders are all
// exercised end-to-end.
func TestCallTool(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with mlwh_call_endpoint", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("E2.2: ResolveStudy with no pagination headers returns the decoded Match without a wrapper", func() {
			stub.respondJSON("/resolve/study/5901", 200, studyMatch())

			res := callTool(t, cs, "mlwh_call_endpoint", map[string]any{
				"method":      "ResolveStudy",
				"path_params": []any{"5901"},
			})

			obj := structuredObject(res)
			So(obj["kind"], ShouldEqual, "study_lims_id")
			So(obj["canonical"], ShouldEqual, "5901")

			study, ok := obj["study"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(study["name"], ShouldEqual, "Cancer Study")

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/resolve/study/5901")
		})

		Convey("E2.1: AllStudies with pagination headers wraps the bare array with total and next_offset", func() {
			stub.respondJSONWithHeaders("/studies", 200, threeStudies()[:2], map[string][]string{
				"X-Total-Count": {"250"},
				"X-Next-Offset": {"100"},
			})

			res := callTool(t, cs, "mlwh_call_endpoint", map[string]any{
				"method":       "AllStudies",
				"query_params": map[string]any{"limit": "2", "offset": "0"},
			})

			obj := structuredObject(res)
			So(obj["total"], ShouldEqual, 250)
			So(obj["next_offset"], ShouldEqual, 100)

			studies, ok := obj["result"].([]any)
			So(ok, ShouldBeTrue)
			So(len(studies), ShouldEqual, 2)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/studies")
			So(req.Query.Get("limit"), ShouldEqual, "2")
			So(req.Query.Get("offset"), ShouldEqual, "0")
		})

		Convey("E2.3: an unknown method is a mapped tool error whose message names the method", func() {
			res := callTool(t, cs, "mlwh_call_endpoint", map[string]any{"method": "NoSuchMethod"})

			So(res.IsError, ShouldBeTrue)
			So(firstTextContent(res), ShouldContainSubstring, "NoSuchMethod")
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("E1.4: SampleDetail with no path params is a tool error indicating a path-param arity mismatch", func() {
			res := callTool(t, cs, "mlwh_call_endpoint", map[string]any{"method": "SampleDetail"})

			So(res.IsError, ShouldBeTrue)

			lower := strings.ToLower(firstTextContent(res))
			So(lower, ShouldContainSubstring, "path param")
			So(lower, ShouldContainSubstring, "expects 1")
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("E2.4: RunStatus without cache_synced_at is not given synthesized freshness", func() {
			stub.respondJSON("/run/52553/status", 200, runStatus52553())

			res := callTool(t, cs, "mlwh_call_endpoint", map[string]any{
				"method":      "RunStatus",
				"path_params": []any{"52553"},
			})

			obj := structuredObject(res)
			So(obj["id_run"], ShouldEqual, 52553)
			So(obj["current"], ShouldEqual, "qc complete")

			_, hasCacheSyncedAt := obj["cache_synced_at"]
			So(hasCacheSyncedAt, ShouldBeFalse)
		})

		Convey("E1.5: the registered tool has NO output schema (Out is any)", func() {
			tool, ok := toolByName(t, cs, "mlwh_call_endpoint")
			So(ok, ShouldBeTrue)
			So(tool.OutputSchema, ShouldBeNil)
		})

		Convey("E2.4: the description points agents to mlwh_freshness for responses without cache_synced_at", func() {
			tool, ok := toolByName(t, cs, "mlwh_call_endpoint")
			So(ok, ShouldBeTrue)
			So(tool.Description, ShouldContainSubstring, "mlwh://workflow")
			So(strings.ToLower(tool.Description), ShouldContainSubstring, "escape hatch")
			So(tool.Description, ShouldContainSubstring, "no cache_synced_at")
			So(tool.Description, ShouldContainSubstring, "mlwh_freshness")
		})
	})
}

// studyMatch is a canned study Match returned by the stub for /resolve/study/5901
// (E1.1): a Match whose kind is study_lims_id carrying the matched study, the JSON
// shape wa's RemoteClient decodes into a *wa.Match via the generic Call dispatcher.
func studyMatch() wa.Match {
	return wa.Match{
		Kind:      wa.KindStudyLimsID,
		Canonical: "5901",
		Study:     &wa.Study{IDStudyTmp: 1, IDStudyLims: "5901", Name: "Cancer Study"},
	}
}

// TestCallToolResultSizeGuard covers A2.5 at the MLWH boundary: a dynamic
// mlwh_call_endpoint response is subject to the same core result-size guard as
// typed curated tools, and the over-budget payload is not returned to the
// caller.
func TestCallToolResultSizeGuard(t *testing.T) {
	Convey("Given mlwh_call_endpoint would return a dynamic payload larger than MaxToolResultBytes", t, func() {
		stub := newStubMLWH(t)
		oversizedName := strings.Repeat("x", 1024)
		stub.respondJSON("/studies", 200, []wa.Study{{IDStudyTmp: 1, IDStudyLims: "S1", Name: oversizedName}})

		cs, cleanup := runMLWHServerWithClientOptions(t, stub, core.Options{
			MaxToolResultBytes:     200,
			ToolResultSizeGuidance: "use mlwh_count_studies or pass limit and offset",
		})
		defer cleanup()

		res := callTool(t, cs, "mlwh_call_endpoint", map[string]any{"method": "AllStudies"})

		Convey("A2.5: the result is the structured tool_result_too_large error", func() {
			obj := callSizeErrorObject(res)
			So(obj["code"], ShouldEqual, "tool_result_too_large")
			So(numericJSON(obj["limit_bytes"]), ShouldEqual, float64(200))
			So(numericJSON(obj["actual_bytes"]), ShouldBeGreaterThan, float64(200))
			So(obj["guidance"], ShouldEqual, "use mlwh_count_studies or pass limit and offset")
		})

		Convey("the dynamic payload is absent from the returned content", func() {
			So(firstTextContent(res), ShouldNotContainSubstring, oversizedName)
		})
	})
}

func callSizeErrorObject(res *mcp.CallToolResult) map[string]any {
	So(res.IsError, ShouldBeTrue)

	obj, ok := res.StructuredContent.(map[string]any)
	So(ok, ShouldBeTrue)

	return obj
}

func numericJSON(value any) float64 {
	switch n := value.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return -1
	}
}
