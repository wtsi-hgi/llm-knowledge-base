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

// fetchAllLimit is the upstream fetch-all sentinel (wa's mlwhServerFetchAllLimit,
// verified equal to 1_000_000 in wa/mlwh/server.go). It is the limit the
// paginated fan-out handlers send when the caller omits limit, so the agent
// receives every row by default.
//
// This default is the OPPOSITE of the search tools' bounded default, and it is
// required: the typed []T Queryer methods take (limit, offset int) and the
// remote client serialises both literally (remotePagination -> strconv.Itoa), so
// an explicit limit=0 reaches the server as LIMIT 0 and returns ZERO rows. The
// server substitutes its fetch-all page only when the limit query param is
// ABSENT, which the int method cannot express; so the handler must default the
// limit to this sentinel itself rather than passing 0.
const fetchAllLimit = 1_000_000

// fetchAllPaginationNote is the clause appended to every paginated fan-out tool's
// description so the agent knows omitting limit fetches all rows (and that an
// explicit limit/offset pages instead). It makes the fetch-all default explicit
// regardless of the upstream Registry wording.
const fetchAllPaginationNote = " Omitting limit fetches all matching rows (the default); " +
	"set limit (and offset) to page through the results instead."

// fanOutPagination resolves the effective (limit, offset) a paginated fan-out
// handler sends upstream. An omitted (zero or negative) limit becomes
// fetchAllLimit so the caller receives every row; an explicit positive limit is
// passed through unchanged. The offset is passed through, defaulting to 0.
func fanOutPagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = fetchAllLimit
	}

	return limit, offset
}

// fanOutSliceSchemas builds the slice-wrapper output schemas (F2) shared by the
// paginated fan-out tools, keyed by the wrapper property name so each tool picks
// the one matching its element type.
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
		schema, err := outputSchemaForSlice(spec.property, spec.component)
		if err != nil {
			return nil, fmt.Errorf("mlwh: build %s output schema: %w", spec.property, err)
		}

		schemas[spec.property] = schema
	}

	return schemas, nil
}

// paginatedFanOutDescription derives a paginated fan-out tool's LLM-facing
// description from its Registry entry (Summary + Description) and appends the
// fetch-all note, so the description tracks the upstream Registry yet always
// states the fetch-all default this server applies.
func paginatedFanOutDescription(method string) (string, error) {
	base, err := resolveDescription(method)
	if err != nil {
		return "", err
	}

	return base + fetchAllPaginationNote, nil
}

// registerDetailTools adds the grouped detail tools (Story C1) and the fan-out
// enumeration tools (Story C2) to the server through the Registrar. Each typed
// tool pre-sets its OpenAPI-sourced output schema so the upstream doc: field
// descriptions survive (the SDK's own reflection would drop them), and every
// handler maps an upstream error to a clear tool error via mapToolError. The
// paginated fan-out tools default an omitted limit to the fetch-all sentinel so
// the caller receives every row, never the zero rows an explicit limit=0 would
// yield. Building a schema or deriving a description fails only on a programming
// error (the schemas and Registry are compiled in), so such a failure is a
// registration error.
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
// Sanger sample name and no pagination (mlwh_sample_detail, mlwh_studies_for_sample).
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
// study id and no pagination (mlwh_study_detail, mlwh_count_samples_for_study).
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
// pagination (mlwh_run_detail).
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

// libraryDetailInput is the input for mlwh_library_detail: a library identified
// by its pipeline LIMS id within a study, so it carries both path params in the
// order the upstream /library/:pipeline/study/:study/detail path declares them.
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

// registerFanOutTools adds the fan-out enumeration tools (Story C2): the
// paginated list tools (which default an omitted limit to the fetch-all
// sentinel) and the two non-paginated tools (studies-for-sample and the
// samples-in-study count).
func (p *provider) registerFanOutTools(r core.Registrar) error {
	if err := p.registerPaginatedFanOuts(r); err != nil {
		return err
	}

	return p.registerNonPaginatedFanOuts(r)
}

