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
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

type irodsToolCase struct {
	name string
	path string
	args func() map[string]any
}

func irodsListToolCases() []irodsToolCase {
	return []irodsToolCase{
		{"mlwh_irods_paths_for_sample", "/sample/S1/irods", func() map[string]any { return map[string]any{"sanger_name": "S1"} }},
		{"mlwh_irods_paths_for_study", "/study/ST1/irods", func() map[string]any { return map[string]any{"study_lims_id": "ST1"} }},
		{"mlwh_irods_paths_for_run", "/run/52553/irods", func() map[string]any { return map[string]any{"id_run": "52553"} }},
	}
}

func irodsCountToolCases() []irodsToolCase {
	return []irodsToolCase{
		{"mlwh_count_irods_paths_for_sample", "/sample/S1/irods/count", func() map[string]any { return map[string]any{"sanger_name": "S1"} }},
		{"mlwh_count_irods_paths_for_study", "/study/ST1/irods/count", func() map[string]any { return map[string]any{"study_lims_id": "ST1"} }},
		{"mlwh_count_irods_paths_for_run", "/run/52553/irods/count", func() map[string]any { return map[string]any{"id_run": "52553"} }},
	}
}

// TestAvailabilityToolsC1 covers spec C1: the samples-with-data count/list
// tools and the samples-without-data list tool. Each assertion drives the
// public MCP boundary against the hermetic HTTP stub, proving tool
// registration, upstream paths, query propagation, output shape, and mapped
// upstream errors end-to-end.
func TestAvailabilityToolsC1(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the sample availability tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("C1.1: mlwh_count_samples_with_data_for_study passes an exact since window to upstream", func() {
			since := "2026-06-21T00:00:00Z"
			stub.respondJSON("/study/S1/samples-with-data/count", http.StatusOK, wa.Count{Count: 2})

			res := callTool(t, cs, "mlwh_count_samples_with_data_for_study", map[string]any{
				"study_lims_id": "S1",
				"since":         since,
			})

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, 2)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/S1/samples-with-data/count")
			So(req.Query.Get("since"), ShouldEqual, since)
			So(req.Query.Get("until"), ShouldEqual, "")
		})

		Convey("C1.2: mlwh_samples_with_data_for_study preserves window query and header pagination", func() {
			since := "2026-06-21T00:00:00Z"
			until := "2026-06-28T00:00:00Z"
			stub.respondJSONWithHeaders("/study/S1/samples-with-data", http.StatusOK, []wa.SampleWithData{
				{Sample: wa.Sample{IDSampleTmp: 1, Name: "S1-A"}, Platforms: []string{"Illumina"}},
			}, irodsPageHeaders("2", "100"))

			res := callTool(t, cs, "mlwh_samples_with_data_for_study", map[string]any{
				"study_lims_id": "S1",
				"since":         since,
				"until":         until,
			})

			obj := structuredObject(res)
			samples, ok := obj["samples"].([]any)
			So(ok, ShouldBeTrue)
			So(len(samples), ShouldEqual, 1)
			So(obj["total"], ShouldEqual, 2)
			So(obj["next_offset"], ShouldEqual, 100)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/S1/samples-with-data")
			So(req.Query.Get("since"), ShouldEqual, since)
			So(req.Query.Get("until"), ShouldEqual, until)
			So(req.Query.Get("limit"), ShouldEqual, "100")
			So(req.Query.Get("offset"), ShouldEqual, "0")
		})

		Convey("C1.3: mlwh_samples_without_data_for_study preserves ONT platform values", func() {
			stub.respondJSONWithHeaders("/study/S1/samples-without-data", http.StatusOK, []wa.SampleWithData{
				{Sample: wa.Sample{IDSampleTmp: 2, Name: "ONT-1"}, Platforms: []string{"ONT"}},
			}, irodsPageHeaders("1", "-1"))

			res := callTool(t, cs, "mlwh_samples_without_data_for_study", map[string]any{"study_lims_id": "S1"})

			obj := structuredObject(res)
			samples, ok := obj["samples"].([]any)
			So(ok, ShouldBeTrue)
			So(len(samples), ShouldEqual, 1)

			first, ok := samples[0].(map[string]any)
			So(ok, ShouldBeTrue)
			platforms, ok := first["platforms"].([]any)
			So(ok, ShouldBeTrue)
			So(platforms, ShouldResemble, []any{"ONT"})
		})

		Convey("C1.4: mlwh_samples_with_data_for_study maps upstream 400 until-without-since errors", func() {
			upstreamText := "until requires since"
			stub.respondError("/study/S1/samples-with-data", http.StatusBadRequest, "bad_request", upstreamText)

			res := callTool(t, cs, "mlwh_samples_with_data_for_study", map[string]any{
				"study_lims_id": "S1",
				"until":         "2026-06-28T00:00:00Z",
			})

			So(res.IsError, ShouldBeTrue)
			So(firstTextContent(res), ShouldContainSubstring, upstreamText)
		})

		Convey("C1.5: mlwh_count_samples_with_data_for_study maps upstream 400 malformed-since errors", func() {
			upstreamText := `parse since "not-a-time"`
			stub.respondError("/study/S1/samples-with-data/count", http.StatusBadRequest, "bad_request", upstreamText)

			res := callTool(t, cs, "mlwh_count_samples_with_data_for_study", map[string]any{
				"study_lims_id": "S1",
				"since":         "not-a-time",
			})

			So(res.IsError, ShouldBeTrue)
			So(firstTextContent(res), ShouldContainSubstring, upstreamText)
		})

		Convey("C1.6: list tool descriptions explain the cache as-of caveat lives in mlwh_freshness", func() {
			for _, name := range []string{
				"mlwh_samples_with_data_for_study",
				"mlwh_samples_without_data_for_study",
			} {
				tool, ok := toolByName(t, cs, name)
				So(ok, ShouldBeTrue)

				description := strings.ToLower(tool.Description)
				So(description, ShouldContainSubstring, "bare list responses")
				So(description, ShouldContainSubstring, "no cache_synced_at")
				So(description, ShouldContainSubstring, "mlwh_freshness")
			}
		})
	})
}

