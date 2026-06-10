# Sanger-AI Feature Inventory

Last reviewed from code: 2026-06-10

This document describes end-user-visible capabilities in the current
ai-rag-tools/sanger-ai codebase, without treating the current implementation as
the desired architecture. It is intended as a cleanup checkpoint before
designing a simpler replacement.

Status labels:

- **Complete or in use**: wired into the web UI, backend, proxy, or admin
  workflow in a way that appears to support a current user journey.
- **Partial, stub, or legacy**: code exists, but the feature is hidden,
  disconnected, brittle, duplicated by a newer path, or looks like an older
  experiment.
- **Desired new feature**: not implemented in the current system, but requested
  as part of the future product direction.

## Product Jobs

The codebase appears to be trying to support these high-level jobs:

- Let Sanger users ask natural-language questions in a web chat UI.
- Let users ask questions over uploaded documents using retrieval augmented
  generation (RAG).
- Let users ask natural-language questions against MLWH and other databases.
- Let users bring files, images, videos, web pages, repositories, and notebooks
  into a chat as context.
- Let administrators inspect indexed documents, users, groups, and document
  permissions.
- Keep conversations, uploaded artefacts, indexed document data, and generated
  outputs available across sessions.

## Complete or In-Use Features

### Authenticated Web UI

Users can access a Sanger-branded web application with a public landing page,
sign-in/sign-up flows, password reset, and a protected chat workspace.

The UI presents Sanger-AI as a tool for asking questions across research
documents, operational systems, and LIMS-style data. Authenticated users can
open the chat workspace, choose models, upload files, and return to previous
sessions.

Notes:

- Authentication and account management are part of the user journey.
- Credit/subscription data is shown and checked before model calls.
- The repository appears to expect some deployment-specific authentication
  configuration that is not fully present in source control.

### Chat Workspace and Conversation Management

Users can create and revisit chat sessions. The interface supports session
history, searching previous conversations, deleting sessions, deleting messages,
renaming sessions, and saving large message artefacts outside the primary chat
record.

The chat workspace is the main entry point for most features. Several commands
inside the chat trigger specialised workflows, such as document Q&A, MLWH
queries, web search, or database queries.

### General LLM Chat

Users can send free-form prompts to selected LLMs. The visible model selector
includes Sanger-branded DiNA models, GPT-family models, and Claude-family
models.

The product behaviour is a normal assistant chat with optional attached context
from files, URLs, images, videos, search results, or database query results.

### Document Upload and Document Q&A

Users can upload PDFs and TIFFs for durable indexing. The system can extract
text, fall back to OCR for scanned content, create page snapshots, generate
document summaries, store document-level metadata, and make the content
available for later retrieval.

Users can then ask document questions from chat, primarily through the `/doc`
workflow. Answers can include source documents, page references, quoted
evidence, page snapshots, and links back to stored source artefacts.

The retrieval workflow is designed to search across multiple documents that the
current user is allowed to access, then generate an answer from the most relevant
document sections.

Current status:

- This appears to be one of the strongest and most complete areas of the
  current product.
- PDF and TIFF are the best-supported durable RAG document types.
- Other file types may be usable as immediate chat context, but are not all
  clearly indexed for future RAG queries.

### Broad File Upload as Chat Context

Users can attach many kinds of files to a chat, including PDFs, TIFFs, DOCX,
XLSX, CSV, text/code files, Markdown, HTML, XML, JSON, images, videos, and ZIP
archives.

Depending on file type, the UI can extract text, render previews, convert
spreadsheets to tables, show formatted code/text, analyse images, analyse video,
or upload artefacts for later access from the conversation.

Current status:

- This is broadly wired into the chat UI.
- The durable indexing story is much clearer for PDFs and TIFFs than for the
  rest of the file types.

### MLWH Natural-Language Queries

Users can ask natural-language questions about MLWH from chat using `/mlwh` or
`/mldwh`. The system translates the user request into a MySQL query, executes
it, formats returned rows, and then asks the selected LLM to provide a short
commentary on the result.

This is a core feature because it connects natural-language chat to structured
Sanger operational data.

Current status:

- The user-facing workflow appears complete and in use.
- The current behaviour depends on generating SQL, validating it, and running it
  against MySQL.
