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
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

// searchTermMinLength mirrors the upstream free-text search contract. Study
// searches and unfiltered sample searches require at least three characters;
// exact sample filters make shorter terms valid.
const searchTermMinLength = 3

// searchSamplesDescription, searchStudiesDescription, countSamplesDescription,
// countStudySearchDescription, and countStudiesDescription are the LLM-facing
// tool descriptions. They convey the search/pagination/count semantics the spec
// requires the agent to understand (sample literal/word-prefix modes, study
// substring fields, exact filters, page bounds, and the 10000 count floor).
const (
	searchSamplesDescription = "Search samples by case-insensitive literal whole-value prefix over " +
		"name, supplier_name, common_name, and donor_id by default. Set words=true for the opt-in " +
		"separator-agnostic word-prefix mode; mid-word substring matching is unsupported. Exact filters " +
		"AND-combine and permit a term shorter than the usual 3-character minimum: organism matches " +
		"whole-word common-name membership including subspecies, library type is exact, QC is the " +
		"sample-level fail>pending>pass roll-up, and deliverables_only uses the upstream deliverable " +
		"discriminator rather than is_spiked, with PacBio/ONT pass-through. " +
		"Defaults to a page of 100 rows, maximum 1000 (a larger limit is rejected, not clamped); " +
		"use offset to page." + bareListFreshnessNote

	searchStudiesDescription = "Search studies by substring for study-name/id lookup questions " +
		"(for example, \"What study id matches this name?\"): returns studies whose " +
		"name, study_title, programme, or faculty_sponsor contains the term " +
		"(case-insensitive substring match, minimum 3 characters). " +
		"Defaults to a page of 100 rows, maximum 1000 (a larger limit is rejected, not clamped); " +
		"use offset to page. Rows expose study identifiers and context fields such as " +
		"id_study_lims, name, study_title, programme, faculty_sponsor, and accession_number " +
		"to disambiguate candidate study ids." + bareListFreshnessNote

	countSamplesDescription = "Count samples matching mlwh_search_samples with identical options: the default " +
		"is a case-insensitive literal whole-value prefix over name, supplier_name, common_name, and donor_id; " +
		"words=true is opt-in separator-agnostic word-prefix mode and mid-word substring matching is unsupported. " +
		"Exact filters AND-combine and permit a short term: organism is whole-word common-name membership, " +
		"library type is exact, QC is the sample-level fail>pending>pass roll-up, and deliverables_only uses " +
		"the upstream deliverable discriminator rather than is_spiked, with PacBio/ONT pass-through. " +
		"The count is exact up to 10000; a returned count of exactly 10000 means \"at least 10000\" " +
		"(a floor) for very common terms."

	countStudySearchDescription = "Count studies matching a substring search, the count counterpart of " +
		"mlwh_search_studies (same case-insensitive substring over name, study_title, programme, " +
		"faculty_sponsor; minimum 3 characters), without transferring rows."

	countStudiesDescription = "Count all studies mirrored in the warehouse cache, the count counterpart " +
		"of mlwh_all_studies. Takes no input."
)

// sampleSearchInput is the input for mlwh_search_samples. Its option vocabulary
// mirrors wa.SampleSearchOptions, while limit and offset control the bounded
// header-aware page returned by the tool.
type sampleSearchInput struct {
	Term             string `json:"term" jsonschema:"the literal whole-value prefix term; minimum 3 characters unless an exact filter is supplied"`
	Words            bool   `json:"words,omitempty" jsonschema:"opt into separator-agnostic word-prefix matching; false uses literal whole-value prefix"`
	Organism         string `json:"organism,omitempty" jsonschema:"exact filter by whole-word common-name membership, including subspecies"`
	LibraryType      string `json:"library_type,omitempty" jsonschema:"exact library type filter"`
	QC               string `json:"qc,omitempty" jsonschema:"sample-level QC roll-up filter: fail, pending, or pass"`
	DeliverablesOnly bool   `json:"deliverables_only,omitempty" jsonschema:"filter with the upstream deliverable discriminator, not is_spiked; PacBio and ONT pass through"`
	Limit            int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset           int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
}