// TestIRODSToolsC2 covers spec C2: the sample, study, and run iRODS tools
// preserve every upstream option, row field, page header, suffix/error
// semantic, and freshness description through the public MCP boundary.
func TestIRODSToolsC2(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the iRODS availability tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("C2.1: all list options reach each exact path in one request", func() {
			for index, tc := range irodsListToolCases() {
				stub.respondJSONWithHeaders(tc.path, http.StatusOK, []wa.IRODSPath{}, irodsPageHeaders("0", "-1"))

				args := tc.args()
				args["file_type"] = "cram"
				args["deliverables_only"] = true
				args["order_by"] = "created_desc"
				args["since"] = "2026-06-01T00:00:00Z"
				args["until"] = "2026-07-01T00:00:00Z"
				args["limit"] = 7
				args["offset"] = 14

				res := callTool(t, cs, tc.name, args)
				So(res.IsError, ShouldBeFalse)
				So(stub.requestCount(), ShouldEqual, index+1)

				req, ok := stub.lastRequest()
				So(ok, ShouldBeTrue)
				So(req.Path, ShouldEqual, tc.path)
				So(req.Query, ShouldResemble, url.Values{
					"file_type":         {"cram"},
					"deliverables_only": {"true"},
					"order_by":          {"created_desc"},
					"since":             {"2026-06-01T00:00:00Z"},
					"until":             {"2026-07-01T00:00:00Z"},
					"limit":             {"7"},
					"offset":            {"14"},
				})
			}
		})

		Convey("C2.2: matching counts send every filter and no list-only option", func() {
			for index, tc := range irodsCountToolCases() {
				stub.respondJSON(tc.path, http.StatusOK, wa.Count{Count: index + 1})

				args := tc.args()
				args["file_type"] = "cram"
				args["deliverables_only"] = true
				args["since"] = "2026-06-01T00:00:00Z"
				args["until"] = "2026-07-01T00:00:00Z"

				res := callTool(t, cs, tc.name, args)
				So(structuredObject(res)["count"], ShouldEqual, index+1)

				req, ok := stub.lastRequest()
				So(ok, ShouldBeTrue)
				So(req.Path, ShouldEqual, tc.path)
				So(req.Query, ShouldResemble, url.Values{
					"file_type":         {"cram"},
					"deliverables_only": {"true"},
					"since":             {"2026-06-01T00:00:00Z"},
					"until":             {"2026-07-01T00:00:00Z"},
				})
			}
		})

		Convey("C2.3: all list wrappers preserve header page metadata", func() {
			for _, tc := range irodsListToolCases() {
				stub.respondJSONWithHeaders(tc.path, http.StatusOK, []wa.IRODSPath{{IDProduct: "P1"}}, irodsPageHeaders("23", "20"))

				obj := structuredObject(callTool(t, cs, tc.name, tc.args()))
				paths, ok := obj["irods_paths"].([]any)
				So(ok, ShouldBeTrue)
				So(len(paths), ShouldEqual, 1)
				So(obj["total"], ShouldEqual, 23)
				So(obj["next_offset"], ShouldEqual, 20)
			}
		})

		Convey("C2.4: ordinary and merged rows preserve every exact IRODSPath field", func() {
			trueValue := true
			falseValue := false
			rows := []wa.IRODSPath{
				{
					IDProduct: "P1", Collection: "/seq/1", DataObject: "a.cram", IRODSPath: "/seq/1/a.cram",
					IDSampleTmp: 123, Name: "S1", SupplierName: "supplier-1", SangerSampleID: "SAN1",
					AccessionNumber: "ERS1", IDStudyLims: "ST1", StudyAccessionNumber: "ERP1",
					Created: "2026-06-20T12:00:00Z", IDRun: 52553, Position: 2, TagIndex: 7,
					Platform: "illumina", ManualQC: "pass", Deliverable: &trueValue,
				},
				{
					IDProduct: "P2", Collection: "/seq/2", DataObject: "merged.cram", IRODSPath: "/seq/2/merged.cram",
					IDSampleTmp: 124, Name: "S2", SupplierName: "supplier-2", SangerSampleID: "SAN2",
					AccessionNumber: "ERS2", IDStudyLims: "ST2", StudyAccessionNumber: "ERP2",
					Created: "2026-06-21T12:00:00Z", Platform: "illumina", Merged: true,
					ManualQC: "fail", Deliverable: &falseValue,
				},
			}
			stub.respondJSONWithHeaders("/sample/S1/irods", http.StatusOK, rows, irodsPageHeaders("2", "-1"))

			obj := structuredObject(callTool(t, cs, "mlwh_irods_paths_for_sample", map[string]any{"sanger_name": "S1"}))
			paths := obj["irods_paths"].([]any)
			So(paths, ShouldResemble, []any{
				map[string]any{
					"id_product": "P1", "collection": "/seq/1", "data_object": "a.cram", "irods_path": "/seq/1/a.cram",
					"id_sample_tmp": float64(123), "name": "S1", "supplier_name": "supplier-1", "sanger_sample_id": "SAN1",
					"accession_number": "ERS1", "id_study_lims": "ST1", "study_accession_number": "ERP1",
					"created": "2026-06-20T12:00:00Z", "id_run": float64(52553), "lane": float64(2), "tag_index": float64(7),
					"platform": "illumina", "merged": false, "manual_qc": "pass", "deliverable": true,
				},
				map[string]any{
					"id_product": "P2", "collection": "/seq/2", "data_object": "merged.cram", "irods_path": "/seq/2/merged.cram",
					"id_sample_tmp": float64(124), "name": "S2", "supplier_name": "supplier-2", "sanger_sample_id": "SAN2",
					"accession_number": "ERS2", "id_study_lims": "ST2", "study_accession_number": "ERP2",
					"created": "2026-06-21T12:00:00Z", "id_run": float64(0), "lane": float64(0), "tag_index": float64(0),
					"platform": "illumina", "merged": true, "manual_qc": "fail", "deliverable": false,
				},
			})
		})

		Convey("C2.5: deliverable remains true, false, or null", func() {
			trueValue := true
			falseValue := false
			stub.respondJSONWithHeaders("/run/52553/irods", http.StatusOK, []wa.IRODSPath{
				{IDProduct: "P1", Deliverable: &trueValue},
				{IDProduct: "P2", Deliverable: &falseValue},
				{IDProduct: "P3", Deliverable: nil},
			}, irodsPageHeaders("3", "-1"))

			obj := structuredObject(callTool(t, cs, "mlwh_irods_paths_for_run", map[string]any{"id_run": "52553"}))
			paths := obj["irods_paths"].([]any)
			So(paths[0].(map[string]any)["deliverable"], ShouldEqual, true)
			So(paths[1].(map[string]any)["deliverable"], ShouldEqual, false)
			So(paths[2].(map[string]any)["deliverable"], ShouldBeNil)
		})

		Convey("C2.6: all suffix outcomes are decided upstream", func() {
			stub.respondJSONWithHeaders("/study/S1/irods", http.StatusOK, []wa.IRODSPath{{IDProduct: "P1"}}, irodsPageHeaders("1", "-1"))
			matched := structuredObject(callTool(t, cs, "mlwh_irods_paths_for_study", map[string]any{
				"study_lims_id": "S1", "file_type": ".CRAM",
			}))
			So(len(matched["irods_paths"].([]any)), ShouldEqual, 1)
			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Query.Get("file_type"), ShouldEqual, ".CRAM")

			stub.respondJSONWithHeaders("/study/S1/irods", http.StatusOK, []wa.IRODSPath{}, irodsPageHeaders("0", "-1"))
			unmatched := structuredObject(callTool(t, cs, "mlwh_irods_paths_for_study", map[string]any{
				"study_lims_id": "S1", "file_type": "vcf",
			}))
			So(len(unmatched["irods_paths"].([]any)), ShouldEqual, 0)

			failures := countInvalidIRODSToolFailures(t, cs, stub, []string{" ", "%", "_", "/"})
			So(failures, ShouldEqual, 0)
		})

		Convey("C2.7: schemas and descriptions explain ordering, row time, deliverability, and freshness", func() {
			for _, tc := range irodsListToolCases() {
				tool, ok := toolByName(t, cs, tc.name)
				So(ok, ShouldBeTrue)
				properties := tool.InputSchema.(map[string]any)["properties"].(map[string]any)
				So(properties, ShouldContainKey, "order_by")
				assertIRODSDescription(tool.Description)
			}

			for _, tc := range irodsCountToolCases() {
				tool, ok := toolByName(t, cs, tc.name)
				So(ok, ShouldBeTrue)
				properties := tool.InputSchema.(map[string]any)["properties"].(map[string]any)
				So(properties, ShouldNotContainKey, "order_by")
				assertIRODSDescription(tool.Description)
			}
		})
	})
}