- This should be treated as a feature to preserve, not as an implementation to
  preserve. Generated SQL creates risks around correctness, performance, and
  accidental expensive queries.

### Other Natural-Language Database Queries

The product also exposes natural-language query paths for Oracle and SQL Server.
These workflows translate text into SQL, execute the query, and return data for
the chat to explain.

Current status:

- These appear implemented as secondary database-query features.
- Their future value should be confirmed, because the core strategic database
  mentioned by the product direction is MLWH.

### Document Access and Permission Administration

The current system has an application-level permission model for indexed
documents. Documents can be associated with users, users can belong to groups,
and documents can be granted to users or groups.

Administrators have a dashboard for inspecting indexed documents, viewing
metadata, searching document contents, managing users and groups, granting or
revoking permissions, and deleting documents with associated indexed data.

Current status:

- User/group based access checks appear to be part of document retrieval before
  answers are generated.
- This is not the Unix file-permission model requested for the future.
- Direct access to stored source files, snapshots, or generated URLs should be
  reviewed. The retrieval path has permission checks, but all source-display and
  download paths must enforce the same policy.

### Web Search and External Content Extraction

Users can ask the chat to search the web, retrieve image search results, extract
web page content, process YouTube transcripts, inspect GitHub repositories, and
process notebook links.

Current status:

- These are wired into the chat as helper workflows.
- They are auxiliary to the core Sanger document and MLWH use cases.

### Multimodal Analysis and Generation

Users can add images and videos to a chat. The system can analyse uploaded image
files or image URLs, analyse video files, and generate images through several
model providers.

The chat UI also includes speech-oriented features: browser microphone input,
read-aloud controls, and text-to-speech service integration.

Current status:

- Image upload/analysis, image generation, and video analysis appear to be
  visible user features.
- Speech and TTS support exists, but should be validated against the desired
  deployment environment before treating it as a core feature.

### Rich Answer Rendering and Export

The chat can render structured outputs beyond plain text. Supported answer
formats include Markdown, code blocks, JSON-like structured data, tables, charts,
Mermaid diagrams, PlantUML, Graphviz, Cytoscape diagrams, LaTeX/TikZ, and Google
Charts.

Users can export or download some generated outputs, such as reports, tables, or
visual artefacts.

Current status:

- This is a real usability feature for technical and scientific users.
- The future product should keep the useful rendering/export behaviours without
  carrying forward every specialised renderer unless users need them.

### Conversation, Artefact, and Source Persistence

The system persists chat sessions, messages, uploaded files, generated content,
document cache data, page snapshots, and large message artefacts. Users can
search previous messages and return to older sessions.

Current status:

- This is important for a durable research assistant experience.
- Future designs should keep persistence, search, and auditability as first
  class requirements.

### Credit, Subscription, and Payment Flows

The web app checks user credit before model calls, deducts credit after use,
shows credit history, and includes Stripe-oriented subscription or payment
flows.

Current status:

- These flows are wired into the UI and API layer.
- Their relevance should be explicitly confirmed. They may be unnecessary or
  undesirable for an internal Sanger enterprise deployment.

### Operational Admin Surfaces

The deployed stack exposes health checks, API documentation, document/vector
administration, object-store administration, and supporting local services such
as text-to-speech and model-serving containers.

Current status:

- These are operational features for administrators and developers, not normal
  end-user product features.

## Partial, Stub, or Legacy Features

### BigQuery and Athena Workflows

The chat recognises BigQuery/Athena-style commands and there are API routes for
some of them, but the active proxy/backend paths do not appear consistently wired
for these commands.

Status: partial or legacy.

### Legacy RAG Paths

There are older or parallel RAG implementations, including routes that spawn
scripts directly, BigQuery-backed embedding experiments, Streamlit scripts,
Byaldi/Mistral experiments, and older PDF-processing paths.

Status: legacy or experimental. The current durable document path appears to be
the PDF/TIFF indexing and multi-document query flow.

### Hidden or Historical Model Providers

The code includes routes or mappings for many model providers and local model
options beyond the currently visible model selector. Some are commented out or
appear to be older experiments.

Status: mixed. Keep only the providers needed for the future product.

### Browser Agent Workflow

There is an endpoint and routing logic for browser-agent style tasks, and older
model mappings mention browser-use agents. This is not clearly exposed as a
current mainstream user workflow.

