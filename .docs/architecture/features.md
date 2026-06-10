# Sanger-AI Feature Inventory

This document describes end-user-visible desired capabilities. It is mostly a
subset of features-actual.md, but with some changes.

## Overview

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

## Authenticated Web UI

Users can access a Sanger-branded web application with a public landing page,
sign-in flow (okta), and a protected chat workspace.

The UI presents Sanger-AI as a tool for asking questions across research
documents, operational systems, and LIMS-style data. Authenticated users can
open the chat workspace, choose models, upload files, and return to previous
sessions.

## Chat Workspace and Conversation Management

Users can create and revisit chat sessions. The interface supports session
history, searching previous conversations, deleting sessions, deleting messages,
renaming sessions, and saving large message artefacts outside the primary chat
record.

The chat workspace is the main entry point for most features. Several commands
inside the chat trigger specialised workflows, such as document Q&A, MLWH
queries, web search, or database queries.

## General LLM Chat

Users can send free-form prompts to selected LLMs. The visible model selector
includes Sanger-branded DiNA models, GPT-family models, and Claude-family
models.

The product behaviour is a normal assistant chat with optional attached context
from files, URLs, images, videos, search results, or database query results.

## Document Upload and Document Q&A

Users can upload many finds of iles (including PDFs, HTML, DOCX, XLSX, CSV, TSV,
text, Markdown, JSON and image files) for durable indexing. The system can
extract text, fall back to OCR for scanned content, create page snapshots,
generate document summaries, store document-level metadata, and make the content
available for later retrieval.

Users can then ask document questions from chat using `/docs`. Answers can
include source documents, page references, quoted evidence, page snapshots, and
links back to stored source artefacts.

The retrieval workflow is designed to search across multiple documents that the
current user is allowed to access, then generate an answer from the most
relevant document sections.

## Broad File Upload as Chat Context

Users can attach many kinds of files to a chat, including PDFs, DOCX, XLSX, CSV,
TSV, text files, Markdown, HTML, XML, JSON, images and videos.

Depending on file type, the UI can extract text, render previews, convert
spreadsheets to tables, show formatted code/text, analyse images, analyse video,
or upload artefacts for later access from the conversation.

## MLWH Natural-Language Queries

Users can ask natural-language questions about MLWH (a MySQL database) from chat
using `/mlwh`. The system translates the user request into an appropriate form
for querying MLWH (which could be an MLWH query API service), formats returned
results, and then asks the selected LLM to provide a short commentary on the
result.

Implementations should prefer an approved service or API that exposes safe,
optimized MLWH queries over raw generated SQL where possible.

## Document Access and Permission Administration

There is an application-level permission model for indexed documents. Documents
can be associated with users, users can belong to groups, and documents can be
granted to users or groups.

Administrators have a dashboard for inspecting indexed documents, viewing
metadata, searching document contents, managing users and groups, granting or
revoking permissions, and deleting documents with associated indexed data.

## Web Search and External Content Extraction

Users can ask the chat to search the web, retrieve image search results, extract
web page content, process YouTube transcripts, inspect GitHub repositories, and
process notebook links.

## Multimodal Analysis and Generation

Users can add images and videos to a chat. The system can analyse uploaded image
files or image URLs, analyse video files, and generate images through several
model providers.

The chat UI also includes speech-oriented features: browser microphone input,
read-aloud controls, and text-to-speech service integration.

## Rich Answer Rendering and Export

The chat can render structured outputs beyond plain text. Supported answer
formats include Markdown, code blocks, JSON-like structured data, tables,
charts, Mermaid diagrams, PlantUML, Graphviz, Cytoscape diagrams, LaTeX/TikZ,
and Google Charts.

Users can export or download some generated outputs, such as reports, tables, or
visual artefacts.

## Conversation, Artefact, and Source Persistence

The system persists chat sessions, messages, uploaded files, generated content,
document cache data, page snapshots, and large message artefacts. Users can
search previous messages and return to older sessions.

## Enterprise CLI and Agent Access

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

The desired permission model for documents uploaded from a CLI-like context
is Unix-file-permission aware:

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
