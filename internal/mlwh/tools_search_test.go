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
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/smartystreets/goconvey/convey"
)

// TestSearchToolsErrorMapping proves an upstream error reaching the search tools
// is surfaced as a mapped tool error (IsError) rather than a protocol error,
// exercising the stub's error-envelope path end-to-end.
func TestSearchToolsErrorMapping(t *testing.T) {
	Convey("Given a stub that returns a 503 cache-never-synced envelope", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		stub.respondError("/search/sample/mus", 503, "cache_never_synced", "cache not synced")

		res := callTool(t, cs, "mlwh_search_samples", map[string]any{"term": "mus"})

		So(res.IsError, ShouldBeTrue)
		So(strings.ToLower(firstTextContent(res)), ShouldContainSubstring, "synced")
	})
}

// TestSearchSamplesTool covers Story A1 (mlwh_search_samples) and the
// end-to-end half of Story F2 (F2.1). Every assertion drives the tool over the
// real in-memory MCP client against the hermetic stub.
func TestSearchSamplesTool(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with mlwh_search_samples", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("A1.1/A1.2/A3.4/F2.1: two samples for /search/sample/mus with header page metadata", func() {
			stub.respondJSONWithHeaders("/search/sample/mus", 200, twoSamples(), http.Header{
				"X-Total-Count": {"7"},
				"X-Next-Offset": {"-1"},
			})

			res := callTool(t, cs, "mlwh_search_samples", map[string]any{"term": "mus"})

			Convey("A1.1/A3.4: StructuredContent holds samples, total, and next_offset", func() {
				obj := structuredObject(res)
				So(len(obj), ShouldEqual, 3)

				samples, ok := obj["samples"].([]any)
				So(ok, ShouldBeTrue)
				So(len(samples), ShouldEqual, 2)
				So(obj["total"], ShouldEqual, 7)
				So(obj["next_offset"], ShouldEqual, -1)

				first, ok := samples[0].(map[string]any)
				So(ok, ShouldBeTrue)
				So(first["name"], ShouldEqual, "Mus musculus A")
			})

			Convey("F2.1: StructuredContent is an object {\"samples\":[...]}, not a bare array", func() {
				_, isArray := res.StructuredContent.([]any)
				So(isArray, ShouldBeFalse)

				obj := structuredObject(res)
				_, hasSamples := obj["samples"]
				So(hasSamples, ShouldBeTrue)
			})

			Convey("A1.2: Content has one text block whose JSON parses to the same two samples", func() {
				text := firstTextContent(res)
				So(text, ShouldNotBeBlank)

				var decoded samplesResult
				err := json.Unmarshal([]byte(text), &decoded)
				So(err, ShouldBeNil)
				So(len(decoded.Samples), ShouldEqual, 2)
				So(decoded.Samples[0].Name, ShouldEqual, "Mus musculus A")
				So(decoded.Samples[1].Name, ShouldEqual, "Mus musculus B")
			})
		})

		Convey("A1.3: term \"mu\" (length 2) is a tool error mentioning the 3-char minimum, no request made", func() {
			res := callTool(t, cs, "mlwh_search_samples", map[string]any{"term": "mu"})

			So(res.IsError, ShouldBeTrue)
			So(strings.ToLower(firstTextContent(res)), ShouldContainSubstring, "3")
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("A1.4: limit 1001 is a tool error mentioning the 1000 maximum, no request made", func() {
			res := callTool(t, cs, "mlwh_search_samples", map[string]any{"term": "mus", "limit": 1001})

			So(res.IsError, ShouldBeTrue)
			So(firstTextContent(res), ShouldContainSubstring, "1000")
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("A1.5: limit=5, offset=10 reach the stub as query parameters", func() {
			stub.respondJSON("/search/sample/mus", 200, twoSamples())

			callTool(t, cs, "mlwh_search_samples", map[string]any{"term": "mus", "limit": 5, "offset": 10})

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/search/sample/mus")
			So(req.Query.Get("limit"), ShouldEqual, "5")
			So(req.Query.Get("offset"), ShouldEqual, "10")
		})

		Convey("A1.6: no limit/offset sends limit=100 and offset=0", func() {
			stub.respondJSON("/search/sample/mus", 200, twoSamples())

			callTool(t, cs, "mlwh_search_samples", map[string]any{"term": "mus"})

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Query.Get("limit"), ShouldEqual, "100")
			So(req.Query.Get("offset"), ShouldEqual, "0")
		})

		Convey("A1.7: the tool name and description name word-prefix, the minimum 3, and 1000", func() {
			tool, ok := toolByName(t, cs, "mlwh_search_samples")
			So(ok, ShouldBeTrue)
			So(tool.Name, ShouldEqual, "mlwh_search_samples")

			lower := strings.ToLower(tool.Description)
			So(lower, ShouldContainSubstring, "word-prefix")
			So(lower, ShouldContainSubstring, "minimum")
			So(tool.Description, ShouldContainSubstring, "3")
			So(tool.Description, ShouldContainSubstring, "1000")
			So(lower, ShouldContainSubstring, "supplier_name")
		})
	})
}

