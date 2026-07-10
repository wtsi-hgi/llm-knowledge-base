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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

func TestExportToolValidationErrorsB1(t *testing.T) {
	Convey("B1.5: Given relationship, column, filter, and parent validation errors, their actionable messages survive MCP mapping", t, func() {
		stub := newStubMLWH(t)
		stub.respondError("/export/products/study/MISSING", http.StatusNotFound, "not_found", "parent MISSING does not exist")
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		cases := []struct {
			name      string
			arguments map[string]any
			message   string
		}{
			{
				name: "relationship",
				arguments: map[string]any{
					"children": "runs", "parent_kind": "library", "parent_id": "LIB1",
				},
				message: "unsupported export relationship",
			},
			{
				name: "column",
				arguments: map[string]any{
					"children": "products", "parent_kind": "study", "parent_id": "S1",
					"columns": []any{"email"},
				},
				message: "unknown export column",
			},
			{
				name: "filter",
				arguments: map[string]any{
					"children": "products", "parent_kind": "study", "parent_id": "S1",
					"sort": "created-desc",
				},
				message: "supported only for iRODS exports",
			},
			{
				name: "parent",
				arguments: map[string]any{
					"children": "products", "parent_kind": "study", "parent_id": "MISSING",
				},
				message: "parent MISSING does not exist",
			},
		}

		for _, testCase := range cases {
			result := callTool(t, cs, "mlwh_export", testCase.arguments)
			So(result.IsError, ShouldBeTrue)
			So(firstTextContent(result), ShouldContainSubstring, testCase.message)
		}
	})
}

func TestExportToolContinuationValidationB2(t *testing.T) {
	Convey("B2.5: Given a cursor on samples or created-desc iRODS, upstream validation returns an actionable unsupported-input tool error", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		cases := []struct {
			arguments map[string]any
			message   string
		}{
			{
				arguments: map[string]any{
					"children": "samples", "parent_kind": "study", "parent_id": "S1",
					"limit": 10, "cursor": "opaque",
				},
				message: "cursor",
			},
			{
				arguments: map[string]any{
					"children": "irods", "parent_kind": "study", "parent_id": "S1",
					"limit": 10, "sort": "created-desc", "cursor": "opaque",
				},
				message: "bounded limit/offset pages",
			},
		}

		for _, testCase := range cases {
			result := callTool(t, cs, "mlwh_export", testCase.arguments)
			So(result.IsError, ShouldBeTrue)
			So(strings.ToLower(firstTextContent(result)), ShouldContainSubstring, testCase.message)
			So(strings.ToLower(firstTextContent(result)), ShouldContainSubstring, "not supported")
		}
		So(stub.requestCount(), ShouldEqual, 0)
	})
}

func TestExportToolMaterializesAllB2(t *testing.T) {
	Convey("B2.6: Given all=true with three streamed rows, the tool materializes the complete matrix", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSON("/export/products/study/S1", http.StatusOK, wa.ExportResult{
			Columns: []string{"name"}, Rows: [][]string{{"S1"}, {"S2"}, {"S3"}},
			Total: 3, Complete: true, Format: "tsv",
		})
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		result := structuredObject(callTool(t, cs, "mlwh_export", map[string]any{
			"children": "products", "parent_kind": "study", "parent_id": "S1", "all": true,
		}))

		So(result["Rows"], ShouldResemble, []any{[]any{"S1"}, []any{"S2"}, []any{"S3"}})
		So(result["Total"], ShouldEqual, -1)
		So(result["Complete"], ShouldBeTrue)
		So(result["NextCursor"], ShouldBeEmpty)

		request, ok := stub.lastRequest()
		So(ok, ShouldBeTrue)
		So(request.Query, ShouldResemble, url.Values{"limit": {"1000"}})
	})
}

