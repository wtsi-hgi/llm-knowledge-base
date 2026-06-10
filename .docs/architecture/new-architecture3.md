# Architecture 3: OpenSearch Security-First Platform

Last researched: 2026-06-10

## Summary

Use OpenSearch as the central retrieval and access-control platform. It handles
BM25 search, vector search, hybrid search, indexes, audit-friendly operations,
and document-level security. A Sanger FastAPI service owns ingestion,
generation, MLWH routing, and MCP tools.

This architecture optimises for search governance and operational familiarity.
It is less elegant for pure RAG than a dedicated vector store, but stronger if
Sanger wants one security-aware search platform for keyword, semantic, metadata,
and audit-heavy workloads.

## Product Shape

- Users authenticate through Okta.
- Document chunks are indexed into OpenSearch with vectors, text, metadata, and
  access-control fields.
- OpenSearch document-level security filters hidden chunks during read
  operations.
- FastAPI orchestrates answers, citations, snapshots, downloads, sessions, and
  admin workflows.
- MLWH queries use `wa`.
- CLI and agent access use MCP servers backed by the same FastAPI service.

## Main Components

### Search And Retrieval

Use OpenSearch for:

- Full-text search.
- Neural/vector search.
- Hybrid search combining lexical and semantic retrieval.
- Metadata filtering.
- Document-level security.
- Search APIs and ranking experiments.

Index model:

- One index for document chunks.
- One index for page/table/snapshot metadata, or PostgreSQL tables if simpler.
- Payload fields for document ID, source path, owner, group, mode bits,
  allowed Unix groups, uploader, MIME type, page number, chunk type, and
  retention state.
- Dense vector fields and lexical fields on the same chunks.

### Document Processing

Use Docling for conversion and OCR-capable document understanding. Store
original files and page snapshots in MinIO, structured metadata in PostgreSQL,
and search chunks in OpenSearch.

Processing flow:

1. Receive upload or CLI ingest.
2. Record Unix metadata and application metadata.
3. Run Docling conversion.
4. Create chunk records with permission metadata.
5. Index chunks into OpenSearch.
6. Store original and generated artefacts in MinIO.

### Permission Model

Use both OpenSearch security and an application policy layer.

OpenSearch:

- Map authenticated users and groups to OpenSearch roles.
- Use document-level security to hide chunks from search and get operations.
- Include group and visibility fields in the indexed chunk documents.

Application policy:

- Use OPA or equivalent policy-as-code before snapshot display, source
  download, document metadata search, admin actions, and generation.
- Do not rely only on OpenSearch, because many desired actions are outside
  search reads.

This dual enforcement is more complex, but it makes accidental leakage through
retrieval less likely.

### MLWH

Keep MLWH outside OpenSearch. Use `wa seqmeta` and `wa mlwh` for safe,
cache-backed MLWH access. The chat backend turns natural language into typed
endpoint calls and asks an LLM to summarise the returned data.

OpenSearch may index MLWH result documentation or API descriptions, but not the
MLWH database itself unless a future governance decision explicitly allows it.

### CLI And Agent Access

Provide the same `sanger-docs-mcp` and `sanger-mlwh-mcp` concept as
Architecture 1. The difference is that document search tools call FastAPI,
which calls OpenSearch under the user's mapped roles and application policy.

## New Code Required

- Ingestion service around Docling.
- OpenSearch index definitions and deployment configuration.
- Okta-to-OpenSearch role mapping.
- FastAPI query orchestration and generation.
- OPA or equivalent non-search policy enforcement.
- Admin UI and audit reports.
- MCP servers and CLI wrapper.
- `wa` endpoint extensions for unsupported MLWH questions.

## Strengths

- Strong central search platform with document-level security.
- Good fit if Sanger already operates Elasticsearch/OpenSearch-like systems.
- Handles keyword and semantic search in one place.
- Easier to build administrative search and audit views than with a pure vector
  store.
- Reduces chance that retrieval accidentally includes forbidden chunks.

## Weaknesses

- More operationally heavy than Qdrant plus PostgreSQL.
- OpenSearch document-level security does not cover writes, object-store
  downloads, snapshots, or generation by itself.
- Role and DLS mappings can become hard to reason about for many Unix groups.
- Hybrid/vector RAG ergonomics may lag purpose-built RAG/vector stacks.
- There have been compatibility concerns in the ecosystem around hybrid search
  and document-level security, so this needs an early proof of concept.

## Best Fit

Choose this if centralised search governance, keyword search, document-level
security, and operational standardisation matter more than having the simplest
RAG stack.

## Sources Used

- Desired features: `.docs/architecture/features.md`
- OpenSearch: https://opensearch.org/
- OpenSearch neural search: https://docs.opensearch.org/latest/search-plugins/neural-search/
- OpenSearch vector search: https://docs.opensearch.org/latest/vector-search/
- OpenSearch hybrid search: https://docs.opensearch.org/latest/vector-search/ai-search/hybrid-search/index/
- OpenSearch document-level security: https://docs.opensearch.org/latest/security/access-control/document-level-security/
- Docling: https://docling-project.github.io/docling/
- `wtsi-hgi/wa`: https://github.com/wtsi-hgi/wa
- Open Policy Agent: https://www.openpolicyagent.org/docs/latest/
- Model Context Protocol: https://modelcontextprotocol.io/docs/getting-started/intro
- Codex MCP: https://developers.openai.com/codex/mcp
