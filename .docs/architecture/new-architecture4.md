# Architecture 4: MCP-First Agent Fabric

Last researched: 2026-06-10

## Summary

Make MCP servers the primary product surface. The web UI becomes one client of
the same governed document and MLWH APIs, while Claude Code, Codex CLI, and a
Sanger CLI are first-class users from the beginning.

This architecture is deliberately agent-first. It is attractive because the new
desired feature list explicitly says users should be able to ask enterprise
coding agents about MLWH and indexed documents, and ideally ask those agents to
add documents.

## Product Shape

- A Sanger Knowledge API owns documents, permissions, retrieval, MLWH routing,
  sessions, audit, and artefacts.
- Two or three MCP servers expose narrow, well-described tools.
- Claude Code and Codex CLI users add the MCP servers once and then ask normal
  natural-language questions from their agent workflows.
- The web UI is implemented later or in parallel as a thin client over the same
  API.

## Main Components

### Knowledge API

Use the same core backend stack as Architecture 1:

- FastAPI for the governed API.
- PostgreSQL for metadata, sessions, jobs, audit events, permissions, and
  artefact pointers.
- S3 for originals, snapshots, exports, and large artefacts.
- Docling for document conversion.
- Qdrant for filtered hybrid retrieval, or OpenSearch if Architecture 3's
  security model wins in a proof of concept.
- `wa` for MLWH.
- OPA for policy decisions.

The key difference is sequencing: build the API and MCP tools before investing
deeply in a polished web chat workspace.

### MCP Servers

Create separate MCP servers with tight tool boundaries.

#### `sanger-docs-mcp`

Read tools:

- `search_docs(query, filters)`
- `answer_docs(question, filters)`
- `list_accessible_docs(filters)`
- `get_excerpt(document_id, page_or_chunk_id)`
- `get_page_snapshot(document_id, page)`

Write tools:

- `ingest_file(path, title, tags, group_hint)`
- `ingest_url(url, title, tags)`
- `delete_document(document_id)`

Side-effecting tools should advertise their side effects so agent clients can
ask for confirmation according to their own approval model.

#### `sanger-mlwh-mcp`

Tools:

- `list_supported_mlwh_questions()`
- `classify_mlwh_question(question)`
- `resolve_identifier(identifier)`
- `samples_for_study(study_identifier, filters)`
- `studies_for_sample(sample_identifier)`
- `libraries_for_study(study_identifier)`
- `files_for_sample(sample_identifier)`
- `runs_for_sample(sample_identifier)`
- `answer_mlwh(question)`

Each tool calls `wa` or a Sanger MLWH gateway endpoint. None of them accepts
free-form SQL.

#### Optional `sanger-admin-mcp`

For administrators only:

- Inspect ingestion failures.
- Re-run document conversion.
- View permission decisions.
- Quarantine a document.
- Produce audit reports.

This should be a separate server or require separate scopes so normal users do
not see admin tools.

### Remote And Local Transports

Use both MCP deployment modes:

- Remote Streamable HTTP MCP servers protected by Okta/OAuth for normal
  enterprise use.
- Local stdio wrappers for Unix-aware ingestion, where the wrapper can inspect
  the local file before upload.

The local wrapper captures:

- Absolute path.
- Device and inode where useful.
- Owner UID/name.
- Group GID/name.
- Mode bits.
- File size and mtime.
- Checksum.
- Calling Unix username and group list.

The remote API still re-evaluates policy and records the decision.

### Web UI

Use `wtsi-hgi/llm-knowledge-base` as the later web shell. It should call the
same Knowledge API as the MCP servers. This prevents drift between web,
CLI, and agent behaviour.

Minimal first web UI:

- Okta login.
- Chat workspace.
- `/docs` and `/mlwh`.
- Upload page.
- Source/citation display.
- Admin document table.

Richer renderers, multimodal upload, conversation search, and exports can be
added after the MCP paths prove the core workflows.

## New Code Required

- Knowledge API.
- MCP servers and server instructions.
- CLI wrappers and install/setup docs for Codex and Claude Code.
- Policy layer and ingestion pipeline.
- Minimal web UI.
- `wa` endpoint extensions.

## Strengths

- Best direct match for enterprise coding-agent access.
- Encourages narrow, typed tools instead of one large chat backend.
- Less duplicated business logic between UI and agents.
- Easier to roll out to power users before the full web UX is complete.
- Codex CLI and IDE support stdio and Streamable HTTP MCP, including OAuth for
  Streamable HTTP. Claude's platform also supports MCP connectors.

## Weaknesses

- Non-agent users get a weaker first experience unless the web UI is built in
  parallel.
- MCP client capability and UX varies across tools and versions.
- File ingestion from agents is subtle: the agent may run in a different
  environment than the user's original Unix shell.
- Rich answer rendering, exports, and chat history still need a web product.
- Administrators will need clear controls for what MCP tools are allowed.

## Best Fit

Choose this if the primary strategic goal is to make Sanger data available to
Claude Code and Codex CLI users quickly. It is also a strong companion to
Architecture 1: build Architecture 1, but include Architecture 4's MCP layer in
the first release rather than treating it as an integration afterthought.

## Sources Used

- Desired features: `.docs/architecture/features.md`
- Model Context Protocol introduction: https://modelcontextprotocol.io/docs/getting-started/intro
- MCP transports: https://modelcontextprotocol.io/specification/2025-06-18/basic/transports
- MCP authorization: https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization
- MCP security best practices: https://modelcontextprotocol.io/specification/2025-06-18/basic/security_best_practices
- Python MCP SDK and FastMCP: https://github.com/modelcontextprotocol/python-sdk
- Codex MCP: https://developers.openai.com/codex/mcp
- Codex configuration: https://developers.openai.com/codex/config-basic
- Claude Code MCP: https://docs.anthropic.com/en/docs/claude-code/mcp
- Claude MCP connector: https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector
- `wtsi-hgi/wa`: https://github.com/wtsi-hgi/wa
- `wtsi-hgi/llm-knowledge-base`: https://github.com/wtsi-hgi/llm-knowledge-base