func TestExportToolMaterializationCancellationB2(t *testing.T) {
	Convey("B2.7: Given cancellation during streamed materialization, no successful partial matrix is returned", t, func() {
		stub := newStubMLWH(t)
		secondRequestStarted := make(chan struct{})
		var requestNumber atomic.Int32
		stub.respondHandler("/export/products/study/S1", func(w http.ResponseWriter, request *http.Request) {
			if requestNumber.Add(1) == 1 {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(wa.ExportResult{
					Columns: []string{"name"}, Rows: [][]string{{"S1"}}, Total: 2,
					NextCursor: "next", Complete: false, Format: "tsv",
				})

				return
			}

			close(secondRequestStarted)
			<-request.Context().Done()
		})
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		type callOutcome struct {
			result *mcp.CallToolResult
			err    error
		}
		callDone := make(chan callOutcome, 1)
		go func() {
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{
				Name: "mlwh_export",
				Arguments: map[string]any{
					"children": "products", "parent_kind": "study", "parent_id": "S1", "all": true,
				},
			})
			callDone <- callOutcome{result: result, err: err}
		}()

		select {
		case <-secondRequestStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("streamed export did not request its second page")
		}
		cancel()

		var outcome callOutcome
		select {
		case outcome = <-callDone:
		case <-time.After(5 * time.Second):
			t.Fatal("cancelled streamed export did not return")
		}

		So(outcome.result, ShouldBeNil)
		So(outcome.err, ShouldNotBeNil)
		So(errors.Is(outcome.err, context.Canceled), ShouldBeTrue)
	})

	Convey("Given an iterator error after a streamed row, no successful partial matrix is returned", t, func() {
		stub := newStubMLWH(t)
		var requestNumber atomic.Int32
		stub.respondHandler("/export/products/study/S1", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if requestNumber.Add(1) == 1 {
				_ = json.NewEncoder(w).Encode(wa.ExportResult{
					Columns: []string{"name"}, Rows: [][]string{{"partial-row"}}, Total: 2,
					NextCursor: "next", Complete: false, Format: "tsv",
				})

				return
			}

			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(httpErrorBody{Code: "upstream_impaired", Message: "stream page failed"})
		})
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		result := callTool(t, cs, "mlwh_export", map[string]any{
			"children": "products", "parent_kind": "study", "parent_id": "S1", "all": true,
		})

		So(result.IsError, ShouldBeTrue)
		So(firstTextContent(result), ShouldContainSubstring, "stream page failed")
		So(firstTextContent(result), ShouldNotContainSubstring, "partial-row")
	})
}

func TestExportToolMaterializedResultSizeGuardB2(t *testing.T) {
	Convey("B2.8: Given a materialized export above the core byte guard, guidance recommends a smaller limit and the export continuations", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSON("/export/products/study/S1", http.StatusOK, wa.ExportResult{
			Columns: []string{"name"}, Rows: [][]string{{strings.Repeat("x", 1024)}},
			Total: 1, Complete: true, Format: "tsv",
		})
		cs, cleanup := runMLWHServerWithClientOptions(t, stub, core.Options{
			MaxToolResultBytes:     250,
			ToolResultSizeGuidance: ToolResultSizeGuidance,
		})
		defer cleanup()

		result := callTool(t, cs, "mlwh_export", map[string]any{
			"children": "products", "parent_kind": "study", "parent_id": "S1", "all": true,
		})
		object := callSizeErrorObject(result)
		guidance := object["guidance"].(string)

		So(object["code"], ShouldEqual, "tool_result_too_large")
		So(strings.ToLower(guidance), ShouldContainSubstring, "smaller limit")
		So(guidance, ShouldContainSubstring, "Total")
		So(guidance, ShouldContainSubstring, "NextCursor")
		So(strings.ToLower(guidance), ShouldContainSubstring, "offset")
		So(strings.ToLower(guidance), ShouldNotContainSubstring, "count_export")
		So(strings.ToLower(guidance), ShouldNotContainSubstring, "count export")
	})
}

type exportRelationshipCaseForTest struct {
	arguments map[string]any
	path      string
}

func exportRelationshipCasesForTest() []exportRelationshipCaseForTest {
	cases := []exportRelationshipCaseForTest{}
	for _, relationship := range wa.ExportRelationshipDescriptions() {
		childrenValues := append([]string{relationship.Children}, relationship.Aliases...)
		for _, children := range childrenValues {
			for _, parentKind := range relationship.ParentKinds {
				cases = append(cases, exportRelationshipCaseForTest{
					arguments: map[string]any{
						"children": children, "parent_kind": parentKind, "parent_id": "PARENT",
					},
					path: "/export/" + children + "/" + parentKind + "/PARENT",
				})
			}
		}
	}

	return cases
}

