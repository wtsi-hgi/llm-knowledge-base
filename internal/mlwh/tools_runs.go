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

const globalRunsPaginationNote = " Defaults to a page of 100 rows, maximum 1000 " +
	"(a larger limit is rejected, not clamped). To continue, pass the last row's id as the next cursor; " +
	"no total or next_offset is returned."

// sequencingAggregateInput requires explicit dimensions and count unit, plus
// the date/platform filters shared with the run aggregate endpoints.
type sequencingAggregateInput struct {
	GroupBy  []string `json:"group_by" jsonschema:"required grouping dimensions: month, platform, manufacturer, programme, or faculty_sponsor"`
	Unit     string   `json:"unit" jsonschema:"required counted unit: runs, samples, or products"`
	Since    string   `json:"since,omitempty" jsonschema:"optional inclusive lower date bound; runs use per-platform dates, samples/products use iRODS created"`
	Until    string   `json:"until,omitempty" jsonschema:"optional exclusive upper date bound; runs use per-platform dates, samples/products use iRODS created"`
	Platform []string `json:"platform,omitempty" jsonschema:"optional repeatable platform filters passed through unchanged"`
}

func (p *provider) addSequencingAggregate(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("SequencingAggregate")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_sequencing_aggregate",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in sequencingAggregateInput) (*mcp.CallToolResult, sequencingAggregatesResult, error) {
		rows, err := client.SequencingAggregate(ctx, wa.SequencingAggregateOptions{
			GroupBy: in.GroupBy, Unit: in.Unit, Since: in.Since, Until: in.Until, Platforms: in.Platform,
		})
		if err != nil {
			return core.ToolError[sequencingAggregatesResult](mapToolError(err))
		}

		return nil, sequencingAggregatesResult{Aggregates: rows}, nil
	})

	return nil
}

func runAggregationOptions(since, until string, platforms []string) wa.RunAggregationOptions {
	return wa.RunAggregationOptions{Since: since, Until: until, Platforms: platforms}
}

// registerRunTools adds the global run list, count, and server-side aggregate
// tools. Their descriptions come from the upstream Registry so run grain and
// platform-specific date semantics stay aligned with the API contract.
func (p *provider) registerRunTools(r core.Registrar) error {
	listingSchema, err := outputSchemaForSlice("runs", "RunListingRow")
	if err != nil {
		return fmt.Errorf("mlwh: build global runs output schema: %w", err)
	}

	countSchema, err := outputSchemaFor("Count")
	if err != nil {
		return fmt.Errorf("mlwh: build global run count output schema: %w", err)
	}

	monthlySchema, err := outputSchemaForSlice("monthly_run_counts", "MonthlyRunCount")
	if err != nil {
		return fmt.Errorf("mlwh: build monthly run counts output schema: %w", err)
	}

	aggregateSchema, err := outputSchemaForSlice("aggregates", "SequencingAggregateRow")
	if err != nil {
		return fmt.Errorf("mlwh: build sequencing aggregate output schema: %w", err)
	}

	if err := p.addRunListing(r, listingSchema); err != nil {
		return err
	}

	if err := p.addCountRunListing(r, countSchema); err != nil {
		return err
	}

	if err := p.addMonthlyRunCounts(r, monthlySchema); err != nil {
		return err
	}

	return p.addSequencingAggregate(r, aggregateSchema)
}

// runListingInput is the global run filter set plus bounded keyset controls.
type runListingInput struct {
	Since    string   `json:"since,omitempty" jsonschema:"optional inclusive lower bound over each platform's labelled run-date basis; passed through unchanged"`
	Until    string   `json:"until,omitempty" jsonschema:"optional exclusive upper bound over each platform's labelled run-date basis; passed through unchanged"`
	Platform []string `json:"platform,omitempty" jsonschema:"optional repeatable platform filters passed through unchanged"`
	Limit    int      `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Cursor   string   `json:"cursor,omitempty" jsonschema:"keyset cursor; pass the last row's composite id to continue"`
}

func (p *provider) addRunListing(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("RunListing")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_runs",
		Description:  description + globalRunsPaginationNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runListingInput) (*mcp.CallToolResult, runListingsResult, error) {
		limit, _, err := boundedPagination(in.Limit, 0)
		if err != nil {
			return core.ToolError[runListingsResult](err)
		}

		runs, err := client.RunListing(ctx, runAggregationOptions(in.Since, in.Until, in.Platform), limit, in.Cursor)
		if err != nil {
			return core.ToolError[runListingsResult](mapToolError(err))
		}

		return nil, runListingsResult{Runs: runs}, nil
	})

	return nil
}

// runFiltersInput is the shared date/platform input for global run counts and
// later run aggregates. Platform is repeatable upstream.
type runFiltersInput struct {
	Since    string   `json:"since,omitempty" jsonschema:"optional inclusive lower bound over each platform's labelled run-date basis; passed through unchanged"`
	Until    string   `json:"until,omitempty" jsonschema:"optional exclusive upper bound over each platform's labelled run-date basis; passed through unchanged"`
	Platform []string `json:"platform,omitempty" jsonschema:"optional repeatable platform filters passed through unchanged"`
}

func (p *provider) addCountRunListing(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountRunListing")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_runs",
		Description:  description + countFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runFiltersInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountRunListing(ctx, runAggregationOptions(in.Since, in.Until, in.Platform))
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

func (p *provider) addMonthlyRunCounts(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("MonthlyRunCounts")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_monthly_run_counts",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runFiltersInput) (*mcp.CallToolResult, monthlyRunCountsResult, error) {
		rows, err := client.MonthlyRunCounts(ctx, runAggregationOptions(in.Since, in.Until, in.Platform))
		if err != nil {
			return core.ToolError[monthlyRunCountsResult](mapToolError(err))
		}

		return nil, monthlyRunCountsResult{MonthlyRunCounts: rows}, nil
	})

	return nil
}
