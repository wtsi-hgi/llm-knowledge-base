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

	. "github.com/smartystreets/goconvey/convey"
)

// TestResolveTools covers Story B1 (the seven resolve/classify tools) and
// realises Story F1.2's tool-level assertion on mlwh_resolve_sample's output
// schema. Every assertion drives a tool over the real in-memory MCP client
// against the hermetic stub.
func TestResolveTools(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the resolve/classify tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("B1.1: a sample Match (kind sanger_sample_name) for /resolve/sample/ABC123", func() {
			stub.respondJSON("/resolve/sample/ABC123", 200, sampleMatch())

			res := callTool(t, cs, "mlwh_resolve_sample", map[string]any{"identifier": "ABC123"})

			obj := structuredObject(res)
			So(obj["kind"], ShouldEqual, "sanger_sample_name")
			So(obj["canonical"], ShouldEqual, "ABC123")

			sample, ok := obj["sample"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(sample["name"], ShouldEqual, "Mus musculus A")
		})

		Convey("B1.2: a 404 not_found envelope yields a tool error indicating not found", func() {
			stub.respondError("/resolve/sample/ABC123", 404, "not_found", "no such sample")

			res := callTool(t, cs, "mlwh_resolve_sample", map[string]any{"identifier": "ABC123"})

			So(res.IsError, ShouldBeTrue)
			So(strings.ToLower(firstTextContent(res)), ShouldContainSubstring, "not found")
		})

		Convey("B1.3: a 409 ambiguous envelope yields a tool error suggesting disambiguation", func() {
			stub.respondError("/resolve/sample/ABC123", 409, "ambiguous", "matches several")

			res := callTool(t, cs, "mlwh_resolve_sample", map[string]any{"identifier": "ABC123"})

			So(res.IsError, ShouldBeTrue)
			lower := strings.ToLower(firstTextContent(res))
			So(lower, ShouldContainSubstring, "multiple records")
			So(lower, ShouldContainSubstring, "disambiguate")
		})

		Convey("B1.4: a 422 unsupported_identifier envelope yields a tool error indicating an unsupported form", func() {
			stub.respondError("/resolve/sample/ABC123", 422, "unsupported_identifier", "cannot use that")

			res := callTool(t, cs, "mlwh_resolve_sample", map[string]any{"identifier": "ABC123"})

			So(res.IsError, ShouldBeTrue)
			lower := strings.ToLower(firstTextContent(res))
			So(lower, ShouldContainSubstring, "identifier form")
			So(lower, ShouldContainSubstring, "not supported")
		})

		Convey("B1.5: all seven resolve/classify tools are registered with the listed names", func() {
			names := []string{
				"mlwh_classify_identifier",
				"mlwh_resolve_sample",
				"mlwh_resolve_sample_name",
				"mlwh_resolve_study",
				"mlwh_resolve_run",
				"mlwh_resolve_library",
				"mlwh_resolve_library_identifier",
			}

			missing := 0
			for _, name := range names {
				if _, ok := toolByName(t, cs, name); !ok {
					missing++
				}
			}

			So(missing, ShouldEqual, 0)
		})

		Convey("B1: each resolve tool's description derives from its Registry Summary/Description", func() {
			tool, ok := toolByName(t, cs, "mlwh_resolve_sample")
			So(ok, ShouldBeTrue)

			entry, found := registryEntryByMethod("ResolveSample")
			So(found, ShouldBeTrue)
			So(tool.Description, ShouldContainSubstring, entry.Summary)
			So(tool.Description, ShouldContainSubstring, entry.Description)
		})

		Convey("F1.2: mlwh_resolve_sample's output schema is non-nil, object-typed, and carries the Match field descriptions", func() {
			tool, ok := toolByName(t, cs, "mlwh_resolve_sample")
			So(ok, ShouldBeTrue)

			schema, ok := tool.OutputSchema.(map[string]any)
			So(ok, ShouldBeTrue)
			So(schema["type"], ShouldEqual, "object")

			properties, ok := schema["properties"].(map[string]any)
			So(ok, ShouldBeTrue)

			kind, ok := properties["kind"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(kind["description"], ShouldEqual, "kind of the resolved identifier")
		})
	})
}

// sampleMatch is a canned resolver result the stub returns for the resolve
// tools: a Match whose kind is sanger_sample_name carrying the matched sample,
// the JSON shape wa's RemoteClient decodes into a wa.Match.
func sampleMatch() wa.Match {
	return wa.Match{
		Kind:      wa.KindSangerSampleName,
		Canonical: "ABC123",
		Sample:    &wa.Sample{IDSampleTmp: 1, Name: "Mus musculus A", SupplierName: "supA"},
	}
}