// TestLatestDataToolsC3 covers spec C3: study- and faculty-sponsor-scoped
// latest-data pages and their matching counts preserve upstream ordering,
// pagination, row fields, filtering, and agent-facing semantics.
func TestLatestDataToolsC3(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the latest-data tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("C3.1: a study list defaults to a ten-row newest-first page", func() {
			stub.respondJSONWithHeaders("/study/S1/latest-data", http.StatusOK, []wa.RecentDataRow{
				{Created: "2026-07-02T10:00:00Z", IRODSPath: "/seq/new.cram"},
				{Created: "2026-07-01T10:00:00Z", IRODSPath: "/seq/old.cram"},
			}, irodsPageHeaders("2", "-1"))

			obj := structuredObject(callTool(t, cs, "mlwh_latest_data_for_study", map[string]any{
				"study_lims_id": "S1",
			}))
			rows, ok := obj["latest_data"].([]any)
			So(ok, ShouldBeTrue)
			So(rows, ShouldHaveLength, 2)
			So(rows[0].(map[string]any)["irods_path"], ShouldEqual, "/seq/new.cram")

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/S1/latest-data")
			So(req.Query, ShouldResemble, url.Values{
				"limit":  {"10"},
				"offset": {"0"},
			})
		})

		Convey("C3.2: a sponsor list preserves file type and explicit pagination", func() {
			stub.respondJSONWithHeaders("/latest-data/faculty-sponsor/Ada", http.StatusOK, []wa.RecentDataRow{}, irodsPageHeaders("80", "60"))

			res := callTool(t, cs, "mlwh_latest_data_for_faculty_sponsor", map[string]any{
				"faculty_sponsor": "Ada",
				"file_type":       "cram",
				"limit":           20,
				"offset":          40,
			})
			So(res.IsError, ShouldBeFalse)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/latest-data/faculty-sponsor/Ada")
			So(req.Query, ShouldResemble, url.Values{
				"file_type": {"cram"},
				"limit":     {"20"},
				"offset":    {"40"},
			})

			overLimit := callTool(t, cs, "mlwh_latest_data_for_faculty_sponsor", map[string]any{
				"faculty_sponsor": "Ada", "limit": 1001,
			})
			So(overLimit.IsError, ShouldBeTrue)
			So(firstTextContent(overLimit), ShouldContainSubstring, "maximum of 1000")
			So(stub.requestCount(), ShouldEqual, 1)

			negativeOffset := callTool(t, cs, "mlwh_latest_data_for_faculty_sponsor", map[string]any{
				"faculty_sponsor": "Ada", "offset": -1,
			})
			So(negativeOffset.IsError, ShouldBeTrue)
			So(firstTextContent(negativeOffset), ShouldContainSubstring, "non-negative")
			So(stub.requestCount(), ShouldEqual, 1)
		})

		Convey("C3.3: a latest-data page preserves metadata and every RecentDataRow field", func() {
			row := wa.RecentDataRow{
				Created:      "2026-07-02T10:00:00Z",
				IRODSPath:    "/seq/52553/2/7/sample.cram",
				IDStudyLims:  "S1",
				StudyName:    "Study one",
				Name:         "SANGER-1",
				SupplierName: "Supplier 1",
				IDRun:        52553,
				Position:     2,
				TagIndex:     7,
				Platform:     "illumina",
				Merged:       true,
			}
			stub.respondJSONWithHeaders("/study/S1/latest-data", http.StatusOK, []wa.RecentDataRow{row}, irodsPageHeaders("31", "20"))

			obj := structuredObject(callTool(t, cs, "mlwh_latest_data_for_study", map[string]any{
				"study_lims_id": "S1", "limit": 10, "offset": 10,
			}))
			So(obj["total"], ShouldEqual, 31)
			So(obj["next_offset"], ShouldEqual, 20)
			So(obj["latest_data"].([]any), ShouldResemble, []any{map[string]any{
				"created":       "2026-07-02T10:00:00Z",
				"irods_path":    "/seq/52553/2/7/sample.cram",
				"id_study_lims": "S1",
				"study_name":    "Study one",
				"name":          "SANGER-1",
				"supplier_name": "Supplier 1",
				"id_run":        float64(52553),
				"lane":          float64(2),
				"tag_index":     float64(7),
				"platform":      "illumina",
				"merged":        true,
			}})

			tool, ok := toolByName(t, cs, "mlwh_latest_data_for_study")
			So(ok, ShouldBeTrue)
			outputSchema := tool.OutputSchema.(map[string]any)
			properties := outputSchema["properties"].(map[string]any)
			So(properties, ShouldContainKey, "latest_data")
			So(properties, ShouldContainKey, "total")
			So(properties, ShouldContainKey, "next_offset")
		})

		Convey("C3.4: study and sponsor counts preserve file type and exact count paths", func() {
			cases := []struct {
				name  string
				path  string
				args  map[string]any
				count int
			}{
				{
					name: "mlwh_count_latest_data_for_study", path: "/study/S1/latest-data/count",
					args: map[string]any{"study_lims_id": "S1", "file_type": "cram"}, count: 31,
				},
				{
					name: "mlwh_count_latest_data_for_faculty_sponsor", path: "/latest-data/faculty-sponsor/Ada/count",
					args: map[string]any{"faculty_sponsor": "Ada", "file_type": "cram"}, count: 47,
				},
			}

			for _, tc := range cases {
				stub.respondJSON(tc.path, http.StatusOK, wa.Count{Count: tc.count})

				obj := structuredObject(callTool(t, cs, tc.name, tc.args))
				So(len(obj), ShouldEqual, 1)
				So(obj["count"], ShouldEqual, tc.count)

				req, ok := stub.lastRequest()
				So(ok, ShouldBeTrue)
				So(req.Path, ShouldEqual, tc.path)
				So(req.Query, ShouldResemble, url.Values{"file_type": {"cram"}})
			}
		})

		Convey("C3.5: descriptions explain bounded pages, data-added time, and freshness", func() {
			for _, name := range []string{
				"mlwh_latest_data_for_study",
				"mlwh_latest_data_for_faculty_sponsor",
			} {
				tool, ok := toolByName(t, cs, name)
				So(ok, ShouldBeTrue)

				description := strings.ToLower(tool.Description)
				So(description, ShouldContainSubstring, "bounded")
				So(description, ShouldContainSubstring, "page")
				So(description, ShouldContainSubstring, "not every row tied for the maximum created")
				assertLatestDataDescription(description)
			}

			for _, name := range []string{
				"mlwh_count_latest_data_for_study",
				"mlwh_count_latest_data_for_faculty_sponsor",
			} {
				tool, ok := toolByName(t, cs, name)
				So(ok, ShouldBeTrue)
				assertLatestDataDescription(tool.Description)
			}
		})
	})
}

