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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStudyManifestToolSchemaAndDescription(t *testing.T) {
	Convey("Given the registered mlwh_study_manifest tool", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		tool, ok := toolByName(t, cs, "mlwh_study_manifest")
		So(ok, ShouldBeTrue)

		Convey("C3.5: the output schema exposes flattened manifest rows and paging metadata", func() {
			schema, ok := tool.OutputSchema.(map[string]any)
			So(ok, ShouldBeTrue)

			properties, ok := schema["properties"].(map[string]any)
			So(ok, ShouldBeTrue)
			_, wrapped := properties["study_manifest"]
			So(wrapped, ShouldBeFalse)
			So(properties, ShouldContainKey, "total")
			So(properties, ShouldContainKey, "next_offset")

			rows, ok := properties["rows"].(map[string]any)
			So(ok, ShouldBeTrue)
			items, ok := rows["items"].(map[string]any)
			So(ok, ShouldBeTrue)
			rowProperties, ok := items["properties"].(map[string]any)
			So(ok, ShouldBeTrue)

			missing := 0
			for _, field := range []string{
				"name",
				"supplier_name",
				"accession_number",
				"sanger_sample_id",
				"id_run",
				"lane",
				"tag_index",
				"irods_path",
			} {
				if _, ok := rowProperties[field]; !ok {
					missing++
				}
			}

			So(missing, ShouldEqual, 0)
		})

		Convey("the description advertises with_irods/file_type semantics and bounded paging", func() {
			lower := strings.ToLower(tool.Description)
			So(lower, ShouldContainSubstring, "with_irods")
			So(lower, ShouldContainSubstring, "file_type")
			So(tool.Description, ShouldContainSubstring, "does NOT default to cram")
			So(lower, ShouldContainSubstring, "defaults to a page of 100")
			So(tool.Description, ShouldContainSubstring, "1000")
		})
	})
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

