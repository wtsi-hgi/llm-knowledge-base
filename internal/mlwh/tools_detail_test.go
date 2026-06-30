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

// TestFanOutCountTools covers Story D3: count tools for the remaining large
// upstream fan-out lists, each returning wa.Count and hitting the documented
// Registry /count path.
func TestFanOutCountTools(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the fan-out count tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		assertCountTool := func(path string, count int, tool string, args map[string]any) {
			stub.respondJSON(path, http.StatusOK, wa.Count{Count: count})

			res := callTool(t, cs, tool, args)

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, count)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, path)
		}

		Convey("D3.1: mlwh_count_samples_for_run routes to /run/52553/samples/count", func() {
			assertCountTool(
				"/run/52553/samples/count",
				10,
				"mlwh_count_samples_for_run",
				map[string]any{"id_run": "52553"},
			)
		})

		Convey("D3.2: mlwh_count_runs_for_study returns the count from /study/S1/runs/count", func() {
			assertCountTool(
				"/study/S1/runs/count",
				2,
				"mlwh_count_runs_for_study",
				map[string]any{"study_lims_id": "S1"},
			)
		})

		Convey("D3.3: mlwh_count_libraries_for_study returns the count from /study/S1/libraries/count", func() {
			assertCountTool(
				"/study/S1/libraries/count",
				5,
				"mlwh_count_libraries_for_study",
				map[string]any{"study_lims_id": "S1"},
			)
		})

		Convey("D3.4: mlwh_count_lanes_for_sample returns the count from /sample/S1/lanes/count", func() {
			assertCountTool(
				"/sample/S1/lanes/count",
				4,
				"mlwh_count_lanes_for_sample",
				map[string]any{"sanger_name": "S1"},
			)
		})

		Convey("D3.5: mlwh_count_samples_for_library sends library and study path params in order", func() {
			assertCountTool(
				"/library/P1/study/S1/samples/count",
				3,
				"mlwh_count_samples_for_library",
				map[string]any{
					"pipeline_id_lims": "P1",
					"study_lims_id":    "S1",
				},
			)
		})

		Convey("D3.6: mlwh_count_samples_for_library_id routes to /library-id/LIB123/samples/count", func() {
			assertCountTool(
				"/library-id/LIB123/samples/count",
				6,
				"mlwh_count_samples_for_library_id",
				map[string]any{"library_id": "LIB123"},
			)
		})

		Convey("D3.7: mlwh_count_samples_for_library_lims_id routes to /library-lims-id/LIMS123/samples/count", func() {
			assertCountTool(
				"/library-lims-id/LIMS123/samples/count",
				7,
				"mlwh_count_samples_for_library_lims_id",
				map[string]any{"library_lims_id": "LIMS123"},
			)
		})

		Convey("D3.8: mlwh_count_samples_for_library_type routes to /library-type/WGS/samples/count", func() {
			assertCountTool(
				"/library-type/WGS/samples/count",
				8,
				"mlwh_count_samples_for_library_type",
				map[string]any{"library_type": "WGS"},
			)
		})
	})
}

// TestDetailTools covers Story C1: the four grouped detail tools
// (mlwh_sample_detail, mlwh_study_detail, mlwh_run_detail, mlwh_library_detail).
// Every assertion drives the tool over the real in-memory MCP client against the
// hermetic stub, so registration, the upstream path, output shape, and error
// mapping are exercised end-to-end.
func TestDetailTools(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the detail tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("C1.1: mlwh_sample_detail returns the SampleDetail aggregate for /sample/S1/detail", func() {
			stub.respondJSON("/sample/S1/detail", 200, sampleDetailS1())

			res := callTool(t, cs, "mlwh_sample_detail", map[string]any{"sanger_name": "S1"})

			obj := structuredObject(res)

			sample, ok := obj["sample"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(sample["name"], ShouldEqual, "Mus musculus A")

			lanes, ok := obj["lanes"].([]any)
			So(ok, ShouldBeTrue)
			So(len(lanes), ShouldEqual, 2)

			libraries, ok := obj["libraries"].([]any)
			So(ok, ShouldBeTrue)
			So(len(libraries), ShouldEqual, 1)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/sample/S1/detail")
		})

		Convey("C1.2: mlwh_library_detail routes to /library/P1/study/5901/detail", func() {
			stub.respondJSON("/library/P1/study/5901/detail", 200, libraryDetailP1())

			res := callTool(t, cs, "mlwh_library_detail", map[string]any{
				"pipeline_id_lims": "P1",
				"study_lims_id":    "5901",
			})

			So(res.IsError, ShouldBeFalse)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/library/P1/study/5901/detail")
		})

		Convey("C1.3: mlwh_study_detail on a 503 cache_never_synced is a tool error about the cache not being synced", func() {
			stub.respondError("/study/X/detail", 503, "cache_never_synced", "cache not synced")

			res := callTool(t, cs, "mlwh_study_detail", map[string]any{"study_lims_id": "X"})

			So(res.IsError, ShouldBeTrue)
			So(strings.ToLower(firstTextContent(res)), ShouldContainSubstring, "synced")
		})

		Convey("C1.4: the four detail tools are registered with the listed names", func() {
			for _, name := range []string{
				"mlwh_sample_detail",
				"mlwh_study_detail",
				"mlwh_run_detail",
				"mlwh_library_detail",
			} {
				_, ok := toolByName(t, cs, name)
				So(ok, ShouldBeTrue)
			}
		})
	})
}