// TestSampleCRAMToolsE3 covers spec E3: the merged-aware per-sample CRAM page
// and its count counterpart preserve upstream selection, canonical row fields,
// semantic pagination metadata, exact paths, and cache caveats through MCP.
func TestSampleCRAMToolsE3(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with sample CRAM tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("E3.1: ordinary and merged selections return one row per sample with the merged composite preferred", func() {
			stub.respondJSONWithHeaders("/study/S1/sample-crams", http.StatusOK, []wa.SampleCRAM{
				{Name: "ordinary", AccessionNumber: "ERS1", IRODSPath: "/seq/ordinary.cram"},
				{Name: "multi-lane", AccessionNumber: "ERS2", IRODSPath: "/seq/multi-lane.merged.cram", Merged: true},
			}, irodsPageHeaders("2", "-1"))

			obj := structuredObject(callTool(t, cs, "mlwh_sample_crams_for_study", map[string]any{
				"study_lims_id": "S1",
			}))
			rows, ok := obj["sample_crams"].([]any)
			So(ok, ShouldBeTrue)
			So(rows, ShouldHaveLength, 2)
			So(rows[0].(map[string]any)["name"], ShouldEqual, "ordinary")
			So(rows[0].(map[string]any)["merged"], ShouldEqual, false)
			So(rows[1].(map[string]any)["name"], ShouldEqual, "multi-lane")
			So(rows[1].(map[string]any)["merged"], ShouldEqual, true)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/S1/sample-crams")
			So(req.Query, ShouldResemble, url.Values{"limit": {"100"}, "offset": {"0"}})
		})

		Convey("E3.2: legacy upstream aliases decode but MCP emits only the four canonical fields", func() {
			stub.respondJSONWithHeaders("/study/S1/sample-crams", http.StatusOK, []map[string]any{
				{
					"name": "legacy", "ega_id": "ERS3", "irods_cram_path": "/seq/legacy.cram", "merged": true,
				},
			}, irodsPageHeaders("1", "-1"))

			obj := structuredObject(callTool(t, cs, "mlwh_sample_crams_for_study", map[string]any{
				"study_lims_id": "S1",
			}))
			rows := obj["sample_crams"].([]any)
			So(rows, ShouldResemble, []any{map[string]any{
				"name": "legacy", "accession_number": "ERS3", "irods_path": "/seq/legacy.cram", "merged": true,
			}})
			So(rows[0].(map[string]any), ShouldNotContainKey, "ega_id")
			So(rows[0].(map[string]any), ShouldNotContainKey, "irods_cram_path")
		})

		Convey("E3.3: response headers and the provider schema expose exact semantic page metadata", func() {
			stub.respondJSONWithHeaders("/study/S1/sample-crams", http.StatusOK, []wa.SampleCRAM{
				{Name: "sample-3", AccessionNumber: "ERS3", IRODSPath: "/seq/sample-3.cram"},
			}, irodsPageHeaders("17", "10"))

			obj := structuredObject(callTool(t, cs, "mlwh_sample_crams_for_study", map[string]any{
				"study_lims_id": "S1", "limit": 5, "offset": 5,
			}))
			So(obj, ShouldContainKey, "sample_crams")
			So(obj["total"], ShouldEqual, 17)
			So(obj["next_offset"], ShouldEqual, 10)

			tool, ok := toolByName(t, cs, "mlwh_sample_crams_for_study")
			So(ok, ShouldBeTrue)
			output := tool.OutputSchema.(map[string]any)
			properties := output["properties"].(map[string]any)
			So(properties, ShouldContainKey, "sample_crams")
			So(properties, ShouldContainKey, "total")
			So(properties, ShouldContainKey, "next_offset")
			itemProperties := properties["sample_crams"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
			So(itemProperties, ShouldHaveLength, 4)
			So(itemProperties, ShouldContainKey, "name")
			So(itemProperties, ShouldContainKey, "accession_number")
			So(itemProperties, ShouldContainKey, "irods_path")
			So(itemProperties, ShouldContainKey, "merged")
		})

		Convey("E3.4: the count tool uses the exact count endpoint and returns the matching selected-row count", func() {
			stub.respondJSON("/study/S1/sample-crams/count", http.StatusOK, wa.Count{Count: 2})

			obj := structuredObject(callTool(t, cs, "mlwh_count_sample_crams_for_study", map[string]any{
				"study_lims_id": "S1",
			}))
			So(obj["count"], ShouldEqual, 2)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/S1/sample-crams/count")
			So(req.Query, ShouldBeEmpty)
		})

		Convey("E3.5: descriptions distinguish product attachments from sample CRAM absence and direct callers to freshness", func() {
			for _, name := range []string{"mlwh_sample_crams_for_study", "mlwh_count_sample_crams_for_study"} {
				tool, ok := toolByName(t, cs, name)
				So(ok, ShouldBeTrue)

				description := strings.ToLower(tool.Description)
				So(description, ShouldContainSubstring, "empty product-level attachment")
				So(description, ShouldContainSubstring, "does not prove")
				So(description, ShouldContainSubstring, "sample-level cram is absent")
				So(description, ShouldContainSubstring, "mlwh_freshness")
				if strings.Contains(name, "count") {
					So(description, ShouldContainSubstring, "count responses have no cache_synced_at")
				} else {
					So(description, ShouldContainSubstring, "bare list responses have no cache_synced_at")
				}
			}
		})
	})
}

