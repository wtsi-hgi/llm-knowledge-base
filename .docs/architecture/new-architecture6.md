# Architecture 6: Google Drive Source-Of-Truth RAG

Last researched: 2026-06-15

## Summary

Use company Google Drive as the authoritative document store instead of asking
users to upload durable RAG documents into a Sanger-owned S3-compatible object
store. The RAG system indexes derived text, chunks, metadata, and permission
snapshots from Drive, but Drive remains the system of record for originals,
ownership, sharing, deletion, and source links.

This is feasible, with important caveats. The workable shape is not a normal
"admin account with full access to everything" that the application signs in as.
The safer and more accurate shape is a Google Workspace service account granted
domain-wide delegation by a Workspace super administrator, with tightly scoped
Drive and Directory API permissions. The connector can then impersonate users
when it needs to crawl My Drive content and can use shared-drive administrator
paths for shared drives.

The main architecture question shifts from file storage to identity and access
control: can the system mirror Google Drive ACLs accurately enough that search,
answer generation, citations, excerpts, snapshots, and source links never reveal
content beyond what Drive would allow?

## Product Shape

- Users sign in to Sanger-AI through Okta, mapped to their Google Workspace
  primary email address.
- `/docs` searches indexed Google Drive content rather than uploaded S3 files.
- Source links open the original item in Google Drive, letting Google enforce
  access again at click time.
- The system stores extracted text, chunks, embeddings, document metadata,
  permission snapshots, sync state, audit events, and optional page snapshots.
- It does not store Drive originals unless there is a separate retention,
  offline-availability, or legal requirement.
- Administrators inspect Drive sync status, indexed metadata, ACL snapshots,
  permission decisions, failed exports, failed conversions, and tombstoned
  files.

This can sit inside Architecture 1 by replacing the upload/S3 ingestion path
with a Drive connector. Qdrant, PostgreSQL, Docling, the Sanger web/API shell,
MCP tools, and the MLWH path through `wa` can stay broadly the same.

## Feasibility Answer

### Can One Admin Account Access Everything?

Not in the simple product sense.

A Workspace administrator account can administer Drive settings and shared
drives, but Google Drive APIs are still permissioned around users, files, and
shared drives. For My Drive content owned by individual users, the production
pattern is domain-wide delegation: an administrator authorizes a service account
for selected OAuth scopes, and the service account acts on behalf of users in
the domain.

That means a crawler can cover enterprise Drive content, but it should be
designed as controlled user impersonation, not as a human admin mailbox whose
Drive happens to contain all files.

For shared drives, Drive has domain-administrator paths such as
`drives.list?useDomainAdminAccess=true`, which can enumerate all shared drives
in a domain where the requester is an administrator. Shared drive permissions
also need shared-drive-aware API parameters such as `supportsAllDrives` and
`includeItemsFromAllDrives`.

### Can We Preserve Drive Access Control?

Yes, if the RAG system treats Drive ACLs as the source of truth and never serves
cached content without its own policy check.

Drive permissions include `user`, `group`, `domain`, and `anyone` permission
types, with roles such as `owner`, `organizer`, `fileOrganizer`, `writer`,
`commenter`, and `reader`. Folder permissions propagate to child items, and
shared drives have additional inheritance and membership details. The connector
must normalize these into an application permission model and keep that model
fresh as Drive sharing changes.

The recommended enforcement model is hybrid:

- **Early binding**: store normalized ACL data beside every indexed document and
  chunk, then filter retrieval by the querying user's Google email, Google
  Groups, domain, and explicit visibility flags.
- **Late binding**: before showing citations, excerpts, page snapshots, cached
  exports, or download/proxy content, re-check the current policy snapshot and,
  for high-risk paths, optionally verify current Drive access as the user.

Early binding keeps retrieval fast. Late binding reduces the chance of stale ACL
leakage after a sharing change.

### What About "Anyone With The Link"?

This is the trickiest access-control edge case.

Drive can represent domain-wide or public permissions that are discoverable
through search, and also link-style permissions where `allowFileDiscovery` is
false. A user who has the link can read the file, but a generic search UI does
not know whether the user actually possesses that link.

