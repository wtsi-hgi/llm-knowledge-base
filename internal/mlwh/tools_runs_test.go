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
	"net/http"
	"strings"
	"testing"

	wa "github.com/wtsi-hgi/wa/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

// TestGlobalRunToolsD2 covers the global keyset-paged run listing and its
// filtered count counterpart from spec D2.
func TestGlobalRunToolsD2(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with global run tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("D2.1: the list forwards date bounds, repeated platforms, limit, and cursor exactly", func() {
			stub.respondJSON("/runs", http.StatusOK, []wa.RunListingRow{})

			res := callTool(t, cs, "mlwh_runs", map[string]any{
				"since":    "2025-01-01",
				"until":    "2025-02-01",
				"platform": []any{"illumina", "pacbio"},
				"limit":    50,
				"cursor":   "pacbio:r2",
			})
			So(res.IsError, ShouldBeFalse)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/runs")
			So(req.Query["platform"], ShouldResemble, []string{"illumina", "pacbio"})
			So(req.Query.Get("since"), ShouldEqual, "2025-01-01")
			So(req.Query.Get("until"), ShouldEqual, "2025-02-01")
			So(req.Query.Get("limit"), ShouldEqual, "50")
			So(req.Query.Get("cursor"), ShouldEqual, "pacbio:r2")

			defaulted := callTool(t, cs, "mlwh_runs", map[string]any{})
			So(defaulted.IsError, ShouldBeFalse)
			req, ok = stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Query.Get("limit"), ShouldEqual, "100")

			before := stub.requestCount()
			overLimit := callTool(t, cs, "mlwh_runs", map[string]any{"limit": 1001})
			So(overLimit.IsError, ShouldBeTrue)
			So(firstTextContent(overLimit), ShouldContainSubstring, "maximum of 1000")
			So(stub.requestCount(), ShouldEqual, before)
		})

		Convey("D2.2: a row preserves every RunListingRow field under runs without offset metadata", func() {
			stub.respondJSON("/runs", http.StatusOK, []wa.RunListingRow{{
				ID:            "ont:e9",
				Platform:      "ont",
				NativeID:      "e9",
				Manufacturer:  "Oxford Nanopore",
				RunDate:       "2025-01-17",
				DateBasis:     "warehouse load time",
				CacheSyncedAt: "2025-01-18T12:34:56Z",
			}})

			obj := structuredObject(callTool(t, cs, "mlwh_runs", map[string]any{}))
			runs, ok := obj["runs"].([]any)
			So(ok, ShouldBeTrue)
			So(len(runs), ShouldEqual, 1)

			row, ok := runs[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(row, ShouldResemble, map[string]any{
				"id":              "ont:e9",
				"platform":        "ont",
				"native_id":       "e9",
				"manufacturer":    "Oxford Nanopore",
				"run_date":        "2025-01-17",
				"date_basis":      "warehouse load time",
				"cache_synced_at": "2025-01-18T12:34:56Z",
			})
			So(obj, ShouldNotContainKey, "total")
			So(obj, ShouldNotContainKey, "next_offset")
		})

		Convey("D2.3: continuation guidance says the last row id is the next cursor", func() {
			stub.respondJSON("/runs", http.StatusOK, []wa.RunListingRow{{ID: "ont:e8"}, {ID: "ont:e9"}})

			obj := structuredObject(callTool(t, cs, "mlwh_runs", map[string]any{}))
			runs, ok := obj["runs"].([]any)
			So(ok, ShouldBeTrue)
			last, ok := runs[len(runs)-1].(map[string]any)
			So(ok, ShouldBeTrue)
			So(last["id"], ShouldEqual, "ont:e9")

			tool, ok := toolByName(t, cs, "mlwh_runs")
			So(ok, ShouldBeTrue)
			So(strings.ToLower(tool.Description), ShouldContainSubstring, "pass the last row's id as the next cursor")
		})

		Convey("D2.4: count forwards identical filters and returns the exact Count", func() {
			stub.respondJSON("/runs/count", http.StatusOK, wa.Count{Count: 73})

			obj := structuredObject(callTool(t, cs, "mlwh_count_runs", map[string]any{
				"since":    "2025-01-01",
				"until":    "2025-02-01",
				"platform": []any{"illumina", "pacbio"},
			}))
			So(obj["count"], ShouldEqual, 73)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/runs/count")
			So(req.Query["platform"], ShouldResemble, []string{"illumina", "pacbio"})
			So(req.Query.Get("since"), ShouldEqual, "2025-01-01")
			So(req.Query.Get("until"), ShouldEqual, "2025-02-01")
			So(req.Query, ShouldNotContainKey, "limit")
			So(req.Query, ShouldNotContainKey, "cursor")
		})

		Convey("D2.5: descriptions define platform run grain and the ONT date caveat", func() {
			listTool, ok := toolByName(t, cs, "mlwh_runs")
			So(ok, ShouldBeTrue)
			countTool, ok := toolByName(t, cs, "mlwh_count_runs")
			So(ok, ShouldBeTrue)

			description := strings.ToLower(listTool.Description + " " + countTool.Description)
			So(description, ShouldContainSubstring, "one row per platform-native run identifier")
			So(description, ShouldContainSubstring, "ont uses warehouse load time - not a true sequencing date")
		})
	})
}