// twoSamples and threeStudies are canned upstream payloads. They are returned by
// the stub as the JSON shapes wa's RemoteClient decodes (a JSON array of Sample
// / Study), so a green test also proves the harness response shape round-trips
// through the real typed client.
func twoSamples() []wa.Sample {
	return []wa.Sample{
		{IDSampleTmp: 1, Name: "Mus musculus A", SupplierName: "supA"},
		{IDSampleTmp: 2, Name: "Mus musculus B", SupplierName: "supB"},
	}
}

// TestCountSamplesTool covers Story A2 (mlwh_count_samples).
func TestCountSamplesTool(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with mlwh_count_samples", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("A2.1: {\"count\":42} for /search/sample/mus/count", func() {
			stub.respondJSON("/search/sample/mus/count", 200, wa.Count{Count: 42})

			res := callTool(t, cs, "mlwh_count_samples", map[string]any{"term": "mus"})

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, 42)
		})

		Convey("A2.2: {\"count\":10000} (term \"abc\") and the description explains the floor", func() {
			stub.respondJSON("/search/sample/abc/count", 200, wa.Count{Count: 10000})

			res := callTool(t, cs, "mlwh_count_samples", map[string]any{"term": "abc"})

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, 10000)

			tool, ok := toolByName(t, cs, "mlwh_count_samples")
			So(ok, ShouldBeTrue)
			So(tool.Description, ShouldContainSubstring, "10000")
			So(strings.ToLower(tool.Description), ShouldContainSubstring, "at least")
		})

		Convey("A2.3: term \"ab\" is a tool error mentioning the 3-char minimum, no request made", func() {
			res := callTool(t, cs, "mlwh_count_samples", map[string]any{"term": "ab"})

			So(res.IsError, ShouldBeTrue)
			So(firstTextContent(res), ShouldContainSubstring, "3")
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("A2.4: the tool name and description contain 10000 and \"at least\"", func() {
			tool, ok := toolByName(t, cs, "mlwh_count_samples")
			So(ok, ShouldBeTrue)
			So(tool.Name, ShouldEqual, "mlwh_count_samples")
			So(tool.Description, ShouldContainSubstring, "10000")
			So(strings.ToLower(tool.Description), ShouldContainSubstring, "at least")
		})
	})
}

