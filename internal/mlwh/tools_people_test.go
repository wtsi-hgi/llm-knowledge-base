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

// TestProgrammeTools covers E1: exact programme membership, vocabulary, and
// the discovery/freshness guidance exposed through the curated MCP tools.
func TestProgrammeTools(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with programme tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("E1.1: programme list and count use exact paths with pagination only on the list", func() {
			stub.respondJSONWithHeaders("/studies/programme/Cancer", http.StatusOK, []wa.Study{
				{IDStudyLims: "5901", Name: "Cancer study", Programme: "Cancer"},
			}, http.Header{
				"X-Total-Count": {"1"},
				"X-Next-Offset": {"-1"},
			})
			stub.respondJSON("/studies/programme/Cancer/count", http.StatusOK, wa.Count{Count: 1})

			list := callTool(t, cs, "mlwh_studies_for_programme", map[string]any{
				"programme": "Cancer",
				"limit":     5,
				"offset":    10,
			})

			listObject := structuredObject(list)
			studies, ok := listObject["studies"].([]any)
			So(ok, ShouldBeTrue)
			So(len(studies), ShouldEqual, 1)
			So(listObject["total"], ShouldEqual, 1)
			So(listObject["next_offset"], ShouldEqual, -1)

			study, ok := studies[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(study["programme"], ShouldEqual, "Cancer")

			listRequest, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(listRequest.Path, ShouldEqual, "/studies/programme/Cancer")
			So(listRequest.Query.Get("limit"), ShouldEqual, "5")
			So(listRequest.Query.Get("offset"), ShouldEqual, "10")

			count := callTool(t, cs, "mlwh_count_studies_for_programme", map[string]any{
				"programme": "Cancer",
			})

			So(structuredObject(count)["count"], ShouldEqual, 1)

			countRequest, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(countRequest.Path, ShouldEqual, "/studies/programme/Cancer/count")
			So(countRequest.Query, ShouldBeEmpty)
		})

		Convey("E1.2: programme vocabulary preserves distinct non-empty names and exact counts", func() {
			stub.respondJSON("/programmes", http.StatusOK, []wa.Programme{
				{Name: "Cancer", StudyCount: 12},
				{Name: "Rare Disease", StudyCount: 3},
			})

			res := callTool(t, cs, "mlwh_programmes", map[string]any{})

			obj := structuredObject(res)
			programmes, ok := obj["programmes"].([]any)
			So(ok, ShouldBeTrue)
			So(len(programmes), ShouldEqual, 2)
			cancer, ok := programmes[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(cancer["name"], ShouldEqual, "Cancer")
			So(cancer["study_count"], ShouldEqual, 12)
			rareDisease, ok := programmes[1].(map[string]any)
			So(ok, ShouldBeTrue)
			So(rareDisease["name"], ShouldEqual, "Rare Disease")
			So(rareDisease["study_count"], ShouldEqual, 3)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/programmes")
			So(req.Query, ShouldBeEmpty)
		})

		Convey("E1.4: programme descriptions point to exact-value discovery and cache state", func() {
			for _, name := range []string{
				"mlwh_studies_for_programme",
				"mlwh_count_studies_for_programme",
				"mlwh_programmes",
			} {
				tool, ok := toolByName(t, cs, name)
				So(ok, ShouldBeTrue)
				So(tool.Description, ShouldContainSubstring, "mlwh_programmes")
				So(tool.Description, ShouldContainSubstring, "mlwh_freshness")
			}
		})
	})
}

