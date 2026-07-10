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
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

const (
	bareListFreshnessNote = " Bare list responses have no cache_synced_at; " +
		"call mlwh_freshness for the cache as-of caveat."
	countFreshnessNote = " Count responses have no cache_synced_at; call mlwh_freshness for the cache as-of caveat."
	irodsSemanticsNote = " The created field is data-added time. Deliverability approximates iRODS target=1 " +
		"and is not is_spiked. Call mlwh_freshness for the cache as-of state."
	latestDataSemanticsNote = " This is one bounded page, not every row tied for the maximum created value." +
		" The created field is data-added time. Call mlwh_freshness for the cache as-of state."
	latestDataCountSemanticsNote = " The created field is data-added time." +
		" Call mlwh_freshness for the cache as-of state."
)

// irodsSamplePageInput is the input for mlwh_irods_paths_for_sample: a Sanger
// sample name, optional upstream file_type suffix filter, and bounded page
// controls.
type irodsSamplePageInput struct {
	SangerName       string `json:"sanger_name" jsonschema:"the Sanger sample name to enumerate"`
	FileType         string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
	DeliverablesOnly bool   `json:"deliverables_only,omitempty" jsonschema:"return only rows upstream classifies as deliverable; an approximation of target and not is_spiked"`
	OrderBy          string `json:"order_by,omitempty" jsonschema:"optional list ordering; created_desc returns newest data-added rows first"`
	Since            string `json:"since,omitempty" jsonschema:"optional RFC3339 inclusive lower bound over iRODS created (data-added time); passed through unchanged"`
	Until            string `json:"until,omitempty" jsonschema:"optional RFC3339 exclusive upper bound over iRODS created (data-added time); passed through unchanged and requires since"`
	Limit            int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset           int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// irodsStudyPageInput is the input for mlwh_irods_paths_for_study: a LIMS study
// id, optional upstream file_type suffix filter, and bounded page controls.
type irodsStudyPageInput struct {
	StudyLimsID      string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study to enumerate"`
	FileType         string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
	DeliverablesOnly bool   `json:"deliverables_only,omitempty" jsonschema:"return only rows upstream classifies as deliverable; an approximation of target and not is_spiked"`
	OrderBy          string `json:"order_by,omitempty" jsonschema:"optional list ordering; created_desc returns newest data-added rows first"`
	Since            string `json:"since,omitempty" jsonschema:"optional RFC3339 inclusive lower bound over iRODS created (data-added time); passed through unchanged"`
	Until            string `json:"until,omitempty" jsonschema:"optional RFC3339 exclusive upper bound over iRODS created (data-added time); passed through unchanged and requires since"`
	Limit            int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset           int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
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

	return base + pagedFanOutPaginationNote + bareListFreshnessNote, nil
}

func irodsPathOptions(fileType string, deliverablesOnly bool, orderBy, since, until string) wa.IRODSPathOptions {
	return wa.IRODSPathOptions{
		FileType:         fileType,
		DeliverablesOnly: deliverablesOnly,
		OrderBy:          orderBy,
		Since:            since,
		Until:            until,
	}
}

func irodsPathsPage(
	ctx context.Context,
	client *wa.RemoteClient,
	method string,
	id string,
	opts wa.IRODSPathOptions,
	limit int,
	offset int,
) (pagedIRODSPathsResult, error) {
	result, headers, err := client.CallWithHeaders(ctx, method, []string{id}, irodsPathQuery(opts, limit, offset))
	if err != nil {
		return pagedIRODSPathsResult{}, err
	}

	paths, ok := result.(*[]wa.IRODSPath)
	if !ok {
		return pagedIRODSPathsResult{}, fmt.Errorf("%w: registry result for %s has type %T", wa.ErrUpstreamImpaired, method, result)
	}

	return pagedIRODSPathsResult{
		IRODSPaths: *paths,
		Total:      headerInt(headers, "X-Total-Count", 0),
		NextOffset: headerInt(headers, "X-Next-Offset", -1),
	}, nil
}

func irodsPathQuery(opts wa.IRODSPathOptions, limit, offset int) url.Values {
	query := url.Values{
		"limit":  {strconv.Itoa(limit)},
		"offset": {strconv.Itoa(offset)},
	}

	setNonEmptyQueryValue(query, "file_type", opts.FileType)
	if opts.DeliverablesOnly {
		query.Set("deliverables_only", "true")
	}
	setNonEmptyQueryValue(query, "order_by", opts.OrderBy)
	setNonEmptyQueryValue(query, "since", opts.Since)
	setNonEmptyQueryValue(query, "until", opts.Until)

	return query
}

func setNonEmptyQueryValue(query url.Values, name, value string) {
	if value != "" {
		query.Set(name, value)
	}
}

func latestDataPagination(limit, offset int) (int, int, error) {
	if limit > pagedMaxLimit {
		return 0, 0, fmt.Errorf("limit %d exceeds the maximum of %d (a larger limit is rejected, not clamped); request a smaller page", limit, pagedMaxLimit)
	}

	if offset < 0 {
		return 0, 0, fmt.Errorf("offset %d must be non-negative", offset)
	}

	if limit <= 0 {
		limit = latestDataDefaultLimit
	}

	return limit, offset, nil
}

func latestDataPageResult(page wa.Page[wa.RecentDataRow]) pagedLatestDataResult {
	return pagedLatestDataResult{
		LatestData: page.Items,
		Total:      page.Total,
		NextOffset: page.NextOffset,
	}
}

// registerAvailabilityTools adds sample availability and iRODS run/count
// tools. Window and file_type values are passed through unchanged to wa,
// preserving upstream RFC3339 and bad-request semantics.
func (p *provider) registerAvailabilityTools(r core.Registrar) error {
	samplesSchema, err := outputSchemaForPagedSlice("samples", "SampleWithData")
	if err != nil {
		return fmt.Errorf("mlwh: build sample availability output schema: %w", err)
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

	if err := p.addIRODSPathsForRun(r, irodsSchema); err != nil {
		return err
	}

	if err := p.addCountIRODSPathsForSample(r, countSchema); err != nil {
		return err
	}

	if err := p.addCountIRODSPathsForStudy(r, countSchema); err != nil {
		return err
	}

	if err := p.addCountIRODSPathsForRun(r, countSchema); err != nil {
		return err
	}

	return p.registerLatestDataTools(r, countSchema)
}

func (p *provider) registerLatestDataTools(r core.Registrar, countSchema map[string]any) error {
	latestDataSchema, err := outputSchemaForPagedSlice("latest_data", "RecentDataRow")
	if err != nil {
		return fmt.Errorf("mlwh: build latest_data output schema: %w", err)
	}

	if err := p.addLatestDataForStudy(r, latestDataSchema); err != nil {
		return err
	}

	if err := p.addLatestDataForFacultySponsor(r, latestDataSchema); err != nil {
		return err
	}

	if err := p.addCountLatestDataForStudy(r, countSchema); err != nil {
		return err
	}

	return p.addCountLatestDataForFacultySponsor(r, countSchema)
}

// latestDataForStudyInput is the input for a study-scoped latest-data page.
type latestDataForStudyInput struct {
	StudyLimsID string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study whose newest data objects should be listed"`
	FileType    string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
	Limit       int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 10, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset      int    `json:"offset,omitempty" jsonschema:"non-negative number of leading rows to skip before returning results; defaults to 0"`
}

func (p *provider) addLatestDataForStudy(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("LatestDataForStudy")
	if err != nil {
		return err
	}

	client := p.client
	mcp.AddTool(r.Server(), &mcp.Tool{
		Name: "mlwh_latest_data_for_study", Description: description + latestDataSemanticsNote, OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in latestDataForStudyInput) (*mcp.CallToolResult, pagedLatestDataResult, error) {
		limit, offset, err := latestDataPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedLatestDataResult](err)
		}

		page, err := client.LatestDataForStudyPage(ctx, in.StudyLimsID, in.FileType, limit, offset)
		if err != nil {
			return core.ToolError[pagedLatestDataResult](mapToolError(err))
		}

		return nil, latestDataPageResult(page), nil
	})

	return nil
}

