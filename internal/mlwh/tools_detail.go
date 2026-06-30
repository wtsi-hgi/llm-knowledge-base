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

// pagedFanOutPaginationNote is appended to every paged fan-out tool's
// description so the agent sees the A3 bounded-page default and maximum even
// when the upstream Registry wording is terse.
const pagedFanOutPaginationNote = " Defaults to a page of 100 rows, maximum 1000 " +
	"(a larger limit is rejected, not clamped); use offset to page."

// addFanOutCountTool registers a typed Count tool backed by one Registry count
// method. The caller supplies the input-shaping closure so JSON field names stay
// explicit at the tool boundary while the shared handler preserves upstream error
// mapping and the OpenAPI-derived Count schema.
func addFanOutCountTool[Input any](
	r core.Registrar,
	outputSchema map[string]any,
	toolName string,
	registryMethod string,
	call func(context.Context, Input) (wa.Count, error),
) error {
	description, err := resolveDescription(registryMethod)
	if err != nil {
		return err
	}

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         toolName,
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in Input) (*mcp.CallToolResult, wa.Count, error) {
		count, err := call(ctx, in)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

// fanOutSliceSchemas builds the paged slice-wrapper output schemas shared by
// the paged fan-out tools, keyed by the wrapper property name so each tool
// picks the one matching its element type.
func fanOutSliceSchemas() (map[string]map[string]any, error) {
	specs := []struct {
		property  string
		component string
	}{
		{"studies", "Study"},
		{"samples", "Sample"},
		{"libraries", "Library"},
		{"runs", "Run"},
		{"lanes", "Lane"},
		{"irods_paths", "IRODSPath"},
	}

	schemas := make(map[string]map[string]any, len(specs))

	for _, spec := range specs {
		schema, err := outputSchemaForPagedSlice(spec.property, spec.component)
		if err != nil {
			return nil, fmt.Errorf("mlwh: build %s output schema: %w", spec.property, err)
		}

		schemas[spec.property] = schema
	}

	return schemas, nil
}

// paginatedFanOutDescription derives a paged fan-out tool's LLM-facing
// description from its Registry entry (Summary + Description) and appends the
// bounded pagination note.
func paginatedFanOutDescription(method string) (string, error) {
	base, err := resolveDescription(method)
	if err != nil {
		return "", err
	}

	return base + pagedFanOutPaginationNote, nil
}

// registerDetailTools adds the grouped detail tools (Story C1) and the fan-out
// enumeration/count tools (Story C2/D3) to the server through the Registrar.
// Each typed tool pre-sets its OpenAPI-sourced output schema so the upstream doc: field
// descriptions survive (the SDK's own reflection would drop them), and every
// handler maps an upstream error to a clear tool error via mapToolError. The
// paged fan-out tools default an omitted limit to 100 and reject limits above
// 1000 before HTTP. Building a schema or deriving a description fails only on a
// programming error (the schemas and Registry are compiled in), so such a
// failure is a registration error.
func (p *provider) registerDetailTools(r core.Registrar) error {
	if err := p.registerDetailGroup(r); err != nil {
		return err
	}

	if err := p.registerFanOutTools(r); err != nil {
		return err
	}

	return nil
}

// registerDetailGroup adds the four entity detail tools (Story C1), each
// returning its typed detail aggregate with a pre-set OpenAPI output schema.
func (p *provider) registerDetailGroup(r core.Registrar) error {
	schemas := map[string]map[string]any{}

	for _, component := range []string{"SampleDetail", "StudyDetail", "RunDetail", "LibraryDetail"} {
		schema, err := outputSchemaFor(component)
		if err != nil {
			return fmt.Errorf("mlwh: build %s output schema: %w", component, err)
		}

		schemas[component] = schema
	}

	if err := p.addSampleDetail(r, schemas["SampleDetail"]); err != nil {
		return err
	}

	if err := p.addStudyDetail(r, schemas["StudyDetail"]); err != nil {
		return err
	}

	if err := p.addRunDetail(r, schemas["RunDetail"]); err != nil {
		return err
	}

	return p.addLibraryDetail(r, schemas["LibraryDetail"])
}

// sampleNameInput is the input for the sample-keyed tools that take only a
// Sanger sample name and no pagination (mlwh_sample_detail,
// mlwh_studies_for_sample, mlwh_count_lanes_for_sample).
type sampleNameInput struct {
	SangerName string `json:"sanger_name" jsonschema:"the Sanger sample name to look up"`
}

// addSampleDetail registers mlwh_sample_detail (Story C1): it returns the given
// sample (by Sanger sample name) with its study, lanes, libraries, and iRODS
// paths, the SampleDetail aggregate.
func (p *provider) addSampleDetail(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("SampleDetail")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_sample_detail",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in sampleNameInput) (*mcp.CallToolResult, wa.SampleDetail, error) {
		detail, err := client.SampleDetail(ctx, in.SangerName)
		if err != nil {
			return core.ToolError[wa.SampleDetail](mapToolError(err))
		}

		return nil, detail, nil
	})

	return nil
}