Status: partial or hidden.

### `Knowledge Extractor` Chat Mode

Several document-query paths reference a `Knowledge Extractor` mode, but it does
not appear to be a visible current model choice.

Status: likely legacy.

### Unreachable or Incomplete Chat Commands

Some command branches look unreachable or incomplete, including aliases such as
`/structured`, `/erp`, `/arotron`, `/ardax`, and `/legal`, plus some proxy
routes that do not match the active backend.

Status: partial or legacy.

### ZIP-Based Durable Document Indexing

ZIP upload handling can extract embedded files and appears intended to process
embedded PDFs/TIFFs, but that path does not look as consistent as direct PDF/TIFF
upload.

Status: partial. Direct PDF/TIFF upload should be considered the complete path.

### Cached-Document User Association

The product tries to associate a user with an already-cached document when they
upload a duplicate file. The supporting path looks fragile and should be tested
before relying on it.

Status: partial or buggy.

### Finance, Recommender, and Demo-Like Commands

There are workflows for stock/forex commentary and recommender-style requests.
These look more like demo or generic chatbot features than core Sanger document
or MLWH features.

Status: probably removable unless users actively need them.

### Webcam or Continuous Capture

There is code related to image capture or periodic frame processing, but the
normal user workflow is not obvious from the current UI.

Status: partial or experimental.

### Domain-Specific Medical Metadata

The document pipeline includes metadata concepts that look IVF/clinical or
medical-literature specific. These may not match the broader Sanger-AI document
use case.

Status: legacy domain bias. Future metadata should reflect Sanger document and
data-governance needs.

### Build and Deployment Configuration Gaps

Some runtime configuration appears external, missing from the repository, or
hard-coded to old domains/providers. That affects whether features can be run
cleanly from source.

Status: deployment-specific or incomplete.

## Desired New Feature: Enterprise CLI and Agent Access

In addition to the web UI, users working in tools such as Claude Code and Codex
CLI with enterprise subscriptions should be able to ask natural-language
questions about MLWH and previously stored documents.

The desired user experience is:

- A user can ask about MLWH from the web UI, a CLI, or an enterprise coding
  agent.
- A user can ask about indexed documents from the web UI, a CLI, or an
  enterprise coding agent.
- A user can add documents to the system through the web UI.
- A user can add documents through a CLI.
- Ideally, a user can ask an enterprise coding agent to add a document, and the
  agent can use a sanctioned local or remote interface to ingest it.
- Answers can include citations, page references, and relevant excerpts only
  when the querying user is allowed to see them.

The desired permission model for documents is Unix-file-permission aware:

- At ingestion time, the system should record the source file path, Unix owner,
  Unix group, mode bits, and any other permission metadata that is needed for
  policy decisions.
- A querying user should be mapped to a Unix identity and group membership.
- At minimum, if the querying user belongs to the Unix group that owned the file
  when it was ingested, they are allowed to retrieve information from that file.
- The policy should be applied before retrieval, answer generation, excerpt
  display, page snapshot display, source download, cache lookup, and admin/API
  access.
- Permission decisions should be auditable.

For MLWH, the desired feature is natural-language access to trusted structured
data, not necessarily natural-language SQL generation. Future architectures
should prefer an approved service or API that exposes safe, optimized MLWH
queries over raw generated SQL where possible.

Current status:

- The web UI exists.
- Document RAG exists.
- MLWH natural-language querying exists through generated SQL.
- CLI/agent access does not appear to exist.
- Unix-owner/group based document permissions do not appear to exist.

## Cleanup Questions for Product Owners

Before designing the replacement architecture, this document should be trimmed
to the features that are genuinely wanted.

Please decide whether to keep, remove, or de-scope:

- Credit, subscription, and Stripe payment flows.
- Image generation.
- Image and video analysis.
- Web search, YouTube, GitHub, and notebook extraction.
- Oracle and SQL Server natural-language query support.
- BigQuery and Athena support.
- Finance, recommender, and other demo-like commands.
- Browser-agent workflows.
- Rich diagram renderers beyond Markdown, tables, charts, and code blocks.
- Admin dashboards for direct vector/object-store management.
- Any legacy medical/IVF-specific document metadata.