func (in sampleSearchInput) query(limit, offset int) url.Values {
	query := url.Values{
		"limit":  {strconv.Itoa(limit)},
		"offset": {strconv.Itoa(offset)},
	}
	addSampleSearchOptions(query, in.options())

	return query
}

func addSampleSearchOptions(query url.Values, opts wa.SampleSearchOptions) {
	if opts.Words {
		query.Set("words", "true")
	}
	if hasSampleSearchTextOption(opts.Organism) {
		query.Set("organism", opts.Organism)
	}
	if hasSampleSearchTextOption(opts.LibraryType) {
		query.Set("library_type", opts.LibraryType)
	}
	if hasSampleSearchTextOption(opts.QC) {
		query.Set("qc", opts.QC)
	}
	if opts.DeliverablesOnly {
		query.Set("deliverables_only", "true")
	}
}

func (in sampleSearchInput) options() wa.SampleSearchOptions {
	return wa.SampleSearchOptions{
		Words: in.Words, Organism: in.Organism, LibraryType: in.LibraryType,
		QC: in.QC, DeliverablesOnly: in.DeliverablesOnly,
	}
}

func (in sampleSearchInput) hasOptions() bool {
	return hasSampleSearchOptions(in.options())
}

func hasSampleSearchOptions(opts wa.SampleSearchOptions) bool {
	return opts.Words || hasSampleExactFilter(opts)
}

func (in sampleSearchInput) hasExactFilter() bool {
	return hasSampleExactFilter(in.options())
}

func hasSampleExactFilter(opts wa.SampleSearchOptions) bool {
	return hasSampleSearchTextOption(opts.Organism) || hasSampleSearchTextOption(opts.LibraryType) ||
		hasSampleSearchTextOption(opts.QC) || opts.DeliverablesOnly
}

// addSearchSamples registers mlwh_search_samples. Free-text-only calls retain
// the three-character guard and page helper. Optioned calls preserve the exact
// query and page headers in one CallWithHeaders request.
func (p *provider) addSearchSamples(r core.Registrar, outputSchema map[string]any) {
	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_search_samples",
		Description:  searchSamplesDescription,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in sampleSearchInput) (*mcp.CallToolResult, pagedSamplesResult, error) {
		limit, offset, err := guardSampleSearch(in)
		if err != nil {
			return core.ToolError[pagedSamplesResult](err)
		}

		page, err := sampleSearchPage(ctx, client, in, limit, offset)
		if err != nil {
			return core.ToolError[pagedSamplesResult](mapToolError(err))
		}

		return nil, page, nil
	})
}

func guardSampleSearch(in sampleSearchInput) (limit, offset int, err error) {
	if !in.hasExactFilter() {
		if err = guardTerm(in.Term); err != nil {
			return 0, 0, err
		}
	}

	return boundedPagination(in.Limit, in.Offset)
}

// guardTerm rejects a free-text-only search term shorter than the minimum
// length before HTTP, with a message that names the minimum.
func guardTerm(term string) error {
	if len(term) < searchTermMinLength {
		return fmt.Errorf("the search term %q is too short: a minimum of %d characters is required", term, searchTermMinLength)
	}

	return nil
}

func sampleSearchPage(
	ctx context.Context,
	client *wa.RemoteClient,
	in sampleSearchInput,
	limit, offset int,
) (pagedSamplesResult, error) {
	if !in.hasOptions() {
		page, err := client.SearchSamplesPage(ctx, in.Term, limit, offset)

		return pagedSamplesResult{Samples: page.Items, Total: page.Total, NextOffset: page.NextOffset}, err
	}

	decoded, headers, err := client.CallWithHeaders(ctx, "SearchSamples", []string{in.Term}, in.query(limit, offset))
	if err != nil {
		return pagedSamplesResult{}, err
	}

	samples, ok := decoded.(*[]wa.Sample)
	if !ok {
		return pagedSamplesResult{}, fmt.Errorf("%w: registry result for SearchSamples has type %T", wa.ErrUpstreamImpaired, decoded)
	}

	return pagedSamplesResult{
		Samples: *samples, Total: headerInt(headers, "X-Total-Count", 0),
		NextOffset: headerInt(headers, "X-Next-Offset", -1),
	}, nil
}