There are two possible product choices:

- **Strict default**: do not make link-only files globally searchable just
  because they are technically readable by anyone with the link. Return them
  only when the file was already visible in that user's Drive crawl, when the
  user supplies the Drive URL, or when the file is discoverable to the domain.
- **Convenience mode**: treat `anyone with link` or `domain with link` as
  globally searchable for all eligible users. This is easier to explain, but it
  exposes content more broadly than normal Drive search behavior.

Use the strict default unless Sanger explicitly decides that link-shared Drive
files are acceptable in the global RAG corpus.

### Can It Auto-Update?

Yes, but the reliable design is change feeds plus periodic reconciliation, not
one permanent watch over the whole enterprise.

Drive provides a changes feed for users and shared drives. The connector should
store a `startPageToken` for each crawled user and each shared drive, call
`changes.list` with `includeRemoved=true`, and process additions, edits,
permission changes, moves, trashing, deletions, and loss of access. Drive push
notifications can wake the sync service when watched files or change feeds move,
but the notification should be treated as a prompt to pull the checkpointed
changes feed, not as the full event payload.

Google Workspace Events API subscriptions can also deliver Drive events through
Cloud Pub/Sub, but the current subscription model has expiry and lifecycle
events, so it should not replace checkpointed Drive change processing yet.

The practical sync loop is:

1. Initial crawl of users and shared drives.
2. Continuous or frequent incremental sync from stored change tokens.
3. Scheduled reconciliation crawls to catch missed events, expired watches,
   group membership changes, and ACL edge cases.
4. Vector deletion or tombstoning when Drive reports deletion, trashing,
   ownership transfer outside scope, or permanent loss of access.

With a successful initial crawl, new and changed files can normally be reflected
within minutes. A full re-trawl of a large enterprise Drive may take hours or
days depending on document count, export size, API quota, and document
conversion throughput, so it should be a backstop rather than the primary
freshness mechanism.

## Main Components

### Google Workspace Connector

Create a dedicated connector service responsible for:

- Domain-wide delegation setup and token generation.
- Drive API crawling and file export/download.
- Admin SDK Directory API user and group discovery.
- Shared drive enumeration.
- ACL normalization.
- Change token management.
- Sync job scheduling and retries.
- Audit records for every indexed, skipped, updated, tombstoned, or deleted
  Drive item.

Required Google-side setup:

- Enable Drive API access for the organization.
- Create a dedicated Google Cloud project for the connector.
- Create a dedicated service account with domain-wide delegation.
- Prefer keyless service account signing over long-lived service account keys.
- Authorize only required scopes, likely starting with:
  - `https://www.googleapis.com/auth/drive.readonly`
  - `https://www.googleapis.com/auth/drive.metadata.readonly`
  - Admin Directory read scopes for users and groups.
- Restrict who can impersonate or administer the service account.
- Record Google Cloud and Workspace audit logs for all connector activity.

Domain-wide delegation is powerful and should be treated as a high-value
credential. Google's own best-practice guidance recommends avoiding it when
possible, restricting OAuth scopes when it is necessary, avoiding service
account keys, and hosting delegated service accounts in dedicated projects.

### Crawling Strategy

Use two complementary crawlers.

#### User My Drive Crawler

1. List active Workspace users through Admin SDK Directory API.
2. Impersonate each user.
3. List files visible in that user's My Drive and explicitly shared-to-user
   scope.
4. Capture file metadata, owner data, permissions, version, modified time,
   MIME type, parents, trashed state, shared state, and source links.
5. Deduplicate by Drive `fileId`.

This is required because individual My Drive files do not become globally
visible to an admin account just because the account is an administrator.

#### Shared Drive Crawler

1. List all domain shared drives using shared-drive administrator access.
2. Crawl each shared drive with `corpora=drive`, the shared drive ID,
   `supportsAllDrives=true`, and `includeItemsFromAllDrives=true`.
3. Capture drive-level membership, folder/file-level grants, inherited
   permission details, restrictions, and item metadata.
