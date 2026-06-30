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

// TestPeopleTools covers D2: the faculty_sponsor tools, study_users tools, and
// resolve-person tools keep their routing and descriptions distinct.
func TestPeopleTools(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with person-aware tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("D2.1: mlwh_studies_for_faculty_sponsor returns studies without role from the sponsor path", func() {
			stub.respondJSONWithHeaders("/studies/faculty-sponsor/Carl", http.StatusOK, []wa.PersonStudy{
				{
					Study: wa.Study{
						IDStudyLims:    "5901",
						Name:           "Carl Study",
						FacultySponsor: "Carl Anderson",
					},
				},
			}, http.Header{
				"X-Total-Count": {"1"},
				"X-Next-Offset": {"-1"},
			})

			res := callTool(t, cs, "mlwh_studies_for_faculty_sponsor", map[string]any{"name": "Carl"})

			obj := structuredObject(res)
			studies, ok := obj["studies"].([]any)
			So(ok, ShouldBeTrue)
			So(len(studies), ShouldEqual, 1)
			So(obj["total"], ShouldEqual, 1)
			So(obj["next_offset"], ShouldEqual, -1)

			first, ok := studies[0].(map[string]any)
			So(ok, ShouldBeTrue)
			_, hasRole := first["role"]
			So(hasRole, ShouldBeFalse)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/studies/faculty-sponsor/Carl")
		})

		Convey("D2.2: mlwh_studies_for_user without role omits role and returns upstream default-role rows", func() {
			stub.respondJSONWithHeaders("/studies/user/cwa", http.StatusOK, []wa.PersonStudy{
				{Study: wa.Study{IDStudyLims: "5901", Name: "Owner Study"}, Role: "owner"},
				{Study: wa.Study{IDStudyLims: "5902", Name: "DAC Study"}, Role: "data_access_contact"},
			}, http.Header{
				"X-Total-Count": {"2"},
				"X-Next-Offset": {"-1"},
			})

			res := callTool(t, cs, "mlwh_studies_for_user", map[string]any{"person": "cwa"})

			obj := structuredObject(res)
			studies, ok := obj["studies"].([]any)
			So(ok, ShouldBeTrue)
			So(len(studies), ShouldEqual, 2)
			So(obj["total"], ShouldEqual, 2)
			So(obj["next_offset"], ShouldEqual, -1)

			first, ok := studies[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(first["role"], ShouldEqual, "owner")
			second, ok := studies[1].(map[string]any)
			So(ok, ShouldBeTrue)
			So(second["role"], ShouldEqual, "data_access_contact")

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/studies/user/cwa")
			So(req.Query.Get("limit"), ShouldEqual, "100")
			So(req.Query.Get("offset"), ShouldEqual, "0")
			_, hasRole := req.Query["role"]
			So(hasRole, ShouldBeFalse)
		})

		Convey("D2.3: mlwh_count_studies_for_user without role omits role and returns the default-role count", func() {
			stub.respondJSON("/studies/user/cwa/count", http.StatusOK, wa.Count{Count: 2})

			res := callTool(t, cs, "mlwh_count_studies_for_user", map[string]any{"person": "cwa"})

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, 2)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/studies/user/cwa/count")
			_, hasRole := req.Query["role"]
			So(hasRole, ShouldBeFalse)
		})

		Convey("the faculty sponsor count counterpart uses the sponsor count endpoint", func() {
			stub.respondJSON("/studies/faculty-sponsor/Carl/count", http.StatusOK, wa.Count{Count: 1})

			res := callTool(t, cs, "mlwh_count_studies_for_faculty_sponsor", map[string]any{"name": "Carl"})

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, 1)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/studies/faculty-sponsor/Carl/count")
		})

		Convey("D2.4: mlwh_studies_for_user preserves a supplied role query exactly", func() {
			stub.respondJSONWithHeaders("/studies/user/cwa", http.StatusOK, []wa.PersonStudy{
				{Study: wa.Study{IDStudyLims: "5903", Name: "Follower Study"}, Role: "follower"},
			}, http.Header{
				"X-Total-Count": {"1"},
				"X-Next-Offset": {"-1"},
			})

			res := callTool(t, cs, "mlwh_studies_for_user", map[string]any{
				"person": "cwa",
				"role":   "Follower",
			})

			obj := structuredObject(res)
			studies, ok := obj["studies"].([]any)
			So(ok, ShouldBeTrue)
			So(len(studies), ShouldEqual, 1)
			first, ok := studies[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(first["role"], ShouldEqual, "follower")

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Query.Get("role"), ShouldEqual, "Follower")
			So(req.Query.Get("limit"), ShouldEqual, "100")
			So(req.Query.Get("offset"), ShouldEqual, "0")
		})

		Convey("D2.5: mlwh_count_studies_for_user preserves a supplied role query exactly", func() {
			stub.respondJSON("/studies/user/cwa/count", http.StatusOK, wa.Count{Count: 1})

			res := callTool(t, cs, "mlwh_count_studies_for_user", map[string]any{
				"person": "cwa",
				"role":   "DATA_ACCESS_CONTACT",
			})

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, 1)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/studies/user/cwa/count")
			So(req.Query.Get("role"), ShouldEqual, "DATA_ACCESS_CONTACT")
		})

		Convey("D2.6: mlwh_resolve_person wraps candidates under people with page metadata", func() {
			stub.respondJSONWithHeaders("/resolve-person/carl", http.StatusOK, []wa.PersonCandidate{
				{Source: "faculty_sponsor", Name: "Carl Anderson", StudyCount: 1},
				{Source: "study_users", Name: "Carl A.", Login: "cwa", Email: "cwa@example.org", Role: "owner", StudyCount: 2},
			}, http.Header{
				"X-Total-Count": {"2"},
				"X-Next-Offset": {"-1"},
			})

			res := callTool(t, cs, "mlwh_resolve_person", map[string]any{"term": "carl"})

			obj := structuredObject(res)
			people, ok := obj["people"].([]any)
			So(ok, ShouldBeTrue)
			So(len(people), ShouldEqual, 2)
			So(obj["total"], ShouldEqual, 2)
			So(obj["next_offset"], ShouldEqual, -1)

			for _, row := range people {
				person, ok := row.(map[string]any)
				So(ok, ShouldBeTrue)
				So(person, ShouldContainKey, "source")
				So(person, ShouldContainKey, "name")
				So(person, ShouldContainKey, "study_count")
			}

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/resolve-person/carl")
			So(req.Query.Get("limit"), ShouldEqual, "100")
			So(req.Query.Get("offset"), ShouldEqual, "0")
		})

		Convey("the resolve-person count counterpart uses the resolve-person count endpoint", func() {
			stub.respondJSON("/resolve-person/carl/count", http.StatusOK, wa.Count{Count: 2})

			res := callTool(t, cs, "mlwh_count_resolve_person", map[string]any{"term": "carl"})

			obj := structuredObject(res)
			So(obj["count"], ShouldEqual, 2)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/resolve-person/carl/count")
		})

		Convey("D2.7: person tool descriptions preserve faculty_sponsor versus study_users wording", func() {
			for _, name := range []string{
				"mlwh_studies_for_faculty_sponsor",
				"mlwh_count_studies_for_faculty_sponsor",
				"mlwh_studies_for_user",
				"mlwh_count_studies_for_user",
				"mlwh_resolve_person",
				"mlwh_count_resolve_person",
			} {
				_, ok := toolByName(t, cs, name)
				So(ok, ShouldBeTrue)
			}

			sponsor, ok := toolByName(t, cs, "mlwh_studies_for_faculty_sponsor")
			So(ok, ShouldBeTrue)
			So(sponsor.Description, ShouldContainSubstring, "faculty_sponsor")

			user, ok := toolByName(t, cs, "mlwh_studies_for_user")
			So(ok, ShouldBeTrue)
			So(user.Description, ShouldContainSubstring, "study_users")
			So(strings.ToLower(user.Description), ShouldContainSubstring, "default role")
		})
	})
}