// TestFindSamplesTool covers Story B2 (the single enum-driven mlwh_find_samples
// tool unifying the five FindSamplesBy* endpoints).
func TestFindSamplesTool(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with mlwh_find_samples", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("B2.1: one sample for /find/sample/accession/SAMEA1", func() {
			stub.respondJSON("/find/sample/accession/SAMEA1", 200, []wa.Sample{{IDSampleTmp: 7, Name: "Found One"}})

			res := callTool(t, cs, "mlwh_find_samples", map[string]any{"field": "accession", "value": "SAMEA1"})

			obj := structuredObject(res)
			samples, ok := obj["samples"].([]any)
			So(ok, ShouldBeTrue)
			So(len(samples), ShouldEqual, 1)

			first, ok := samples[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(first["name"], ShouldEqual, "Found One")
		})

		Convey("B2.2: field sanger_id routes to the hyphenated path /find/sample/sanger-id/S1", func() {
			stub.respondJSON("/find/sample/sanger-id/S1", 200, []wa.Sample{{IDSampleTmp: 8, Name: "Sanger One"}})

			callTool(t, cs, "mlwh_find_samples", map[string]any{"field": "sanger_id", "value": "S1"})

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/find/sample/sanger-id/S1")
		})

		Convey("B2.3: an invalid enum value is rejected at the schema with no HTTP request", func() {
			res := callTool(t, cs, "mlwh_find_samples", map[string]any{"field": "nonsense", "value": "x"})

			So(res.IsError, ShouldBeTrue)
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("B2.4: the field property's enum is exactly the five field names in Registry order", func() {
			tool, ok := toolByName(t, cs, "mlwh_find_samples")
			So(ok, ShouldBeTrue)

			enum := inputPropertyEnum(tool, "field")
			So(enum, ShouldResemble, []string{"sanger_id", "lims_id", "accession", "supplier_name", "library_type"})
		})
	})
}

// TestExpandTools covers Story B3 (the three expand tools).
func TestExpandTools(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the expand tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("B3.1: two TaggedIDs for /expand/study_lims_id/5901", func() {
			stub.respondJSON("/expand/study_lims_id/5901", 200, twoTaggedIDs())

			res := callTool(t, cs, "mlwh_expand_identifier", map[string]any{"kind": "study_lims_id", "canonical": "5901"})

			obj := structuredObject(res)
			tagged, ok := obj["tagged_ids"].([]any)
			So(ok, ShouldBeTrue)
			So(len(tagged), ShouldEqual, 2)

			first, ok := tagged[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(first["kind"], ShouldEqual, "study_lims_id")
			So(first["canonical"], ShouldEqual, "5901")
		})

		Convey("B3.2: an invalid kind is rejected at the schema with no HTTP request", func() {
			res := callTool(t, cs, "mlwh_expand_identifier", map[string]any{"kind": "bogus_kind", "canonical": "x"})

			So(res.IsError, ShouldBeTrue)
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("B3.3: the kind enum equals IdentifierKinds()'s string values in order", func() {
			tool, ok := toolByName(t, cs, "mlwh_expand_identifier")
			So(ok, ShouldBeTrue)

			enum := inputPropertyEnum(tool, "kind")

			kinds := wa.IdentifierKinds()
			want := make([]string, len(kinds))
			for i, kind := range kinds {
				want[i] = string(kind)
			}

			So(len(enum), ShouldEqual, 15)
			So(enum[0], ShouldEqual, "sample_uuid")
			So(enum[len(enum)-1], ShouldEqual, "id_library_lims")
			So(enum, ShouldResemble, want)
		})

		Convey("B3: mlwh_expand_search_values returns the SearchValues object", func() {
			stub.respondJSON("/expand-search/study_lims_id/5901", 200, wa.SearchValues{
				Samples: []string{"s1"},
				Runs:    []string{"r1"},
				Lanes:   []string{"l1"},
			})

			res := callTool(t, cs, "mlwh_expand_search_values", map[string]any{"kind": "study_lims_id", "canonical": "5901"})

			obj := structuredObject(res)
			samples, ok := obj["samples"].([]any)
			So(ok, ShouldBeTrue)
			So(len(samples), ShouldEqual, 1)
			So(samples[0], ShouldEqual, "s1")
		})

		Convey("B3: mlwh_expand_sample_search_values wraps the string list under values", func() {
			stub.respondJSON("/expand-sample-search/study_lims_id/5901", 200, []string{"a", "b", "c"})

			res := callTool(t, cs, "mlwh_expand_sample_search_values", map[string]any{"kind": "study_lims_id", "canonical": "5901"})

			obj := structuredObject(res)
			values, ok := obj["values"].([]any)
			So(ok, ShouldBeTrue)
			So(len(values), ShouldEqual, 3)
			So(values[0], ShouldEqual, "a")
		})
	})
}

// twoTaggedIDs is a canned expansion result the stub returns for
// mlwh_expand_identifier: two related canonical identifiers, the JSON array
// shape wa's RemoteClient decodes into []wa.TaggedID.
func twoTaggedIDs() []wa.TaggedID {
	return []wa.TaggedID{
		{Kind: wa.KindStudyLimsID, Canonical: "5901"},
		{Kind: wa.KindStudyAccession, Canonical: "EGAS00001000001"},
	}
}

// inputPropertyEnum reads the enum of a named string property from a registered
// tool's input schema as a []string, asserting the schema is an object whose
// property carries an enum. It lets a test prove the code-sourced enum reached
// the input schema the SDK validates against (Stories B2.4 and B3.3).
func inputPropertyEnum(tool *mcp.Tool, property string) []string {
	schema, ok := tool.InputSchema.(map[string]any)
	So(ok, ShouldBeTrue)

	properties, ok := schema["properties"].(map[string]any)
	So(ok, ShouldBeTrue)

	prop, ok := properties[property].(map[string]any)
	So(ok, ShouldBeTrue)

	rawEnum, ok := prop["enum"].([]any)
	So(ok, ShouldBeTrue)

	enum := make([]string, len(rawEnum))
	for i, value := range rawEnum {
		str, ok := value.(string)
		So(ok, ShouldBeTrue)
		enum[i] = str
	}

	return enum
}
