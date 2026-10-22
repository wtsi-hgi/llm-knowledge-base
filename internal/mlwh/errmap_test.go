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
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	wa "github.com/wtsi-hgi/wa/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMapToolError(t *testing.T) {
	Convey("mapToolError turns wa/mlwh sentinels into clear tool errors", t, func() {
		Convey("J1.1: a joined never-synced+not-found maps to cache-not-synced, not not-found", func() {
			err := mapToolError(errors.Join(wa.ErrCacheNeverSynced, wa.ErrNotFound))
			So(err, ShouldNotBeNil)

			msg := strings.ToLower(err.Error())
			So(msg, ShouldContainSubstring, "synced")
			So(msg, ShouldNotContainSubstring, "not found")
		})

		Convey("J1.2: a not-found error mentions the identifier was not found", func() {
			err := mapToolError(fmt.Errorf("x: %w", wa.ErrNotFound))
			So(err, ShouldNotBeNil)

			msg := strings.ToLower(err.Error())
			So(msg, ShouldContainSubstring, "not found")
		})

		Convey("J1.3: an ambiguous error mentions multiple records and disambiguating", func() {
			err := mapToolError(fmt.Errorf("x: %w", wa.ErrAmbiguous))
			So(err, ShouldNotBeNil)

			msg := strings.ToLower(err.Error())
			So(msg, ShouldContainSubstring, "multiple")
			So(msg, ShouldContainSubstring, "disambiguat")
		})

		Convey("J1.7: an ambiguous error routes a shared supplier name or donor id to find, search, or multi-sample export", func() {
			err := mapToolError(fmt.Errorf("x: %w", wa.ErrAmbiguous))
			So(err, ShouldNotBeNil)

			msg := err.Error()
			So(msg, ShouldContainSubstring, "supplier name or donor id")
			So(msg, ShouldContainSubstring, "mlwh_find_samples field=supplier_name value=<the name>")
			So(msg, ShouldContainSubstring, "mlwh_search_samples term=")
			So(msg, ShouldContainSubstring, "multi_sample=true")
		})

		Convey("J1.9: an API 1.13.0 ambiguity keeps the named candidate Sanger samples and says to use them", func() {
			err := mapToolError(fmt.Errorf("%s: %w", `"PD42487a" matches 2 samples: A, B`, wa.ErrAmbiguous))
			So(err, ShouldNotBeNil)

			msg := err.Error()
			So(msg, ShouldStartWith, `"PD42487a" matches 2 samples: A, B: `+wa.ErrAmbiguous.Error()+" (")
			So(msg, ShouldContainSubstring, "names candidate Sanger samples")
			So(msg, ShouldContainSubstring, "use directly")
		})

		Convey("J1.8: a multi-sample-unsupported error wins over its wrapped ambiguity and advises retrying without multi_sample", func() {
			upstream := fmt.Errorf("%w: \"PD1\" matches samples 2 and 3", wa.ErrAmbiguous)
			err := mapToolError(&wa.MultiSampleUnsupportedError{
				ServerURL: "http://old.example", ServerAPIVersion: "1.10.0", Err: upstream,
			})
			So(err, ShouldNotBeNil)

			msg := err.Error()
			So(msg, ShouldContainSubstring, "http://old.example runs API 1.10.0")
			So(msg, ShouldContainSubstring, "needs API 1.11.0: \"PD1\" matches samples 2 and 3")
			So(msg, ShouldContainSubstring, "retry without multi_sample")
			So(msg, ShouldContainSubstring, "mlwh_search_samples term=")
			So(msg, ShouldNotContainSubstring, "mlwh_find_samples")
			So(msg, ShouldNotContainSubstring, "multi_sample=true")
			So(strings.Count(msg, wa.ErrMultiSampleUnsupported.Error()), ShouldEqual, 1)
			So(msg, ShouldNotContainSubstring, wa.ErrAmbiguous.Error())
		})

		Convey("J1.4: an unsupported-identifier error mentions the form is unsupported", func() {
			err := mapToolError(fmt.Errorf("x: %w", wa.ErrUnsupportedIdentifier))
			So(err, ShouldNotBeNil)

			msg := strings.ToLower(err.Error())
			So(msg, ShouldContainSubstring, "identifier")
			So(msg, ShouldContainSubstring, "not supported")
		})

		Convey("J1.5: a 400 bad request keeps its upstream message once and says to fix the input", func() {
			err := mapToolError(&wa.RemoteRequestError{StatusCode: http.StatusBadRequest, Code: "bad_request", Message: "limit must not exceed 1000"})
			So(err, ShouldNotBeNil)

			msg := err.Error()
			So(msg, ShouldStartWith, "limit must not exceed 1000: "+wa.ErrBadRequest.Error()+" (")
			So(strings.Count(msg, wa.ErrBadRequest.Error()), ShouldEqual, 1)
			So(msg, ShouldNotContainSubstring, wa.ErrRequestRejected.Error())
			So(msg, ShouldNotContainSubstring, wa.ErrUpstreamImpaired.Error())
			So(msg, ShouldContainSubstring, "fix the input")
			So(msg, ShouldNotContainSubstring, "retry later")
		})

		Convey("J1.10: another rejected 4xx keeps its status, code, and message and says to fix the request", func() {
			err := mapToolError(&wa.RemoteRequestError{StatusCode: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "use GET"})
			So(err, ShouldNotBeNil)

			msg := err.Error()
			So(msg, ShouldStartWith, "use GET: "+wa.ErrRequestRejected.Error()+" (405 method_not_allowed) (")
			So(strings.Count(msg, "405 method_not_allowed"), ShouldEqual, 1)
			So(msg, ShouldNotContainSubstring, wa.ErrBadRequest.Error())
			So(msg, ShouldContainSubstring, "fix the request")
			So(msg, ShouldNotContainSubstring, "retry later")
		})

		Convey("J1.11: a rejected 4xx with a non-envelope body and no message keeps just its status", func() {
			err := mapToolError(&wa.RemoteRequestError{StatusCode: http.StatusForbidden})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldStartWith, wa.ErrRequestRejected.Error()+" (403) (")
		})

		Convey("a bare ErrUpstreamImpaired (502) says to retry later, not to fix the input", func() {
			err := mapToolError(fmt.Errorf("x: %w", wa.ErrUpstreamImpaired))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, wa.ErrUpstreamImpaired.Error())
			So(err.Error(), ShouldContainSubstring, "retry later")
			So(err.Error(), ShouldNotContainSubstring, "fix the input")
		})

		Convey("J1.6: a nil error maps to nil", func() {
			So(mapToolError(nil), ShouldBeNil)
		})

		Convey("an unrecognised (non-sentinel) error is returned with its message preserved", func() {
			err := mapToolError(errors.New("some random failure"))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "some random failure")
		})
	})
}