// TestSearchStudiesTool covers Story A3 (mlwh_search_studies) and D1
// (study-name/id lookup through bounded search results).
func TestSearchStudiesTool(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with mlwh_search_studies", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("A3.1/D1.1: cancer study-name lookup returns bounded candidate rows with page metadata", func() {
			stub.respondJSONWithHeaders("/search/study/cancer", 200, twoCancerStudies(), http.Header{
				"X-Total-Count": {"2"},
				"X-Next-Offset": {"-1"},
			})

			res := callTool(t, cs, "mlwh_search_studies", map[string]any{"term": "cancer"})

			obj := structuredObject(res)
			studies, ok := obj["studies"].([]any)
			So(ok, ShouldBeTrue)
			So(len(studies), ShouldEqual, 2)
			So(obj["total"], ShouldEqual, 2)
			So(obj["next_offset"], ShouldEqual, -1)

			first, ok := studies[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(first["id_study_lims"], ShouldEqual, "S1")
			So(first["name"], ShouldEqual, "Cancer One")
			So(first["study_title"], ShouldEqual, "Tumour WGS")
			So(first["programme"], ShouldEqual, "Cancer")
			So(first["faculty_sponsor"], ShouldEqual, "Carl")
			So(first["accession_number"], ShouldEqual, "ERP1")

			second, ok := studies[1].(map[string]any)
			So(ok, ShouldBeTrue)
			So(second["id_study_lims"], ShouldEqual, "S2")
			So(second["name"], ShouldEqual, "Cancer Two")
			So(second["study_title"], ShouldEqual, "Tumour RNA")
			So(second["programme"], ShouldEqual, "Cancer")
			So(second["faculty_sponsor"], ShouldEqual, "Carla")
			So(second["accession_number"], ShouldEqual, "ERP2")

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/search/study/cancer")
			So(req.Query.Get("limit"), ShouldEqual, "100")
			So(req.Query.Get("offset"), ShouldEqual, "0")
		})

		Convey("A3.2: term \"ab\" is a tool error mentioning the 3-char minimum, no request made", func() {
			res := callTool(t, cs, "mlwh_search_studies", map[string]any{"term": "ab"})

			So(res.IsError, ShouldBeTrue)
			So(firstTextContent(res), ShouldContainSubstring, "3")
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("A3.3: term \"xyz\" with limit 1001 is a tool error mentioning the 1000 maximum, no request made", func() {
			res := callTool(t, cs, "mlwh_search_studies", map[string]any{"term": "xyz", "limit": 1001})

			So(res.IsError, ShouldBeTrue)
			So(firstTextContent(res), ShouldContainSubstring, "1000")
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("A3.4/D1.2: the tool name and description explain substring study-name/id lookup", func() {
			tool, ok := toolByName(t, cs, "mlwh_search_studies")
			So(ok, ShouldBeTrue)
			So(tool.Name, ShouldEqual, "mlwh_search_studies")

			lower := strings.ToLower(tool.Description)
			So(lower, ShouldContainSubstring, "substring")
			So(lower, ShouldContainSubstring, "study-name/id lookup")
			So(lower, ShouldContainSubstring, "what study id matches this name?")
			So(lower, ShouldContainSubstring, "name")
			So(lower, ShouldContainSubstring, "study_title")
			So(lower, ShouldContainSubstring, "programme")
			So(lower, ShouldContainSubstring, "faculty_sponsor")
			So(lower, ShouldContainSubstring, "disambiguate candidate study ids")
			So(tool.Description, ShouldContainSubstring, "3")
			So(tool.Description, ShouldContainSubstring, "1000")
		})
	})
}

func threeStudies() []wa.Study {
	return []wa.Study{
		{IDStudyTmp: 10, Name: "Cancer One", StudyTitle: "A cancer study"},
		{IDStudyTmp: 11, Name: "Cancer Two", StudyTitle: "Another cancer study"},
		{IDStudyTmp: 12, Name: "Cancer Three", StudyTitle: "Yet another cancer study"},
	}
}

func twoCancerStudies() []wa.Study {
	return []wa.Study{
		{
			IDStudyTmp:      10,
			IDStudyLims:     "S1",
			Name:            "Cancer One",
			StudyTitle:      "Tumour WGS",
			Programme:       "Cancer",
			FacultySponsor:  "Carl",
			AccessionNumber: "ERP1",
		},
		{
			IDStudyTmp:      11,
			IDStudyLims:     "S2",
			Name:            "Cancer Two",
			StudyTitle:      "Tumour RNA",
			Programme:       "Cancer",
			FacultySponsor:  "Carla",
			AccessionNumber: "ERP2",
		},
	}
}

// TestCountStudiesTools covers Story A4 (mlwh_count_studies_search and the
// no-input mlwh_count_studies).
func TestCountStudiesTools(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the study-count tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("A4.1: {\"count\":7} for /search/study/abc/count", func() {
			stub.respondJSON("/search/study/abc/count", 200, wa.Count{Count: 7})

			res := callTool(t, cs, "mlwh_count_studies_search", map[string]any{"term": "abc"})

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, 7)
		})

		Convey("A4.2: mlwh_count_studies_search with term \"ab\" is a tool error mentioning the minimum, no request made", func() {
			res := callTool(t, cs, "mlwh_count_studies_search", map[string]any{"term": "ab"})

			So(res.IsError, ShouldBeTrue)
			So(firstTextContent(res), ShouldContainSubstring, "3")
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("A4.3: mlwh_count_studies with {} returns {\"count\":1234} from /studies/count", func() {
			stub.respondJSON("/studies/count", 200, wa.Count{Count: 1234})

			res := callTool(t, cs, "mlwh_count_studies", map[string]any{})

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, 1234)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/studies/count")
		})

		Convey("mlwh_count_studies takes empty input: its input schema is an object", func() {
			tool, ok := toolByName(t, cs, "mlwh_count_studies")
			So(ok, ShouldBeTrue)

			schema, ok := tool.InputSchema.(map[string]any)
			So(ok, ShouldBeTrue)
			So(schema["type"], ShouldEqual, "object")
		})
	})
}

// structuredObject asserts the result is a successful, object-typed structured
// result (not a bare array) and returns the decoded object. MCP requires
// StructuredContent to be a JSON object; on the client it arrives as a
// map[string]any.
func structuredObject(res *mcp.CallToolResult) map[string]any {
	So(res.IsError, ShouldBeFalse)

	obj, ok := res.StructuredContent.(map[string]any)
	So(ok, ShouldBeTrue)

	return obj
}