// studyIDInput is the input for the study-keyed tools that take only a LIMS
// study id and no pagination (mlwh_study_detail,
// mlwh_count_samples_for_study, mlwh_count_runs_for_study,
// mlwh_count_libraries_for_study).
type studyIDInput struct {
	StudyLimsID string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study to look up"`
}

// addStudyDetail registers mlwh_study_detail (Story C1): it returns the given
// study with the detail of each of its libraries and their samples.
func (p *provider) addStudyDetail(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("StudyDetail")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_study_detail",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyIDInput) (*mcp.CallToolResult, wa.StudyDetail, error) {
		detail, err := client.StudyDetail(ctx, in.StudyLimsID)
		if err != nil {
			return core.ToolError[wa.StudyDetail](mapToolError(err))
		}

		return nil, detail, nil
	})

	return nil
}

// runIDInput is the input for the run-keyed tools that take only a run id and no
// pagination (mlwh_run_detail, mlwh_count_samples_for_run).
type runIDInput struct {
	IDRun string `json:"id_run" jsonschema:"the sequencing run identifier to look up"`
}

// addRunDetail registers mlwh_run_detail (Story C1): it returns the given run
// with its related samples, studies, and per-study detail.
func (p *provider) addRunDetail(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("RunDetail")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_run_detail",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runIDInput) (*mcp.CallToolResult, wa.RunDetail, error) {
		detail, err := client.RunDetail(ctx, in.IDRun)
		if err != nil {
			return core.ToolError[wa.RunDetail](mapToolError(err))
		}

		return nil, detail, nil
	})

	return nil
}

// libraryDetailInput is the input for library/study-scoped tools: a library
// identified by its pipeline LIMS id within a study, so it carries both path
// params in the order the upstream /library/:pipeline/study/:study/* paths
// declare them.
type libraryDetailInput struct {
	PipelineIDLims string `json:"pipeline_id_lims" jsonschema:"the pipeline LIMS identifier of the library"`
	StudyLimsID    string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study the library belongs to"`
}

// addLibraryDetail registers mlwh_library_detail (Story C1): it returns the
// library identified by its pipeline LIMS id within the given study, together
// with the samples it covers, sending the two path params in declaration order
// so the upstream path is /library/:pipeline/study/:study/detail.
func (p *provider) addLibraryDetail(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("LibraryDetail")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_library_detail",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in libraryDetailInput) (*mcp.CallToolResult, wa.LibraryDetail, error) {
		detail, err := client.LibraryDetail(ctx, in.PipelineIDLims, in.StudyLimsID)
		if err != nil {
			return core.ToolError[wa.LibraryDetail](mapToolError(err))
		}

		return nil, detail, nil
	})

	return nil
}

// registerFanOutTools adds the fan-out enumeration tools: the paged list tools
// and the two non-paged tools (studies-for-sample and the samples-in-study
// count).
func (p *provider) registerFanOutTools(r core.Registrar) error {
	if err := p.registerPaginatedFanOuts(r); err != nil {
		return err
	}

	return p.registerNonPaginatedFanOuts(r)
}