func TestExportToolPreservesProductGrainB3(t *testing.T) {
	Convey("B3.1: Given three products and two iRODS attachments, all product rows and the upstream Total are preserved", t, func() {
		stub := newStubMLWH(t)
		columns := []string{"id_run", "lane", "tag_index", "irods_path"}
		rows := [][]string{
			{"61010", "1", "1", "/seq/61010_1#1.cram"},
			{"61010", "1", "2", "/seq/61010_1#2.cram"},
			{"61010", "1", "3", ""},
		}
		stub.respondJSON("/export/products/study/S1", http.StatusOK, exportResultForTest(columns, rows))
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		result := structuredObject(callTool(t, cs, "mlwh_export", map[string]any{
			"children": "products", "parent_kind": "study", "parent_id": "S1",
			"columns": []any{"id_run", "lane", "tag_index", "irods_path"},
		}))

		So(result["Columns"], ShouldResemble, []any{"id_run", "lane", "tag_index", "irods_path"})
		So(result["Rows"], ShouldResemble, []any{
			[]any{"61010", "1", "1", "/seq/61010_1#1.cram"},
			[]any{"61010", "1", "2", "/seq/61010_1#2.cram"},
			[]any{"61010", "1", "3", ""},
		})
		So(result["Total"], ShouldEqual, 3)
		So(result, ShouldNotContainKey, "products_without_irods")
		So(result, ShouldNotContainKey, "cache_synced_at")
	})
}

func TestExportToolProductFileTypeOnlyChangesAttachmentB3(t *testing.T) {
	Convey("B3.2: Given file_type=bam with no BAM attachment, all product rows and Total remain while paths are empty", t, func() {
		stub := newStubMLWH(t)
		columns := []string{"id_run", "lane", "tag_index", "irods_path"}
		rows := [][]string{
			{"61010", "1", "1", ""},
			{"61010", "1", "2", ""},
			{"61010", "1", "3", ""},
		}
		stub.respondJSON("/export/products/study/S1", http.StatusOK, exportResultForTest(columns, rows))
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		result := structuredObject(callTool(t, cs, "mlwh_export", map[string]any{
			"children": "products", "parent_kind": "study", "parent_id": "S1",
			"columns": []any{"id_run", "lane", "tag_index", "irods_path"}, "file_type": "bam",
		}))

		So(result["Rows"], ShouldResemble, []any{
			[]any{"61010", "1", "1", ""},
			[]any{"61010", "1", "2", ""},
			[]any{"61010", "1", "3", ""},
		})
		So(result["Total"], ShouldEqual, 3)
		request, ok := stub.lastRequest()
		So(ok, ShouldBeTrue)
		So(request.Query, ShouldResemble, url.Values{
			"columns": {"id_run,lane,tag_index,irods_path"}, "file_type": {"bam"},
		})
	})
}

