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
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

// searchCursorPagingNote is appended to the sample and study search tool
// descriptions: it says how to page with cursor or offset and when to choose
// each.
const searchCursorPagingNote = " Page with cursor or offset. To walk every page of a large search, pass the " +
	"returned next_cursor back as cursor with the same term and options and no offset, until next_cursor is " +
	"empty (the last page): deep cursor pages stay fast, and if the cache syncs between pages, rows whose match " +
	"did not change are neither repeated nor skipped. stale=true (with warning) means the cache synced since the " +
	"first page: later pages may include newer matches, and new matches that sort before the cursor are missed " +
	"until a fresh search. Use offset (next_offset) to jump to a page; offset pages can repeat or skip rows when " +
	"the cache syncs between pages. Cursor and offset cannot be combined. Cursor paging needs MLWH API " +
	wa.SearchCursorAPIVersion + "; an older server returns no next_cursor, so page it with offset."

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
		"Defaults to a page of 100 rows, maximum 1000 (a larger limit is rejected, not clamped)." +
		searchCursorPagingNote + bareListFreshnessNote

	searchStudiesDescription = "Search studies by substring for study-name/id lookup questions " +
		"(for example, \"What study id matches this name?\"): returns studies whose " +
		"name, study_title, programme, or faculty_sponsor contains the term " +
		"(case-insensitive substring match, minimum 3 characters). " +
		"Defaults to a page of 100 rows, maximum 1000 (a larger limit is rejected, not clamped). " +
		"Rows expose study identifiers and context fields such as " +
		"id_study_lims, name, study_title, programme, faculty_sponsor, and accession_number " +
		"to disambiguate candidate study ids." + searchCursorPagingNote + bareListFreshnessNote

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
	Offset           int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0; cannot be combined with cursor"`
	Cursor           string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page of this same search (same term and options); returns the rows after that page; cannot be combined with offset"`
}

func (in sampleSearchInput) query(limit, offset int) url.Values {
	query := searchPageQuery(limit, offset, in.Cursor)
	addSampleSearchOptions(query, in.options())

	return query
}

// searchPageQuery encodes a search page request: limit plus either the cursor
// or, without one, the offset.
func searchPageQuery(limit, offset int, cursor string) url.Values {
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	if cursor = strings.TrimSpace(cursor); cursor != "" {
		query.Set("cursor", cursor)

		return query
	}

	query.Set("offset", strconv.Itoa(offset))

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
// the three-character guard. Every page is one CallWithHeaders request whose
// sizing, cursor and warning headers become the page metadata.
func (p *provider) addSearchSamples(r core.Registrar, outputSchema map[string]any) {
	client := p.client

	mcp.AddTool(r.Server(), &mcp.Tool{
		Name:         "mlwh_search_samples",
		Description:  searchSamplesDescription,
		OutputSchema: outputSchema,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in sampleSearchInput) (*mcp.CallToolResult, searchSamplesResult, error) {
		limit, offset, err := guardSampleSearch(in)
		if err != nil {
			return core.ToolError[searchSamplesResult](err)
		}

		samples, meta, err := searchCursorPage[wa.Sample](ctx, client, "SearchSamples", in.Term, in.query(limit, offset))
		if err != nil {
			return core.ToolError[searchSamplesResult](mapToolError(err))
		}

		return nil, searchSamplesResult{Samples: samples, searchPageMeta: meta}, nil
	})
}

func guardSampleSearch(in sampleSearchInput) (limit, offset int, err error) {
	if err = guardSampleSearchTerm(in.Term, in.hasExactFilter()); err != nil {
		return 0, 0, err
	}

	return searchPagination(in.Limit, in.Offset, in.Cursor)
}

func guardSampleSearchTerm(term string, hasExactFilter bool) error {
	if strings.TrimSpace(term) == "" {
		return fmt.Errorf("the search term %q must not be blank", term)
	}
	if hasExactFilter {
		return nil
	}

	return guardTerm(term)
}

// guardTerm rejects a free-text-only search term shorter than the minimum
// length before HTTP, with a message that names the minimum.
func guardTerm(term string) error {
	if len(term) < searchTermMinLength {
		return fmt.Errorf("the search term %q is too short: a minimum of %d characters is required", term, searchTermMinLength)
	}

	return nil
}

// searchPagination resolves a search page's limit and offset like
// boundedPagination, and rejects a cursor combined with a non-zero offset
// before HTTP, as the upstream would.
func searchPagination(limit, offset int, cursor string) (int, int, error) {
	limit, offset, err := boundedPagination(limit, offset)
	if err != nil {
		return 0, 0, err
	}
	if offset != 0 && strings.TrimSpace(cursor) != "" {
		return 0, 0, fmt.Errorf("cursor and offset %d cannot be combined: continue a search with its next_cursor "+
			"alone (omit offset), or page by offset without a cursor", offset)
	}

	return limit, offset, nil
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

func (in sampleSearchCountInput) guard() error {
	return guardSampleSearchTerm(in.Term, in.hasExactFilter())
}

func (in sampleSearchCountInput) count(ctx context.Context, client *wa.RemoteClient) (wa.Count, error) {
	opts := in.options()
	if in.hasOptions() {
		return client.CountSampleSearchWithOptions(ctx, in.Term, opts)
	}

	return client.CountSampleSearch(ctx, in.Term)
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
		if err := in.guard(); err != nil {
			return core.ToolError[wa.Count](err)
		}

		count, err := in.count(ctx, client)
		if err != nil {
			return core.ToolError[wa.Count](mapToolError(err))
		}

		return nil, count, nil
	})
}