// TestRunAggregateToolsD3 covers monthly run counts and the general grouped
// sequencing aggregate from spec D3.
func TestRunAggregateToolsD3(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with run aggregate tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("D3.1: monthly counts forward exact filters once and preserve every row field", func() {
			rows := []wa.MonthlyRunCount{
				{
					Month:         "2025-01",
					Manufacturer:  "Illumina",
					Platform:      "Illumina",
					Count:         12,
					DateBasis:     "run complete",
					CacheSyncedAt: "2025-03-01T12:00:00Z",
				},
				{
					Month:         "2025-02",
					Manufacturer:  "PacBio",
					Platform:      "PacBio",
					Count:         7,
					DateBasis:     "run_complete",
					CacheSyncedAt: "2025-03-01T11:00:00Z",
				},
			}
			stub.respondJSON("/runs/monthly", http.StatusOK, rows)

			obj := structuredObject(callTool(t, cs, "mlwh_monthly_run_counts", map[string]any{
				"since":    "2025-01-01",
				"until":    "2025-03-01",
				"platform": []any{"Illumina", "PacBio"},
			}))
			So(obj["monthly_run_counts"], ShouldResemble, []any{
				map[string]any{
					"month":           "2025-01",
					"manufacturer":    "Illumina",
					"platform":        "Illumina",
					"count":           float64(12),
					"date_basis":      "run complete",
					"cache_synced_at": "2025-03-01T12:00:00Z",
				},
				map[string]any{
					"month":           "2025-02",
					"manufacturer":    "PacBio",
					"platform":        "PacBio",
					"count":           float64(7),
					"date_basis":      "run_complete",
					"cache_synced_at": "2025-03-01T11:00:00Z",
				},
			})

			So(stub.requestCount(), ShouldEqual, 1)
			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/runs/monthly")
			So(req.Query["platform"], ShouldResemble, []string{"Illumina", "PacBio"})
			So(req.Query.Get("since"), ShouldEqual, "2025-01-01")
			So(req.Query.Get("until"), ShouldEqual, "2025-03-01")
		})

		Convey("D3.2: the general aggregate forwards repeated groups and platforms plus exact scalars once", func() {
			stub.respondJSON("/sequencing/aggregate", http.StatusOK, []wa.SequencingAggregateRow{})

			res := callTool(t, cs, "mlwh_sequencing_aggregate", map[string]any{
				"group_by": []any{"month", "programme", "platform"},
				"unit":     "products",
				"since":    "2025-01-01T00:00:00Z",
				"until":    "2025-03-01T00:00:00Z",
				"platform": []any{"Illumina", "PacBio"},
			})
			So(res.IsError, ShouldBeFalse)
			So(stub.requestCount(), ShouldEqual, 1)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/sequencing/aggregate")
			So(req.Query["group_by"], ShouldResemble, []string{"month", "programme", "platform"})
			So(req.Query["platform"], ShouldResemble, []string{"Illumina", "PacBio"})
			So(req.Query.Get("unit"), ShouldEqual, "products")
			So(req.Query.Get("since"), ShouldEqual, "2025-01-01T00:00:00Z")
			So(req.Query.Get("until"), ShouldEqual, "2025-03-01T00:00:00Z")
		})

		Convey("D3.3: an aggregate row preserves only its requested group keys and every scalar", func() {
			stub.respondJSON("/sequencing/aggregate", http.StatusOK, []wa.SequencingAggregateRow{{
				Group: map[string]string{
					"month":     "2025-01",
					"programme": "Cancer",
					"platform":  "PacBio",
				},
				Unit:          "products",
				Count:         29,
				DateBasis:     "iRODS created",
				CacheSyncedAt: "2025-03-01T11:00:00Z",
			}})

			obj := structuredObject(callTool(t, cs, "mlwh_sequencing_aggregate", map[string]any{
				"group_by": []any{"month", "programme", "platform"},
				"unit":     "products",
			}))
			So(obj["aggregates"], ShouldResemble, []any{map[string]any{
				"group": map[string]any{
					"month":     "2025-01",
					"programme": "Cancer",
					"platform":  "PacBio",
				},
				"unit":            "products",
				"count":           float64(29),
				"date_basis":      "iRODS created",
				"cache_synced_at": "2025-03-01T11:00:00Z",
			}})
		})

		Convey("D3.4: missing or invalid group_by and unit are actionable errors without upstream calls", func() {
			cases := []struct {
				name string
				args map[string]any
				want string
			}{
				{name: "missing group_by", args: map[string]any{"unit": "runs"}, want: "group_by"},
				{name: "empty group_by", args: map[string]any{"group_by": []any{}, "unit": "runs"}, want: "group_by"},
				{name: "invalid group_by", args: map[string]any{"group_by": []any{"study"}, "unit": "runs"}, want: "group_by"},
				{name: "missing unit", args: map[string]any{"group_by": []any{"month"}}, want: "unit"},
				{name: "invalid unit", args: map[string]any{"group_by": []any{"month"}, "unit": "reads"}, want: "unit"},
			}

			for _, tc := range cases {
				res := callTool(t, cs, "mlwh_sequencing_aggregate", tc.args)
				So(res.IsError, ShouldBeTrue)
				So(strings.ToLower(firstTextContent(res)), ShouldContainSubstring, tc.want)
			}
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("D3.5: descriptions explain date bases and multi-study run attribution", func() {
			monthlyTool, ok := toolByName(t, cs, "mlwh_monthly_run_counts")
			So(ok, ShouldBeTrue)
			aggregateTool, ok := toolByName(t, cs, "mlwh_sequencing_aggregate")
			So(ok, ShouldBeTrue)

			description := strings.ToLower(monthlyTool.Description + " " + aggregateTool.Description)
			So(description, ShouldContainSubstring, "runs counts platform-native run identifiers using the per-platform date basis")
			So(description, ShouldContainSubstring, "samples and products are data-grain aggregates")
			So(description, ShouldContainSubstring, "windowed by irods created")
			So(description, ShouldContainSubstring, "a run spanning multiple requested study groups counts once in each group it touches")
		})
	})
}
