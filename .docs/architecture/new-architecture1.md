# Architecture 1: Sanger Knowledge Gateway

Last researched: 2026-06-10

## Summary

Build a small Sanger-owned knowledge gateway, using the
`wtsi-hgi/llm-knowledge-base` Next.js plus FastAPI scaffold as the starting web
application, `wtsi-hgi/wa` as the safe MLWH integration path, and off-the-shelf
open-source components for document conversion, storage, retrieval, policy, and
observability.

This is the most balanced architecture: it keeps the Sanger-specific product
surface, removes generated SQL, centralises Unix-aware permissions, and exposes
the same governed capabilities to the web UI, CLI, Claude Code, and Codex CLI.

## Product Shape

- Users sign in through Okta and land in a Sanger chat workspace.
- `/docs` searches only documents the user is permitted to read.
- `/mlwh` turns natural language into calls to curated MLWH API endpoints, not
  raw SQL.
- CLI users can ingest files and ask document or MLWH questions.
- Claude Code and Codex CLI users can connect to Sanger MCP servers and use the
  same governed tools from their enterprise agent sessions.
- Administrators can inspect documents, metadata, permissions, audit events,
  failed ingestions, and supported MLWH question types.

## Main Components

### Web And API Shell

Use `wtsi-hgi/llm-knowledge-base` as the starting point because it already gives
the desired shape: Next.js 16 App Router, React 19, shadcn/ui, Tailwind CSS v4,
FastAPI, server actions, typed Zod contracts, Vitest, and pytest.

Add product code in narrow slices:

- Next.js chat workspace, session browser, upload surfaces, model selector,
  answer renderers, and admin screens.
- FastAPI routers for chat, document ingestion, document search, MLWH queries,
  permissions, artefacts, and audit events.
- Shared contract schemas so the frontend and backend fail fast when API shapes
  drift.

### Authentication And Identity

- Use Okta OIDC for sign-in.
- Use Auth.js or a simple OIDC reverse-proxy pattern for the Next.js side.
- Validate bearer tokens in FastAPI and map the user to:
  - Okta subject and email.
  - Sanger username.
  - Unix UID and group memberships from LDAP.
- Keep the Unix identity mapping in the backend, not in browser code.

### Document Ingestion

Use Docling for document conversion. It is a current open-source document
processing toolkit with support for advanced PDF understanding and many common
formats, including office documents, spreadsheets, HTML, images, OCR, tables,
layout, Markdown export, and structured JSON export.

Pipeline:

1. Upload or CLI ingest request arrives.
2. The ingestion service records source path, owner, group, mode bits, stat
   time, uploader, source channel, checksum, and original filename.
3. The original file is stored in an S3-compatible object store.
4. Docling converts it to structured text, tables, page-level content, and
   snapshots where appropriate.
5. Chunks are created with document/page/table metadata.
6. Embeddings and lexical terms are stored in Qdrant.
7. Metadata, permissions, audit records, jobs, summaries, and artefact pointers
   are stored in PostgreSQL.

### Retrieval

Use Qdrant as the main vector store because it supports payload filtering and
modern hybrid search patterns. Store dense vectors, sparse vectors or lexical
signals, chunk payloads, document IDs, page IDs, and permission metadata.

FastAPI owns retrieval orchestration:

- Resolve the user identity and groups.
- Ask the policy layer which documents and chunks are visible.
- Run a filtered hybrid retrieval query.
- Optionally rerank the allowed candidates.
- Build the answer prompt only from allowed chunks.
- Return citations, excerpts, page snapshots, and source links only after the
  same policy check has passed.

### Permissions And Policy

Use Open Policy Agent as the policy decision point, with application-side
enforcement in every data path.

Every indexed document records:

- Source path at ingestion.
- Unix owner UID/name.
- Unix owning group GID/name.
- Mode bits.
- Uploader and ingestion channel.
- Optional application grants and revocations.
- Retention and deletion state.

Minimum rule:

- If the querying user belongs to the Unix group that owned the file at
  ingestion, the user can retrieve information from that file.

The policy service should also answer:

- Can this user retrieve this chunk?
- Can this user see this excerpt?
- Can this user see this page snapshot?
- Can this user download the original?
- Can this user search document metadata?
- Can this admin inspect or override this document?

Every allow or deny is auditable.

### MLWH

Use `wtsi-hgi/wa` rather than generating SQL. The `wa` repository already has:

- An `mlwh` Go package with cache-backed study, sample, library, run, and iRODS
  lookups.
- A `seqmeta` REST API backed by MLWH caches.
- Endpoints such as `GET /studies`, `GET /study/{id}/samples`,
  `GET /diff/study/{id}`, `GET /diff/sample/{id}`, `GET /enrich/*`, and
  `GET /validate/*`.