// TestStudyUserTools covers E2: study-to-user membership preserves the
// upstream all-role default, optional role set, page shape, errors, and the
// direction-specific routing guidance.
func TestStudyUserTools(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with inverse study-user tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("E2.1: omitted role lists and counts all role assignments without a role query", func() {
			stub.respondJSONWithHeaders("/study/S1/users", http.StatusOK, []wa.StudyUser{
				{Role: "owner", Name: "Olive Owner", Login: "oo1", Email: "oo1@example.org"},
				{Role: "manager", Name: "Maya Manager", Login: "mm1", Email: "mm1@example.org"},
				{Role: "follower", Name: "Fran Follower", Login: "ff1", Email: "ff1@example.org"},
			}, http.Header{
				"X-Total-Count": {"3"},
				"X-Next-Offset": {"-1"},
			})
			stub.respondJSON("/study/S1/users/count", http.StatusOK, wa.Count{Count: 3})

			list := callTool(t, cs, "mlwh_study_users", map[string]any{"study_lims_id": "S1"})
			users, ok := structuredObject(list)["users"].([]any)
			So(ok, ShouldBeTrue)
			So(len(users), ShouldEqual, 3)

			listRequest, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(listRequest.Path, ShouldEqual, "/study/S1/users")
			So(listRequest.Query.Get("limit"), ShouldEqual, "100")
			So(listRequest.Query.Get("offset"), ShouldEqual, "0")
			_, hasRole := listRequest.Query["role"]
			So(hasRole, ShouldBeFalse)

			count := callTool(t, cs, "mlwh_count_study_users", map[string]any{"study_lims_id": "S1"})
			So(structuredObject(count)["count"], ShouldEqual, 3)

			countRequest, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(countRequest.Path, ShouldEqual, "/study/S1/users/count")
			So(countRequest.Query, ShouldBeEmpty)
		})

		Convey("E2.2: owner,follower is forwarded identically by list and count and returns only that exact set", func() {
			stub.respondJSONWithHeaders("/study/S1/users", http.StatusOK, []wa.StudyUser{
				{Role: "owner", Name: "Olive Owner", Login: "oo1", Email: "oo1@example.org"},
				{Role: "follower", Name: "Fran Follower", Login: "ff1", Email: "ff1@example.org"},
			}, http.Header{
				"X-Total-Count": {"2"},
				"X-Next-Offset": {"-1"},
			})
			stub.respondJSON("/study/S1/users/count", http.StatusOK, wa.Count{Count: 2})

			args := map[string]any{"study_lims_id": "S1", "role": "owner,follower", "limit": 2, "offset": 1}
			list := callTool(t, cs, "mlwh_study_users", args)
			users := structuredObject(list)["users"].([]any)
			So(len(users), ShouldEqual, 2)
			So(users[0].(map[string]any)["role"], ShouldEqual, "owner")
			So(users[1].(map[string]any)["role"], ShouldEqual, "follower")

			listRequest, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(listRequest.Query.Get("role"), ShouldEqual, "owner,follower")
			So(listRequest.Query.Get("limit"), ShouldEqual, "2")
			So(listRequest.Query.Get("offset"), ShouldEqual, "1")

			count := callTool(t, cs, "mlwh_count_study_users", map[string]any{
				"study_lims_id": "S1", "role": "owner,follower",
			})
			So(structuredObject(count)["count"], ShouldEqual, 2)
			countRequest, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(countRequest.Query.Get("role"), ShouldEqual, listRequest.Query.Get("role"))
		})

		Convey("E2.3: the user page and OpenAPI-backed schema expose exact rows and semantic metadata", func() {
			stub.respondJSONWithHeaders("/study/S2/users", http.StatusOK, []wa.StudyUser{
				{Role: "administrator", Name: "Ada Admin", Login: "aa1", Email: "aa1@example.org"},
			}, http.Header{
				"X-Total-Count": {"7"},
				"X-Next-Offset": {"5"},
			})

			result := callTool(t, cs, "mlwh_study_users", map[string]any{
				"study_lims_id": "S2", "limit": 1, "offset": 4,
			})
			object := structuredObject(result)
			So(object["total"], ShouldEqual, 7)
			So(object["next_offset"], ShouldEqual, 5)
			users, ok := object["users"].([]any)
			So(ok, ShouldBeTrue)
			So(len(users), ShouldEqual, 1)
			So(users[0], ShouldResemble, map[string]any{
				"role": "administrator", "name": "Ada Admin", "login": "aa1", "email": "aa1@example.org",
			})

			tool, ok := toolByName(t, cs, "mlwh_study_users")
			So(ok, ShouldBeTrue)
			output := tool.OutputSchema.(map[string]any)
			properties := output["properties"].(map[string]any)
			So(properties, ShouldContainKey, "users")
			So(properties, ShouldContainKey, "total")
			So(properties, ShouldContainKey, "next_offset")
			items := properties["users"].(map[string]any)["items"].(map[string]any)
			userProperties := items["properties"].(map[string]any)
			So(len(userProperties), ShouldEqual, 4)
			So(userProperties, ShouldContainKey, "role")
			So(userProperties, ShouldContainKey, "name")
			So(userProperties, ShouldContainKey, "login")
			So(userProperties, ShouldContainKey, "email")
		})

		Convey("E2.4: an invalid role remains an upstream mapped tool error", func() {
			upstreamText := `invalid role "viewer"; allowed roles are owner, manager, data_access_contact, follower, slf_manager, lab_manager, administrator`
			stub.respondError("/study/S1/users", http.StatusBadRequest, "bad_request", upstreamText)

			result := callTool(t, cs, "mlwh_study_users", map[string]any{
				"study_lims_id": "S1", "role": "viewer",
			})

			So(result.IsError, ShouldBeTrue)
			So(firstTextContent(result), ShouldContainSubstring, upstreamText)
			request, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(request.Query.Get("role"), ShouldEqual, "viewer")
		})

		Convey("E2.5: descriptions distinguish inverse membership from sponsor and person defaults", func() {
			for _, name := range []string{"mlwh_study_users", "mlwh_count_study_users"} {
				tool, ok := toolByName(t, cs, name)
				So(ok, ShouldBeTrue)
				description := strings.ToLower(tool.Description)
				So(description, ShouldContainSubstring, "faculty_sponsor")
				So(description, ShouldContainSubstring, "study field")
				So(description, ShouldContainSubstring, "mlwh_studies_for_user")
				So(description, ShouldContainSubstring, "owner, manager, and data_access_contact")
			}

			list, _ := toolByName(t, cs, "mlwh_study_users")
			input := list.InputSchema.(map[string]any)
			role := input["properties"].(map[string]any)["role"].(map[string]any)
			roleDescription := strings.ToLower(role["description"].(string))
			So(roleDescription, ShouldContainSubstring, "exact case-insensitive set")
			for _, allowed := range []string{
				"owner", "manager", "data_access_contact", "follower", "slf_manager", "lab_manager", "administrator",
			} {
				So(roleDescription, ShouldContainSubstring, allowed)
			}
		})
	})
}
