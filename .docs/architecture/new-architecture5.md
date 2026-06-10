# Architecture 5: Turnkey Chat UI With Governed External Tools

Last researched: 2026-06-10

## Summary

Adopt an existing open-source chat/RAG product, such as Open WebUI or
AnythingLLM, for most of the chat interface and model-provider UX. Keep Sanger
documents and MLWH behind governed external services exposed as OpenAPI tools,
MCP servers, or both.

This is the fastest route to a broad demo, but it is the weakest fit for the
full desired product because the hard requirements are not generic chat. They
are Unix-aware document permissions, MLWH safety, Sanger admin/audit behaviour,
and consistent web/CLI/agent access.

## Product Shape

- Users sign in through Okta or an Okta-protected reverse proxy.
- The chat UI comes mostly from a turnkey product.
- Built-in document/RAG features may be used for low-risk personal or pilot
  data.
- Governed Sanger document Q&A goes through a Sanger Knowledge Gateway tool.
- MLWH queries go through a Sanger MLWH tool backed by `wa`.
- Claude Code and Codex CLI use the same Sanger MCP tools directly.

## Candidate Products

### Open WebUI

Open WebUI provides a rich self-hosted AI UI with chat, model integrations,
RAG-oriented features, SSO/OIDC options, OpenAPI tool servers, and native MCP
support in recent versions.

Important caveat: verify the current licence and internal legal acceptability
before choosing it. Recent Open WebUI licensing is not as simple as a permissive
MIT/Apache project, so it may not satisfy a strict "open source only" policy in
all organisations.

### AnythingLLM

AnythingLLM is positioned as an all-in-one private AI application with RAG,
agents, multi-user support, vector databases, and document pipelines. Public
materials describe it as MIT licensed. It may be a cleaner licensing fit than
Open WebUI, but Okta/enterprise SSO, Unix permission semantics, and Sanger
admin requirements must be validated carefully.

## Main Components

### Turnkey Chat Layer

Use the chosen product for:

- Chat UI.
- Model configuration.
- File attachment UI where acceptable.
- Basic conversation history.
- Built-in RAG for non-governed or pilot data if allowed.
- Tool invocation.

Avoid depending on its built-in RAG for governed Sanger documents unless it can
prove all required permission checks before retrieval, generation, citation
display, snapshot display, source download, cache lookup, and admin/API access.

### Sanger Knowledge Gateway

Even in this architecture, build a small Sanger gateway:

- Okta token validation or reverse-proxy identity headers.
- Unix identity mapping.
- Document ingestion with Unix metadata.
- Docling conversion.
- Search index.
- Permission enforcement.
- Citations, excerpts, snapshots, and downloads.
- Audit trail.

The turnkey UI calls this gateway as a tool rather than owning the governed
document store itself.

### MLWH Gateway

Build the MLWH tool exactly as in the other architectures:

- Natural language intent classification.
- Typed calls to `wa seqmeta` and new `wa` endpoints where needed.
- No arbitrary generated SQL.
- Bounded result sets and LLM commentary.

### MCP And OpenAPI Tools

Expose Sanger capabilities as both:

- MCP servers for Claude Code, Codex CLI, and other MCP-capable clients.
- OpenAPI tools if the chosen turnkey UI integrates more easily through
  OpenAPI.

This prevents the turnkey UI from becoming the only client.

## New Code Required

- Sanger Knowledge Gateway.
- MLWH gateway over `wa`.
- Tool adapters for OpenAPI and MCP.
- Permission/audit layer.
- Ingestion pipeline.
- Product customisation and deployment glue for the chosen chat UI.

This looks like little code at first, but the governed Sanger parts still need
to be built. The real saving is mainly chat UI and model-provider UX.

## Strengths

- Fastest demo.
- Rich chat UX with comparatively little frontend work.
- Existing model-provider and local-model integrations.
- OpenAPI/MCP tool support can expose Sanger services without deep UI changes.
- Useful for pilots and stakeholder feedback.

## Weaknesses

- Risk of two RAG systems: built-in RAG plus governed Sanger RAG.
- Unix permission requirements are unlikely to fit cleanly without bypass
  risks.
- Sanger-specific admin, audit, source snapshots, rich artefact persistence,
  and MLWH workflows still need custom code.
- Product customisation can become harder than owning a small tailored UI.
- Licensing and enterprise SSO details need careful validation.

## Best Fit

Choose this only for a rapid pilot or if the organisation strongly prefers
adopting a mature chat UI over owning the web product. Do not make it the core
production architecture unless the permission model, licensing, SSO, and
tool-boundary risks are resolved.

## Sources Used

- Desired features: `.docs/architecture/features.md`
- Open WebUI docs: https://docs.openwebui.com/
- Open WebUI features: https://docs.openwebui.com/features/
- Open WebUI MCP: https://docs.openwebui.com/features/extensibility/mcp/
- Open WebUI OpenAPI tool servers: https://github.com/open-webui/openapi-servers
- AnythingLLM docs: https://docs.anythingllm.com/
- AnythingLLM MCP compatibility: https://docs.anythingllm.com/mcp-compatibility/overview
- AnythingLLM GitHub: https://github.com/Mintplex-Labs/anything-llm
- LiteLLM Proxy: https://docs.litellm.ai/docs/
- `wtsi-hgi/wa`: https://github.com/wtsi-hgi/wa
- Model Context Protocol: https://modelcontextprotocol.io/docs/getting-started/intro
- Codex MCP: https://developers.openai.com/codex/mcp