func TestExportToolProductDeliverabilityTriStateB3(t *testing.T) {
	Convey("B3.3: Given control and deliverable products, omitted, false, and true preserve upstream row selection", t, func() {
		stub := newStubMLWH(t)
		stub.respondHandler("/export/products/study/S1", func(w http.ResponseWriter, request *http.Request) {
			rows := [][]string{{"deliverable"}, {"control"}}
			if request.URL.Query().Get("deliverables_only") == "true" {
				rows = rows[:1]
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(exportResultForTest([]string{"name"}, rows))
		})
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		cases := []struct {
			name         string
			argument     any
			includeValue bool
			rows         []any
		}{
			{name: "omitted", rows: []any{[]any{"deliverable"}, []any{"control"}}},
			{name: "false", argument: false, includeValue: true, rows: []any{[]any{"deliverable"}, []any{"control"}}},
			{name: "true", argument: true, includeValue: true, rows: []any{[]any{"deliverable"}}},
		}

		for _, testCase := range cases {
			arguments := map[string]any{
				"children": "products", "parent_kind": "study", "parent_id": "S1",
				"columns": []any{"name"},
			}
			if testCase.includeValue {
				arguments["deliverables_only"] = testCase.argument
			}

			result := structuredObject(callTool(t, cs, "mlwh_export", arguments))
			So(result["Rows"], ShouldResemble, testCase.rows)
			So(result["Total"], ShouldEqual, len(testCase.rows))

			request, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			if testCase.includeValue {
				So(request.Query.Get("deliverables_only"), ShouldEqual, fmt.Sprint(testCase.argument))
			} else {
				So(request.Query.Has("deliverables_only"), ShouldBeFalse)
			}
		}
	})
}

func TestExportToolPreservesProductQCValuesB3(t *testing.T) {
	Convey("B3.4: Given product QC values, manual_qc projection and qc=fail preserve upstream roll-up values and filtering", t, func() {
		stub := newStubMLWH(t)
		stub.respondHandler("/export/products/study/S1", func(w http.ResponseWriter, request *http.Request) {
			rows := [][]string{{"pass"}, {"fail"}, {"pending"}}
			if request.URL.Query().Get("qc") == "fail" {
				rows = rows[1:2]
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(exportResultForTest([]string{"manual_qc"}, rows))
		})
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		unfiltered := structuredObject(callTool(t, cs, "mlwh_export", map[string]any{
			"children": "products", "parent_kind": "study", "parent_id": "S1",
			"columns": []any{"manual_qc"},
		}))
		So(unfiltered["Columns"], ShouldResemble, []any{"manual_qc"})
		So(unfiltered["Rows"], ShouldResemble, []any{[]any{"pass"}, []any{"fail"}, []any{"pending"}})

		failed := structuredObject(callTool(t, cs, "mlwh_export", map[string]any{
			"children": "products", "parent_kind": "study", "parent_id": "S1",
			"columns": []any{"manual_qc"}, "qc": "fail",
		}))
		So(failed["Columns"], ShouldResemble, []any{"manual_qc"})
		So(failed["Rows"], ShouldResemble, []any{[]any{"fail"}})
		So(failed["Total"], ShouldEqual, 1)
		request, ok := stub.lastRequest()
		So(ok, ShouldBeTrue)
		So(request.Query, ShouldResemble, url.Values{"columns": {"manual_qc"}, "qc": {"fail"}})
	})
}

func TestExportToolPreservesMergedMultilaneReasonB3(t *testing.T) {
	Convey("B3.5: Given a product represented only by a merged multi-lane CRAM, the exact upstream gap row is preserved", t, func() {
		stub := newStubMLWH(t)
		columns := []string{"irods_path", "irods_unmatched", "reason"}
		rows := [][]string{{"", "true", "merged_multilane"}}
		stub.respondJSON("/export/products/study/S1", http.StatusOK, exportResultForTest(columns, rows))
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		result := structuredObject(callTool(t, cs, "mlwh_export", map[string]any{
			"children": "products", "parent_kind": "study", "parent_id": "S1",
			"columns": []any{"irods_path", "irods_unmatched", "reason"}, "file_type": "cram",
		}))

		So(result["Columns"], ShouldResemble, []any{"irods_path", "irods_unmatched", "reason"})
		So(result["Rows"], ShouldResemble, []any{[]any{"", "true", "merged_multilane"}})
		So(result["Total"], ShouldEqual, 1)
	})
}

func TestExportToolFileRelationshipDefaultsB3(t *testing.T) {
	Convey("B3.6: Given iRODS and sample-CRAM exports, omitted file options preserve upstream CRAM/deliverable defaults and false includes controls", t, func() {
		stub := newStubMLWH(t)
		paths := []string{"/export/irods/study/S1", "/export/sample-crams/study/S1"}
		for _, path := range paths {
			stub.respondHandler(path, func(w http.ResponseWriter, request *http.Request) {
				rows := [][]string{{"/seq/deliverable.cram"}}
				if request.URL.Query().Get("deliverables_only") == "false" {
					rows = append(rows, []string{"/seq/control.cram"})
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(exportResultForTest([]string{"irods_path"}, rows))
			})
		}
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		for _, children := range []string{"irods", "sample-crams"} {
			arguments := map[string]any{
				"children": children, "parent_kind": "study", "parent_id": "S1",
				"columns": []any{"irods_path"},
			}
			defaults := structuredObject(callTool(t, cs, "mlwh_export", arguments))
			So(defaults["Rows"], ShouldResemble, []any{[]any{"/seq/deliverable.cram"}})
			request, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(request.Query.Has("file_type"), ShouldBeFalse)
			So(request.Query.Has("deliverables_only"), ShouldBeFalse)

			arguments["deliverables_only"] = false
			withControls := structuredObject(callTool(t, cs, "mlwh_export", arguments))
			So(withControls["Rows"], ShouldResemble, []any{
				[]any{"/seq/deliverable.cram"}, []any{"/seq/control.cram"},
			})
			request, ok = stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(request.Query.Get("deliverables_only"), ShouldEqual, "false")
			So(request.Query.Has("file_type"), ShouldBeFalse)
		}
	})
}

func TestExportToolRelationshipSpecificSharedFiltersB3(t *testing.T) {
	Convey("B3.7: Given valid relationship-specific filters, exact queries/results are preserved and unsupported pairs are rejected", t, func() {
		stub := newStubMLWH(t)
		irodsRows := [][]string{{"mouse-sample", "pass", "2026-07-01T08:30:00Z", "/seq/mouse.cram"}}
		stub.respondJSON("/export/irods/study/S1", http.StatusOK, exportResultForTest(
			[]string{"name", "manual_qc", "created", "irods_path"}, irodsRows,
		))
		stub.respondJSON("/export/users/study/S1", http.StatusOK, exportResultForTest(
			[]string{"role", "name"}, [][]string{{"owner", "Ada Owner"}},
		))
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		irods := structuredObject(callTool(t, cs, "mlwh_export", map[string]any{
			"children": "irods", "parent_kind": "study", "parent_id": "S1",
			"columns":   []any{"name", "manual_qc", "created", "irods_path"},
			"file_type": "cram", "deliverables_only": false, "qc": "pass",
			"library_type": "Standard", "organism": "musculus", "sort": "created_desc",
			"since": "2026-07-01T00:00:00Z", "until": "2026-07-02T00:00:00Z",
		}))
		So(irods["Rows"], ShouldResemble, []any{
			[]any{"mouse-sample", "pass", "2026-07-01T08:30:00Z", "/seq/mouse.cram"},
		})
		request, ok := stub.lastRequest()
		So(ok, ShouldBeTrue)
		So(request.Query, ShouldResemble, url.Values{
			"columns":           {"name,manual_qc,created,irods_path"},
			"file_type":         {"cram"},
			"deliverables_only": {"false"},
			"qc":                {"pass"},
			"library_type":      {"Standard"},
			"organism":          {"musculus"},
			"sort":              {"created_desc"},
			"since":             {"2026-07-01T00:00:00Z"},
			"until":             {"2026-07-02T00:00:00Z"},
		})

		users := structuredObject(callTool(t, cs, "mlwh_export", map[string]any{
			"children": "users", "parent_kind": "study", "parent_id": "S1",
			"columns": []any{"role", "name"}, "role": "owner,manager",
		}))
		So(users["Rows"], ShouldResemble, []any{[]any{"owner", "Ada Owner"}})
		request, ok = stub.lastRequest()
		So(ok, ShouldBeTrue)
		So(request.Query, ShouldResemble, url.Values{
			"columns": {"role,name"}, "role": {"owner,manager"},
		})

		requestCount := stub.requestCount()
		unsupported := callTool(t, cs, "mlwh_export", map[string]any{
			"children": "runs", "parent_kind": "study", "parent_id": "S1", "qc": "pass",
		})
		So(unsupported.IsError, ShouldBeTrue)
		So(firstTextContent(unsupported), ShouldContainSubstring,
			"shared sample export filters are backed for iRODS/files, samples, sample-crams, and products exports")
		So(stub.requestCount(), ShouldEqual, requestCount)
	})
}

func exportResultForTest(columns []string, rows [][]string) wa.ExportResult {
	return wa.ExportResult{
		Columns: columns, Rows: rows, Total: len(rows), Complete: true, Format: "tsv",
	}
}

func TestExportToolContractB1(t *testing.T) {
	Convey("B1.1: Given registered tools, mlwh_export exposes the exact export input contract and no count twin", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		tool, ok := toolByName(t, cs, "mlwh_export")
		So(ok, ShouldBeTrue)

		_, hasCount := toolByName(t, cs, "mlwh_count_export")
		So(hasCount, ShouldBeFalse)

		schema, ok := tool.InputSchema.(map[string]any)
		So(ok, ShouldBeTrue)
		So(schema["type"], ShouldEqual, "object")
		So(schema["additionalProperties"], ShouldEqual, false)
		So(stringSet(schema["required"]), ShouldResemble, map[string]bool{
			"children": true, "parent_kind": true, "parent_id": true,
		})

		properties, ok := schema["properties"].(map[string]any)
		So(ok, ShouldBeTrue)
		So(stringSetFromKeys(properties), ShouldResemble, map[string]bool{
			"children": true, "parent_kind": true, "parent_id": true,
			"columns": true, "file_type": true, "deliverables_only": true,
			"role": true, "qc": true, "library_type": true,
			"organism": true, "sort": true, "since": true, "until": true,
			"limit": true, "offset": true, "all": true, "cursor": true,
			"format": true,
		})
	})
}

func stringSet(raw any) map[string]bool {
	values, _ := raw.([]any)
	set := make(map[string]bool, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			set[text] = true
		}
	}

	return set
}

func TestExportToolBoundedMatricesB2(t *testing.T) {
	Convey("B2.1: Given a chosen-column bounded matrix, the structured result has exactly the upstream fields and aligned rows", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSON("/export/products/study/S1", http.StatusOK, wa.ExportResult{
			Columns: []string{"name", "irods_path"}, Rows: [][]string{{"S1", "/a.cram"}},
			Total: 1, Complete: true, Format: "tsv",
		})
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		result := callTool(t, cs, "mlwh_export", map[string]any{
			"children": "products", "parent_kind": "study", "parent_id": "S1",
			"columns": []any{"name", "irods_path"},
		})
		object := structuredObject(result)

		So(stringSetFromKeys(object), ShouldResemble, map[string]bool{
			"Columns": true, "Rows": true, "Total": true, "NextCursor": true,
			"Complete": true, "Format": true,
		})
		So(object["Columns"], ShouldResemble, []any{"name", "irods_path"})
		So(object["Rows"], ShouldResemble, []any{[]any{"S1", "/a.cram"}})
	})

	Convey("B2.2: Given an omitted product limit, the request uses the bounded 1000-row upstream default and never all=true", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSON("/export/products/study/S1", http.StatusOK, wa.ExportResult{
			Columns: []string{"name"}, Rows: [][]string{{"S1"}}, Total: 1500,
			NextCursor: "opaque-next", Complete: false, Format: "tsv",
		})
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		result := callTool(t, cs, "mlwh_export", map[string]any{
			"children": "products", "parent_kind": "study", "parent_id": "S1",
		})
		So(result.IsError, ShouldBeFalse)

		request, ok := stub.lastRequest()
		So(ok, ShouldBeTrue)
		So(request.Query.Has("limit"), ShouldBeFalse)
		So(request.Query.Has("all"), ShouldBeFalse)

		tool, ok := toolByName(t, cs, "mlwh_export")
		So(ok, ShouldBeTrue)
		properties := tool.InputSchema.(map[string]any)["properties"].(map[string]any)
		limit := properties["limit"].(map[string]any)
		So(limit["description"], ShouldContainSubstring, "defaults to 1000")
	})

	Convey("B2.3: Given an incomplete keyset page, Complete and its opaque NextCursor are preserved", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSON("/export/products/study/S1", http.StatusOK, wa.ExportResult{
			Columns: []string{"name"}, Rows: [][]string{{"S1"}}, Total: 2,
			NextCursor: "MQkyCTMJNA", Complete: false, Format: "json",
		})
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		result := structuredObject(callTool(t, cs, "mlwh_export", map[string]any{
			"children": "products", "parent_kind": "study", "parent_id": "S1", "limit": 1,
		}))

		So(result["Complete"], ShouldBeFalse)
		So(result["NextCursor"], ShouldEqual, "MQkyCTMJNA")
	})

	Convey("B2.4: Given 40 offset-backed rows at offset 100, tool and workflow guidance direct offset 140 rather than a cursor", t, func() {
		stub := newStubMLWH(t)
		rows := make([][]string, 40)
		for index := range rows {
			rows[index] = []string{"sample"}
		}
		stub.respondJSON("/export/samples/study/S1", http.StatusOK, wa.ExportResult{
			Columns: []string{"name"}, Rows: rows, Total: 500, Complete: false, Format: "tsv",
		})
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		result := structuredObject(callTool(t, cs, "mlwh_export", map[string]any{
			"children": "samples", "parent_kind": "study", "parent_id": "S1",
			"limit": 40, "offset": 100,
		}))
		So(result["Complete"], ShouldBeFalse)
		So(result["NextCursor"], ShouldBeEmpty)
		So(len(result["Rows"].([]any)), ShouldEqual, 40)

		tool, ok := toolByName(t, cs, "mlwh_export")
		So(ok, ShouldBeTrue)
		So(tool.Description, ShouldContainSubstring, "offset 100 plus 40 returned Rows means offset 140")
		So(tool.Description, ShouldContainSubstring, "Do not use a cursor for offset-backed")
		So(workflowResourceBody(), ShouldContainSubstring, "offset 100 plus 40 returned Rows means offset 140")
		So(workflowResourceBody(), ShouldContainSubstring, "Do not use a cursor for offset-backed")
	})
}