// registerPaginatedFanOuts adds the eight paginated fan-out tools. Each pre-sets
// its slice wrapper output schema (F2) and appends the fetch-all note to its
// Registry-derived description, then registers a handler that defaults an omitted
// limit to the fetch-all sentinel before the typed call.
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

// pageInput is the input for the paginated fan-out tool that takes no path param
// (mlwh_all_studies): just the fetch-all pagination controls. An omitted Limit
// triggers the fetch-all default; an omitted Offset is 0.
type pageInput struct {
	Limit  int `json:"limit,omitempty" jsonschema:"maximum rows to return; omit to fetch all rows (the default), or set to page"`
	Offset int `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// addAllStudies registers mlwh_all_studies (Story C2): it lists every study,
// defaulting an omitted limit to the fetch-all sentinel, and wraps the result
// under {"studies":[...]}.
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
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pageInput) (*mcp.CallToolResult, studiesResult, error) {
		limit, offset := fanOutPagination(in.Limit, in.Offset)

		studies, err := client.AllStudies(ctx, limit, offset)
		if err != nil {
			return core.ToolError[studiesResult](mapToolError(err))
		}

		return nil, studiesResult{Studies: studies}, nil
	})

	return nil
}

// studyPageInput is the input for the study-keyed paginated fan-out tools
// (mlwh_samples_for_study, mlwh_libraries_for_study, mlwh_runs_for_study,
// mlwh_irods_paths_for_study): a LIMS study id plus the fetch-all pagination.
type studyPageInput struct {
	StudyLimsID string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study to enumerate"`
	Limit       int    `json:"limit,omitempty" jsonschema:"maximum rows to return; omit to fetch all rows (the default), or set to page"`
	Offset      int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// addSamplesForStudy registers mlwh_samples_for_study (Story C2): it lists the
// distinct samples linked to a study, with the fetch-all default, wrapping the
// result under {"samples":[...]}.
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
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyPageInput) (*mcp.CallToolResult, samplesResult, error) {
		limit, offset := fanOutPagination(in.Limit, in.Offset)

		samples, err := client.SamplesForStudy(ctx, in.StudyLimsID, limit, offset)
		if err != nil {
			return core.ToolError[samplesResult](mapToolError(err))
		}

		return nil, samplesResult{Samples: samples}, nil
	})

	return nil
}

// runPageInput is the input for the run-keyed paginated fan-out tool
// (mlwh_samples_for_run): a run id plus the fetch-all pagination.
type runPageInput struct {
	IDRun  string `json:"id_run" jsonschema:"the sequencing run identifier to enumerate"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum rows to return; omit to fetch all rows (the default), or set to page"`
	Offset int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// addSamplesForRun registers mlwh_samples_for_run (Story C2): it lists the
// samples sequenced on a run, with the fetch-all default, wrapping the result
// under {"samples":[...]}.
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
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runPageInput) (*mcp.CallToolResult, samplesResult, error) {
		limit, offset := fanOutPagination(in.Limit, in.Offset)

		samples, err := client.SamplesForRun(ctx, in.IDRun, limit, offset)
		if err != nil {
			return core.ToolError[samplesResult](mapToolError(err))
		}

		return nil, samplesResult{Samples: samples}, nil
	})

	return nil
}

// addLibrariesForStudy registers mlwh_libraries_for_study (Story C2): it lists
// the libraries belonging to a study, with the fetch-all default, wrapping the
// result under {"libraries":[...]}.
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
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyPageInput) (*mcp.CallToolResult, librariesResult, error) {
		limit, offset := fanOutPagination(in.Limit, in.Offset)

		libraries, err := client.LibrariesForStudy(ctx, in.StudyLimsID, limit, offset)
		if err != nil {
			return core.ToolError[librariesResult](mapToolError(err))
		}

		return nil, librariesResult{Libraries: libraries}, nil
	})

	return nil
}