func TestF1ToolErrorMapping(t *testing.T) {
	Convey("F1: new MLWH tools map upstream errors to actionable MCP tool errors", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		cases := []struct {
			name       string
			tool       string
			args       map[string]any
			path       string
			status     int
			code       string
			message    string
			substrings []string
		}{
			{
				name:    "F1.1: a new list tool preserves invalid file_type bad-request text",
				tool:    "mlwh_irods_paths_for_study",
				args:    map[string]any{"study_lims_id": "S1", "file_type": " "},
				path:    "/study/S1/irods",
				status:  http.StatusBadRequest,
				code:    "bad_request",
				message: "invalid file_type: must be a non-empty suffix",
				substrings: []string{
					"invalid file_type",
				},
			},
			{
				name:    "F1.2: mlwh_resolve_person preserves whitespace person-term guidance",
				tool:    "mlwh_resolve_person",
				args:    map[string]any{"term": " "},
				path:    "/resolve-person/ ",
				status:  http.StatusBadRequest,
				code:    "bad_request",
				message: "person term must not be blank",
				substrings: []string{
					"person term",
					"must not be blank",
				},
			},
			{
				name:    "F1.3: a new study tool tells callers to disambiguate ambiguous studies",
				tool:    "mlwh_study_overview",
				args:    map[string]any{"study_lims_id": "ambiguous-study"},
				path:    "/study/ambiguous-study/overview",
				status:  http.StatusConflict,
				code:    "ambiguous_identifier",
				message: "ambiguous study identifier; disambiguate the study",
				substrings: []string{
					"disambiguate",
					"study",
				},
			},
			{
				name:    "F1.4: mlwh_irods_paths_for_run preserves unsupported run identifier text",
				tool:    "mlwh_irods_paths_for_run",
				args:    map[string]any{"id_run": "ONT-1", "file_type": "cram"},
				path:    "/run/ONT-1/irods",
				status:  http.StatusBadRequest,
				code:    "unsupported_identifier",
				message: "run identifier is unsupported for this query",
				substrings: []string{
					"run identifier",
					"unsupported",
				},
			},
			{
				name:    "F1.5: a new aggregate tool tells callers to check a missing identifier",
				tool:    "mlwh_run_overview",
				args:    map[string]any{"id_run": "missing-run"},
				path:    "/run/missing-run/overview",
				status:  http.StatusNotFound,
				code:    "not_found",
				message: "not_found: check identifier",
				substrings: []string{
					"not_found",
					"check identifier",
				},
			},
			{
				name:    "F1.6: a new aggregate tool says the cache has never synced",
				tool:    "mlwh_study_status_breakdown",
				args:    map[string]any{"study_lims_id": "S1"},
				path:    "/study/S1/status-breakdown",
				status:  http.StatusServiceUnavailable,
				code:    "cache_never_synced",
				message: "cache_never_synced: cache has never synced",
				substrings: []string{
					"cache_never_synced",
					"never synced",
				},
			},
			{
				name:    "F1.8: a new person tool advises retrying an impaired upstream later",
				tool:    "mlwh_studies_for_user",
				args:    map[string]any{"person": "cwa"},
				path:    "/studies/user/cwa",
				status:  http.StatusBadGateway,
				code:    "upstream_impaired",
				message: "person upstream impaired",
				substrings: []string{
					"person upstream impaired",
					"retry later",
				},
			},
		}

		for _, tc := range cases {
			Convey(tc.name, func() {
				stub.respondError(tc.path, tc.status, tc.code, tc.message)

				res := callTool(t, cs, tc.tool, tc.args)

				So(res.IsError, ShouldBeTrue)
				text := strings.ToLower(firstTextContent(res))
				for _, substring := range tc.substrings {
					So(text, ShouldContainSubstring, substring)
				}
			})
		}
	})
}