4. Deduplicate by Drive `fileId`.

Shared drives are operationally cleaner than My Drive for enterprise RAG because
ownership belongs to the organization and administrator enumeration is better
supported.

### Content Extraction

For Google Workspace-native files:

- Export Docs to Markdown, plain text, DOCX, or PDF.
- Export Sheets to XLSX or PDF rather than relying only on CSV/TSV, because CSV
  and TSV exports cover only the first sheet.
- Export Slides to PPTX, PDF, or plain text.
- Export Drawings to PDF or image formats.

Drive's `files.export` endpoint has a 10 MB exported-content limit, so large
Google-native documents need special handling. Options include skipping with an
admin-visible error, using product-specific APIs where practical, or indexing
metadata and Drive links only until a supported extraction path is chosen.

For binary files stored in Drive:

- Download through Drive API using the delegated service.
- Convert PDFs, Office files, images, HTML, text, CSV, TSV, Markdown, JSON, and
  other supported formats through Docling.
- Store only derived text/chunks/snapshots unless source retention is explicitly
  required.

Unsupported or oversized files should be represented in metadata search with a
clear indexing status rather than silently disappearing.

### Metadata And Permission Model

Store Drive metadata in PostgreSQL, for example:

- `drive_files`: file ID, drive ID, name, MIME type, source type, owners,
  webViewLink, resource key, created time, modified time, version, checksum
  where available, trashed/deleted state, last crawl state, and extraction
  state.
- `drive_acl_entries`: file ID, permission ID, type, role, email, domain,
  `allowFileDiscovery`, inherited flag, inherited source, expiration time, and
  deleted-principal flag.
- `drive_principals`: users, groups, domains, shared-drive memberships, and
  group expansion state.
- `drive_crawl_tokens`: per-user and per-shared-drive change tokens.
- `drive_sync_events`: immutable audit trail for crawl, export, ACL update,
  deletion, and error events.
- `document_chunks`: chunk text, embedding ID, lexical terms, file ID, page or
  section metadata, and permission-filter payload.

The retrieval index should store only the minimum ACL payload needed for fast
filtering. PostgreSQL remains the authoritative local permission snapshot and
audit store.

### Retrieval And Policy

At query time:

1. Validate the user's Okta session.
2. Resolve the user's Google Workspace email and current Google Groups.
3. Build a Drive visibility context:
   - direct user grants
   - group grants
   - shared drive memberships
   - domain-discoverable grants
   - optionally link-known grants
4. Run hybrid retrieval with payload filters over allowed document chunks.
5. Re-check policy before prompt assembly.
6. Re-check policy before returning citations, excerpts, snapshots, cached
   extracted text, or source links.
7. Return Drive `webViewLink` as the preferred original-source link.

The application should still keep OPA or equivalent policy-as-code, but the
input facts change from Unix owner/group/mode bits to Google Drive principals,
roles, domains, shared-drive memberships, link visibility, and crawl state.

### Sync And Deletion Semantics

Handle these cases separately:

- **Content changed**: re-export or re-download, re-run Docling, replace chunks
  and embeddings for that file ID.
- **Metadata changed**: update title, MIME type, parent, source link, or status.
- **Permissions changed**: update ACL snapshot and retrieval payloads without
  re-embedding unchanged content.
- **File moved**: update metadata and inherited permissions; do not duplicate
  content.
- **File trashed or deleted**: remove chunks and embeddings, retain tombstone
  and audit metadata if allowed by retention policy.
- **User lost access**: remove that user's visibility. Delete content only if no
  in-scope principal can still access it.
- **Owner leaves or file transfers**: re-evaluate whether the file remains in
  the organization's indexing scope.
- **Group membership changes**: refresh group expansion and permission filters,
  because file-level change feeds may not fire for every effective membership
  change.

Vector stores must support deletion by stable document ID. Every chunk should
carry the Drive `fileId` so deletion and re-index are exact.

## Access-Control Edge Cases