func stringSetFromKeys(values map[string]any) map[string]bool {
	set := make(map[string]bool, len(values))
	for value := range values {
		set[value] = true
	}

	return set
}

func TestExportToolGeneratedSchemaB1(t *testing.T) {
	Convey("B1.2: Given the upstream export vocabularies, the MCP input schema advertises them without drift", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		tool, ok := toolByName(t, cs, "mlwh_export")
		So(ok, ShouldBeTrue)

		schema := tool.InputSchema.(map[string]any)
		properties := schema["properties"].(map[string]any)
		children := properties["children"].(map[string]any)
		parents := properties["parent_kind"].(map[string]any)
		columns := properties["columns"].(map[string]any)
		columnItems := columns["items"].(map[string]any)

		relationships := wa.ExportRelationshipDescriptions()
		vocabularies := wa.ExportColumnVocabularies()
		So(children["enum"], ShouldResemble, exportRelationshipEnumForTest(relationships))
		So(parents["enum"], ShouldResemble, exportParentEnumForTest(relationships))
		So(columnItems["enum"], ShouldResemble, exportColumnEnumForTest(vocabularies))

		childrenDescription := children["description"].(string)
		parentDescription := parents["description"].(string)
		columnsDescription := columns["description"].(string)
		for index, relationship := range relationships {
			label := exportVocabularyLabelForTest(relationship.Children, relationship.Aliases)
			So(childrenDescription, ShouldContainSubstring, label+" ("+relationship.Description+")")
			So(parentDescription, ShouldContainSubstring, label+": "+strings.Join(relationship.ParentKinds, ","))

			vocabulary := vocabularies[index]
			So(vocabulary.Children, ShouldEqual, relationship.Children)
			So(columnsDescription, ShouldContainSubstring,
				exportColumnsDescriptionForTest(vocabulary))
		}
	})
}

