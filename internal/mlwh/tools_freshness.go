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
// the agent the tool reports, per mirrored sync table, the high-water mark and
// last-run timestamps plus the ever_synced flag, and that it SUCCEEDS even on a
// never-synced cache (every table then reporting ever_synced=false with empty
// timestamps) so an answer can be caveated rather than failing outright.
const freshnessDescription = "Report MLWH cache freshness: for each mirrored sync table, returns its " +
	"high_water mark and last_run timestamp (UTC RFC3339, empty if never synced) and an ever_synced flag. " +
	"Use it to caveat answers about data staleness and to detect a never-synced cache. Takes no input. " +
	"This SUCCEEDS even on a never-synced cache: every table then reports ever_synced=false with empty " +
	"timestamps (the never-synced state), so it is not an error."

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

		return nil, freshness, nil
	})

	return nil
}