func TestF3NewToolErrorMapping(t *testing.T) {
	Convey("F3.3: every upstream sentinel keeps its precedence, context, and actionable hint through a new tool", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		cases := []struct {
			tool       string
			path       string
			arguments  map[string]any
			status     int
			code       string
			message    string
			substrings []string
		}{
			{
				tool: "mlwh_latest_data_for_study", path: "/study/S1/latest-data",
				arguments: map[string]any{"study_lims_id": "S1"}, status: http.StatusBadRequest,
				code: "bad_request", message: "since must be RFC3339", substrings: []string{"since must be rfc3339: mlwh: bad request (", "fix the input"},
			},
			{
				tool: "mlwh_sample_crams_for_study", path: "/study/MISSING/sample-crams",
				arguments: map[string]any{"study_lims_id": "MISSING"}, status: http.StatusNotFound,
				code: "not_found", message: "study MISSING is absent", substrings: []string{"study missing is absent", "not found"},
			},
			{
				tool: "mlwh_studies_for_programme", path: "/studies/programme/Cancer",
				arguments: map[string]any{"programme": "Cancer"}, status: http.StatusConflict,
				code: "ambiguous", message: "programme matches multiple records", substrings: []string{"multiple records", "disambiguate"},
			},
			{
				tool: "mlwh_runs", path: "/runs", arguments: map[string]any{}, status: http.StatusUnprocessableEntity,
				code: "unsupported_identifier", message: "run cursor kind is unsupported", substrings: []string{"run cursor kind is unsupported", "not supported"},
			},
			{
				tool: "mlwh_study_users", path: "/study/S1/users",
				arguments: map[string]any{"study_lims_id": "S1"}, status: http.StatusBadGateway,
				code: "upstream_impaired", message: "warehouse unavailable", substrings: []string{"warehouse unavailable", "retry later"},
			},
			{
				tool: "mlwh_runs_for_sample", path: "/sample/S1/runs",
				arguments: map[string]any{"sanger_name": "S1"}, status: http.StatusServiceUnavailable,
				code: "cache_never_synced", message: "cache has never synced", substrings: []string{"never synced", "retry"},
			},
		}

		for index, testCase := range cases {
			stub.respondError(testCase.path, testCase.status, testCase.code, testCase.message)
			result := callTool(t, cs, testCase.tool, testCase.arguments)
			So(result.IsError, ShouldBeTrue)
			text := strings.ToLower(firstTextContent(result))
			for _, substring := range testCase.substrings {
				So(text, ShouldContainSubstring, substring)
			}
			So(stub.requestCount(), ShouldEqual, index+1)
		}
	})

	Convey("F3.4: a 4xx with no sentinel code is a rejected request to fix, not an impaired upstream to retry", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		stub.respondError("/study/S1/users", http.StatusForbidden, "forbidden", "admins only")
		result := callTool(t, cs, "mlwh_study_users", map[string]any{"study_lims_id": "S1"})
		So(result.IsError, ShouldBeTrue)

		text := firstTextContent(result)
		So(text, ShouldStartWith, "admins only: mlwh: request rejected (403 forbidden) (")
		So(text, ShouldContainSubstring, "fix the request")
		So(text, ShouldNotContainSubstring, wa.ErrUpstreamImpaired.Error())
		So(text, ShouldNotContainSubstring, "retry later")
	})

	Convey("F3.5: a wa v1.1.0 never-synced envelope on a list endpoint maps to the sync remedy with no not-found text", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		stub.respondError("/sample/S1/runs", http.StatusServiceUnavailable, "cache_never_synced", wa.ErrCacheNeverSynced.Error())
		result := callTool(t, cs, "mlwh_runs_for_sample", map[string]any{"sanger_name": "S1"})
		So(result.IsError, ShouldBeTrue)

		text := firstTextContent(result)
		So(text, ShouldStartWith, wa.ErrCacheNeverSynced.Error()+" (")
		So(strings.Count(text, wa.ErrCacheNeverSynced.Error()), ShouldEqual, 1)
		So(strings.ToLower(text), ShouldNotContainSubstring, "not found")
		So(text, ShouldContainSubstring, "retry once a sync has run")
	})

	Convey("F3.3: cache-never-synced wins over a joined not-found sentinel without leaking the wrong remedy", t, func() {
		err := mapToolError(fmt.Errorf("loading page: %w", errors.Join(wa.ErrNotFound, wa.ErrCacheNeverSynced)))
		So(err, ShouldNotBeNil)
		message := strings.ToLower(err.Error())
		So(message, ShouldContainSubstring, "loading page")
		So(message, ShouldContainSubstring, "never been synced")
		So(message, ShouldNotContainSubstring, "not found")
	})
}
