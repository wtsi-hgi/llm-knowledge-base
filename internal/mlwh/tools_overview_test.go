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
	"net/http"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	wa "github.com/wtsi-hgi/wa/mlwh"
)

// TestStudyOverviewTool covers Story B1: the small study overview tool. It
// drives the real MCP tool against the hermetic MLWH stub so registration,
// upstream routing, structured output, and error mapping are exercised
// end-to-end.
func TestStudyOverviewTool(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the study overview tool", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("B1.1: mlwh_study_overview returns the StudyOverview fields unchanged", func() {
			stub.respondJSON("/study/S1/overview", 200, studyOverviewS1())

			res := callTool(t, cs, "mlwh_study_overview", map[string]any{"study_lims_id": "S1"})

			obj := structuredObject(res)
			So(obj["id_study_lims"], ShouldEqual, "S1")
			So(obj["samples_total"], ShouldEqual, 5)
			So(obj["samples_with_data"], ShouldEqual, 3)
			So(obj["samples_without_data"], ShouldEqual, 2)
			So(obj["samples_sequenced_no_data"], ShouldEqual, 1)
			So(obj["data_access_group"], ShouldEqual, "dag1")
			So(obj["newest_data_added"], ShouldEqual, "2026-06-26T00:00:00Z")
			So(obj["added_last_7_days"], ShouldEqual, 2)
			So(obj["cache_synced_at"], ShouldEqual, "2026-06-30T09:00:00Z")
		})

		Convey("B1.2: mlwh_study_overview calls /study/S1/overview and no list endpoint", func() {
			stub.respondJSON("/study/S1/overview", 200, studyOverviewS1())

			res := callTool(t, cs, "mlwh_study_overview", map[string]any{"study_lims_id": "S1"})
			So(res.IsError, ShouldBeFalse)

			So(stub.requestCount(), ShouldEqual, 1)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/S1/overview")
			So(req.Path, ShouldNotEqual, "/study/S1/samples")
		})

		Convey("B1.3: mlwh_study_overview maps 404 not_found to an actionable identifier error", func() {
			stub.respondError("/study/MISSING/overview", 404, "not_found", "study not found")

			res := callTool(t, cs, "mlwh_study_overview", map[string]any{"study_lims_id": "MISSING"})

			So(res.IsError, ShouldBeTrue)

			lower := strings.ToLower(firstTextContent(res))
			So(lower, ShouldContainSubstring, "check")
			So(lower, ShouldContainSubstring, "identifier")
		})
	})
}

func studyOverviewS1() wa.StudyOverview {
	return wa.StudyOverview{
		IDStudyLims:            "S1",
		DataAccessGroup:        "dag1",
		SamplesTotal:           5,
		SamplesWithData:        3,
		SamplesWithoutData:     2,
		SamplesSequencedNoData: 1,
		LibraryTypes:           []string{},
		NewestDataAdded:        "2026-06-26T00:00:00Z",
		AddedLast7Days:         2,
		CacheSyncedAt:          "2026-06-30T09:00:00Z",
	}
}

