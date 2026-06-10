# Architecture Recommendation

Last researched: 2026-06-10

## Recommendation

Pick **Architecture 1: Sanger Knowledge Gateway** as the main architecture, and
include the MCP layer from **Architecture 4: MCP-First Agent Fabric** in the
first production slice.

In practice, this means:

- Start from `wtsi-hgi/llm-knowledge-base` for the web/API shape.
- Use `wtsi-hgi/wa` for MLWH, extending it with typed endpoints through PRs as
  needed.
- Use Docling for document conversion.
- Use Qdrant plus PostgreSQL plus MinIO for governed document RAG.
- Use OPA or an equivalent policy-as-code layer for Unix-aware permissions.
- Expose Sanger Docs and Sanger MLWH as MCP servers for Claude Code and Codex
  CLI from the start.
- Avoid generated SQL in the production MLWH path.

This gives the smallest architecture that still truly fits the desired product.
The hard parts of the product are not generic chat or generic RAG; they are
Sanger-specific governance, Unix-aware document access, safe MLWH questions,
auditable excerpts, and consistent access from web, CLI, and enterprise agents.

## Comparison

| Criterion | 1. Sanger Gateway | 2. RAGFlow-Centred | 3. OpenSearch Platform | 4. MCP-First | 5. Turnkey Chat UI |
| --- | --- | --- | --- | --- | --- |
| Unix permission correctness | Strong, central policy | Risky unless heavily gated | Strong for search reads, extra policy still needed | Strong if backed by same gateway | Risky unless built-in RAG is bypassed |
| CLI and agent access | Strong via MCP plus CLI | Strong if gateway fronts RAGFlow | Strong via MCP | Best | Depends on external Sanger tools |
| MLWH safety | Strong via `wa`, no SQL generation | Strong if kept outside RAGFlow | Strong via `wa` | Strong via `wa` | Strong only in Sanger tool |
| Web UI effort | Moderate | Low to moderate | Moderate | Moderate to high unless web comes later | Lowest |
| RAG implementation effort | Moderate | Lowest | Moderate | Moderate | Low for demo, moderate for governed RAG |
| New custom code | Moderate, mostly glue | Low at first, may rise for ACLs | Moderate to high ops/config | Moderate, tool-heavy | Low demo, hidden cost later |
| Operational complexity | Moderate | Moderate to high | Highest | Moderate | Low to moderate |
| Licence/product-fit risk | Low | Low to moderate | Low | Low | Moderate, especially Open WebUI |
| Time to credible MVP | Good | Fastest RAG pilot | Slower | Fast for power users | Fastest demo |
| Production fit | Best | Conditional | Good for security-first search | Best companion to Architecture 1 | Weak unless constraints soften |

## Why Architecture 1 Wins

Architecture 1 keeps one governed core behind every interface. That matters
because the same permission decision must apply before retrieval, generation,
excerpt display, page snapshot display, source download, cache lookup, admin
inspection, API access, CLI access, and MCP access. A generic RAG product can
answer questions quickly, but the desired system needs to prove what a user was
allowed to see and why.

It also gives the cleanest MLWH story. `wa` already has an MLWH-backed cache,
identifier resolution, `seqmeta` REST endpoints, and Go APIs for common study,
sample, library, run, lane, and iRODS questions. Extending `wa` with typed
endpoints is safer than asking an LLM to generate SQL against MLWH.

The MCP layer should not be postponed. Codex CLI supports stdio and Streamable
HTTP MCP servers, including bearer token and OAuth authentication for
Streamable HTTP. Claude also has MCP support. Building the tools early means
the web UI, CLI, and agents exercise the same contracts and policy checks.

## How To Use The Other Architectures

- Use Architecture 2 as a benchmark or pilot. RAGFlow may be useful for testing
  document parsing, chunking, citation UX, and RAG quality quickly. Do not adopt
  it for production until Unix-aware permission enforcement has been proven
  across all retrieval and source-display paths.
- Use Architecture 3 as the fallback if Qdrant plus application policy is not
  acceptable for security or operations. OpenSearch document-level security is
  attractive, but it brings more operational and role-mapping complexity.
- Use Architecture 4 inside Architecture 1. The recommended system should have
  MCP servers in its first release, not as a later integration.