- **Nested Google Groups**: decide whether to expand nested groups through
  Directory API or Cloud Identity Groups API. Test this explicitly.
- **External users**: decide whether externally owned files shared with Sanger
  users should be indexed. Default to excluding external-owner files unless a
  business owner opts in.
- **Personal My Drive content**: decide whether all employee-owned My Drive
  documents are in scope. A safer launch focuses on shared drives and selected
  organizational folders first.
- **Shortcuts**: index the target file once, not every shortcut path.
- **Multiple parents and moves**: rely on `fileId`, not path, as identity.
- **Comments and suggestions**: decide whether to index comments, resolved
  comments, suggestions, and revision history. Default to current document body
  only.
- **Drive labels and DLP**: if Sanger uses Drive labels or Google DLP signals,
  mirror them into policy inputs before indexing sensitive categories.
- **Download restrictions**: do not expose cached exports or snapshots if Drive
  policy intends content to be view-only.
- **Resource keys**: preserve resource keys for link-shared files where needed,
  but do not treat possession of a stored resource key as permission for every
  user.

## Operational Expectations

Initial crawl speed depends on four bottlenecks:

- Drive API listing and download/export quota.
- Number of users and shared drives.
- Export size and file type mix.
- Docling/OCR/conversion throughput.

Google Drive API quotas are high enough that a well-designed enterprise crawler
is plausible, but quota is still a design constraint. As of the current Drive
API limits, quota is expressed in quota units, with separate per-minute project
and per-minute per-user limits. The published limits on 2026-06-15 are
1,000,000 quota units per minute per project, 325,000 quota units per minute per
user per project, and 1 TB per day per project before charges apply. Example
method costs are 100 units for list calls such as `files.list`, 5 units for read
calls such as `files.get`, and 200 units for downloads. The connector needs
bounded concurrency, backoff, checkpointing, and quota dashboards.

Expected freshness targets:

- Shared drive changes: minutes after the initial crawl, if change tokens and
  watches are healthy.
- My Drive changes: minutes to tens of minutes, depending on user count and
  crawl scheduling.
- Group membership changes: near the group-sync interval, probably hourly or
  daily unless there is a stronger event source.
- Full reconciliation: nightly or weekly, depending on tenant size.

## Existing Systems To Evaluate

There is already open-source and source-available work in this area. That does
not remove the need for a Sanger-governed design, but it changes the
implementation strategy: evaluate existing systems before building a custom
Drive connector from scratch.

### Onyx / Danswer

Onyx, formerly Danswer, is the closest existing product shape. It is an
enterprise search and AI assistant platform with RAG, chat, many connectors,
Google Drive support, service-account setup, and source-system permission
syncing.

It is the first system to test because it already overlaps strongly with this
architecture:

- Google Drive connector.
- Service account setup for Google Workspace.
- Domain-wide delegation scopes for Drive metadata, Drive content, users, and
  groups.
- Automatic connector refresh and pruning controls.
- Permission-aware retrieval across supported source systems.
- A web AI/search product surface rather than only a backend library.

Important caveat: Onyx should be treated as open-core or source-available for
this use case, not assumed to be fully open source. Its public documentation
states that permission-syncing connectors are an Enterprise Edition feature, and
that Google Drive permission syncing requires a service account or Google
Workspace Admin OAuth credentials. That may still be a good buy-or-adopt option,
but it must be checked against Sanger's licensing, deployment, security, and
customisation requirements.

Onyx proof-of-fit questions:

- Can Sanger self-host the edition that includes Google Drive permission sync?
- Does Google Drive permission sync handle My Drive, shared drives, folder
  inheritance, Google Groups, domain grants, and deleted or moved files as
  required?
- Can Okta identities map cleanly to Google Workspace users and groups?
- Can the retrieval and citation paths be audited at the level Sanger needs?
- Can MCP, CLI, MLWH, and Sanger-specific admin workflows be integrated without
  fighting the product?

### RAGFlow

RAGFlow is an open-source RAG product with a documented Google Drive data source
flow. It is useful for a fast document-RAG pilot and for comparing parsing,
chunking, retrieval quality, citation behavior, and user experience.

