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

// searchTermMinLength mirrors the upstream searchPaginationParams contract:
// both searches require at least three characters. Page bounds are shared with
// other typed paged tools through boundedPagination.
const searchTermMinLength = 3

// searchSamplesDescription, searchStudiesDescription, countSamplesDescription,
// countStudySearchDescription, and countStudiesDescription are the LLM-facing
// tool descriptions. They convey the search/pagination/count semantics the spec
// requires the agent to understand (word-prefix vs substring fields, the minimum
// term length, the 100/1000 page bounds, and the 10000 count floor).
const (
	searchSamplesDescription = "Search samples by word prefix: returns samples having a word in " +
		"name, supplier_name, common_name, or donor_id that starts with the term " +
		"(case-insensitive word-prefix match, minimum 3 characters). So \"musculus\" " +
		"and \"mus\" both match \"Mus Musculus\"; a substring inside a word does not. " +
		"Defaults to a page of 100 rows, maximum 1000 (a larger limit is rejected, not clamped); " +
		"use offset to page."

	searchStudiesDescription = "Search studies by substring for study-name/id lookup questions " +
		"(for example, \"What study id matches this name?\"): returns studies whose " +
		"name, study_title, programme, or faculty_sponsor contains the term " +
		"(case-insensitive substring match, minimum 3 characters). " +
		"Defaults to a page of 100 rows, maximum 1000 (a larger limit is rejected, not clamped); " +
		"use offset to page. Rows expose study identifiers and context fields such as " +
		"id_study_lims, name, study_title, programme, faculty_sponsor, and accession_number " +
		"to disambiguate candidate study ids."

	countSamplesDescription = "Count samples matching a word-prefix search, the count counterpart of " +
		"mlwh_search_samples (same case-insensitive word-prefix over name, supplier_name, " +
		"common_name, donor_id; minimum 3 characters), without transferring rows. " +
		"The count is exact up to 10000; a returned count of exactly 10000 means \"at least 10000\" " +
		"(a floor) for very common terms."

	countStudySearchDescription = "Count studies matching a substring search, the count counterpart of " +
		"mlwh_search_studies (same case-insensitive substring over name, study_title, programme, " +
		"faculty_sponsor; minimum 3 characters), without transferring rows."

	countStudiesDescription = "Count all studies mirrored in the warehouse cache, the count counterpart " +
		"of mlwh_all_studies. Takes no input."
)

