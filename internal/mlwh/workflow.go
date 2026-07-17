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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wa "github.com/wtsi-hgi/wa/mlwh"

	"github.com/wtsi-hgi/llm-knowledge-base/internal/core"
)

// workflowResourceURI is the URI of the MLWH workflow / endpoint-catalogue
// resource (Story G1). The core's Instructions point clients at this same URI.
const workflowResourceURI = "mlwh://workflow"

// workflowGuidance routes common questions to the cheapest accurate curated
// tool before the live endpoint catalogue. It also preserves the established
// resolve -> detail -> expand path for questions without a direct route.
const workflowGuidance = "# MLWH workflows\n\n" +
	"This server bridges the read-only `wa mlwh` API. Prefer one cheap curated call " +
	"that preserves the upstream row grain. For identifiers without a direct route, " +
	"**resolve** a raw identifier into " +
	"a canonical `Match` (e.g. `mlwh_resolve_sample`, `mlwh_resolve_study`), fetch " +
	"**detail** aggregates only when cheaper tools do not answer the question (e.g. " +
	"`mlwh_sample_detail`, `mlwh_study_detail`), then **expand** the canonical identifier " +
	"into related identifiers or downstream search values (`mlwh_expand_identifier`, " +
	"`mlwh_expand_search_values`).\n\n" +
	"Curated question routes:\n" +
	"- Chosen-column product table for a study: use " +
	"`mlwh_export(children=products,parent_kind=study,columns=...,format=...)`. " +
	"Products without files remain rows. `file_type` only narrows an attachment; it " +
	"does not remove product rows or change `Total`. Select fields such as `manual_qc`, " +
	"`irods_path`, `irods_unmatched`, and `reason` with `columns`; use " +
	"`deliverables_only` only when the product rows themselves should be filtered. " +
	"Use bounded `Total` for sizing and the exact `NextCursor` for continuation.\n" +
	"- Chosen-column table of actual files: use " +
	"`mlwh_export(children=irods,parent_kind=study|sample|run,columns=...,format=...)`; " +
	"`children=irods` returns file rows, unlike a product attachment. Omitted file " +
	"options select CRAM deliverables; set `file_type` or `deliverables_only` explicitly " +
	"when different file semantics are required.\n" +
	"- One selected CRAM for every sample in a study: use " +
	"`mlwh_sample_crams_for_study`. This route is merged-aware and prefers a merged " +
	"composite. An empty product-level attachment does not prove a sample-level CRAM " +
	"is absent.\n" +
	"- Literal sample starts-with search: use `mlwh_search_samples`. Literal " +
	"whole-value prefix is the default; `words=true` is opt-in for " +
	"separator-agnostic word-prefix intent. There is no substring mode. Pass exact " +
	"`organism`, `library_type`, `qc`, and `deliverables_only` filters to this tool; " +
	"use a supported `mlwh_export` relationship only when a chosen-column table is " +
	"also required.\n" +
	"- Newest data added to iRODS: use `mlwh_latest_data_for_study` or " +
	"`mlwh_latest_data_for_faculty_sponsor`. For sample/run scopes or a caller-defined " +
	"window, use the matching curated iRODS path tool with " +
	"`order_by=created_desc`, `since`, and `until`.\n" +
	"- Runs for one sample: use `mlwh_runs_for_sample`; use " +
	"`mlwh_count_runs_for_sample` only when the count alone answers the question.\n" +
	"- Runs per month: use `mlwh_monthly_run_counts`. Use `mlwh_runs` for global " +
	"keyset-paged drill-down and `mlwh_count_runs` for global sizing.\n" +
	"- Sequencing by programme, faculty sponsor, platform, manufacturer, or month: " +
	"use `mlwh_sequencing_aggregate` with required `group_by` and " +
	"`unit=runs|samples|products`; do not fan out over studies.\n" +
	"- Studies in a programme: use `mlwh_studies_for_programme`; discover exact " +
	"programme values with `mlwh_programmes`.\n" +
	"- Study owners, managers, contacts, or followers: use `mlwh_study_users` with " +
	"optional `role`; omission returns all study roles.\n\n" +
	"Meanings that affect routing:\n" +
	"- Recency: iRODS `created` means data \"added to iRODS\". `last_changed` and " +
	"`last_updated` are source-row mutation times; `cache_synced_at` and " +
	"`mlwh_freshness` describe cache completeness/as-of state. For the cheapest " +
	"overview, prefer `added_last_7_days` from `mlwh_study_overview`; otherwise use " +
	"explicit `since` and `until`.\n" +
	"- Runs: Run grain is one native run identifier: `id_run` for " +
	"Illumina/Elembio/Ultimagen, `pac_bio_run_name` for PacBio, and " +
	"`experiment_name` for ONT. Preserve each row's `date_basis`: Illumina/Elembio " +
	"use run completion, Ultimagen uses archival, PacBio uses `run_complete`, and " +
	"ONT uses warehouse load time, not a true sequencing date.\n" +
	"- Aggregates: preserve the requested `group_by` keys and " +
	"`unit=runs|samples|products`. Run dates use each platform's run basis; " +
	"sample/product dates use iRODS `created`.\n" +
	"- People: faculty sponsor is a separate Study field; use " +
	"`mlwh_studies_for_faculty_sponsor` for sponsor membership and the sponsor " +
	"latest-data tool for recency. A programme is exact study membership, while " +
	"study-user roles are owner, manager, data_access_contact, follower, slf_manager, " +
	"lab_manager, or administrator. Use `mlwh_resolve_person` for ambiguous names.\n\n" +
	"Other cheap-first routes:\n" +
	"- Availability/counts: use `mlwh_study_overview` or " +
	"`mlwh_count_samples_with_data_for_study`; do not page iRODS or use " +
	"`mlwh_study_detail` for availability/count questions.\n" +
	"- Data-access-group: use `mlwh_study_overview` or `mlwh_resolve_study`; do not use " +
	"study detail for data-access-group questions.\n" +
	"- QC counts: use `mlwh_study_status_breakdown`.\n" +
	"- Sample/run progress: use `mlwh_sample_progress` and `mlwh_run_status`; compute " +
	"open phase elapsed time on the agent side from `reached_at` or `entered_at`.\n" +
	"- Generic CRAM file paths outside the per-study selected-CRAM workflow: use the " +
	"matching iRODS path list or count tool with `file_type=cram`.\n" +
	"- Export pagination: " + exportContinuationGuidance + "\n" +
	"- Freshness: use response `cache_synced_at` when present; use `mlwh_freshness` for " +
	"bare lists, counts, `mlwh_run_status`, and `mlwh_call_endpoint` responses without " +
	"cache_synced_at. From `mlwh_freshness` use per-table `last_run` (the oldest relevant " +
	"across tables) for cache currency and as-of caveats; `high_water` is only a " +
	"source-progress watermark that can lag when the source is unchanged, so never report " +
	"it as \"synced through\" or treat it as refresh currency.\n\n" +
	"Use `mlwh_call_endpoint` only when no curated workflow covers the question. The full, " +
	"always-current `wa.EndpointReference()` catalogue follows; choose a Registry Method " +
	"from the generic tool's input enum, then use the catalogue for its path and query " +
	"parameters.\n\n" +
	"---\n\n"

// workflowResourceBody assembles the workflow resource body: the workflow
// guidance prefix followed by wa.EndpointReference()'s always-current,
// Registry-derived Markdown catalogue. It calls EndpointReference() rather than
// embedding a copied doc, so the catalogue can never drift from the upstream
// API.
func workflowResourceBody() string {
	return workflowGuidance + wa.EndpointReference()
}

// registerWorkflowResource adds the mlwh://workflow resource (Story G1) through
// the Registrar. Its body is the workflow guidance prefix plus the live
// wa.EndpointReference() catalogue; its MIME type is text/markdown. The body is
// computed per read so the catalogue always reflects the compiled-in Registry.
func (p *provider) registerWorkflowResource(r core.Registrar) error {
	r.AddResource(&mcp.Resource{
		URI:         workflowResourceURI,
		Name:        "mlwh-workflow",
		Title:       "MLWH workflows and endpoint catalogue",
		Description: "How the MLWH endpoints compose into multi-step workflows (resolve -> detail -> expand), followed by the full Registry-derived endpoint catalogue.",
		MIMEType:    "text/markdown",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: "text/markdown",
				Text:     workflowResourceBody(),
			}},
		}, nil
	})

	return nil
}
