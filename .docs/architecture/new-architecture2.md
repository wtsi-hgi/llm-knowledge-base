# Architecture 2: RAGFlow-Centred Knowledge Product

Last researched: 2026-06-10

## Summary

Make RAGFlow the centre of the document knowledge system, and build only the
Sanger-specific pieces around it: Okta identity, Unix-aware permission
enforcement, MLWH access through `wa`, a lightweight web shell where needed, and
MCP adapters for agents.

RAGFlow is an open-source RAG engine based on deep document understanding. It
already provides document ingestion, parsing, datasets, chat, citation-backed
answers, APIs, and agent-oriented features. This architecture minimises custom
RAG implementation code, but it has a serious fit question: the desired Unix
permission model is stricter and more Sanger-specific than the default
knowledge-base model in most turnkey RAG systems.

## Product Shape

- Users sign in to a Sanger web shell or a customised RAGFlow front door.
- Documents are ingested into RAGFlow datasets after a Sanger permission gateway
  records source ownership and Unix mode metadata.
- `/docs` calls a Sanger retrieval wrapper that either queries RAGFlow through
  its APIs or routes to group-scoped RAGFlow datasets.
- `/mlwh` remains outside RAGFlow and uses `wa`.
- Claude Code and Codex CLI use Sanger MCP tools that call RAGFlow and `wa`
  behind a governed API.

## Main Components

### RAGFlow As RAG Core

Use RAGFlow for:

- Document parsing and chunking.
- Dataset management.
- Embedding and retrieval workflows.
- Citation-backed answers.
- RAG APIs.
- Optional built-in chat and agent features.
- Optional MCP support where it fits.

This avoids writing most of the document ingestion and RAG orchestration stack.

### Sanger Gateway

Build a small Sanger gateway in FastAPI:

- Okta token validation.
- Unix identity lookup.
- Permission policy enforcement.
- Upload and ingestion preflight.
- Dataset routing.
- Audit events.
- MLWH intent routing.
- MCP server implementation.

The gateway must be the only approved way to access governed Sanger documents.
Direct RAGFlow admin or API access should be restricted to service operators,
because bypassing the gateway could bypass Sanger permission checks.

### Permission Strategies

There are two plausible permission models.

#### Option A: Dataset Per Unix Group

Ingest each document into one or more RAGFlow datasets based on its Unix group
and application grants. At query time, choose only datasets the user can read.

Pros:

- Simple to explain.
- Uses more of RAGFlow's existing retrieval flow.
- Faster MVP.

Cons:

- Dataset explosion is likely.
- Documents with changing grants are operationally awkward.
- It is hard to represent all Unix mode-bit and admin-policy cases.
- Built-in dataset administration can become a permission bypass.

#### Option B: External ACL-Filtered Retrieval

Use RAGFlow for parsing and possibly chunk storage, but keep document and chunk
metadata in Sanger PostgreSQL. The Sanger gateway filters accessible chunks
before generation.

Pros:

- Better fit for Unix-aware policy.
- Central audit trail.
- Less chance that citations or snapshots leak across permissions.

Cons:

- More custom code.
- Uses RAGFlow less as an end-to-end product.
- May fight RAGFlow internals depending on APIs.

Option B is safer for production. Option A is a reasonable pilot strategy only
if the pilot data has simple group ownership and low confidentiality risk.

### MLWH

Use `wa seqmeta` and `wa mlwh` as in Architecture 1. RAGFlow should not be used
to generate SQL or query MLWH directly. The MLWH workflow remains:

- Natural language intent classification.
- Typed endpoint selection.
- Bounded API call to `wa`.
- LLM commentary over typed results.

### CLI And Agent Access

Expose Sanger MCP servers in front of RAGFlow and `wa`:

- `sanger-docs-mcp` calls the Sanger permission gateway, not RAGFlow directly.
- `sanger-mlwh-mcp` calls the MLWH gateway backed by `wa`.
- A local ingestion wrapper captures Unix `stat` metadata before upload.

## New Code Required

- Sanger gateway.
- Okta and Unix identity mapping.
- Permission and audit layer.
- RAGFlow dataset integration and access control adapters.
- MLWH gateway over `wa`.
- MCP servers and CLI wrapper.
- Optional Sanger web shell if RAGFlow's UI is not sufficient.

## Strengths

- Lowest custom RAG implementation effort.
- Faster path to a convincing document Q&A pilot.
- RAGFlow includes document-centric UX, APIs, citations, and agent features.
- Parsing, chunking, and retrieval quality can be evaluated before building
  custom equivalents.

## Weaknesses

- Unix-aware access control is the hardest fit.
- Built-in RAGFlow UI, APIs, caches, admin tools, and datasets can become
  bypass paths unless tightly isolated.
- MLWH remains a separate custom integration.
- Sanger-specific chat rendering and artefact persistence may require either
  custom UI work or deep product customisation.
- Operational complexity may be high if RAGFlow brings its own database, queue,
  object store, model, and search assumptions.

## Best Fit

Choose this for a fast RAG pilot or benchmark. Do not choose it as the default
production architecture unless a proof of concept demonstrates that Unix
permissions can be enforced before retrieval, generation, citation display,
snapshot display, and source download without bypasses.

## Sources Used

- Desired features: `.docs/architecture/features.md`
- RAGFlow GitHub: https://github.com/infiniflow/ragflow
- RAGFlow docs: https://ragflow.io/docs/
- Model Context Protocol: https://modelcontextprotocol.io/docs/getting-started/intro
- Codex MCP: https://developers.openai.com/codex/mcp
- Claude Code MCP: https://docs.anthropic.com/en/docs/claude-code/mcp
- Claude MCP connector: https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector
- `wtsi-hgi/wa`: https://github.com/wtsi-hgi/wa
- Open Policy Agent: https://www.openpolicyagent.org/docs/latest/
- Okta groups claims: https://developer.okta.com/docs/guides/customize-tokens-groups-claim/main/