// TestIRODSToolsC2 covers Story C2: sample/study/run iRODS path tools accept
// optional upstream file_type filtering, expose page metadata, and provide count
// counterparts for bounded cram-path workflows.
func TestIRODSToolsC2(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the iRODS availability tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("C2.1: mlwh_irods_paths_for_study sends file_type and returns iRODS rows plus page metadata", func() {
			stub.respondJSONWithHeaders("/study/S1/irods", http.StatusOK, []wa.IRODSPath{
				{IDProduct: "P1", IDRun: 52553, Platform: "illumina"},
			}, irodsPageHeaders("4", "-1"))

			res := callTool(t, cs, "mlwh_irods_paths_for_study", map[string]any{
				"study_lims_id": "S1",
				"file_type":     "cram",
			})

			obj := structuredObject(res)
			paths, ok := obj["irods_paths"].([]any)
			So(ok, ShouldBeTrue)
			So(len(paths), ShouldEqual, 1)
			path, ok := paths[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(path["id_run"], ShouldEqual, 52553)
			So(path["platform"], ShouldEqual, "illumina")
			So(obj["total"], ShouldEqual, 4)
			So(obj["next_offset"], ShouldEqual, -1)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/S1/irods")
			So(req.Query.Get("file_type"), ShouldEqual, "cram")
			So(req.Query.Get("limit"), ShouldEqual, "100")
			So(req.Query.Get("offset"), ShouldEqual, "0")
		})

		Convey("C2.2: mlwh_count_irods_paths_for_sample preserves a dotted uppercase file_type", func() {
			stub.respondJSON("/sample/S1/irods/count", http.StatusOK, wa.Count{Count: 4})

			res := callTool(t, cs, "mlwh_count_irods_paths_for_sample", map[string]any{
				"sanger_name": "S1",
				"file_type":   ".CRAM",
			})

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, 4)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/sample/S1/irods/count")
			So(req.Query.Get("file_type"), ShouldEqual, ".CRAM")
		})

		Convey("C2.3: mlwh_count_irods_paths_for_study preserves file_type and returns the count", func() {
			stub.respondJSON("/study/S1/irods/count", http.StatusOK, wa.Count{Count: 9})

			res := callTool(t, cs, "mlwh_count_irods_paths_for_study", map[string]any{
				"study_lims_id": "S1",
				"file_type":     "cram",
			})

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, 9)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/S1/irods/count")
			So(req.Query.Get("file_type"), ShouldEqual, "cram")
		})

		Convey("C2.4: mlwh_irods_paths_for_sample returns semantic irods_paths with all IRODSPath fields", func() {
			stub.respondJSONWithHeaders("/sample/S1/irods", http.StatusOK, []wa.IRODSPath{
				{
					IDProduct:   "P1",
					Collection:  "/seq/1",
					DataObject:  "a.cram",
					IRODSPath:   "/seq/1/a.cram",
					IDSampleTmp: 123,
					Name:        "S1",
					IDRun:       52553,
					Platform:    "illumina",
				},
			}, irodsPageHeaders("1", "-1"))

			res := callTool(t, cs, "mlwh_irods_paths_for_sample", map[string]any{
				"sanger_name": "S1",
				"file_type":   "cram",
			})

			obj := structuredObject(res)
			paths, ok := obj["irods_paths"].([]any)
			So(ok, ShouldBeTrue)
			So(len(paths), ShouldEqual, 1)
			_, hasItems := obj["items"]
			So(hasItems, ShouldBeFalse)
			path, ok := paths[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(path["id_product"], ShouldEqual, "P1")
			So(path["collection"], ShouldEqual, "/seq/1")
			So(path["data_object"], ShouldEqual, "a.cram")
			So(path["irods_path"], ShouldEqual, "/seq/1/a.cram")
			So(path["id_sample_tmp"], ShouldEqual, 123)
			So(path["name"], ShouldEqual, "S1")
			So(path["id_run"], ShouldEqual, 52553)
			So(path["platform"], ShouldEqual, "illumina")
			So(obj["total"], ShouldEqual, 1)
			So(obj["next_offset"], ShouldEqual, -1)
		})

		Convey("C2.5: mlwh_irods_paths_for_run calls the run iRODS path and wraps rows under irods_paths", func() {
			stub.respondJSONWithHeaders("/run/52553/irods", http.StatusOK, []wa.IRODSPath{
				{IDProduct: "P1", IRODSPath: "/seq/1/a.cram"},
				{IDProduct: "P2", IRODSPath: "/seq/2/b.cram"},
			}, irodsPageHeaders("2", "-1"))

			res := callTool(t, cs, "mlwh_irods_paths_for_run", map[string]any{
				"id_run":    "52553",
				"file_type": "cram",
			})

			obj := structuredObject(res)
			paths, ok := obj["irods_paths"].([]any)
			So(ok, ShouldBeTrue)
			So(len(paths), ShouldEqual, 2)
			_, hasItems := obj["items"]
			So(hasItems, ShouldBeFalse)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/run/52553/irods")
			So(req.Query.Get("file_type"), ShouldEqual, "cram")
		})

		Convey("C2.6: mlwh_irods_paths_for_study returns an empty page for an unmatched valid suffix", func() {
			stub.respondJSONWithHeaders("/study/S1/irods", http.StatusOK, []wa.IRODSPath{}, irodsPageHeaders("0", "-1"))

			res := callTool(t, cs, "mlwh_irods_paths_for_study", map[string]any{
				"study_lims_id": "S1",
				"file_type":     "vcf",
			})

			obj := structuredObject(res)
			paths, ok := obj["irods_paths"].([]any)
			So(ok, ShouldBeTrue)
			So(len(paths), ShouldEqual, 0)
			So(obj["total"], ShouldEqual, 0)
			So(obj["next_offset"], ShouldEqual, -1)
		})

		Convey("C2.7: mlwh_count_irods_paths_for_run returns exactly a zero count for an unmatched valid suffix", func() {
			stub.respondJSON("/run/52553/irods/count", http.StatusOK, wa.Count{Count: 0})

			res := callTool(t, cs, "mlwh_count_irods_paths_for_run", map[string]any{
				"id_run":    "52553",
				"file_type": "vcf",
			})

			obj := structuredObject(res)
			So(len(obj), ShouldEqual, 1)
			So(obj["count"], ShouldEqual, 0)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/run/52553/irods/count")
			So(req.Query.Get("file_type"), ShouldEqual, "vcf")
		})

		Convey("C2.8: invalid iRODS file_type values are left to upstream and mapped as tool errors", func() {
			failures := countInvalidIRODSToolFailures(t, cs, stub, []string{" ", "%", "_"})

			So(failures, ShouldEqual, 0)
		})

		Convey("C2.9: a slash file_type is left to upstream and mapped as a tool error", func() {
			failures := countInvalidIRODSToolFailures(t, cs, stub, []string{"/"})

			So(failures, ShouldEqual, 0)
		})
	})
}