// searchInput is the shared input for the two paginated substring searches
// (mlwh_search_samples, mlwh_search_studies). An omitted (zero) Limit becomes
// searchDefaultLimit and an omitted Offset is 0; these defaults are applied by
// the handler, not the schema, so a zero value is unambiguous for a search tool.
type searchInput struct {
	Term   string `json:"term" jsonschema:"the search term; minimum 3 characters"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

// addSearchSamples registers mlwh_search_samples (Story A1). The handler rejects
// a too-short term and an over-max limit before any HTTP call, defaults the page
// to 100 and the offset to 0, then wraps the upstream []wa.Sample under
// {"samples":[...]} so the structured result is the object MCP requires.
func (p *provider) addSearchSamples(r core.Registrar, outputSchema map[string]any) {
	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_search_samples",
		Description:  searchSamplesDescription,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, pagedSamplesResult, error) {
		limit, offset, err := guardSearch(in)
		if err != nil {
			return core.ToolError[pagedSamplesResult](err)
		}

		page, err := client.SearchSamplesPage(ctx, in.Term, limit, offset)
		if err != nil {
			return core.ToolError[pagedSamplesResult](mapToolError(err))
		}

		return nil, pagedSamplesResult{
			Samples:    page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})
}

// addSearchStudies registers mlwh_search_studies (Story A3/D1), mirroring
// addSearchSamples but over the substring study search and wrapping each
// bounded page under {"studies":[...]} with the upstream wa.Study fields needed
// to disambiguate candidate study ids.
func (p *provider) addSearchStudies(r core.Registrar, outputSchema map[string]any) {
	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_search_studies",
		Description:  searchStudiesDescription,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, pagedStudiesResult, error) {
		limit, offset, err := guardSearch(in)
		if err != nil {
			return core.ToolError[pagedStudiesResult](err)
		}

		page, err := client.SearchStudiesPage(ctx, in.Term, limit, offset)
		if err != nil {
			return core.ToolError[pagedStudiesResult](mapToolError(err))
		}

		return nil, pagedStudiesResult{
			Studies:    page.Items,
			Total:      page.Total,
			NextOffset: page.NextOffset,
		}, nil
	})
}

// guardSearch applies the cheap input bounds for a paginated search before any
// HTTP call: it rejects a term shorter than the minimum and a limit above the
// maximum (matching the upstream, which rejects rather than clamps), then
// resolves the effective limit (an omitted/zero limit becomes the default page)
// and offset (omitted is 0). Returning an error here short-circuits the handler
// so no request reaches the warehouse.
func guardSearch(in searchInput) (limit, offset int, err error) {
	if err = guardTerm(in.Term); err != nil {
		return 0, 0, err
	}

	return boundedPagination(in.Limit, in.Offset)
}

// guardTerm rejects a search term shorter than the minimum length before any
// HTTP call, with a message that names the 3-character minimum so the agent can
// fix the input immediately.
func guardTerm(term string) error {
	if len(term) < searchTermMinLength {
		return fmt.Errorf("the search term %q is too short: a minimum of %d characters is required", term, searchTermMinLength)
	}

	return nil
}

// registerSearchTools adds the sample/study search and count tools (Stories A1,
// A2, A3, A4) to the server through the Registrar. Each tool's handler closes
// over the provider's remote client; the typed slice tools pre-set their
// OpenAPI-sourced output schema (so the upstream doc: field descriptions
// survive, which the SDK's own jsonschema reflection would drop) before
// mcp.AddTool, and every tool maps an upstream error to a clear tool error via
// mapToolError. Building an output schema fails only on a programming error
// (the schemas come from the compiled-in OpenAPI document), so such a failure is
// surfaced as a registration error.
func (p *provider) registerSearchTools(r core.Registrar) error {
	samplesSchema, err := outputSchemaForPagedSlice("samples", "Sample")
	if err != nil {
		return fmt.Errorf("mlwh: build samples output schema: %w", err)
	}

	studiesSchema, err := outputSchemaForPagedSlice("studies", "Study")
	if err != nil {
		return fmt.Errorf("mlwh: build studies output schema: %w", err)
	}

	countSchema, err := outputSchemaFor("Count")
	if err != nil {
		return fmt.Errorf("mlwh: build count output schema: %w", err)
	}

	p.addSearchSamples(r, samplesSchema)
	p.addCountSamples(r, countSchema)
	p.addSearchStudies(r, studiesSchema)
	p.addCountStudiesSearch(r, countSchema)
	p.addCountStudies(r, countSchema)

	return nil
}

// termInput is the shared input for the term-only count tools
// (mlwh_count_samples, mlwh_count_studies_search): a single search term subject
// to the same minimum length as the search it sizes.
type termInput struct {
	Term string `json:"term" jsonschema:"the search term; minimum 3 characters"`
}

// addCountSamples registers mlwh_count_samples (Story A2): it rejects a
// too-short term before the call, then returns the upstream Count unchanged.
func (p *provider) addCountSamples(r core.Registrar, outputSchema map[string]any) {
	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_samples",
		Description:  countSamplesDescription,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in termInput) (*mcp.CallToolResult, wa.Count, error) {
		if err := guardTerm(in.Term); err != nil {
			return core.ToolError[wa.Count](err)
		}

		count, err := client.CountSampleSearch(ctx, in.Term)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})
}

// addCountStudiesSearch registers mlwh_count_studies_search (Story A4): it
// rejects a too-short term before the call, then returns the upstream Count.
func (p *provider) addCountStudiesSearch(r core.Registrar, outputSchema map[string]any) {
	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_studies_search",
		Description:  countStudySearchDescription,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in termInput) (*mcp.CallToolResult, wa.Count, error) {
		if err := guardTerm(in.Term); err != nil {
			return core.ToolError[wa.Count](err)
		}

		count, err := client.CountStudySearch(ctx, in.Term)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})
}

// emptyInput is the input for mlwh_count_studies, which takes no parameters.
// Being an empty struct it infers the input schema {"type":"object"}, the
// no-argument shape MCP requires; the handler ignores it.
type emptyInput struct{}

// addCountStudies registers mlwh_count_studies (Story A4): it takes no input
// (its empty input struct infers the {"type":"object"} schema MCP requires for a
// no-argument tool) and returns the whole-set study Count.
func (p *provider) addCountStudies(r core.Registrar, outputSchema map[string]any) {
	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_studies",
		Description:  countStudiesDescription,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountStudies(ctx)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})
}