func exportRelationshipEnumForTest(relationships []wa.ExportRelationshipDescription) []any {
	values := make([]any, 0, len(relationships))
	for _, relationship := range relationships {
		values = append(values, relationship.Children)
		for _, alias := range relationship.Aliases {
			values = append(values, alias)
		}
	}

	return values
}

func exportParentEnumForTest(relationships []wa.ExportRelationshipDescription) []any {
	seen := map[string]bool{}
	values := []any{}
	for _, relationship := range relationships {
		for _, parent := range relationship.ParentKinds {
			if !seen[parent] {
				values = append(values, parent)
				seen[parent] = true
			}
		}
	}

	return values
}

func exportColumnEnumForTest(vocabularies []wa.ExportColumnVocabulary) []any {
	seen := map[string]bool{}
	values := []any{}
	for _, vocabulary := range vocabularies {
		for _, column := range vocabulary.Columns {
			for _, value := range append([]string{column.Name}, column.Aliases...) {
				if !seen[value] {
					values = append(values, value)
					seen[value] = true
				}
			}
		}
	}

	return values
}

func exportColumnsDescriptionForTest(vocabulary wa.ExportColumnVocabulary) string {
	columns := make([]string, len(vocabulary.Columns))
	for index, column := range vocabulary.Columns {
		columns[index] = column.Name
		if len(column.Aliases) > 0 {
			columns[index] += " (aliases: " + strings.Join(column.Aliases, ",") + ")"
		}
	}

	return exportVocabularyLabelForTest(vocabulary.Children, vocabulary.Aliases) +
		" default " + strings.Join(vocabulary.Default, ",") +
		"; available " + strings.Join(columns, ",")
}