func TestAvailabilityToolsF3Cancellation(t *testing.T) {
	Convey("F3.2: cancellation before a remote call makes no request and returns no successful result", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSONWithHeaders("/study/S1/latest-data", http.StatusOK, []wa.RecentDataRow{}, irodsPageHeaders("0", "-1"))
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{
			Name: "mlwh_latest_data_for_study", Arguments: map[string]any{"study_lims_id": "S1"},
		})

		So(result, ShouldBeNil)
		So(errors.Is(err, context.Canceled), ShouldBeTrue)
		So(stub.requestCount(), ShouldEqual, 0)
	})

	Convey("F3.2: cancellation during a remote call cancels its HTTP request without retry or a partial success", t, func() {
		stub := newStubMLWH(t)
		requestStarted := make(chan struct{})
		stub.respondHandler("/study/S1/latest-data", func(_ http.ResponseWriter, request *http.Request) {
			close(requestStarted)
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
				Name: "mlwh_latest_data_for_study", Arguments: map[string]any{"study_lims_id": "S1"},
			})
			callDone <- callOutcome{result: result, err: err}
		}()

		select {
		case <-requestStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("latest-data request did not reach the stub")
		}
		cancel()

		var outcome callOutcome
		select {
		case outcome = <-callDone:
		case <-time.After(5 * time.Second):
			t.Fatal("cancelled latest-data request did not return")
		}

		So(outcome.result, ShouldBeNil)
		So(errors.Is(outcome.err, context.Canceled), ShouldBeTrue)
		So(stub.requestCount(), ShouldEqual, 1)
	})
}

