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

// workflowGuidance is a short note, prefixed before the endpoint catalogue, on
// how the MLWH endpoints compose into common multi-step workflows. It names the
// resolve -> detail -> expand path so an agent can plan a query rather than
// guessing tools. The mention of "resolve" and "detail" is asserted by Story
// G1.3, but the guidance is genuine planning help, not test scaffolding.
const workflowGuidance = "# MLWH workflows\n\n" +
	"This server bridges the read-only `wa mlwh` API. Prefer cheap overview, count, " +
	"status, manifest, person-resolution, and freshness tools before expensive detail " +
	"or list calls. A common identifier workflow is to **resolve** a raw identifier into " +
	"a canonical `Match` (e.g. `mlwh_resolve_sample`, `mlwh_resolve_study`), fetch " +
	"**detail** aggregates only when cheaper tools do not answer the question (e.g. " +
	"`mlwh_sample_detail`, `mlwh_study_detail`), then **expand** the canonical identifier " +
	"into related identifiers or downstream search values (`mlwh_expand_identifier`, " +
	"`mlwh_expand_search_values`).\n\n" +
	"Cheap-first routes:\n" +
	"- Availability/counts: use `mlwh_study_overview` or " +
	"`mlwh_count_samples_with_data_for_study`; do not page iRODS or use " +
	"`mlwh_study_detail` for availability/count questions.\n" +
	"- Recency: prefer `added_last_7_days` from `mlwh_study_overview`; otherwise use " +
	"explicit `since` and `until`. Data \"added to iRODS\" means the iRODS `created` " +
	"timestamp; `last_changed` and `last_updated` are not new-data timestamps.\n" +
	"- Data-access-group: use `mlwh_study_overview` or `mlwh_resolve_study`; do not use " +
	"study detail for data-access-group questions.\n" +
	"- QC counts: use `mlwh_study_status_breakdown`.\n" +
	"- Sample/run progress: use `mlwh_sample_progress` and `mlwh_run_status`; compute " +
	"open phase elapsed time on the agent side from `reached_at` or `entered_at`.\n" +
	"- CRAM paths: count first, then use an iRODS path tool with `file_type=cram`.\n" +
	"- Manifest: use `mlwh_study_manifest`; set `with_irods=true` and " +
	"`file_type=cram` when a CRAM column is requested.\n" +
	"- People routing: send sponsor questions to faculty-sponsor tools, " +
	"login/email/membership questions to user tools, and ambiguous names through " +
	"`mlwh_resolve_person`.\n" +
	"- Freshness: use response `cache_synced_at` when present; use `mlwh_freshness` for " +
	"bare lists, counts, `mlwh_run_status`, and `mlwh_call_endpoint` responses without " +
	"cache_synced_at.\n\n" +
	"The full, always-current endpoint catalogue follows; the generic `mlwh_call_endpoint` " +
	"tool can reach any endpoint by its Registry Method name.\n\n" +
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