// TestStudyStatusBreakdownTool covers Story B2: the study status breakdown
// aggregate. It proves the MCP tool returns wa.StatusBreakdown unchanged and
// routes through the documented upstream endpoint.
func TestStudyStatusBreakdownTool(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the study status breakdown tool", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("B2.1: mlwh_study_status_breakdown returns ladders, QC, timeline, and cache fields unchanged", func() {
			stub.respondJSON("/study/S1/status-breakdown", http.StatusOK, statusBreakdownS1())

			res := callTool(t, cs, "mlwh_study_status_breakdown", map[string]any{"study_lims_id": "S1"})

			obj := structuredObject(res)
			So(obj["id_study_lims"], ShouldEqual, "S1")

			distinct, ok := obj["distinct"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(distinct["with_data"], ShouldEqual, 3)
			So(distinct["sequenced_no_data"], ShouldEqual, 1)
			So(distinct["registered"], ShouldEqual, 1)

			qc, ok := obj["qc"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(qc["qc_pass"], ShouldEqual, 2)
			So(qc["qc_fail"], ShouldEqual, 1)
			So(qc["qc_pending"], ShouldEqual, 1)

			perPlatform, ok := obj["per_platform"].([]any)
			So(ok, ShouldBeTrue)
			So(len(perPlatform), ShouldEqual, 1)

			ont, ok := perPlatform[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(ont["platform"], ShouldEqual, "ONT")

			ladder, ok := ont["ladder"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(ladder["with_data"], ShouldEqual, 0)
			So(ladder["sequenced_no_data"], ShouldEqual, 0)
			So(ladder["registered"], ShouldEqual, 1)

			So(obj["with_detailed_timeline"], ShouldEqual, 2)
			So(obj["cache_synced_at"], ShouldEqual, "2026-06-30T09:00:00Z")
		})

		Convey("B2.2: mlwh_study_status_breakdown routes to /study/S1/status-breakdown", func() {
			stub.respondJSON("/study/S1/status-breakdown", http.StatusOK, statusBreakdownS1())

			res := callTool(t, cs, "mlwh_study_status_breakdown", map[string]any{"study_lims_id": "S1"})

			So(res.IsError, ShouldBeFalse)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/study/S1/status-breakdown")
		})

		Convey("B2.3: a never-synced joined upstream error reports cache sync before not-found wording", func() {
			stub.respondError(
				"/study/S1/status-breakdown",
				http.StatusServiceUnavailable,
				"cache_never_synced",
				errors.Join(wa.ErrCacheNeverSynced, wa.ErrNotFound).Error(),
			)

			res := callTool(t, cs, "mlwh_study_status_breakdown", map[string]any{"study_lims_id": "S1"})

			So(res.IsError, ShouldBeTrue)

			lower := strings.ToLower(firstTextContent(res))
			syncIndex := strings.Index(lower, "sync")
			notFoundIndex := strings.Index(lower, "not found")
			if notFoundIndex == -1 {
				notFoundIndex = len(lower)
			}

			So(syncIndex, ShouldBeGreaterThanOrEqualTo, 0)
			So(syncIndex, ShouldBeLessThan, notFoundIndex)
		})
	})
}

func statusBreakdownS1() wa.StatusBreakdown {
	return wa.StatusBreakdown{
		IDStudyLims: "S1",
		Distinct: wa.PhaseLadder{
			WithData:        3,
			SequencedNoData: 1,
			Registered:      1,
		},
		PerPlatform: []wa.PlatformPhaseLadder{
			{
				Platform: "ONT",
				Ladder: wa.PhaseLadder{
					Registered: 1,
				},
			},
		},
		QC: wa.StudyQCBreakdown{
			QCPass:    2,
			QCFail:    1,
			QCPending: 1,
		},
		WithDetailedTimeline: 2,
		CacheSyncedAt:        "2026-06-30T09:00:00Z",
	}
}