// sampleDetailS1 is a canned SampleDetail returned by the stub for
// /sample/S1/detail. It carries a sample plus two lanes and one library so the
// C1.1 assertion can prove the assembled aggregate (sample + lanes + libraries)
// round-trips through wa's typed client and reaches StructuredContent intact.
func sampleDetailS1() wa.SampleDetail {
	return wa.SampleDetail{
		Sample: wa.Sample{IDSampleTmp: 1, Name: "Mus musculus A", SupplierName: "supA"},
		Lanes: []wa.Lane{
			{IDRun: 100, Position: 1, TagIndex: 1},
			{IDRun: 100, Position: 2, TagIndex: 2},
		},
		Libraries: []wa.Library{
			{PipelineIDLims: "P1", IDStudyLims: "5901"},
		},
	}
}

// libraryDetailP1 is a canned LibraryDetail returned by the stub for
// /library/P1/study/5901/detail, used by C1.2 to prove the two path params land
// in the right order in the upstream request path.
func libraryDetailP1() wa.LibraryDetail {
	return wa.LibraryDetail{
		Library: wa.Library{PipelineIDLims: "P1", IDStudyLims: "5901"},
		Samples: []wa.Sample{{IDSampleTmp: 1, Name: "Mus musculus A"}},
	}
}

// TestFanOutTools covers the fan-out enumeration tools, including the A3
// bounded-page behaviour for paged tools and the unchanged non-paged/count
// tools.
func TestFanOutTools(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the fan-out tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("A3.1: mlwh_all_studies with {} sends limit=100 and offset=0 and returns header page metadata", func() {
			stub.respondJSONWithHeaders("/studies", 200, threeStudies()[:2], http.Header{
				"X-Total-Count": {"250"},
				"X-Next-Offset": {"100"},
			})

			res := callTool(t, cs, "mlwh_all_studies", map[string]any{})

			obj := structuredObject(res)
			studies, ok := obj["studies"].([]any)
			So(ok, ShouldBeTrue)
			So(len(studies), ShouldEqual, 2)
			So(obj["total"], ShouldEqual, 250)
			So(obj["next_offset"], ShouldEqual, 100)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/studies")
			So(req.Query.Get("limit"), ShouldEqual, "100")
			So(req.Query.Get("limit"), ShouldNotEqual, "0")
			So(req.Query.Get("offset"), ShouldEqual, "0")
		})

		Convey("A3.2: mlwh_all_studies with limit=1001 is a tool error mentioning 1000 and makes no request", func() {
			res := callTool(t, cs, "mlwh_all_studies", map[string]any{"limit": 1001})

			So(res.IsError, ShouldBeTrue)
			So(firstTextContent(res), ShouldContainSubstring, "1000")
			So(stub.requestCount(), ShouldEqual, 0)
		})

		Convey("C2.2: mlwh_samples_for_study with explicit limit=50/offset=50 passes them through unchanged", func() {
			stub.respondJSON("/study/5901/samples", 200, twoSamples())

			callTool(t, cs, "mlwh_samples_for_study", map[string]any{
				"study_lims_id": "5901",
				"limit":         50,
				"offset":        50,
			})

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/5901/samples")
			So(req.Query.Get("limit"), ShouldEqual, "50")
			So(req.Query.Get("offset"), ShouldEqual, "50")
		})

		Convey("A3.3: mlwh_samples_for_study returns next_offset=-1 when X-Next-Offset is absent", func() {
			stub.respondJSONWithHeaders("/study/S1/samples", 200, twoSamples(), http.Header{
				"X-Total-Count": {"2"},
			})

			res := callTool(t, cs, "mlwh_samples_for_study", map[string]any{"study_lims_id": "S1"})

			obj := structuredObject(res)
			samples, ok := obj["samples"].([]any)
			So(ok, ShouldBeTrue)
			So(len(samples), ShouldEqual, 2)
			So(obj["total"], ShouldEqual, 2)
			So(obj["next_offset"], ShouldEqual, -1)
		})

		Convey("C2.3: mlwh_count_samples_for_study returns {\"count\":300} from /study/5901/samples/count", func() {
			stub.respondJSON("/study/5901/samples/count", 200, wa.Count{Count: 300})

			res := callTool(t, cs, "mlwh_count_samples_for_study", map[string]any{"study_lims_id": "5901"})

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, 300)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/5901/samples/count")
		})

		Convey("C2.4: mlwh_studies_for_sample with an empty array is IsError=false with an empty studies list", func() {
			stub.respondJSON("/sample/S1/studies", 200, []wa.Study{})

			res := callTool(t, cs, "mlwh_studies_for_sample", map[string]any{"sanger_name": "S1"})

			So(res.IsError, ShouldBeFalse)

			obj := structuredObject(res)
			studies, ok := obj["studies"].([]any)
			So(ok, ShouldBeTrue)
			So(len(studies), ShouldEqual, 0)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/sample/S1/studies")
		})
	})
}