- Use Architecture 5 only for a demo or a non-governed pilot. A turnkey chat UI
  saves frontend time, but it does not remove the need for the governed Sanger
  Knowledge Gateway.

## First Implementation Slice

1. **Foundation**

   Fork or template from `wtsi-hgi/llm-knowledge-base`. Add Okta sign-in,
   FastAPI token validation, PostgreSQL, MinIO, Qdrant, job queue, audit log,
   and a minimal chat workspace.

2. **Document Ingestion And Policy**

   Add web upload and CLI ingestion. Capture Unix source metadata for CLI
   ingests. Convert files with Docling. Store originals and snapshots. Index
   chunks. Enforce OPA policy before search, answer generation, excerpts,
   snapshots, downloads, and admin views.

3. **MLWH Without Generated SQL**

   Deploy `wa seqmeta` against an optimised MLWH cache. Build `/mlwh` intent
   classification and argument extraction. Call typed `wa` endpoints. Add PRs
   to `wa` for missing high-value questions.

4. **MCP And CLI**

   Ship `sanger-docs-mcp`, `sanger-mlwh-mcp`, and a local CLI wrapper. Provide
   setup snippets for Codex CLI and Claude Code. Keep read tools separate from
   side-effecting ingestion and deletion tools.

5. **Web UX And Administration**

   Add session search, source/citation display, artefact persistence, rich
   answer rendering, document administration, user/group views, and permission
   audit inspection.

6. **Quality And Hardening**

   Add permission-leakage tests, retrieval evaluation sets, MLWH intent tests,
   prompt/version tracking, Langfuse tracing, rate limits, model budgets,
   deletion workflows, and operational dashboards.

## Deliberate Non-Goals

- Do not recreate the current generated-SQL MLWH path.
- Do not carry forward payment, credit, or subscription flows unless the desired
  product direction explicitly reintroduces them.
- Do not make the model-provider list the centre of the architecture. Keep it
  behind a small provider abstraction or LiteLLM-compatible gateway.
- Do not let any RAG product, vector store, object store URL, or admin endpoint
  bypass the same policy checks used by `/docs`.

## Open Product Questions

- What is the authoritative mapping from Okta user to Unix username and Unix
  groups: LDAP, Active Directory, `getent` on a trusted host, or another
  service?
- For web uploads with no original Unix path, should the owning group default
  to the user's primary group, a selected group, or an application grant only?
- Are original files always stored, or can some documents be stored as
  extracted text plus snapshots only?
- Which MLWH questions are the first supported set for `/mlwh` and
  `sanger-mlwh-mcp`?
- Should document access include Unix "other read" semantics, or only the
  minimum owning-group rule currently stated in the desired features?

## Sources Used

- Desired features: `.docs/architecture/features.md`
- Current feature inventory: `.docs/architecture/features-actual.md`
- `wtsi-hgi/llm-knowledge-base`: https://github.com/wtsi-hgi/llm-knowledge-base
- `wtsi-hgi/wa`: https://github.com/wtsi-hgi/wa
- Docling: https://docling-project.github.io/docling/
- RAGFlow: https://github.com/infiniflow/ragflow
- Qdrant hybrid queries: https://qdrant.tech/documentation/search/hybrid-queries/
- OpenSearch hybrid search: https://docs.opensearch.org/latest/vector-search/ai-search/hybrid-search/index/
- OpenSearch document-level security: https://docs.opensearch.org/latest/security/access-control/document-level-security/
- Model Context Protocol: https://modelcontextprotocol.io/docs/getting-started/intro
- MCP authorization: https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization
- Codex MCP: https://developers.openai.com/codex/mcp
- Claude Code MCP: https://docs.anthropic.com/en/docs/claude-code/mcp
- Claude MCP connector: https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector
- Auth.js Okta provider: https://authjs.dev/reference/core/providers/okta
- Okta groups claims: https://developer.okta.com/docs/guides/customize-tokens-groups-claim/main/
- Open Policy Agent: https://www.openpolicyagent.org/docs/latest/
- LiteLLM Proxy: https://docs.litellm.ai/docs/
- Langfuse: https://langfuse.com/docs
- Open WebUI docs: https://docs.openwebui.com/
- AnythingLLM docs: https://docs.anythingllm.com/