// registerPaginatedFanOuts adds the eight paged fan-out tools. Each pre-sets
// its paged wrapper output schema and appends the bounded-page note to its
// Registry-derived description, then registers a handler that rejects
// over-large pages before the typed call.
func (p *provider) registerPaginatedFanOuts(r core.Registrar) error {
	schemas, err := fanOutSliceSchemas()
	if err != nil {
		return err
	}

	if err := p.addAllStudies(r, schemas["studies"]); err != nil {
		return err
	}

	if err := p.addSamplesForStudy(r, schemas["samples"]); err != nil {
		return err
	}

	if err := p.addSamplesForRun(r, schemas["samples"]); err != nil {
		return err
	}

	if err := p.addLibrariesForStudy(r, schemas["libraries"]); err != nil {
		return err
	}

	if err := p.addRunsForStudy(r, schemas["runs"]); err != nil {
		return err
	}

	if err := p.addLanesForSample(r, schemas["lanes"]); err != nil {
		return err
	}

	if err := p.addIRODSPathsForSample(r, schemas["irods_paths"]); err != nil {
		return err
	}

	return p.addIRODSPathsForStudy(r, schemas["irods_paths"])
}

// pageInput is the input for the paged fan-out tool that takes no path param
// (mlwh_all_studies): just the bounded pagination controls. An omitted Limit
// defaults to 100; an omitted Offset is 0.
type pageInput struct {
	Limit  int `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset int `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// addAllStudies registers mlwh_all_studies (Story C2): it lists every study,
// defaulting an omitted limit to a bounded page, and wraps the result under
// {"studies":[...],"total":N,"next_offset":M}.
func (p *provider) addAllStudies(r core.Registrar, outputSchema map[string]any) error {
	description, err := paginatedFanOutDescription("AllStudies")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_all_studies",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pageInput) (*mcp.CallToolResult, pagedStudiesResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedStudiesResult](err)
		}

		page, err := client.AllStudiesPage(ctx, limit, offset)
		if err != nil {
			return core.ToolError[pagedStudiesResult](mapToolError(err))
		}

		return nil, pagedStudiesResult{
			Studies:    page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// studyPageInput is the input for the study-keyed paged fan-out tools
// (mlwh_samples_for_study, mlwh_libraries_for_study, mlwh_runs_for_study): a
// LIMS study id plus bounded pagination.
type studyPageInput struct {
	StudyLimsID string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study to enumerate"`
	Limit       int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset      int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// addSamplesForStudy registers mlwh_samples_for_study (Story C2): it lists the
// distinct samples linked to a study, with the bounded page default, wrapping
// the result under {"samples":[...],"total":N,"next_offset":M}.
func (p *provider) addSamplesForStudy(r core.Registrar, outputSchema map[string]any) error {
	description, err := paginatedFanOutDescription("SamplesForStudy")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_samples_for_study",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyPageInput) (*mcp.CallToolResult, pagedSamplesResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedSamplesResult](err)
		}

		page, err := client.SamplesForStudyPage(ctx, in.StudyLimsID, limit, offset)
		if err != nil {
			return core.ToolError[pagedSamplesResult](mapToolError(err))
		}

		return nil, pagedSamplesResult{
			Samples:    page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// runPageInput is the input for the run-keyed paged fan-out tool
// (mlwh_samples_for_run): a run id plus bounded pagination.
type runPageInput struct {
	IDRun  string `json:"id_run" jsonschema:"the sequencing run identifier to enumerate"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// addSamplesForRun registers mlwh_samples_for_run (Story C2): it lists the
// samples sequenced on a run, with the bounded page default, wrapping the
// result under {"samples":[...],"total":N,"next_offset":M}.
func (p *provider) addSamplesForRun(r core.Registrar, outputSchema map[string]any) error {
	description, err := paginatedFanOutDescription("SamplesForRun")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_samples_for_run",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runPageInput) (*mcp.CallToolResult, pagedSamplesResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedSamplesResult](err)
		}

		page, err := client.SamplesForRunPage(ctx, in.IDRun, limit, offset)
		if err != nil {
			return core.ToolError[pagedSamplesResult](mapToolError(err))
		}

		return nil, pagedSamplesResult{
			Samples:    page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// addLibrariesForStudy registers mlwh_libraries_for_study (Story C2): it lists
// the libraries belonging to a study, with the bounded page default, wrapping
// the result under {"libraries":[...],"total":N,"next_offset":M}.
func (p *provider) addLibrariesForStudy(r core.Registrar, outputSchema map[string]any) error {
	description, err := paginatedFanOutDescription("LibrariesForStudy")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_libraries_for_study",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyPageInput) (*mcp.CallToolResult, pagedLibrariesResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedLibrariesResult](err)
		}

		page, err := client.LibrariesForStudyPage(ctx, in.StudyLimsID, limit, offset)
		if err != nil {
			return core.ToolError[pagedLibrariesResult](mapToolError(err))
		}

		return nil, pagedLibrariesResult{
			Libraries:  page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// addRunsForStudy registers mlwh_runs_for_study (Story C2): it lists the
// sequencing runs associated with a study, with the bounded page default,
// wrapping the result under {"runs":[...],"total":N,"next_offset":M}.
func (p *provider) addRunsForStudy(r core.Registrar, outputSchema map[string]any) error {
	description, err := paginatedFanOutDescription("RunsForStudy")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_runs_for_study",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyPageInput) (*mcp.CallToolResult, pagedRunsResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedRunsResult](err)
		}

		page, err := client.RunsForStudyPage(ctx, in.StudyLimsID, limit, offset)
		if err != nil {
			return core.ToolError[pagedRunsResult](mapToolError(err))
		}

		return nil, pagedRunsResult{
			Runs:       page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// samplePageInput is the input for the sample-keyed paged fan-out tools
// (mlwh_lanes_for_sample): a Sanger sample name plus bounded pagination.
type samplePageInput struct {
	SangerName string `json:"sanger_name" jsonschema:"the Sanger sample name to enumerate"`
	Limit      int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset     int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// addLanesForSample registers mlwh_lanes_for_sample (Story C2): it lists the
// run/lane/tag combinations on which a sample was sequenced, with the bounded
// page default, wrapping the result under {"lanes":[...],"total":N,"next_offset":M}.
func (p *provider) addLanesForSample(r core.Registrar, outputSchema map[string]any) error {
	description, err := paginatedFanOutDescription("LanesForSample")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_lanes_for_sample",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in samplePageInput) (*mcp.CallToolResult, pagedLanesResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedLanesResult](err)
		}

		page, err := client.LanesForSamplePage(ctx, in.SangerName, limit, offset)
		if err != nil {
			return core.ToolError[pagedLanesResult](mapToolError(err))
		}

		return nil, pagedLanesResult{
			Lanes:      page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// addIRODSPathsForSample registers mlwh_irods_paths_for_sample (Story C2): it
// lists the iRODS data-object paths exported for a sample, with the bounded
// page default, wrapping the result under
// {"irods_paths":[...],"total":N,"next_offset":M}.
func (p *provider) addIRODSPathsForSample(r core.Registrar, outputSchema map[string]any) error {
	description, err := paginatedFanOutDescription("IRODSPathsForSample")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_irods_paths_for_sample",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in irodsSamplePageInput) (*mcp.CallToolResult, pagedIRODSPathsResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedIRODSPathsResult](err)
		}

		page, err := client.IRODSPathsForSampleByFileTypePage(ctx, in.SangerName, in.FileType, limit, offset)
		if err != nil {
			return core.ToolError[pagedIRODSPathsResult](mapToolError(err))
		}

		return nil, pagedIRODSPathsResult{
			IRODSPaths: page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// addIRODSPathsForStudy registers mlwh_irods_paths_for_study (Story C2): it lists
// the iRODS data-object paths exported for a study, with the bounded page
// default, wrapping the result under
// {"irods_paths":[...],"total":N,"next_offset":M}.
func (p *provider) addIRODSPathsForStudy(r core.Registrar, outputSchema map[string]any) error {
	description, err := paginatedFanOutDescription("IRODSPathsForStudy")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_irods_paths_for_study",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in irodsStudyPageInput) (*mcp.CallToolResult, pagedIRODSPathsResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedIRODSPathsResult](err)
		}

		page, err := client.IRODSPathsForStudyByFileTypePage(ctx, in.StudyLimsID, in.FileType, limit, offset)
		if err != nil {
			return core.ToolError[pagedIRODSPathsResult](mapToolError(err))
		}

		return nil, pagedIRODSPathsResult{
			IRODSPaths: page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// registerNonPaginatedFanOuts adds the non-paginated fan-out tools:
// mlwh_studies_for_sample (the studies a sample belongs to, wrapped under
// {"studies":[...]}) and the typed Count counterparts for the large upstream
// fan-out lists.
func (p *provider) registerNonPaginatedFanOuts(r core.Registrar) error {
	if err := p.addStudiesForSample(r); err != nil {
		return err
	}

	countSchema, err := outputSchemaFor("Count")
	if err != nil {
		return fmt.Errorf("mlwh: build count output schema: %w", err)
	}

	if err := p.addCountSamplesForStudy(r, countSchema); err != nil {
		return err
	}

	if err := p.addCountSamplesForRun(r, countSchema); err != nil {
		return err
	}

	if err := p.addCountRunsForStudy(r, countSchema); err != nil {
		return err
	}

	if err := p.addCountLibrariesForStudy(r, countSchema); err != nil {
		return err
	}

	if err := p.addCountLanesForSample(r, countSchema); err != nil {
		return err
	}

	if err := p.addCountSamplesForLibrary(r, countSchema); err != nil {
		return err
	}

	if err := p.addCountSamplesForLibraryID(r, countSchema); err != nil {
		return err
	}

	if err := p.addCountSamplesForLibraryLimsID(r, countSchema); err != nil {
		return err
	}

	return p.addCountSamplesForLibraryType(r, countSchema)
}

// addStudiesForSample registers mlwh_studies_for_sample (Story C2): a
// non-paginated tool listing the studies a sample (by Sanger sample name)
// belongs to, wrapping the []wa.Study under {"studies":[...]} so an empty result
// is the object {"studies":[]} (not a bare array) and a success is never an error.
func (p *provider) addStudiesForSample(r core.Registrar) error {
	outputSchema, err := outputSchemaForSlice("studies", "Study")
	if err != nil {
		return fmt.Errorf("mlwh: build studies output schema: %w", err)
	}

	description, err := resolveDescription("StudiesForSample")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_studies_for_sample",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in sampleNameInput) (*mcp.CallToolResult, studiesResult, error) {
		studies, err := client.StudiesForSample(ctx, in.SangerName)
		if err != nil {
			return core.ToolError[studiesResult](mapToolError(err))
		}

		return nil, studiesResult{Studies: studies}, nil
	})

	return nil
}

// addCountSamplesForStudy registers mlwh_count_samples_for_study (Story C2): a
// non-paginated tool returning the number of distinct samples linked to a study
// as the typed Count ({"count":N}), the count counterpart of
// mlwh_samples_for_study.
func (p *provider) addCountSamplesForStudy(r core.Registrar, outputSchema map[string]any) error {
	client := p.client

	return addFanOutCountTool[studyIDInput](r, outputSchema, "mlwh_count_samples_for_study", "CountSamplesForStudy",
		func(ctx context.Context, in studyIDInput) (wa.Count, error) {
			return client.CountSamplesForStudy(ctx, in.StudyLimsID)
		})
}

// addCountSamplesForRun registers mlwh_count_samples_for_run, the Count
// counterpart of mlwh_samples_for_run.
func (p *provider) addCountSamplesForRun(r core.Registrar, outputSchema map[string]any) error {
	client := p.client

	return addFanOutCountTool[runIDInput](r, outputSchema, "mlwh_count_samples_for_run", "CountSamplesForRun",
		func(ctx context.Context, in runIDInput) (wa.Count, error) {
			return client.CountSamplesForRun(ctx, in.IDRun)
		})
}

// addCountRunsForStudy registers mlwh_count_runs_for_study, the Count counterpart
// of mlwh_runs_for_study.
func (p *provider) addCountRunsForStudy(r core.Registrar, outputSchema map[string]any) error {
	client := p.client

	return addFanOutCountTool[studyIDInput](r, outputSchema, "mlwh_count_runs_for_study", "CountRunsForStudy",
		func(ctx context.Context, in studyIDInput) (wa.Count, error) {
			return client.CountRunsForStudy(ctx, in.StudyLimsID)
		})
}

// addCountLibrariesForStudy registers mlwh_count_libraries_for_study, the Count
// counterpart of mlwh_libraries_for_study.
func (p *provider) addCountLibrariesForStudy(r core.Registrar, outputSchema map[string]any) error {
	client := p.client

	return addFanOutCountTool[studyIDInput](r, outputSchema, "mlwh_count_libraries_for_study", "CountLibrariesForStudy",
		func(ctx context.Context, in studyIDInput) (wa.Count, error) {
			return client.CountLibrariesForStudy(ctx, in.StudyLimsID)
		})
}

// addCountLanesForSample registers mlwh_count_lanes_for_sample, the Count
// counterpart of mlwh_lanes_for_sample.
func (p *provider) addCountLanesForSample(r core.Registrar, outputSchema map[string]any) error {
	client := p.client

	return addFanOutCountTool[sampleNameInput](r, outputSchema, "mlwh_count_lanes_for_sample", "CountLanesForSample",
		func(ctx context.Context, in sampleNameInput) (wa.Count, error) {
			return client.CountLanesForSample(ctx, in.SangerName)
		})
}

// addCountSamplesForLibrary registers mlwh_count_samples_for_library, the Count
// counterpart of the library/study-scoped SamplesForLibrary endpoint.
func (p *provider) addCountSamplesForLibrary(r core.Registrar, outputSchema map[string]any) error {
	client := p.client

	return addFanOutCountTool[libraryDetailInput](r, outputSchema, "mlwh_count_samples_for_library", "CountSamplesForLibrary",
		func(ctx context.Context, in libraryDetailInput) (wa.Count, error) {
			return client.CountSamplesForLibrary(ctx, in.PipelineIDLims, in.StudyLimsID)
		})
}

// libraryIDInput is the input for tools keyed by an exact library_id.
type libraryIDInput struct {
	LibraryID string `json:"library_id" jsonschema:"the exact library_id to count samples for"`
}

// addCountSamplesForLibraryID registers mlwh_count_samples_for_library_id.
func (p *provider) addCountSamplesForLibraryID(r core.Registrar, outputSchema map[string]any) error {
	client := p.client

	return addFanOutCountTool[libraryIDInput](r, outputSchema, "mlwh_count_samples_for_library_id", "CountSamplesForLibraryID",
		func(ctx context.Context, in libraryIDInput) (wa.Count, error) {
			return client.CountSamplesForLibraryID(ctx, in.LibraryID)
		})
}

// libraryLimsIDInput is the input for tools keyed by an exact id_library_lims.
type libraryLimsIDInput struct {
	LibraryLimsID string `json:"library_lims_id" jsonschema:"the exact LIMS library identifier to count samples for"`
}

// addCountSamplesForLibraryLimsID registers mlwh_count_samples_for_library_lims_id.
func (p *provider) addCountSamplesForLibraryLimsID(r core.Registrar, outputSchema map[string]any) error {
	client := p.client

	return addFanOutCountTool[libraryLimsIDInput](r, outputSchema, "mlwh_count_samples_for_library_lims_id",
		"CountSamplesForLibraryLimsID", func(ctx context.Context, in libraryLimsIDInput) (wa.Count, error) {
			return client.CountSamplesForLibraryLimsID(ctx, in.LibraryLimsID)
		})
}

// libraryTypeInput is the input for tools keyed by pipeline_id_lims / library type.
type libraryTypeInput struct {
	LibraryType string `json:"library_type" jsonschema:"the library type / pipeline LIMS identifier to count samples for"`
}

// addCountSamplesForLibraryType registers mlwh_count_samples_for_library_type.
func (p *provider) addCountSamplesForLibraryType(r core.Registrar, outputSchema map[string]any) error {
	client := p.client

	return addFanOutCountTool[libraryTypeInput](r, outputSchema, "mlwh_count_samples_for_library_type",
		"CountSamplesForLibraryType", func(ctx context.Context, in libraryTypeInput) (wa.Count, error) {
			return client.CountSamplesForLibraryType(ctx, in.LibraryType)
		})
}