- Provider methods for resolving identifiers and asking common sample, study,
  library, run, lane, and iRODS path questions.

The Sanger backend should use an LLM only for intent classification and argument
extraction:

1. Classify the user question into a supported MLWH intent.
2. Extract identifiers and filters.
3. Call the relevant `wa seqmeta` endpoint or a new endpoint added to `wa`.
4. Return bounded, typed results.
5. Ask the selected LLM for a short explanation of those typed results.

When the desired question is not supported by `wa`, add a typed endpoint to `wa`
through a PR. Do not fall back to arbitrary generated SQL in the production
path.

### CLI And Agent Access

Provide two thin MCP servers:

- `sanger-docs-mcp`: tools for ingesting files, searching documents, answering
  from documents, listing accessible documents, and fetching allowed excerpts.
- `sanger-mlwh-mcp`: tools for resolving MLWH identifiers, listing supported
  question types, asking typed MLWH questions, and explaining returned rows.

Expose remote Streamable HTTP MCP servers protected by Okta/OAuth for enterprise
agent clients. Also provide a local stdio wrapper for ingestion from a Unix
shell, so the tool can stat the file in the user's local context before sending
it to the governed backend.

Codex CLI can consume stdio and Streamable HTTP MCP servers, including OAuth for
Streamable HTTP. Claude Code also supports MCP server configuration. Both
clients should use read-only tools for search/answering and explicit
side-effecting tools for ingestion or deletion.

### LLM Gateway And Observability

Use open-source components where they reduce code:

- LiteLLM Proxy as an optional OpenAI-compatible model gateway for GPT, Claude,
  DiNA-compatible, and local models.
- Langfuse as optional self-hosted tracing, prompt, cost, latency, and
  evaluation infrastructure.

The application should not depend on a specific commercial model API. The model
surface is a provider abstraction, while enterprise subscriptions govern the
user-facing Codex and Claude agent clients.

## New Code Required

- Sanger-specific chat UI and admin UI in the `llm-knowledge-base` frontend.
- FastAPI orchestration routers and schemas.
- Ingestion workers around Docling.
- OPA policies and policy test fixtures.
- Qdrant/PostgreSQL/S3 data access code.
- Thin MCP servers and CLI wrappers.
- `wa` PRs for missing typed MLWH questions.

This is still a meaningful build, but the new code is mostly glue and product
workflow code, not document parsing, vector search, auth protocols, SQL
generation, or agent protocol code.

## Strengths

- Best match to the desired feature list.
- Central permission enforcement is possible across web, CLI, MCP, API,
  retrieval, citations, snapshots, and downloads.
- Removes generated SQL from the MLWH path.
- Gives Claude Code and Codex CLI first-class access without duplicating core
  logic.
- Keeps the web UI tailored to Sanger rather than fighting a generic chat
  product.
- Each major subsystem is replaceable.

## Weaknesses

- More integration work than adopting a complete turnkey RAG product.
- The team owns RAG quality, evaluation, and operational behaviour.
- Unix identity mapping must be designed carefully, especially for web-uploaded
  files that do not naturally come from a Unix path.

## Best Fit

Choose this if the product must be reliable, governed, Sanger-specific, and
usable from both web and enterprise coding agents.

## Sources Used

- Desired features: `.docs/architecture/features.md`
- Current feature inventory: `.docs/architecture/features-actual.md`
- `wtsi-hgi/llm-knowledge-base`: https://github.com/wtsi-hgi/llm-knowledge-base
- `wtsi-hgi/wa`: https://github.com/wtsi-hgi/wa
- Docling: https://docling-project.github.io/docling/
- Qdrant hybrid queries: https://qdrant.tech/documentation/search/hybrid-queries/
- Qdrant filtering: https://qdrant.tech/documentation/concepts/filtering/
- Model Context Protocol: https://modelcontextprotocol.io/docs/getting-started/intro
- MCP authorization: https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization
- MCP transports: https://modelcontextprotocol.io/specification/2025-06-18/basic/transports
- Codex MCP: https://developers.openai.com/codex/mcp
- Codex configuration: https://developers.openai.com/codex/config-basic
- Claude Code MCP: https://docs.anthropic.com/en/docs/claude-code/mcp
- Claude MCP connector: https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector
- Auth.js Okta provider: https://authjs.dev/reference/core/providers/okta
- Okta groups claims: https://developer.okta.com/docs/guides/customize-tokens-groups-claim/main/
- Open Policy Agent: https://www.openpolicyagent.org/docs/latest/
- LiteLLM Proxy: https://docs.litellm.ai/docs/
- Langfuse: https://langfuse.com/docs