// addRunsForStudy registers mlwh_runs_for_study (Story C2): it lists the
// sequencing runs associated with a study, with the fetch-all default, wrapping
// the result under {"runs":[...]}.
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
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyPageInput) (*mcp.CallToolResult, runsResult, error) {
		limit, offset := fanOutPagination(in.Limit, in.Offset)

		runs, err := client.RunsForStudy(ctx, in.StudyLimsID, limit, offset)
		if err != nil {
			return core.ToolError[runsResult](mapToolError(err))
		}

		return nil, runsResult{Runs: runs}, nil
	})

	return nil
}

// samplePageInput is the input for the sample-keyed paginated fan-out tools
// (mlwh_lanes_for_sample, mlwh_irods_paths_for_sample): a Sanger sample name
// plus the fetch-all pagination.
type samplePageInput struct {
	SangerName string `json:"sanger_name" jsonschema:"the Sanger sample name to enumerate"`
	Limit      int    `json:"limit,omitempty" jsonschema:"maximum rows to return; omit to fetch all rows (the default), or set to page"`
	Offset     int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// addLanesForSample registers mlwh_lanes_for_sample (Story C2): it lists the
// run/lane/tag combinations on which a sample was sequenced, with the fetch-all
// default, wrapping the result under {"lanes":[...]}.
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
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in samplePageInput) (*mcp.CallToolResult, lanesResult, error) {
		limit, offset := fanOutPagination(in.Limit, in.Offset)

		lanes, err := client.LanesForSample(ctx, in.SangerName, limit, offset)
		if err != nil {
			return core.ToolError[lanesResult](mapToolError(err))
		}

		return nil, lanesResult{Lanes: lanes}, nil
	})

	return nil
}

// addIRODSPathsForSample registers mlwh_irods_paths_for_sample (Story C2): it
// lists the iRODS data-object paths exported for a sample, with the fetch-all
// default, wrapping the result under {"irods_paths":[...]}.
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
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in samplePageInput) (*mcp.CallToolResult, irodsPathsResult, error) {
		limit, offset := fanOutPagination(in.Limit, in.Offset)

		paths, err := client.IRODSPathsForSample(ctx, in.SangerName, limit, offset)
		if err != nil {
			return core.ToolError[irodsPathsResult](mapToolError(err))
		}

		return nil, irodsPathsResult{IRODSPaths: paths}, nil
	})

	return nil
}

// addIRODSPathsForStudy registers mlwh_irods_paths_for_study (Story C2): it lists
// the iRODS data-object paths exported for a study, with the fetch-all default,
// wrapping the result under {"irods_paths":[...]}.
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
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyPageInput) (*mcp.CallToolResult, irodsPathsResult, error) {
		limit, offset := fanOutPagination(in.Limit, in.Offset)

		paths, err := client.IRODSPathsForStudy(ctx, in.StudyLimsID, limit, offset)
		if err != nil {
			return core.ToolError[irodsPathsResult](mapToolError(err))
		}

		return nil, irodsPathsResult{IRODSPaths: paths}, nil
	})

	return nil
}

// registerNonPaginatedFanOuts adds the two non-paginated fan-out tools:
// mlwh_studies_for_sample (the studies a sample belongs to, wrapped under
// {"studies":[...]}) and mlwh_count_samples_for_study (the distinct-sample count
// for a study, returned as the typed Count).
func (p *provider) registerNonPaginatedFanOuts(r core.Registrar) error {
	if err := p.addStudiesForSample(r); err != nil {
		return err
	}

	return p.addCountSamplesForStudy(r)
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
func (p *provider) addCountSamplesForStudy(r core.Registrar) error {
	outputSchema, err := outputSchemaFor("Count")
	if err != nil {
		return fmt.Errorf("mlwh: build count output schema: %w", err)
	}

	description, err := resolveDescription("CountSamplesForStudy")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_samples_for_study",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyIDInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountSamplesForStudy(ctx, in.StudyLimsID)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}
