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

// sampleProgressOutput preserves the upstream SampleProgress shape while keeping
// runs visible as [] for platforms such as ONT that have no within-sequencing
// run timelines. The wa type has omitempty on Runs for server-side HTTP
// responses; the curated MCP tool needs the empty array to be explicit.
type sampleProgressOutput struct {
	wa.SampleProgress
	Runs []wa.RunStatusTimeline `json:"runs"`
}

func newSampleProgressOutput(progress wa.SampleProgress) sampleProgressOutput {
	runs := progress.Runs
	if runs == nil {
		runs = []wa.RunStatusTimeline{}
	}

	return sampleProgressOutput{SampleProgress: progress, Runs: runs}
}

// addSampleProgress registers mlwh_sample_progress (Story B3): it returns the
// sample's baseline phase, QC, optional milestone timeline, runs, and cache
// freshness exactly as the upstream SampleProgress reports them.
func (p *provider) addSampleProgress(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("SampleProgress")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_sample_progress",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in sampleNameInput) (*mcp.CallToolResult, sampleProgressOutput, error) {
		progress, err := client.SampleProgress(ctx, in.SangerName)
		if err != nil {
			return core.ToolError[sampleProgressOutput](mapToolError(err))
		}

		return nil, newSampleProgressOutput(progress), nil
	})

	return nil
}

// registerOverviewTools adds the fixed-size aggregate/status tools from phase 5
// to the server. Each tool returns the upstream aggregate unchanged with an
// OpenAPI-sourced output schema so doc: field descriptions survive in MCP
// metadata.
func (p *provider) registerOverviewTools(r core.Registrar) error {
	schemas := map[string]map[string]any{}

	for _, component := range []string{"StudyOverview", "StatusBreakdown", "RunOverview", "RunStatusTimeline", "SampleProgress"} {
		schema, err := outputSchemaFor(component)
		if err != nil {
			return fmt.Errorf("mlwh: build %s output schema: %w", component, err)
		}

		schemas[component] = schema
	}

	if err := p.addStudyOverview(r, schemas["StudyOverview"]); err != nil {
		return err
	}

	if err := p.addStudyStatusBreakdown(r, schemas["StatusBreakdown"]); err != nil {
		return err
	}

	if err := p.addRunOverview(r, schemas["RunOverview"]); err != nil {
		return err
	}

	if err := p.addRunStatus(r, schemas["RunStatusTimeline"]); err != nil {
		return err
	}

	return p.addSampleProgress(r, schemas["SampleProgress"])
}

// addStudyOverview registers mlwh_study_overview (Story B1): it returns the
// upstream StudyOverview aggregate unchanged for one LIMS study identifier.
func (p *provider) addStudyOverview(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("StudyOverview")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_study_overview",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyIDInput) (*mcp.CallToolResult, wa.StudyOverview, error) {
		overview, err := client.StudyOverview(ctx, in.StudyLimsID)
		if err != nil {
			return core.ToolError[wa.StudyOverview](mapToolError(err))
		}

		return nil, overview, nil
	})

	return nil
}

// addStudyStatusBreakdown registers mlwh_study_status_breakdown (Story B2): it
// returns the upstream StatusBreakdown aggregate exactly as MLWH reports it,
// preserving distinct and per-platform ladders, QC split, detailed-timeline
// count, and cache freshness.
func (p *provider) addStudyStatusBreakdown(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("StatusBreakdown")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_study_status_breakdown",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyIDInput) (*mcp.CallToolResult, wa.StatusBreakdown, error) {
		breakdown, err := client.StatusBreakdown(ctx, in.StudyLimsID)
		if err != nil {
			return core.ToolError[wa.StatusBreakdown](mapToolError(err))
		}

		return nil, breakdown, nil
	})

	return nil
}

// addRunOverview registers mlwh_run_overview (Story B3): it returns the fixed
// size run aggregate for the given Illumina NPG run id.
func (p *provider) addRunOverview(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("RunOverview")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_run_overview",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runIDInput) (*mcp.CallToolResult, wa.RunOverview, error) {
		overview, err := client.RunOverview(ctx, in.IDRun)
		if err != nil {
			return core.ToolError[wa.RunOverview](mapToolError(err))
		}

		return nil, overview, nil
	})

	return nil
}

// addRunStatus registers mlwh_run_status (Story B3): it returns the ordered
// within-sequencing status timeline. The response intentionally has no
// cache_synced_at field, so the description points callers at mlwh_freshness for
// the cache caveat.
func (p *provider) addRunStatus(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("RunStatus")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_run_status",
		Description:  description + " This response has no cache_synced_at; call mlwh_freshness for the cache as-of caveat.",
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runIDInput) (*mcp.CallToolResult, wa.RunStatusTimeline, error) {
		timeline, err := client.RunStatus(ctx, in.IDRun)
		if err != nil {
			return core.ToolError[wa.RunStatusTimeline](mapToolError(err))
		}

		return nil, timeline, nil
	})

	return nil
}