func TestAvailabilityToolsF3DateWindows(t *testing.T) {
	Convey("F3.5: samples-with-data list and count receive the same exact RFC3339 window and return matching totals", t, func() {
		stub := newStubMLWH(t)
		stub.respondJSONWithHeaders("/study/S1/samples-with-data", http.StatusOK, []wa.SampleWithData{
			{Sample: wa.Sample{IDSampleTmp: 1, Name: "S1-A"}, Platforms: []string{}},
		}, irodsPageHeaders("1", "-1"))
		stub.respondJSON("/study/S1/samples-with-data/count", http.StatusOK, wa.Count{Count: 1})
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		since := "2026-07-01T12:34:56Z"
		until := "2026-07-08T12:34:56Z"
		arguments := map[string]any{"study_lims_id": "S1", "since": since, "until": until}

		list := structuredObject(callTool(t, cs, "mlwh_samples_with_data_for_study", arguments))
		So(list["total"], ShouldEqual, 1)
		listRequest, ok := stub.lastRequest()
		So(ok, ShouldBeTrue)
		So(listRequest.Query, ShouldResemble, url.Values{
			"since": {since}, "until": {until}, "limit": {"100"}, "offset": {"0"},
		})

		count := structuredObject(callTool(t, cs, "mlwh_count_samples_with_data_for_study", arguments))
		So(count["count"], ShouldEqual, list["total"])
		countRequest, ok := stub.lastRequest()
		So(ok, ShouldBeTrue)
		So(countRequest.Query, ShouldResemble, url.Values{"since": {since}, "until": {until}})
	})

	Convey("F3.5: until without since remains an upstream bad request for both list and count", t, func() {
		stub := newStubMLWH(t)
		const upstreamMessage = "until requires since"
		stub.respondError("/study/S1/samples-with-data", http.StatusBadRequest, "bad_request", upstreamMessage)
		stub.respondError("/study/S1/samples-with-data/count", http.StatusBadRequest, "bad_request", upstreamMessage)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		until := "2026-07-08T12:34:56Z"
		arguments := map[string]any{"study_lims_id": "S1", "until": until}
		for _, tool := range []string{
			"mlwh_samples_with_data_for_study", "mlwh_count_samples_with_data_for_study",
		} {
			result := callTool(t, cs, tool, arguments)
			So(result.IsError, ShouldBeTrue)
			So(firstTextContent(result), ShouldContainSubstring, upstreamMessage)
			request, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(request.Query.Get("since"), ShouldBeEmpty)
			So(request.Query.Get("until"), ShouldEqual, until)
		}
		So(stub.requestCount(), ShouldEqual, 2)
	})
}