RAGFlow should not be assumed to satisfy the full Architecture 6 access-control
model without testing. Its Google Drive setup appears oriented around OAuth and
folder-based data source configuration. For Sanger, the key validation is
whether it can enforce source Drive permissions before retrieval, generation,
citation display, source display, cache lookup, and admin access, and whether
incremental sync catches new, changed, moved, permission-changed, trashed, and
deleted files reliably.

### Pathway

Pathway provides open-source live document indexing templates for RAG over file
systems, Google Drive, SharePoint, S3, Kafka, PostgreSQL, and other sources.
Its templates explicitly cover additions, updates, and deletions from connected
sources and expose retriever-style APIs.

Pathway is a strong candidate if Sanger wants a reusable live-indexing backend
or wants to accelerate a custom connector. It is less directly a complete
Sanger product because Sanger would still need Okta integration, Google Drive
ACL normalization, policy enforcement, admin workflows, audit views, MCP tools,
and MLWH integration.

### LlamaIndex Google Drive Readers

LlamaIndex has Google Drive readers and an example "live" RAG pipeline over
Google Drive files, including incremental update behavior when the ingestion
pipeline is rerun. This is a useful implementation reference and a quick way to
prototype extraction and indexing.

It is a library-level option, not a governed enterprise application. By itself,
it does not solve domain-wide delegation governance, Drive ACL mirroring,
Okta-to-Google identity mapping, source-link policy, audit, deletion guarantees,
or Sanger admin workflows.

### Paragon RAG Tutorials

Paragon's public RAG tutorial repository includes Google Drive ingestion and a
permissions-system tutorial that models Google Drive and Dropbox permissions,
uses Okta FGA, and keeps permissions up to date. This is not a drop-in
open-source Sanger product, and it depends on Paragon's integration platform,
but it is a useful reference for how to model third-party permissions separately
from the vector index.

### Small Personal Drive RAG Projects

There are small repositories that do personal Google Drive RAG with OAuth,
LangChain or similar loaders, local vector stores such as Chroma, and simple
Gradio or Streamlit UIs. These are useful as tutorials and smoke tests for file
export, chunking, embedding, and local Q&A.

They are not sufficient for this architecture because they generally handle one
user's Drive, rely on explicit OAuth consent, and do not preserve enterprise
Google Workspace ACLs across many users and shared drives.

### Unstructured And Other Connectors

Unstructured has a Google Drive ingestion connector for preprocessing pipelines.
Commercial connector vendors also advertise Google Drive connectors with
metadata, permissions, incremental crawling, personal drives, shared drives, and
domain-wide delegation.

These are useful signals that the connector pattern is feasible, but they do
not remove the need to decide whether Sanger wants an open-source product, a
commercial connector, or a custom connector under the Sanger Knowledge Gateway.

## Adopt, Extend, Or Build

Architecture 6 should start with a bake-off rather than immediate custom
implementation.

1. Test Onyx first if an enterprise/open-core dependency is acceptable. It is
   the strongest candidate for avoiding custom RAG product work.
2. Test RAGFlow for RAG quality and product fit, but treat Drive ACL correctness
   as unproven until demonstrated.
3. Test Pathway and LlamaIndex if Sanger expects to build the Sanger-specific
   gateway but wants to reuse live indexing components.
4. Build a custom Drive connector only if existing products cannot satisfy
   permission correctness, auditability, deployment constraints, MLWH/MCP
   integration, or licensing requirements.

The acceptance bar for adopting an existing system should be the same as for a
custom build: no forbidden chunk should be retrievable, included in prompts,
shown as a citation, exposed as an excerpt, visible in source snapshots,
downloadable from cache, or visible through admin/API bypasses.

## New Code Required

- Google Workspace connector service, unless an adopted product supplies one
  that passes the Drive ACL proof of concept.
- Domain-wide delegation token broker, unless delegated credentials are wholly
  managed by an adopted product in an acceptable way.
- User and shared-drive crawlers, or integration with an existing live indexing
  layer such as Onyx, Pathway, or another approved connector.
