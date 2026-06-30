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

		Convey("J1.4: an unsupported-identifier error mentions the form is unsupported", func() {
			err := mapToolError(fmt.Errorf("x: %w", wa.ErrUnsupportedIdentifier))
			So(err, ShouldNotBeNil)

			msg := strings.ToLower(err.Error())
			So(msg, ShouldContainSubstring, "identifier")
			So(msg, ShouldContainSubstring, "not supported")
		})

		Convey("J1.5: a 400 carried as ErrUpstreamImpaired preserves its upstream message", func() {
			err := mapToolError(fmt.Errorf("term too short: %w", wa.ErrUpstreamImpaired))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "term too short")
		})

		Convey("a bare ErrUpstreamImpaired (502) still maps to a non-nil actionable error", func() {
			err := mapToolError(fmt.Errorf("x: %w", wa.ErrUpstreamImpaired))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, wa.ErrUpstreamImpaired.Error())
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
				name:    "F1.7: a new manifest tool advises retrying after cache or upstream recovery",
				tool:    "mlwh_study_manifest",
				args:    map[string]any{"study_lims_id": "S1", "with_irods": true, "file_type": "cram"},
				path:    "/study/S1/manifest",
				status:  http.StatusBadGateway,
				code:    "upstream_impaired",
				message: "cache/upstream impaired while building manifest",
				substrings: []string{
					"retry",
					"cache",
					"upstream recovery",
				},
			},
			{
				name:    "F1.8: a new person tool advises fixing input or retrying later",
				tool:    "mlwh_studies_for_user",
				args:    map[string]any{"person": "cwa"},
				path:    "/studies/user/cwa",
				status:  http.StatusBadGateway,
				code:    "upstream_impaired",
				message: "person upstream impaired",
				substrings: []string{
					"fix the input",
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