// TestFanOutPaginatedDefaults proves the A3 bounded-page default is applied
// uniformly across the paged fan-out tools (not just mlwh_all_studies): each
// paged fan-out tool, when called without a limit, sends limit=100 and offset=0
// to its upstream path, and an explicit in-range limit is passed through
// unchanged.
func TestFanOutPaginatedDefaults(t *testing.T) {
	cases := []struct {
		tool string
		args map[string]any
		path string
	}{
		{"mlwh_all_studies", map[string]any{}, "/studies"},
		{"mlwh_samples_for_study", map[string]any{"study_lims_id": "5901"}, "/study/5901/samples"},
		{"mlwh_samples_for_run", map[string]any{"id_run": "100"}, "/run/100/samples"},
		{"mlwh_libraries_for_study", map[string]any{"study_lims_id": "5901"}, "/study/5901/libraries"},
		{"mlwh_runs_for_study", map[string]any{"study_lims_id": "5901"}, "/study/5901/runs"},
		{"mlwh_lanes_for_sample", map[string]any{"sanger_name": "S1"}, "/sample/S1/lanes"},
		{"mlwh_irods_paths_for_sample", map[string]any{"sanger_name": "S1"}, "/sample/S1/irods"},
		{"mlwh_irods_paths_for_study", map[string]any{"study_lims_id": "5901"}, "/study/5901/irods"},
	}

	Convey("Given the MLWH server (stub-backed) with the paginated fan-out tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("each paginated fan-out tool defaults an omitted limit to 100 and offset to 0", func() {
			limitMisses := 0
			offsetMisses := 0

			for _, tc := range cases {
				stub.respondJSON(tc.path, 200, []any{})

				callTool(t, cs, tc.tool, tc.args)

				req, ok := stub.lastRequest()
				if !ok || req.Path != tc.path {
					limitMisses++

					continue
				}

				if req.Query.Get("limit") != "100" {
					limitMisses++
				}

				if req.Query.Get("offset") != "0" {
					offsetMisses++
				}
			}

			So(limitMisses, ShouldEqual, 0)
			So(offsetMisses, ShouldEqual, 0)
		})

		Convey("an explicit limit on a paginated fan-out tool is passed through unchanged", func() {
			stub.respondJSON("/study/5901/libraries", 200, []any{})

			callTool(t, cs, "mlwh_libraries_for_study", map[string]any{
				"study_lims_id": "5901",
				"limit":         7,
				"offset":        3,
			})

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Query.Get("limit"), ShouldEqual, "7")
			So(req.Query.Get("offset"), ShouldEqual, "3")
		})
	})
}

// TestFanOutToolDescriptions proves the paged fan-out tools advertise the
// bounded default and maximum, and that the curated fan-out tool set is
// registered.
func TestFanOutToolDescriptions(t *testing.T) {
	paginated := []string{
		"mlwh_all_studies",
		"mlwh_samples_for_study",
		"mlwh_samples_for_run",
		"mlwh_libraries_for_study",
		"mlwh_runs_for_study",
		"mlwh_lanes_for_sample",
		"mlwh_irods_paths_for_sample",
		"mlwh_irods_paths_for_study",
	}

	Convey("Given the MLWH server (stub-backed)", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("each paginated fan-out tool's description states the bounded page default and maximum", func() {
			missing := 0

			for _, name := range paginated {
				tool, ok := toolByName(t, cs, name)
				if !ok {
					missing++

					continue
				}

				lower := strings.ToLower(tool.Description)
				if !strings.Contains(lower, "defaults to a page of 100") ||
					!strings.Contains(tool.Description, "1000") {
					missing++
				}
			}

			So(missing, ShouldEqual, 0)
		})

		Convey("the non-paginated fan-out tools are registered", func() {
			for _, name := range []string{"mlwh_studies_for_sample", "mlwh_count_samples_for_study"} {
				_, ok := toolByName(t, cs, name)
				So(ok, ShouldBeTrue)
			}
		})
	})
}