// latestDataForFacultySponsorInput is the input for a faculty-sponsor-scoped
// latest-data page. FacultySponsor matches the Study field by substring.
type latestDataForFacultySponsorInput struct {
	FacultySponsor string `json:"faculty_sponsor" jsonschema:"substring of the Study faculty_sponsor field whose newest data objects should be listed"`
	FileType       string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
	Limit          int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 10, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset         int    `json:"offset,omitempty" jsonschema:"non-negative number of leading rows to skip before returning results; defaults to 0"`
}

func (p *provider) addLatestDataForFacultySponsor(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("LatestDataForFacultySponsor")
	if err != nil {
		return err
	}

	client := p.client
	mcp.AddTool(r.Server(), &mcp.Tool{
		Name: "mlwh_latest_data_for_faculty_sponsor", Description: description + latestDataSemanticsNote, OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in latestDataForFacultySponsorInput) (*mcp.CallToolResult, pagedLatestDataResult, error) {
		limit, offset, err := latestDataPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedLatestDataResult](err)
		}

		page, err := client.LatestDataForFacultySponsorPage(ctx, in.FacultySponsor, in.FileType, limit, offset)
		if err != nil {
			return core.ToolError[pagedLatestDataResult](mapToolError(err))
		}

		return nil, latestDataPageResult(page), nil
	})

	return nil
}