// searchInput is the input for the paginated study substring search. An omitted
// limit becomes the shared page default and an omitted offset is zero.
type searchInput struct {
	Term   string `json:"term" jsonschema:"the search term; minimum 3 characters"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 1000 (a larger limit is rejected, not clamped)"`
	Offset int    `json:"offset,omitempty" jsonschema:"number of leading rows to skip before returning results; defaults to 0; cannot be combined with cursor"`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page of this same search (same term); returns the rows after that page; cannot be combined with offset"`
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
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchStudiesResult, error) {
		limit, offset, err := guardSearch(in)
		if err != nil {
			return core.ToolError[searchStudiesResult](err)
		}

		query := searchPageQuery(limit, offset, in.Cursor)

		studies, meta, err := searchCursorPage[wa.Study](ctx, client, "SearchStudies", in.Term, query)
		if err != nil {
			return core.ToolError[searchStudiesResult](mapToolError(err))
		}

		return nil, searchStudiesResult{Studies: studies, searchPageMeta: meta}, nil
	})
}

// guardSearch applies the cheap input bounds for the paginated study search
// before any HTTP call: it rejects a term shorter than the minimum, a limit
// above the maximum (matching the upstream, which rejects rather than clamps),
// and a cursor combined with an offset, then resolves the effective limit (an
// omitted/zero limit becomes the default page) and offset (omitted is 0).
// Returning an error here short-circuits the handler so no request reaches the
// warehouse.
func guardSearch(in searchInput) (limit, offset int, err error) {
	if err = guardTerm(in.Term); err != nil {
		return 0, 0, err
	}

	return searchPagination(in.Limit, in.Offset, in.Cursor)
}

// searchCursorPage fetches one study or sample search page through method and
// reads its page metadata from the response headers: X-Total-Count,
// X-Next-Offset (an offset page only), X-Next-Cursor and X-Search-Warning. A
// cursor request answered with X-Next-Offset came from a server that predates
// search cursors and ignored the cursor, so it is reported as
// wa.ErrSearchCursorUnsupported rather than returned as the first page again.
func searchCursorPage[T any](
	ctx context.Context,
	client *wa.RemoteClient,
	method, term string,
	query url.Values,
) ([]T, searchPageMeta, error) {
	decoded, headers, err := client.CallWithHeaders(ctx, method, []string{term}, query)
	if err != nil {
		return nil, searchPageMeta{}, err
	}

	items, ok := decoded.(*[]T)
	if !ok {
		return nil, searchPageMeta{}, fmt.Errorf("%w: registry result for %s has type %T", wa.ErrUpstreamImpaired, method, decoded)
	}

	if query.Get("cursor") != "" && headers.Get("X-Next-Offset") != "" {
		return nil, searchPageMeta{}, fmt.Errorf("%w: the MLWH server ignored the cursor; cursor paging needs API %s or later",
			wa.ErrSearchCursorUnsupported, wa.SearchCursorAPIVersion)
	}

	return *items, searchPageMetaFromHeaders(headers), nil
}

func searchPageMetaFromHeaders(headers http.Header) searchPageMeta {
	warning := headers.Get(wa.SearchWarningHeader)

	return searchPageMeta{
		Total:      headerInt(headers, "X-Total-Count", 0),
		NextOffset: headerInt(headers, "X-Next-Offset", -1),
		NextCursor: headers.Get(wa.SearchNextCursorHeader),
		Stale:      warning != "",
		Warning:    warning,
	}
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
	samplesSchema, err := outputSchemaForSearchPage("samples", "Sample")
	if err != nil {
		return fmt.Errorf("mlwh: build samples output schema: %w", err)
	}

	studiesSchema, err := outputSchemaForSearchPage("studies", "Study")
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