func irodsPageHeaders(total, nextOffset string) http.Header {
	return http.Header{
		"X-Total-Count": {total},
		"X-Next-Offset": {nextOffset},
	}
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
		stub.respondError(tool.path, http.StatusBadRequest, "upstream_impaired", "invalid file_type")
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

func TestStudyManifestTools(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the study manifest tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("C3.1: mlwh_study_manifest flattens manifest metadata, rows, cache freshness, and page headers", func() {
			stub.respondJSONWithHeaders("/study/S1/manifest", http.StatusOK, studyManifestS1("/irods/a.cram"), http.Header{
				"X-Total-Count": {"3"},
				"X-Next-Offset": {"-1"},
			})

			res := callTool(t, cs, "mlwh_study_manifest", map[string]any{
				"study_lims_id": "S1",
				"with_irods":    true,
				"file_type":     "cram",
			})

			obj := structuredObject(res)
			So(obj["id_study_lims"], ShouldEqual, "S1")
			So(obj["name"], ShouldEqual, "Study S1")
			So(obj["cache_synced_at"], ShouldEqual, "2026-06-30T09:00:00Z")
			So(obj["total"], ShouldEqual, 3)
			So(obj["next_offset"], ShouldEqual, -1)
			_, wrapped := obj["study_manifest"]
			So(wrapped, ShouldBeFalse)

			rows, ok := obj["rows"].([]any)
			So(ok, ShouldBeTrue)
			So(len(rows), ShouldEqual, 1)

			row, ok := rows[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(row["name"], ShouldEqual, "S1")
			So(row["supplier_name"], ShouldEqual, "Supplier 1")
			So(row["accession_number"], ShouldEqual, "ERS1")
			So(row["sanger_sample_id"], ShouldEqual, "SANG1")
			So(row["id_run"], ShouldEqual, 52553)
			So(row["lane"], ShouldEqual, 1)
			So(row["tag_index"], ShouldEqual, 2)
			So(row["irods_path"], ShouldEqual, "/irods/a.cram")

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/S1/manifest")
			So(req.Query.Get("with_irods"), ShouldEqual, "true")
			So(req.Query.Get("file_type"), ShouldEqual, "cram")
			So(req.Query.Get("limit"), ShouldEqual, "100")
			So(req.Query.Get("offset"), ShouldEqual, "0")
		})

		Convey("C3.2: mlwh_count_study_manifest returns exactly the upstream count", func() {
			stub.respondJSON("/study/S1/manifest/count", http.StatusOK, wa.Count{Count: 3})

			res := callTool(t, cs, "mlwh_count_study_manifest", map[string]any{"study_lims_id": "S1"})

			obj := structuredObject(res)
			So(len(obj), ShouldEqual, 1)
			So(obj["count"], ShouldEqual, 3)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/S1/manifest/count")
			So(req.Query.Encode(), ShouldEqual, "")
		})

		Convey("C3.3: with_irods=false omits with_irods and rows omit irods_path", func() {
			stub.respondJSONWithHeaders("/study/S1/manifest", http.StatusOK, studyManifestS1(""), http.Header{
				"X-Total-Count": {"1"},
				"X-Next-Offset": {"-1"},
			})

			res := callTool(t, cs, "mlwh_study_manifest", map[string]any{
				"study_lims_id": "S1",
				"with_irods":    false,
			})

			obj := structuredObject(res)
			rows := obj["rows"].([]any)
			row := rows[0].(map[string]any)
			_, hasIRODSPath := row["irods_path"]
			So(hasIRODSPath, ShouldBeFalse)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Query.Get("with_irods"), ShouldEqual, "")
			So(req.Query.Get("file_type"), ShouldEqual, "")
			So(req.Query.Get("limit"), ShouldEqual, "100")
			So(req.Query.Get("offset"), ShouldEqual, "0")
		})

		Convey("C3.4: with_irods=true without file_type does not default to cram", func() {
			stub.respondJSONWithHeaders("/study/S1/manifest", http.StatusOK, studyManifestS1("/irods/a.bam"), http.Header{
				"X-Total-Count": {"1"},
				"X-Next-Offset": {"-1"},
			})

			res := callTool(t, cs, "mlwh_study_manifest", map[string]any{
				"study_lims_id": "S1",
				"with_irods":    true,
			})

			obj := structuredObject(res)
			rows := obj["rows"].([]any)
			row := rows[0].(map[string]any)
			So(row["irods_path"], ShouldEqual, "/irods/a.bam")

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Query.Get("with_irods"), ShouldEqual, "true")
			So(req.Query.Get("file_type"), ShouldEqual, "")
			So(req.Query.Encode(), ShouldNotContainSubstring, "file_type=cram")
		})

		Convey("A3/C3: mlwh_study_manifest rejects limit over 1000 before HTTP", func() {
			before := stub.requestCount()

			res := callTool(t, cs, "mlwh_study_manifest", map[string]any{
				"study_lims_id": "S1",
				"limit":         1001,
			})

			So(res.IsError, ShouldBeTrue)
			So(firstTextContent(res), ShouldContainSubstring, "1000")
			So(stub.requestCount(), ShouldEqual, before)
		})
	})
}

func studyManifestS1(irodsPath string) wa.StudyManifest {
	return wa.StudyManifest{
		IDStudyLims:     "S1",
		Name:            "Study S1",
		AccessionNumber: "EGAS1",
		FacultySponsor:  "Faculty Sponsor",
		DataAccessGroup: "dag1",
		Rows: []wa.ManifestRow{
			{
				Name:            "S1",
				SupplierName:    "Supplier 1",
				AccessionNumber: "ERS1",
				SangerSampleID:  "SANG1",
				IDRun:           52553,
				Position:        1,
				TagIndex:        2,
				IRODSPath:       irodsPath,
			},
		},
		CacheSyncedAt: "2026-06-30T09:00:00Z",
	}
}