// TestOverviewTools covers Phase 5 B3: the run overview, run status, and sample
// progress tools. Each assertion drives the registered MCP tool over a real
// in-memory client backed by the hermetic MLWH stub.
func TestOverviewTools(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the overview tools", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("B3.1: mlwh_run_overview returns run counts and uses /run/52553/overview", func() {
			stub.respondJSON("/run/52553/overview", 200, runOverview52553())

			res := callTool(t, cs, "mlwh_run_overview", map[string]any{"id_run": "52553"})

			obj := structuredObject(res)
			So(obj["id_run"], ShouldEqual, 52553)
			So(obj["samples"], ShouldEqual, 10)
			So(obj["studies"], ShouldEqual, 2)
			So(obj["data_objects"], ShouldEqual, 4)
			So(obj["cache_synced_at"], ShouldEqual, "2026-06-01T10:00:00Z")

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/run/52553/overview")
		})

		Convey("B3.2: mlwh_run_status returns ordered events/current with no cache_synced_at", func() {
			stub.respondJSON("/run/52553/status", 200, runStatus52553())

			res := callTool(t, cs, "mlwh_run_status", map[string]any{"id_run": "52553"})

			obj := structuredObject(res)
			_, hasCacheSyncedAt := obj["cache_synced_at"]
			So(hasCacheSyncedAt, ShouldBeFalse)
			So(obj["current"], ShouldEqual, "qc complete")

			events, ok := obj["events"].([]any)
			So(ok, ShouldBeTrue)
			So(len(events), ShouldEqual, 2)

			first, ok := events[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(first["phase"], ShouldEqual, "run pending")

			second, ok := events[1].(map[string]any)
			So(ok, ShouldBeTrue)
			So(second["phase"], ShouldEqual, "qc complete")
		})

		Convey("B3.3: mlwh_sample_progress preserves delivered, QC, milestone, run, and cache fields", func() {
			stub.respondJSON("/sample/S1/progress", 200, sampleProgressS1())

			res := callTool(t, cs, "mlwh_sample_progress", map[string]any{"sanger_name": "S1"})

			obj := structuredObject(res)
			So(obj["baseline_phase"], ShouldEqual, "delivered")
			So(obj["qc"], ShouldEqual, "pass")
			So(obj["delivered_at"], ShouldEqual, "2026-06-02T12:00:00Z")
			So(obj["detailed_timeline"], ShouldEqual, true)
			So(obj["current_milestone"], ShouldEqual, "sequencing_qc_complete")
			So(obj["cache_synced_at"], ShouldEqual, "2026-06-03T08:00:00Z")

			milestones, ok := obj["milestones"].([]any)
			So(ok, ShouldBeTrue)
			So(len(milestones), ShouldEqual, 1)
			milestone, ok := milestones[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(milestone["name"], ShouldEqual, "sequencing_qc_complete")
			So(milestone["reached_at"], ShouldEqual, "2026-06-02T11:45:00Z")

			runs, ok := obj["runs"].([]any)
			So(ok, ShouldBeTrue)
			So(len(runs), ShouldEqual, 1)
			run, ok := runs[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(run["id_run"], ShouldEqual, 52553)
			So(run["current"], ShouldEqual, "qc complete")
		})

		Convey("B3.4: mlwh_sample_progress preserves ONT/not_tracked and empty runs", func() {
			stub.respondJSON("/sample/ONT1/progress", 200, sampleProgressONT1())

			res := callTool(t, cs, "mlwh_sample_progress", map[string]any{"sanger_name": "ONT1"})

			obj := structuredObject(res)
			platforms, ok := obj["platforms"].([]any)
			So(ok, ShouldBeTrue)
			So(platforms, ShouldResemble, []any{"ONT"})
			So(obj["qc"], ShouldEqual, "not_tracked")
			So(obj["detailed_timeline"], ShouldEqual, false)

			runs, ok := obj["runs"].([]any)
			So(ok, ShouldBeTrue)
			So(len(runs), ShouldEqual, 0)
		})

		Convey("B3.5: mlwh_run_status description explains the cache as-of caveat", func() {
			tool, ok := toolByName(t, cs, "mlwh_run_status")
			So(ok, ShouldBeTrue)

			description := strings.ToLower(tool.Description)
			So(description, ShouldContainSubstring, "no cache_synced_at")
			So(description, ShouldContainSubstring, "mlwh_freshness")
			So(description, ShouldContainSubstring, "cache as-of caveat")
		})
	})
}

func runOverview52553() wa.RunOverview {
	return wa.RunOverview{
		IDRun:         52553,
		Samples:       10,
		Studies:       2,
		DataObjects:   4,
		CacheSyncedAt: "2026-06-01T10:00:00Z",
	}
}

func sampleProgressS1() wa.SampleProgress {
	return wa.SampleProgress{
		Sample: wa.Sample{
			IDSampleTmp:  1,
			IDSampleLims: "S1",
			Name:         "S1",
		},
		Platforms:        []string{"Illumina"},
		BaselinePhase:    "delivered",
		QC:               "pass",
		DeliveredAt:      "2026-06-02T12:00:00Z",
		DetailedTimeline: true,
		Milestones: []wa.Milestone{
			{
				Name:      "sequencing_qc_complete",
				ReachedAt: "2026-06-02T11:45:00Z",
			},
		},
		CurrentMilestone: "sequencing_qc_complete",
		Runs:             []wa.RunStatusTimeline{runStatus52553()},
		CacheSyncedAt:    "2026-06-03T08:00:00Z",
	}
}

func runStatus52553() wa.RunStatusTimeline {
	return wa.RunStatusTimeline{
		IDRun:    52553,
		Platform: "Illumina",
		Events: []wa.RunStatusEvent{
			{
				Phase:     "run pending",
				EnteredAt: "2026-06-01T09:00:00Z",
				Duration:  "PT2H",
			},
			{
				Phase:     "qc complete",
				EnteredAt: "2026-06-01T11:00:00Z",
			},
		},
		Current: "qc complete",
	}
}

func sampleProgressONT1() map[string]any {
	return map[string]any{
		"sample": map[string]any{
			"id_sample_tmp":  2,
			"id_sample_lims": "ONT1",
			"name":           "ONT1",
		},
		"platforms":         []string{"ONT"},
		"baseline_phase":    "registered",
		"qc":                "not_tracked",
		"delivered_at":      "",
		"detailed_timeline": false,
		"runs":              []wa.RunStatusTimeline{},
		"cache_synced_at":   "2026-06-03T08:00:00Z",
	}
}