type latestDataForStudyCountInput struct {
	StudyLimsID string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study whose newest data objects should be counted"`
	FileType    string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
}

func (p *provider) addCountLatestDataForStudy(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountLatestDataForStudy")
	if err != nil {
		return err
	}

	client := p.client
	mcp.AddTool(r.Server(), &mcp.Tool{
		Name: "mlwh_count_latest_data_for_study", Description: description + countFreshnessNote + latestDataCountSemanticsNote, OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in latestDataForStudyCountInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountLatestDataForStudy(ctx, in.StudyLimsID, in.FileType)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

type latestDataForFacultySponsorCountInput struct {
	FacultySponsor string `json:"faculty_sponsor" jsonschema:"substring of the Study faculty_sponsor field whose newest data objects should be counted"`
	FileType       string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
}

func (p *provider) addCountLatestDataForFacultySponsor(r core.Registrar, outputSchema map[string]any) error {
	description, err := resolveDescription("CountLatestDataForFacultySponsor")
	if err != nil {
		return err
	}

	client := p.client
	mcp.AddTool(r.Server(), &mcp.Tool{
		Name: "mlwh_count_latest_data_for_faculty_sponsor", Description: description + countFreshnessNote + latestDataCountSemanticsNote, OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in latestDataForFacultySponsorCountInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountLatestDataForFacultySponsor(ctx, in.FacultySponsor, in.FileType)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

// irodsRunPageInput is the input for mlwh_irods_paths_for_run: an Illumina run
// id, optional upstream file_type suffix filter, and bounded page controls.
type irodsRunPageInput struct {
	IDRun            string `json:"id_run" jsonschema:"the sequencing run identifier to enumerate"`
	FileType         string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
	DeliverablesOnly bool   `json:"deliverables_only,omitempty" jsonschema:"return only rows upstream classifies as deliverable; an approximation of target and not is_spiked"`
	OrderBy          string `json:"order_by,omitempty" jsonschema:"optional list ordering; created_desc returns newest data-added rows first"`
	Since            string `json:"since,omitempty" jsonschema:"optional RFC3339 inclusive lower bound over iRODS created (data-added time); passed through unchanged"`
	Until            string `json:"until,omitempty" jsonschema:"optional RFC3339 exclusive upper bound over iRODS created (data-added time); passed through unchanged and requires since"`
	Limit            int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset           int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
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
		Description:  description + irodsSemanticsNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in irodsRunPageInput) (*mcp.CallToolResult, pagedIRODSPathsResult, error) {
		limit, offset, err := boundedPagination(in.Limit, in.Offset)
		if err != nil {
			return core.ToolError[pagedIRODSPathsResult](err)
		}

		page, err := irodsPathsPage(ctx, client, "IRODSPathsForRun", in.IDRun, irodsPathOptions(
			in.FileType, in.DeliverablesOnly, in.OrderBy, in.Since, in.Until,
		), limit, offset)
		if err != nil {
			return core.ToolError[pagedIRODSPathsResult](mapToolError(err))
		}

		return nil, page, nil
	})

	return nil
}

type irodsSampleCountInput struct {
	SangerName       string `json:"sanger_name" jsonschema:"the Sanger sample name whose iRODS paths should be counted"`
	FileType         string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
	DeliverablesOnly bool   `json:"deliverables_only,omitempty" jsonschema:"count only rows upstream classifies as deliverable; an approximation of target and not is_spiked"`
	Since            string `json:"since,omitempty" jsonschema:"optional RFC3339 inclusive lower bound over iRODS created (data-added time); passed through unchanged"`
	Until            string `json:"until,omitempty" jsonschema:"optional RFC3339 exclusive upper bound over iRODS created (data-added time); passed through unchanged and requires since"`
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
		Description:  description + countFreshnessNote + irodsSemanticsNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in irodsSampleCountInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountIRODSPathsForSampleWithOptions(ctx, in.SangerName, irodsPathOptions(
			in.FileType, in.DeliverablesOnly, "", in.Since, in.Until,
		))
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

type irodsStudyCountInput struct {
	StudyLimsID      string `json:"study_lims_id" jsonschema:"the LIMS identifier of the study whose iRODS paths should be counted"`
	FileType         string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
	DeliverablesOnly bool   `json:"deliverables_only,omitempty" jsonschema:"count only rows upstream classifies as deliverable; an approximation of target and not is_spiked"`
	Since            string `json:"since,omitempty" jsonschema:"optional RFC3339 inclusive lower bound over iRODS created (data-added time); passed through unchanged"`
	Until            string `json:"until,omitempty" jsonschema:"optional RFC3339 exclusive upper bound over iRODS created (data-added time); passed through unchanged and requires since"`
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
		Description:  description + countFreshnessNote + irodsSemanticsNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in irodsStudyCountInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountIRODSPathsForStudyWithOptions(ctx, in.StudyLimsID, irodsPathOptions(
			in.FileType, in.DeliverablesOnly, "", in.Since, in.Until,
		))
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})

	return nil
}

type irodsRunCountInput struct {
	IDRun            string `json:"id_run" jsonschema:"the sequencing run identifier whose iRODS paths should be counted"`
	FileType         string `json:"file_type,omitempty" jsonschema:"optional iRODS filename suffix filter passed through exactly to upstream"`
	DeliverablesOnly bool   `json:"deliverables_only,omitempty" jsonschema:"count only rows upstream classifies as deliverable; an approximation of target and not is_spiked"`
	Since            string `json:"since,omitempty" jsonschema:"optional RFC3339 inclusive lower bound over iRODS created (data-added time); passed through unchanged"`
	Until            string `json:"until,omitempty" jsonschema:"optional RFC3339 exclusive upper bound over iRODS created (data-added time); passed through unchanged and requires since"`
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
		Description:  description + countFreshnessNote + irodsSemanticsNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in irodsRunCountInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountIRODSPathsForRunWithOptions(ctx, in.IDRun, irodsPathOptions(
			in.FileType, in.DeliverablesOnly, "", in.Since, in.Until,
		))
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