func exportVocabularyLabelForTest(children string, aliases []string) string {
	if len(aliases) == 0 {
		return children
	}

	return children + "/" + strings.Join(aliases, "/")
}

func TestExportToolDeliverablesOnlyTriStateB1(t *testing.T) {
	Convey("B1.3: Given omitted, false, and true deliverables_only, the HTTP query preserves all three states", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSON("/export/products/study/S1", http.StatusOK, emptyExportResultForTest("tsv"))
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		cases := []struct {
			name      string
			arguments map[string]any
			want      url.Values
		}{
			{
				name: "omitted",
				arguments: map[string]any{
					"children": "products", "parent_kind": "study", "parent_id": "S1",
				},
				want: url.Values{},
			},
			{
				name: "false",
				arguments: map[string]any{
					"children": "products", "parent_kind": "study", "parent_id": "S1",
					"deliverables_only": false,
				},
				want: url.Values{"deliverables_only": {"false"}},
			},
			{
				name: "true",
				arguments: map[string]any{
					"children": "products", "parent_kind": "study", "parent_id": "S1",
					"deliverables_only": true,
				},
				want: url.Values{"deliverables_only": {"true"}},
			},
		}

		for _, testCase := range cases {
			result := callTool(t, cs, "mlwh_export", testCase.arguments)
			So(result.IsError, ShouldBeFalse)

			request, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(request.Path, ShouldEqual, "/export/products/study/S1")
			So(request.Query, ShouldResemble, testCase.want)
		}
	})
}