- Drive change-feed workers.
- Google Groups and shared-drive membership resolver.
- Drive ACL normalizer and policy facts.
- Drive export/download integration with Docling or an adopted parsing layer.
- Re-index and vector-deletion workflows keyed by Drive `fileId`.
- Admin UI for Drive sync status, failed files, ACL snapshots, and permission
  decisions.
- `/docs` retrieval filters based on Google principals rather than Unix mode
  bits.
- MCP and CLI tools for searching Drive-backed documents and optionally adding a
  Drive URL or shared-drive folder to an allowlisted indexing scope.

This removes the need for new object-storage workflows for original documents,
but it does not remove the need for a governed RAG index, conversion pipeline,
policy layer, audit trail, admin UI, or MCP tools. If Onyx or another existing
system is adopted, some of this becomes configuration and integration work
rather than new connector code.

## Strengths

- No duplicate durable document upload path for content already in Drive.
- Drive remains the familiar source of truth for sharing, ownership, deletion,
  and source links.
- Strong fit for enterprise documents that already live in shared drives.
- Source clicks can fall back to Google's own access enforcement.
- Change feeds make ongoing freshness realistic after the initial crawl.
- Avoids inventing a parallel file-management system.

## Weaknesses

- Domain-wide delegation is a major security and governance approval.
- Exact Drive permission semantics are complex, especially link-shared files,
  inherited permissions, shared drives, nested groups, and external ownership.
- My Drive crawling requires per-user impersonation and careful deduplication.
- Large Google-native exports can hit Drive export limits.
- Stale ACLs are possible unless change feeds, group sync, reconciliation, and
  late checks are engineered carefully.
- The system becomes dependent on Google Workspace APIs, quotas, and admin
  configuration.
- If users expect Unix permission semantics, Google Drive ACLs are a different
  policy universe rather than a drop-in replacement.

## Best Fit

Choose this if Sanger already treats Google Drive as a primary enterprise
document repository and is willing to make Google Drive ACLs the authoritative
permission model for document RAG.

Start with shared drives and a small set of organizational folders. Add broad
My Drive coverage only after the permission model, DWD governance, deletion
workflow, and sync behavior have passed leakage tests.

Do not choose this as the only ingestion strategy if important documents live
outside Drive, if the organization cannot approve domain-wide delegation, or if
the product must exactly preserve Unix filesystem permissions for CLI-ingested
files.

## Proof Of Concept

Run a focused proof of concept before committing:

1. Run a short existing-system bake-off:
   - Onyx for full product and Drive permission-sync fit.
   - RAGFlow for RAG quality and Drive data-source ergonomics.
   - Pathway or LlamaIndex for reusable live-indexing components.
2. Configure a dedicated service account with domain-wide delegation in a test
   Workspace or controlled organizational unit.
3. Crawl three shared drives and 25 to 50 volunteer users.
4. Index at least 10,000 mixed files, including Docs, Sheets, Slides, PDFs,
   DOCX, XLSX, CSV, images, and oversized/unsupported examples.
5. Test ACL cases:
   - owner only
   - direct user share
   - direct group share
   - nested group share
   - shared drive membership
   - folder-inherited permission
   - domain discoverable
   - domain with link
   - anyone with link
   - external owner
   - removed permission
   - trashed and permanently deleted file
6. Measure initial crawl throughput, incremental sync latency, export failures,
   conversion failures, and vector deletion latency.
7. Run adversarial search tests where users try to retrieve documents they
   should not see.
8. Decide the product rule for link-shared files before indexing them broadly.
9. Compare adopt-vs-build cost, including licensing, deployment, security
   review, custom Sanger UI work, MCP integration, and MLWH integration.

## Recommendation Relative To Architecture 1

This is a promising variant of Architecture 1, not a complete replacement for
the governed knowledge gateway.

Architecture 1 should change from "upload originals into S3 and capture Unix
permissions" to "index Google Drive documents and mirror Drive ACLs" if the
organization accepts Drive as the canonical document repository. The rest of
Architecture 1 still matters: Sanger-owned API orchestration, retrieval policy,
auditability, admin screens, RAG evaluation, MCP servers, and MLWH access
through `wa`.

