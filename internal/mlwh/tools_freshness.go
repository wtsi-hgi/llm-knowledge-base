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
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

// freshnessDescription is the LLM-facing description for mlwh_freshness. It tells
// the agent the tool reports, per mirrored sync table, the last_run and
// high_water timestamps plus the ever_synced flag, and crucially that last_run
// (not high_water) is the cache-currency signal for "as of" caveats. high_water
// is only wa's sync-mode-specific source-progress watermark and may stay old
// when the source is unchanged, so it must never be phrased as "synced through".
// It also explains the per-table and overall sync status (MLWH API 1.12.0+) and
// what each status means for the user's answer, and that the tool SUCCEEDS even
// on a never-synced cache (every table then reporting ever_synced=false with
// empty timestamps) so an answer can be caveated rather than failing outright.
const freshnessDescription = "Report MLWH cache freshness and sync health: for each mirrored sync table, returns its " +
	"last_run and high_water timestamps (UTC RFC3339, empty if never synced) and an ever_synced flag. " +
	"last_run is that table's cache sync/refresh time: use it for cache-currentness and \"as of\" caveats, " +
	"and for a multi-table answer use the oldest relevant last_run. high_water is ONLY wa's " +
	"sync-mode-specific source-progress watermark: it may be a source-change time, a full-refresh snapshot " +
	"time, or empty, and it may stay old when the source data is unchanged, so never phrase it as \"synced " +
	"through\" and never use it to judge when the cache was refreshed. Also use last_run to detect a " +
	"never-synced cache. " +
	"Each table also has a status, and the top-level status is the most severe table status " +
	"(failing, then never_synced, then stale, then ok). ok: the table's last sync succeeded within " +
	"stale_after_seconds; answer normally with the last_run caveat. stale: its last successful sync " +
	"(last_success_at) is older than stale_after_seconds, so tell the user the data may be out of date " +
	"and give last_success_at; recent MLWH changes may be missing. failing: its most recent sync failed " +
	"(last_error, last_error_at) or was skipped because the dependency tables in blocked_by failed; the " +
	"cache still serves the data from its last success, so answers may be stale or incomplete for that " +
	"table: tell the user sync is failing and since when, and that the server admin should check it. " +
	"never_synced: the table has no data yet, so empty or zero results from it do not mean the data does " +
	"not exist in MLWH. Only mention status for tables relevant to the answer; an empty status means a " +
	"wa server older than MLWH API 1.12.0, so fall back to last_run. blocked_by is always an array, " +
	"empty unless the table was skipped. Takes no input. This SUCCEEDS even on a never-synced cache: " +
	"every table then reports ever_synced=false with empty timestamps (the never-synced state), so it " +
	"is not an error."

// registerFreshnessTool adds the mlwh_freshness tool (Story D1) to the server
// through the Registrar. It pre-sets the OpenAPI-sourced output schema for
// Freshness so the upstream doc: field descriptions survive (the SDK's own
// reflection would drop them). Building the schema fails only on a programming
// error (the OpenAPI document is compiled in), so such a failure is a
// registration error.
func (p *provider) registerFreshnessTool(r core.Registrar) error {
	outputSchema, err := outputSchemaFor("Freshness")
	if err != nil {
		return fmt.Errorf("mlwh: build freshness output schema: %w", err)
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_freshness",
		Description:  freshnessDescription,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, wa.Freshness, error) {
		// Freshness reads sync_state directly and the /freshness endpoint returns
		// 200 even on a never-synced cache, so a successful call (including the
		// never-synced state) is returned as-is; only a genuine upstream failure
		// is mapped to a tool error.
		freshness, err := client.Freshness(ctx)
		if err != nil {
			return core.ToolError[wa.Freshness](mapToolError(err))
		}

		return nil, normaliseFreshness(freshness), nil
	})

	return nil
}

// normaliseFreshness makes every table's blocked_by an empty array rather than
// null. A wa server older than MLWH API 1.12.0 omits blocked_by, and a JSON
// null decodes to a nil slice; either would marshal as null, which the
// OpenAPI-sourced output schema (an array) rejects.
func normaliseFreshness(freshness wa.Freshness) wa.Freshness {
	for i := range freshness.Tables {
		if freshness.Tables[i].BlockedBy == nil {
			freshness.Tables[i].BlockedBy = []string{}
		}
	}

	return freshness
}
