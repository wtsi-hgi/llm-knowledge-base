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

const (
	availabilityBareListFreshnessNote = " Bare list responses have no cache_synced_at; " +
		"call mlwh_freshness for the cache as-of caveat."
	countFreshnessNote = " Count responses have no cache_synced_at; call mlwh_freshness for the cache as-of caveat."
)

// irodsSamplePageInput is the input for mlwh_irods_paths_for_sample: a Sanger
// sample name, optional upstream file_type suffix filter, and bounded page
// controls.
type irodsSamplePageInput struct {
	SangerName string `json:"sanger_name" jsonschema:"the Sanger sample name to enumerate"`
	FileType   string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
	Limit      int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset     int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// irodsStudyPageInput is the input for mlwh_irods_paths_for_study: a LIMS study
// id, optional upstream file_type suffix filter, and bounded page controls.
type irodsStudyPageInput struct {
	StudyLimsID string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study to enumerate"`
	FileType    string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
	Limit       int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset      int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// studyAddedWindowInput is the input for
// mlwh_count_samples_with_data_for_study: a study id plus optional iRODS
// created-window bounds. Empty window fields are omitted from the upstream
// query; non-empty values are not parsed locally.
type studyAddedWindowInput struct {
	StudyLimsID string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study to count"`
	Since       string `json:"since,omitempty" jsonschema:"optional RFC3339 inclusive lower bound over iRODS created; passed through unchanged"`
	Until       string `json:"until,omitempty" jsonschema:"optional RFC3339 exclusive upper bound over iRODS created; passed through unchanged and only meaningful with since"`
}

func (p *provider) addCountSamplesWithDataForStudy(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountSamplesWithData")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_samples_with_data_for_study",
		Description:  description + countFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyAddedWindowInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := countSamplesWithData(ctx, client, in)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

func countSamplesWithData(ctx context.Context, client *wa.RemoteClient, in studyAddedWindowInput) (wa.Count, error) {
	if in.Since != "" || in.Until != "" {
		return client.CountSamplesWithDataSince(ctx, in.StudyLimsID, in.Since, in.Until)
	}

	return client.CountSamplesWithData(ctx, in.StudyLimsID)
}

func availabilityListDescription(method string) (string, error) {
	base, err := resolveDescription(method)
	if err != nil {
		return "", err
	}

	return base + pagedFanOutPaginationNote + availabilityBareListFreshnessNote, nil
}

// registerAvailabilityTools adds the phase 6 availability-family tools: C1
// sample availability, C2 iRODS run/count additions, and C3 study manifest
// list/count tools. Window and file_type values are passed through unchanged to
// wa, preserving upstream RFC3339 and bad-request semantics.
func (p *provider) registerAvailabilityTools(r core.Registrar) error {
	samplesSchema, err := outputSchemaForPagedSlice("samples", "SampleWithData")
	if err != nil {
		return fmt.Errorf("mlwh: build sample availability output schema: %w", err)
	}

	manifestSchema, err := outputSchemaForPagedObject("StudyManifest")
	if err != nil {
		return fmt.Errorf("mlwh: build study manifest output schema: %w", err)
	}

	irodsSchema, err := outputSchemaForPagedSlice("irods_paths", "IRODSPath")
	if err != nil {
		return fmt.Errorf("mlwh: build irods_paths output schema: %w", err)
	}

	countSchema, err := outputSchemaFor("Count")
	if err != nil {
		return fmt.Errorf("mlwh: build count output schema: %w", err)
	}

	if err := p.addCountSamplesWithDataForStudy(r, countSchema); err != nil {
		return err
	}

	if err := p.addSamplesWithDataForStudy(r, samplesSchema); err != nil {
		return err
	}

	if err := p.addSamplesWithoutDataForStudy(r, samplesSchema); err != nil {
		return err
	}

	if err := p.addStudyManifest(r, manifestSchema); err != nil {
		return err
	}

	if err := p.addCountStudyManifest(r, countSchema); err != nil {
		return err
	}

	if err := p.addIRODSPathsForRun(r, irodsSchema); err != nil {
		return err
	}

	if err := p.addCountIRODSPathsForSample(r, countSchema); err != nil {
		return err
	}

	if err := p.addCountIRODSPathsForStudy(r, countSchema); err != nil {
		return err
	}

	return p.addCountIRODSPathsForRun(r, countSchema)
}

// irodsRunPageInput is the input for mlwh_irods_paths_for_run: an Illumina run
// id, optional upstream file_type suffix filter, and bounded page controls.
type irodsRunPageInput struct {
	IDRun    string `json:"id_run" jsonschema:"the sequencing run identifier to enumerate"`
	FileType string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset   int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// addIRODSPathsForRun registers mlwh_irods_paths_for_run (Story C2): it lists
// the iRODS data-object paths exported for a run, with optional upstream
// file_type filtering and page metadata.
func (p *provider) addIRODSPathsForRun(r core.Registrar, outputSchema map[string]any) error {
	description, err := paginatedFanOutDescription("IRODSPathsForRun")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_irods_paths_for_run",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in irodsRunPageInput) (*mcp.CallToolResult, pagedIRODSPathsResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedIRODSPathsResult](err)
		}

		page, err := client.IRODSPathsForRunByFileTypePage(ctx, in.IDRun, in.FileType, limit, offset)
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

type irodsSampleCountInput struct {
	SangerName string `json:"sanger_name" jsonschema:"the Sanger sample name whose iRODS paths should be counted"`
	FileType   string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
}

// addCountIRODSPathsForSample registers mlwh_count_irods_paths_for_sample, the
// count counterpart to mlwh_irods_paths_for_sample with the same optional
// file_type semantics.
func (p *provider) addCountIRODSPathsForSample(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountIRODSPathsForSample")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_irods_paths_for_sample",
		Description:  description + countFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in irodsSampleCountInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountIRODSPathsForSampleByFileType(ctx, in.SangerName, in.FileType)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

type irodsStudyCountInput struct {
	StudyLimsID string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study whose iRODS paths should be counted"`
	FileType    string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
}

// addCountIRODSPathsForStudy registers mlwh_count_irods_paths_for_study, the
// count counterpart to mlwh_irods_paths_for_study with the same optional
// file_type semantics.
func (p *provider) addCountIRODSPathsForStudy(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountIRODSPathsForStudy")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_irods_paths_for_study",
		Description:  description + countFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in irodsStudyCountInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountIRODSPathsForStudyByFileType(ctx, in.StudyLimsID, in.FileType)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

type irodsRunCountInput struct {
	IDRun    string `json:"id_run" jsonschema:"the sequencing run identifier whose iRODS paths should be counted"`
	FileType string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
}

// addCountIRODSPathsForRun registers mlwh_count_irods_paths_for_run, the count
// counterpart to mlwh_irods_paths_for_run with the same optional file_type
// semantics.
func (p *provider) addCountIRODSPathsForRun(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountIRODSPathsForRun")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_irods_paths_for_run",
		Description:  description + countFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in irodsRunCountInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountIRODSPathsForRun(ctx, in.IDRun, in.FileType)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

// studyAvailabilityPageInput is the input for
// mlwh_samples_with_data_for_study: a study id, optional iRODS created-window
// bounds, and bounded pagination controls.
type studyAvailabilityPageInput struct {
	StudyLimsID string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study to enumerate"`
	Since       string `json:"since,omitempty" jsonschema:"optional RFC3339 inclusive lower bound over iRODS created; passed through unchanged"`
	Until       string `json:"until,omitempty" jsonschema:"optional RFC3339 exclusive upper bound over iRODS created; passed through unchanged and only meaningful with since"`
	Limit       int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset      int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

func (p *provider) addSamplesWithDataForStudy(r core.Registrar, outputSchema map[string]any) error {
	description, err := availabilityListDescription("SamplesWithData")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_samples_with_data_for_study",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyAvailabilityPageInput) (*mcp.CallToolResult, pagedSamplesWithDataResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedSamplesWithDataResult](err)
		}

		page, err := client.SamplesWithDataSincePage(ctx, in.StudyLimsID, in.Since, in.Until, limit, offset)
		if err != nil {
			return core.ToolError[pagedSamplesWithDataResult](mapToolError(err))
		}

		return nil, pagedSamplesWithDataResult{
			Samples:    page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

func (p *provider) addSamplesWithoutDataForStudy(r core.Registrar, outputSchema map[string]any) error {
	description, err := availabilityListDescription("SamplesWithoutData")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_samples_without_data_for_study",
		Description:  description,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyPageInput) (*mcp.CallToolResult, pagedSamplesWithDataResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedSamplesWithDataResult](err)
		}

		page, err := client.SamplesWithoutDataPage(ctx, in.StudyLimsID, limit, offset)
		if err != nil {
			return core.ToolError[pagedSamplesWithDataResult](mapToolError(err))
		}

		return nil, pagedSamplesWithDataResult{
			Samples:    page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})

	return nil
}

// pagedStudyManifestResult flattens wa.PagedStudyManifest for MCP: the upstream
// StudyManifest fields stay at top level and the page metadata is added beside
// them, with no study_manifest wrapper.
type pagedStudyManifestResult struct {
	wa.StudyManifest
	Total      int `json:"total"`
	NextOffset int `json:"next_offset"`
}

// studyManifestInput is the input for mlwh_study_manifest: a LIMS study id,
// optional iRODS path enrichment/filtering, and bounded page controls.
type studyManifestInput struct {
	StudyLimsID string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study to manifest"`
	WithIRODS   bool   `json:"with_irods,omitempty" jsonschema:"include irods_path on each manifest row when true"`
	FileType    string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter, used only when with_irods is true"`
	Limit       int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset      int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// addStudyManifest registers mlwh_study_manifest (Story C3): a bounded,
// header-aware study manifest page whose output flattens the upstream manifest
// envelope with top-level total and next_offset.
func (p *provider) addStudyManifest(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("StudyManifest")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_study_manifest",
		Description:  description + pagedFanOutPaginationNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyManifestInput) (*mcp.CallToolResult, pagedStudyManifestResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedStudyManifestResult](err)
		}

		page, err := client.StudyManifestPage(ctx, in.StudyLimsID, in.FileType, in.WithIRODS, limit, offset)
		if err != nil {
			return core.ToolError[pagedStudyManifestResult](mapToolError(err))
		}

		return nil, pagedStudyManifestResult{
			StudyManifest: page.StudyManifest,
			Total:         page.Total,
			NextOffset:    page.NextOffset,
		}, nil
	})

	return nil
}

// addCountStudyManifest registers mlwh_count_study_manifest (Story C3): the
// product-grained count counterpart for mlwh_study_manifest.
func (p *provider) addCountStudyManifest(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountStudyManifest")
	if err != nil {
		return err
	}

	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_study_manifest",
		Description:  description + countFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in studyIDInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountStudyManifest(ctx, in.StudyLimsID)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}