// sampleSearchCountInput is the matching option vocabulary for
// mlwh_count_samples. Pagination is deliberately list-only.
type sampleSearchCountInput struct {
	Term             string `json:"term" jsonschema:"the literal whole-value prefix term; minimum 3 characters unless an exact filter is supplied"`
	Words            bool   `json:"words,omitempty" jsonschema:"opt into separator-agnostic word-prefix matching; false uses literal whole-value prefix"`
	Organism         string `json:"organism,omitempty" jsonschema:"exact filter by whole-word common-name membership, including subspecies"`
	LibraryType      string `json:"library_type,omitempty" jsonschema:"exact library type filter"`
	QC               string `json:"qc,omitempty" jsonschema:"sample-level QC roll-up filter: fail, pending, or pass"`
	DeliverablesOnly bool   `json:"deliverables_only,omitempty" jsonschema:"filter with the upstream deliverable discriminator, not is_spiked; PacBio and ONT pass through"`
}

func (in sampleSearchCountInput) options() wa.SampleSearchOptions {
	return wa.SampleSearchOptions{
		Words: in.Words, Organism: in.Organism, LibraryType: in.LibraryType,
		QC: in.QC, DeliverablesOnly: in.DeliverablesOnly,
	}
}

func (in sampleSearchCountInput) hasOptions() bool {
	return hasSampleSearchOptions(in.options())
}

func (in sampleSearchCountInput) hasExactFilter() bool {
	return hasSampleExactFilter(in.options())
}

// addCountSamples registers mlwh_count_samples with the same modes, exact
// filters, and conditional short-term guard as mlwh_search_samples.
func (p *provider) addCountSamples(r core.Registrar, outputSchema map[string]any) {
	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_samples",
		Description:  countSamplesDescription + countFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in sampleSearchCountInput) (*mcp.CallToolResult, wa.Count, error) {
		if err := guardSampleSearchCount(in); err != nil {
			return core.ToolError[wa.Count](err)
		}

		count, err := countSampleSearch(ctx, client, in)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})
}

func guardSampleSearchCount(in sampleSearchCountInput) error {
	if in.hasExactFilter() {
		return nil
	}

	return guardTerm(in.Term)
}

func countSampleSearch(ctx context.Context, client *wa.RemoteClient, in sampleSearchCountInput) (wa.Count, error) {
	opts := in.options()
	if in.hasOptions() {
		return client.CountSampleSearchWithOptions(ctx, in.Term, opts)
	}

	return client.CountSampleSearch(ctx, in.Term)
}

// searchInput is the input for the paginated study substring search. An omitted
// limit becomes the shared page default and an omitted offset is zero.
type searchInput struct {
	Term   string `json:"term" jsonschema:"the search term; minimum 3 characters"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0"`
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

// guardSearch applies the cheap input bounds for the paginated study search before any
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

func hasSampleSearchTextOption(value string) bool {
	return strings.TrimSpace(value) != ""
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

// termInput is the input for the term-only study search count.
type termInput struct {
	Term string `json:"term" jsonschema:"the search term; minimum 3 characters"`
}

// addCountStudiesSearch registers mlwh_count_studies_search (Story A4): it
// rejects a too-short term before the call, then returns the upstream Count.
func (p *provider) addCountStudiesSearch(r core.Registrar, outputSchema map[string]any) {
	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_count_studies_search",
		Description:  countStudySearchDescription + countFreshnessNote,
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
		Description:  countStudiesDescription + countFreshnessNote,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, wa.Count, error) {
		count, err := client.CountStudies(ctx)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})
}