func TestExportToolRequestPropagationB1(t *testing.T) {
	Convey("B1.4: Given every supported relationship and option, mlwh_export sends exact v0.8.0 paths and query values", t, func() {
		stub := newStubMLWH(t)
		relationships := exportRelationshipCasesForTest()
		for _, relationship := range relationships {
			stub.respondJSON(relationship.path, http.StatusOK, emptyExportResultForTest("tsv"))
		}
		stub.respondJSON("/export/products/study/OPTIONS", http.StatusOK, emptyExportResultForTest("json"))
		stub.respondJSON("/export/irods/run/OPTIONS", http.StatusOK, emptyExportResultForTest("csv"))
		stub.respondJSON("/export/users/study/OPTIONS", http.StatusOK, emptyExportResultForTest("tsv"))
		stub.respondError("/export/products/study/ALL", http.StatusBadRequest, "bad_request", "all option probe")

		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		for _, relationship := range relationships {
			result := callTool(t, cs, "mlwh_export", relationship.arguments)
			So(result.IsError, ShouldBeFalse)

			request, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(request.Path, ShouldEqual, relationship.path)
			So(request.Query, ShouldBeEmpty)
		}

		optionCases := []struct {
			arguments map[string]any
			path      string
			query     url.Values
			wantError bool
		}{
			{
				arguments: map[string]any{
					"children": "products", "parent_kind": "study", "parent_id": "OPTIONS",
					"columns": []any{"position", "supplier_sample_name"}, "file_type": "bam",
					"deliverables_only": false, "qc": "pass", "library_type": "PCR",
					"organism": "human", "limit": 17, "cursor": "MQkyCTMJNA", "format": "json",
					"all": false,
				},
				path: "/export/products/study/OPTIONS",
				query: url.Values{
					"columns": {"position,supplier_sample_name"}, "file_type": {"bam"},
					"deliverables_only": {"false"}, "qc": {"pass"}, "library_type": {"PCR"},
					"organism": {"human"}, "limit": {"17"}, "cursor": {"MQkyCTMJNA"},
					"format": {"json"},
				},
			},
			{
				arguments: map[string]any{
					"children": "irods", "parent_kind": "run", "parent_id": "OPTIONS",
					"columns": []any{"created", "irods_path"}, "file_type": "cram",
					"deliverables_only": true, "sort": "created_desc",
					"since": "2026-07-01T00:00:00Z", "until": "2026-07-02T00:00:00Z",
					"limit": 5, "offset": 9, "format": "csv",
				},
				path: "/export/irods/run/OPTIONS",
				query: url.Values{
					"columns": {"created,irods_path"}, "file_type": {"cram"},
					"deliverables_only": {"true"}, "sort": {"created_desc"},
					"since": {"2026-07-01T00:00:00Z"}, "until": {"2026-07-02T00:00:00Z"},
					"limit": {"5"}, "offset": {"9"}, "format": {"csv"},
				},
			},
			{
				arguments: map[string]any{
					"children": "users", "parent_kind": "study", "parent_id": "OPTIONS",
					"columns": []any{"role", "email"}, "role": "owner,manager",
					"limit": 4, "offset": 8, "format": "tsv",
				},
				path: "/export/users/study/OPTIONS",
				query: url.Values{
					"columns": {"role,email"}, "role": {"owner,manager"},
					"limit": {"4"}, "offset": {"8"}, "format": {"tsv"},
				},
			},
			{
				arguments: map[string]any{
					"children": "products", "parent_kind": "study", "parent_id": "ALL",
					"all": true,
				},
				path:      "/export/products/study/ALL",
				query:     url.Values{"limit": {"1000"}},
				wantError: true,
			},
		}

		for _, testCase := range optionCases {
			result := callTool(t, cs, "mlwh_export", testCase.arguments)
			So(result.IsError, ShouldEqual, testCase.wantError)

			request, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(request.Path, ShouldEqual, testCase.path)
			So(request.Query, ShouldResemble, testCase.query)
		}
	})
}

func emptyExportResultForTest(format string) wa.ExportResult {
	return wa.ExportResult{
		Columns:  []string{},
		Rows:     [][]string{},
		Total:    0,
		Complete: true,
		Format:   format,
	}
}