func irodsPageHeaders(total, nextOffset string) http.Header {
	return http.Header{
		"X-Total-Count": {total},
		"X-Next-Offset": {nextOffset},
	}
}

func assertLatestDataDescription(description string) {
	lower := strings.ToLower(description)
	So(lower, ShouldContainSubstring, "created")
	So(lower, ShouldContainSubstring, "data-added")
	So(lower, ShouldContainSubstring, "mlwh_freshness")
}

func countInvalidIRODSToolFailures(
	t *testing.T,
	cs *mcp.ClientSession,
	stub *stubMLWH,
	invalidValues []string,
) int {
	t.Helper()

	tools := []struct {
		name string
		args func(string) map[string]any
		path string
	}{
		{
			name: "mlwh_irods_paths_for_sample",
			args: func(fileType string) map[string]any {
				return map[string]any{"sanger_name": "S1", "file_type": fileType}
			},
			path: "/sample/S1/irods",
		},
		{
			name: "mlwh_count_irods_paths_for_sample",
			args: func(fileType string) map[string]any {
				return map[string]any{"sanger_name": "S1", "file_type": fileType}
			},
			path: "/sample/S1/irods/count",
		},
		{
			name: "mlwh_irods_paths_for_study",
			args: func(fileType string) map[string]any {
				return map[string]any{"study_lims_id": "S1", "file_type": fileType}
			},
			path: "/study/S1/irods",
		},
		{
			name: "mlwh_count_irods_paths_for_study",
			args: func(fileType string) map[string]any {
				return map[string]any{"study_lims_id": "S1", "file_type": fileType}
			},
			path: "/study/S1/irods/count",
		},
		{
			name: "mlwh_irods_paths_for_run",
			args: func(fileType string) map[string]any {
				return map[string]any{"id_run": "52553", "file_type": fileType}
			},
			path: "/run/52553/irods",
		},
		{
			name: "mlwh_count_irods_paths_for_run",
			args: func(fileType string) map[string]any {
				return map[string]any{"id_run": "52553", "file_type": fileType}
			},
			path: "/run/52553/irods/count",
		},
	}

	for _, tool := range tools {
		stub.respondError(tool.path, http.StatusBadRequest, "bad_request", "invalid file_type")
	}

	failures := 0
	for _, fileType := range invalidValues {
		for _, tool := range tools {
			res := callTool(t, cs, tool.name, tool.args(fileType))
			req, ok := stub.lastRequest()
			if !res.IsError ||
				!strings.Contains(firstTextContent(res), "invalid file_type") ||
				!ok ||
				req.Path != tool.path ||
				req.Query.Get("file_type") != fileType {
				failures++
			}
		}
	}

	return failures
}

func assertIRODSDescription(description string) {
	lower := strings.ToLower(description)
	So(lower, ShouldContainSubstring, "created")
	So(lower, ShouldContainSubstring, "data-added")
	So(lower, ShouldContainSubstring, "approximates irods target=1")
	So(lower, ShouldContainSubstring, "not is_spiked")
	So(lower, ShouldContainSubstring, "mlwh_freshness")
}