The safest roadmap is:

1. Evaluate Onyx, RAGFlow, Pathway, and LlamaIndex before writing a custom
   connector.
2. Build or adopt the Drive connector as the first document ingestion path.
3. Keep the internal RAG index and policy layer Sanger-owned unless an adopted
   product passes the same permission-leakage and audit tests.
4. Use Drive source links instead of storing originals.
5. Launch with shared drives first.
6. Add My Drive and link-shared behavior only after explicit governance review.

If Onyx Enterprise passes the proof of concept and the licence is acceptable,
it could replace a large part of the custom document RAG build. If not, it
still provides a valuable reference implementation for the Drive connector and
permission-sync behavior.

## Sources Used

- Desired features: `.docs/architecture/features.md`
- Current architecture recommendation: `.docs/architecture/recommendation.md`
- Onyx GitHub: https://github.com/onyx-dot-app/onyx
- Onyx connector overview and permission sync: https://docs.onyx.app/admins/connectors/overview
- Onyx Google Drive service account setup: https://docs.onyx.app/admins/connectors/official/google_drive/service_account
- RAGFlow Google Drive data source: https://ragflow.io/docs/add_google_drive
- Pathway live RAG templates: https://github.com/pathwaycom/llm-app
- LlamaIndex live Google Drive RAG example: https://developers.llamaindex.ai/python/examples/ingestion/ingestion_gdrive/
- Paragon RAG tutorials: https://github.com/useparagon/rag-tutorials
- Example personal Google Drive RAG repo: https://github.com/donat-konan33/google-drive-agentic-rag
- Unstructured Google Drive connector: https://docs.unstructured.io/open-source/ingestion/source-connectors/google-drive
- Google Workspace domain-wide delegation: https://knowledge.workspace.google.com/admin/apps/control-api-access-with-domain-wide-delegation
- Google domain-wide delegation best practices: https://knowledge.workspace.google.com/admin/apps/domain-wide-delegation-best-practices
- Google IAM service account best practices: https://docs.cloud.google.com/iam/docs/best-practices-service-accounts
- Google Drive files.list: https://developers.google.com/workspace/drive/api/reference/rest/v3/files/list
- Google Drive shared drive support: https://developers.google.com/workspace/drive/api/guides/enable-shareddrives
- Google Drive shared drive administration: https://developers.google.com/workspace/drive/api/reference/rest/v3/drives/list
- Google Drive sharing and ACLs: https://developers.google.com/workspace/drive/api/guides/manage-sharing
- Google Drive permissions resource: https://developers.google.com/workspace/drive/api/reference/rest/v3/permissions
- Google Drive permissions.list: https://developers.google.com/workspace/drive/api/reference/rest/v3/permissions/list
- Google Drive changes guide: https://developers.google.com/workspace/drive/api/guides/manage-changes
- Google Drive changes.list: https://developers.google.com/workspace/drive/api/reference/rest/v3/changes/list
- Google Drive push notifications: https://developers.google.com/workspace/drive/api/guides/push
- Google Workspace Events subscriptions: https://developers.google.com/workspace/events/reference/rest/v1beta/subscriptions
- Google Workspace Events lifecycle: https://developers.google.com/workspace/events/guides/events-lifecycle
- Google Drive export MIME types: https://developers.google.com/workspace/drive/api/guides/ref-export-formats
- Google Drive files.export: https://developers.google.com/workspace/drive/api/reference/rest/v3/files/export
- Google Drive API usage limits: https://developers.google.com/workspace/drive/api/guides/limits
- Admin SDK Directory API: https://developers.google.com/workspace/admin/directory/reference/rest
- Admin SDK groups.list: https://developers.google.com/workspace/admin/directory/reference/rest/v1/groups/list
- Google Workspace Drive API admin setting: https://knowledge.workspace.google.com/admin/drive/allow-third-party-apps-for-drive-files
