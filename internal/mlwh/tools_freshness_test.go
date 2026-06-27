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
	"testing"

	wa "github.com/wtsi-hgi/wa/mlwh"

	. "github.com/smartystreets/goconvey/convey"
)

// TestFreshnessTool covers Story D1: the mlwh_freshness tool. Every assertion
// drives the tool over the real in-memory MCP client against the hermetic stub,
// so registration, the /freshness path, the per-table output shape, and the
// never-synced no-error behaviour are exercised end-to-end.
func TestFreshnessTool(t *testing.T) {
	Convey("Given the MLWH server (stub-backed) with the freshness tool", t, func() {
		stub := newStubMLWH(t)
		cs, cleanup := runMLWHServerWithClient(t, stub)
		defer cleanup()

		Convey("D1.1: a Freshness with five synced tables yields those five table entries in StructuredContent", func() {
			stub.respondJSON("/freshness", 200, syncedFreshness())

			res := callTool(t, cs, "mlwh_freshness", map[string]any{})

			obj := structuredObject(res)

			tables, ok := obj["tables"].([]any)
			So(ok, ShouldBeTrue)
			So(len(tables), ShouldEqual, 5)

			everSynced := 0
			withHighWater := 0

			for _, raw := range tables {
				entry, ok := raw.(map[string]any)
				So(ok, ShouldBeTrue)

				if synced, _ := entry["ever_synced"].(bool); synced {
					everSynced++
				}

				if hw, _ := entry["high_water"].(string); hw != "" {
					withHighWater++
				}
			}

			So(everSynced, ShouldEqual, 5)
			So(withHighWater, ShouldEqual, 5)

			req, ok := stub.lastRequest()
			So(ok, ShouldBeTrue)
			So(req.Path, ShouldEqual, "/freshness")
		})

		Convey("D1.2: a never-synced Freshness (200) is NOT an error and reflects the never-synced state", func() {
			stub.respondJSON("/freshness", 200, neverSyncedFreshness())

			res := callTool(t, cs, "mlwh_freshness", map[string]any{})

			So(res.IsError, ShouldBeFalse)

			obj := structuredObject(res)

			tables, ok := obj["tables"].([]any)
			So(ok, ShouldBeTrue)
			So(len(tables), ShouldEqual, 5)

			everSynced := 0
			withTimestamps := 0

			for _, raw := range tables {
				entry, ok := raw.(map[string]any)
				So(ok, ShouldBeTrue)

				if synced, _ := entry["ever_synced"].(bool); synced {
					everSynced++
				}

				hw, _ := entry["high_water"].(string)
				lr, _ := entry["last_run"].(string)
				if hw != "" || lr != "" {
					withTimestamps++
				}
			}

			So(everSynced, ShouldEqual, 0)
			So(withTimestamps, ShouldEqual, 0)
		})

		Convey("D1.3: the tool is named mlwh_freshness and accepts empty input {}", func() {
			tool, ok := toolByName(t, cs, "mlwh_freshness")
			So(ok, ShouldBeTrue)

			schema, ok := tool.InputSchema.(map[string]any)
			So(ok, ShouldBeTrue)
			So(schema["type"], ShouldEqual, "object")

			stub.respondJSON("/freshness", 200, syncedFreshness())

			res := callTool(t, cs, "mlwh_freshness", map[string]any{})
			So(res.IsError, ShouldBeFalse)
		})

		Convey("D1: the description states it reports timestamps and ever_synced and succeeds on a never-synced cache", func() {
			tool, ok := toolByName(t, cs, "mlwh_freshness")
			So(ok, ShouldBeTrue)
			So(tool.Description, ShouldContainSubstring, "ever_synced")
			So(tool.Description, ShouldContainSubstring, "never-synced")
		})
	})
}

// syncedFreshness is a canned Freshness returned by the stub for /freshness with
// five tables, each ever_synced=true and carrying non-empty timestamps, used by
// D1.1 to prove a fully-synced cache round-trips its five entries to
// StructuredContent. The five table names mirror wa's mirrored sync tables.
func syncedFreshness() wa.Freshness {
	tables := make([]wa.TableFreshness, 0, 5)

	for _, name := range []string{"sample", "study", "iseq_flowcell", "iseq_product_metrics", "seq_product_irods_locations"} {
		tables = append(tables, wa.TableFreshness{
			Table:      name,
			HighWater:  "2026-06-27T00:00:00Z",
			LastRun:    "2026-06-27T01:00:00Z",
			EverSynced: true,
		})
	}

	return wa.Freshness{Tables: tables}
}

// neverSyncedFreshness is a canned Freshness returned by the stub for /freshness
// where every table reports ever_synced=false with empty timestamps. The
// /freshness endpoint returns 200 even when never-synced, so D1.2 proves the
// tool surfaces this as a successful (IsError=false) result, not an error.
func neverSyncedFreshness() wa.Freshness {
	tables := make([]wa.TableFreshness, 0, 5)

	for _, name := range []string{"sample", "study", "iseq_flowcell", "iseq_product_metrics", "seq_product_irods_locations"} {
		tables = append(tables, wa.TableFreshness{Table: name})
	}

	return wa.Freshness{Tables: tables}
}
