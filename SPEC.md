# Gitwise — Product Specification

**An AI-Native Code Collaboration Platform**

Version 1.0 — April 2026 | CONFIDENTIAL

---

## 1. Executive Summary

Gitwise is a self-hosted code collaboration platform built from the ground up with AI integration as a first-class architectural concern. While V1 delivers the core workflows developers expect (git hosting, pull requests, issues, code review), every data model and API is designed so that future AI capabilities can operate on richer, more structured data than any existing platform provides.

The thesis is simple: GitHub, GitLab, and Bitbucket were designed before LLMs existed. Their schemas, APIs, and data models were built for human consumption. Gitwise is designed for both human and machine consumption from day one, enabling a class of AI-powered workflows that cannot be bolted onto legacy platforms without fundamental re-architecture.

---

## 2. Vision & Positioning

### 2.1 What Gitwise Is

- A fully functional, self-hosted Git platform comparable to Gitea/Forgejo in scope
- A platform where every piece of data is structured, queryable, and embeddable for AI consumption
- An open foundation designed to support deep AI integration that would be controversial or impossible on incumbent platforms

### 2.2 What Gitwise Is Not (V1)

- Not a GitHub replacement for enterprise at scale — V1 targets small-to-mid teams (1–100 developers)
- Not an AI product yet — V1 ships zero AI features; it ships the infrastructure that makes them possible
- Not a SaaS — V1 is self-hosted only, distributed as a single binary + Docker image

### 2.3 Competitive Landscape

| Platform | Open Source | Self-Hosted | AI-Native Data Model | Target |
|---|---|---|---|---|
| GitHub | No | Enterprise only | No (bolted on) | Everyone |
| GitLab | Partial | Yes | No | Enterprise DevOps |
| Gitea/Forgejo | Yes | Yes | No | Self-hosters |
| **Gitwise** | **Yes** | **Yes** | **Yes (foundational)** | **AI-forward teams** |

---

## 3. Architecture Overview

### 3.1 Technology Stack

| Layer | Technology | Rationale |
|---|---|---|
| Backend API | Go (stdlib + chi router) | Performance for git operations, excellent concurrency, single binary deployment |
| Git Operations | go-git + shell to git CLI | go-git for read-heavy operations, shell out for push/clone performance |
| Database | PostgreSQL 16+ | JSONB for flexible metadata, full-text search, pg_trgm for fuzzy matching, pgvector for embeddings |
| Search | PostgreSQL (FTS + pgvector) | Unified search: keyword via tsvector, semantic via pgvector — no separate search infra |
| Cache / Queue | Redis 7+ | Session cache, job queue (background indexing, webhook delivery), pub/sub for real-time |
| Object Storage | MinIO / S3-compatible | Git LFS objects, attachments, build artifacts |
| Frontend | React 18 + TypeScript | Vite build, TanStack Query for data fetching, Zustand for state |
| Code Editor | CodeMirror 6 | Syntax highlighting, in-browser editing, diff rendering |
| Real-time | WebSocket (gorilla/websocket) | Live PR updates, presence indicators, collaborative review |
| Auth | OIDC / OAuth2 + local accounts | SSO support from day one, RBAC with org/team/repo scoping |

### 3.2 Deployment Model

Gitwise ships as a single Go binary with an embedded React frontend. The only external dependencies are PostgreSQL and Redis. A Docker Compose file bundles everything for one-command deployment. MinIO is optional; local filesystem storage is the default for small installations.

### 3.3 System Layers

| Layer | Components | Responsibilities |
|---|---|---|
| Edge | Reverse proxy, TLS termination | Rate limiting, request routing, static assets |
| API | REST API (chi router), WebSocket server | Authentication, authorization, request validation, response serialization |
| Domain | Repo service, PR service, Issue service, Review service, Search service, User service | Business logic, domain rules, cross-entity coordination |
| Infrastructure | Git backend, PostgreSQL, Redis, Object storage, Embedding pipeline | Data persistence, caching, background jobs, file storage |

---

## 4. Data Model (AI-Native Design)

This is where Gitwise fundamentally differs from existing platforms. Every entity carries structured metadata designed for machine consumption alongside human-readable content.

### 4.1 Core Design Principles

- **Everything is embeddable:** Every text field (issue body, PR description, review comment, commit message) gets a vector embedding stored in pgvector. This enables semantic search across the entire platform from day one.
- **Everything is versioned:** Issue descriptions, PR descriptions, and comments store full edit history as JSONB arrays, not just the latest version. This gives future AI full context on how discussions evolved.
- **Everything has structured metadata:** Beyond free-text fields, entities carry typed, queryable metadata. A PR doesn't just have a description — it has structured fields for intent, scope, risk assessment, and affected components.
- **Everything is linked:** Explicit, typed relationships between entities (issue → PR → commit → review → comment) form a traversable graph, not just text references.

### 4.2 Repository

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| owner_id | UUID | FK to users or organizations |
| name | VARCHAR(255) | URL-safe repository name |
| description | TEXT | Human-readable description |
| description_embedding | VECTOR(1536) | Semantic embedding of description |
| default_branch | VARCHAR(255) | Default branch name |
| visibility | ENUM | public, private, internal |
| language_stats | JSONB | Detected languages with percentages |
| topics | TEXT[] | Categorization tags |
| metadata | JSONB | Extensible structured metadata (framework, conventions detected, etc.) |
| created_at / updated_at | TIMESTAMPTZ | Timestamps |

### 4.3 Pull Request

The PR model is the richest entity in the system, designed to capture the full lifecycle of a code change with structured data at every stage.

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| repo_id | UUID | FK to repository |
| number | INTEGER | Repo-scoped sequential number |
| title | VARCHAR(500) | Human-readable title |
| title_embedding | VECTOR(1536) | Semantic embedding |
| body | TEXT | Markdown description |
| body_embedding | VECTOR(1536) | Semantic embedding of body |
| body_history | JSONB[] | Full edit history with timestamps and author |
| source_branch | VARCHAR(255) | Source branch name |
| target_branch | VARCHAR(255) | Target branch name |
| status | ENUM | draft, open, merged, closed |
| intent | JSONB | `{type: feature\|bugfix\|refactor\|chore, scope: string, components: string[]}` |
| diff_stats | JSONB | `{files_changed, insertions, deletions, files: [{path, status, insertions, deletions}]}` |
| review_summary | JSONB | `{approved_by: [], changes_requested_by: [], comments_count, threads_resolved, threads_unresolved}` |
| merge_strategy | ENUM | merge, squash, rebase |
| merged_by | UUID | FK to user who merged |
| merged_at | TIMESTAMPTZ | Merge timestamp |
| created_at / updated_at | TIMESTAMPTZ | Timestamps |

### 4.4 Issue

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| repo_id | UUID | FK to repository |
| number | INTEGER | Repo-scoped sequential number |
| title | VARCHAR(500) | Human-readable title |
| title_embedding | VECTOR(1536) | Semantic embedding |
| body | TEXT | Markdown body |
| body_embedding | VECTOR(1536) | Semantic embedding |
| body_history | JSONB[] | Full edit history |
| status | ENUM | open, closed, duplicate |
| labels | TEXT[] | Categorization labels |
| assignees | UUID[] | Assigned users |
| milestone_id | UUID | Optional milestone FK |
| linked_prs | UUID[] | Explicitly linked PRs |
| priority | ENUM | critical, high, medium, low, none |
| metadata | JSONB | Extensible structured data |
| created_at / updated_at | TIMESTAMPTZ | Timestamps |

### 4.5 Review & Comment

Reviews and comments carry richer structure than any existing platform:

| Field | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| pr_id | UUID | FK to pull request |
| author_id | UUID | FK to user |
| type | ENUM | approval, changes_requested, comment, dismissal |
| body | TEXT | Review-level comment |
| body_embedding | VECTOR(1536) | Semantic embedding |
| comments | JSONB[] | Inline comments: `[{path, line, side, body, body_embedding, thread_id, resolved, created_at}]` |
| submitted_at | TIMESTAMPTZ | Submission timestamp |

### 4.6 Commit Metadata

Beyond what git stores natively, Gitwise indexes additional structured data per commit for future AI analysis:

| Field | Type | Notes |
|---|---|---|
| sha | CHAR(40) | Commit hash |
| repo_id | UUID | FK to repository |
| message | TEXT | Full commit message |
| message_embedding | VECTOR(1536) | Semantic embedding |
| author_email | VARCHAR(255) | Git author email |
| author_id | UUID | Resolved platform user (nullable) |
| diff_stats | JSONB | `{files: [{path, status, insertions, deletions, language}]}` |
| parent_shas | TEXT[] | Parent commit hashes |
| pr_id | UUID | FK to PR this commit belongs to (nullable) |
| committed_at | TIMESTAMPTZ | Commit timestamp |
| indexed_at | TIMESTAMPTZ | When Gitwise processed this commit |

### 4.7 Embedding Pipeline

All embeddings are generated asynchronously via a background job queue. The pipeline is provider-agnostic, supporting OpenAI, local models (via Ollama), or any API that returns a vector. A configuration flag controls whether embeddings are enabled; self-hosters who do not want AI features can disable the pipeline entirely with zero overhead.

The embedding model and dimensionality are stored in a system configuration table, allowing future migration between models. All embeddings are stored with a model_version field so stale embeddings can be re-generated when the model changes.

---

## 5. API Design

### 5.1 API Principles

- **REST with structured responses:** Every endpoint returns JSON with a consistent envelope: `{data, meta, errors}`
- **Cursor-based pagination:** All list endpoints use opaque cursor tokens, not page numbers. This enables stable pagination over changing datasets.
- **Rich filtering:** List endpoints accept structured filters via query parameters. Issues can be filtered by label, assignee, milestone, priority, date range, and semantic similarity.
- **Semantic search endpoint:** A unified `/search` endpoint accepts a natural language query and returns results across repos, issues, PRs, code, and comments, ranked by vector similarity combined with recency and relevance signals.
- **Webhook-first:** Every mutation fires a typed webhook event. Events carry full before/after snapshots, not just IDs.

### 5.2 Core Endpoints

| Method | Path | Description |
|---|---|---|
| POST | /api/v1/repos | Create repository |
| GET | /api/v1/repos/:owner/:repo | Get repository details |
| GET | /api/v1/repos/:owner/:repo/tree/:ref/*path | Browse file tree |
| GET | /api/v1/repos/:owner/:repo/blob/:ref/*path | Get file contents |
| GET | /api/v1/repos/:owner/:repo/commits | List commits with filters |
| POST | /api/v1/repos/:owner/:repo/pulls | Create pull request |
| GET | /api/v1/repos/:owner/:repo/pulls/:number | Get PR with full metadata |
| POST | /api/v1/repos/:owner/:repo/pulls/:number/reviews | Submit review |
| PUT | /api/v1/repos/:owner/:repo/pulls/:number/merge | Merge pull request |
| POST | /api/v1/repos/:owner/:repo/issues | Create issue |
| GET | /api/v1/repos/:owner/:repo/issues/:number | Get issue with full history |
| POST | /api/v1/search | Unified semantic + keyword search |
| GET | /api/v1/repos/:owner/:repo/activity | Repository activity feed |

### 5.3 Git Protocol Support

Gitwise supports git operations over both HTTPS (smart HTTP protocol) and SSH. The git HTTP backend is implemented as a Go handler that proxies to git-http-backend for push/pull operations. SSH access is provided via a built-in SSH server using the gliderlabs/ssh library, authenticating against the user's stored public keys.

---

## 6. Frontend Architecture

### 6.1 Core Pages

| Page | Description | Key Components |
|---|---|---|
| Dashboard | User home: activity feed, your PRs, your issues, starred repos | Activity timeline, PR/issue cards, repo grid |
| Repository | Code browser, file viewer, branch selector, commit log | File tree, syntax-highlighted viewer, blame view, commit graph |
| Pull Request | Diff viewer, review interface, discussion thread, merge controls | Side-by-side/unified diff, inline commenting, review checklist, CI status |
| Issues | List view with filters, detail view with timeline | Filter bar, label management, cross-reference timeline |
| Search | Unified search results across all entities | Faceted results, semantic relevance indicators, code snippet previews |
| Settings | Repo, org, user, and system settings | Branch protection rules, webhook management, access control |

### 6.2 Design Principles

- **Performance-first:** Code diffs and file trees must render at 60fps even for large PRs. Virtualized lists for long diffs. Lazy loading for file trees.
- **Keyboard-navigable:** Full keyboard shortcut support modeled after GitHub's shortcuts (g+i for issues, g+p for PRs, etc.)
- **Dark mode native:** CSS custom properties for theming, dark mode as default with light mode toggle.
- **Responsive:** Functional on mobile for issue triage and PR review, optimized for desktop for code review.

---

## 7. Search Architecture

Search is a first-class feature, not an afterthought. Gitwise provides three search modalities unified behind a single endpoint.

### 7.1 Keyword Search

PostgreSQL full-text search with tsvector indexing on all text fields. Supports boolean operators, phrase matching, and field-scoped queries (`title:bug`, `label:critical`). The pg_trgm extension provides fuzzy matching for typo-tolerant search.

### 7.2 Semantic Search

Every text field's embedding is indexed with pgvector using an HNSW index for approximate nearest neighbor search. Users can search with natural language queries like "that PR where we fixed the authentication timeout" and get results ranked by cosine similarity. Semantic search results are blended with keyword results using reciprocal rank fusion.

### 7.3 Code Search

Code search indexes the content of files in the default branch using PostgreSQL trigram indexes. This supports substring matching, regex search, and language-filtered queries. For large repositories, code indexing runs as a background job after each push to the default branch.

### 7.4 Search Ranking

Results from all three modalities are blended using reciprocal rank fusion (RRF). Additional signals boost ranking: recency (newer content ranks higher), engagement (more comments/reactions), and user affinity (content from repos/users you interact with frequently).

---

## 8. Authentication & Authorization

### 8.1 Authentication

- **Local accounts:** Email + password with argon2id hashing. TOTP-based 2FA support.
- **OAuth2/OIDC:** External identity providers (GitHub, Google, any OIDC-compliant provider). Self-hosters can configure their corporate IdP.
- **API tokens:** Scoped personal access tokens with expiration dates. Fine-grained scopes (`repo:read`, `repo:write`, `issues:read`, etc.)
- **SSH keys:** Public key authentication for git operations. Keys stored per-user with fingerprint display.

### 8.2 Authorization Model

RBAC with three levels of scoping: system-level (admin), organization-level (owner, member), and repository-level (admin, write, triage, read). Permissions are inherited — org owners have admin access to all org repositories. Branch protection rules enforce merge requirements (required reviews, status checks, linear history).

---

## 9. AI-Forward Foundations (V1 Infrastructure)

V1 ships no AI features to end users. Instead, it builds the infrastructure layer that makes future AI integration dramatically easier than on any existing platform. This section documents what V1 builds and what it enables.

### 9.1 What V1 Builds

| Foundation | V1 Implementation | Future AI Use Case |
|---|---|---|
| Embedding pipeline | Background job that generates and stores vector embeddings for all text content | Semantic search, similar issue detection, PR intent analysis |
| Structured PR metadata | Typed intent, scope, and component fields on every PR | Convention drift detection, automated categorization, impact prediction |
| Full edit history | JSONB arrays storing every version of descriptions/comments | Discussion evolution analysis, consensus detection, decision mining |
| Typed entity graph | Explicit, queryable relationships: issue → PR → commit → review | Automated traceability, impact analysis, knowledge graph traversal |
| Commit indexing | Structured metadata per commit beyond what git stores | Temporal pattern analysis, convention evolution tracking |
| Webhook events | Full before/after snapshots on every mutation | Real-time AI agents, automated review triggers, anomaly detection |
| Provider-agnostic embedding config | Configurable embedding provider and model | Self-hosters can use local models for privacy, cloud models for quality |

### 9.2 Future AI Integration Points (Post-V1)

These are not V1 features. They are the capabilities the V1 data model is designed to support:

- **Semantic duplicate detection:** When a user creates an issue, search embeddings for similar existing issues and surface them before submission.
- **PR intent verification:** Compare the stated intent of a PR against the actual diff to flag mismatches.
- **Convention analysis:** Use the commit metadata and PR history to infer how coding patterns have evolved over time and flag PRs that deviate from current conventions.
- **Automated review suggestions:** Use the typed review history to learn what kinds of comments reviewers make on what kinds of changes, and pre-populate review checklists.
- **Natural language queries:** Allow users to ask questions about their codebase in plain English, powered by semantic search across code, issues, PRs, and reviews.
- **Knowledge graph:** Traverse the entity graph to answer questions like "what decisions led to this code existing?" by walking from code → commit → PR → issue → discussion.

---

## 10. Development Roadmap

### 10.1 Phase 1: Core Platform (Weeks 1–8)

Goal: A functional git hosting platform with repository management, authentication, and basic code browsing.

- Go project scaffolding: chi router, PostgreSQL connection, Redis connection, configuration management
- User authentication: local accounts, OAuth2 (GitHub provider), session management, API tokens
- Repository CRUD: create, delete, visibility settings, settings management
- Git backend: smart HTTP protocol handler, SSH server, push/pull/clone operations
- Code browser: file tree rendering, syntax-highlighted file viewer, raw file download
- Commit log: paginated commit history, commit detail view, diff rendering
- Frontend shell: React app scaffold, routing, auth flow, repository pages
- Database migrations: all core tables, indexes, foreign keys

### 10.2 Phase 2: Collaboration (Weeks 9–16)

Goal: Pull requests, code review, and issues — the collaboration layer.

- Pull request creation: branch comparison, diff generation, PR form with structured metadata fields
- Diff viewer: side-by-side and unified diff rendering, syntax highlighting in diffs, file-level navigation
- Code review: inline commenting, review submission (approve/request changes/comment), threaded discussions
- PR lifecycle: draft PRs, merge (merge/squash/rebase), close, reopen, branch deletion after merge
- Branch protection: required reviews, status checks (API-based), linear history enforcement
- Issues: create, edit, close, labels, assignees, milestones, cross-references to PRs
- Notifications: in-app notification system, mention detection, subscription management
- WebSocket: real-time PR updates, new comments, status changes

### 10.3 Phase 3: Search & Intelligence (Weeks 17–22)

Goal: Full-text, semantic, and code search. Embedding pipeline. Activity feeds.

- Full-text search: tsvector indexing on all text fields, pg_trgm for fuzzy matching
- Embedding pipeline: background job queue, provider-agnostic embedding generation, pgvector storage
- Semantic search: vector similarity search, reciprocal rank fusion with keyword results
- Code search: trigram indexing of file content, language-filtered search, regex support
- Unified search UI: combined results page, faceted filtering, code snippet previews
- Activity feed: per-repo and per-user activity streams, event timeline
- Webhook system: event dispatch, delivery tracking, retry logic, secret verification

### 10.4 Phase 4: Polish & Release (Weeks 23–26)

Goal: Production readiness, documentation, deployment tooling.

- Admin panel: system settings, user management, background job monitoring
- Docker image: multi-stage build, Compose file with PostgreSQL + Redis + MinIO
- Documentation: API docs (OpenAPI spec), self-hosting guide, configuration reference
- Performance: load testing, query optimization, caching strategy, git operation benchmarks
- Security audit: dependency scan, CSRF/XSS protections, rate limiting, input validation review
- Migration tooling: import from GitHub/GitLab (repos, issues, PRs via their APIs)

---

## 11. Key Technical Decisions

### 11.1 PostgreSQL as the Unified Data Store

Rather than introducing Elasticsearch for search and a separate vector database for embeddings, Gitwise uses PostgreSQL for everything: relational data, full-text search (tsvector), fuzzy search (pg_trgm), and vector similarity (pgvector). This dramatically simplifies deployment and operations for self-hosters while providing adequate performance for the target scale (up to ~100 developers, ~1000 repositories). If scale demands it in the future, search can be extracted to a dedicated service behind the same API.

### 11.2 Embedding Pipeline as Optional Subsystem

The embedding pipeline is a background worker that can be enabled or disabled via configuration. When disabled, semantic search falls back to keyword-only search. When enabled, embeddings are generated asynchronously and never block user-facing operations. This ensures self-hosters who do not want to send data to an LLM provider (or run a local model) experience zero overhead.

### 11.3 JSONB for Flexible Metadata

Structured fields like PR intent, diff stats, and review summaries are stored as JSONB rather than normalized tables. This allows the schema to evolve without migrations, supports complex queries via PostgreSQL's JSONB operators, and makes the data naturally serializable to JSON API responses. The tradeoff is weaker type enforcement at the database level, which is mitigated by application-level validation using Go structs with JSON tags.

### 11.4 Single Binary Distribution

The React frontend is embedded in the Go binary using `go:embed`. This means the entire application (API server, git backend, SSH server, frontend assets) ships as a single executable. Combined with the Docker Compose file that bundles PostgreSQL and Redis, Gitwise can be deployed with a single command on any Linux server.

---

## 12. Non-Functional Requirements

| Requirement | Target | Measurement |
|---|---|---|
| Git clone (1GB repo) | < 30 seconds on LAN | End-to-end clone time |
| API response (p95) | < 200ms | Server-side latency excluding network |
| Diff rendering (1000 lines) | < 500ms to interactive | Time to first meaningful paint |
| Search (keyword) | < 100ms | Server-side query time |
| Search (semantic) | < 500ms | Including embedding generation for query |
| Concurrent users | 100+ simultaneous | No degradation at target concurrency |
| Uptime target | 99.9% | Measured monthly, excluding planned maintenance |
| Backup/restore | < 15 min for 50GB | Full pg_dump + git repo backup |
| Cold start | < 5 seconds | Binary start to accepting requests |

---

## 13. Risks & Mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| Git operation performance at scale | High | Shell out to native git for push/pull; use go-git only for read operations (tree browsing, blame, log) |
| PostgreSQL as vector DB at scale | Medium | pgvector with HNSW is adequate for <1M vectors; migration path to dedicated vector DB if needed |
| Embedding cost for large codebases | Medium | Embeddings are optional; support local models via Ollama; batch processing during off-peak hours |
| Feature parity expectations vs GitHub | High | Explicitly scope V1 as core workflows only; publish a public roadmap with clear boundaries |
| Self-hosting complexity | Medium | Single binary + Docker Compose; minimal external dependencies; comprehensive docs |
| Security of self-hosted git hosting | High | Follow Gitea's security model as baseline; regular dependency audits; sandboxed git operations |

---

## 14. Success Metrics

### 14.1 V1 Launch Criteria

- A single developer can clone, push, create a PR, review, and merge with zero friction
- A team of 10 can use Gitwise as their primary git host for a real project for 2 weeks without hitting blockers
- Semantic search returns relevant results for natural language queries across issues and PRs
- Self-hosting deployment takes < 10 minutes from zero to working instance
- Import a medium-sized GitHub repository (with issues and PRs) completes successfully

### 14.2 AI Readiness Criteria

- All text content has corresponding vector embeddings within 60 seconds of creation
- The entity graph (issue → PR → commit → review) is fully traversable via API
- Full edit history is preserved and queryable for all descriptions and comments
- Structured PR metadata (intent, scope, components) is populated on > 80% of PRs via UI guidance
- Webhook events carry full before/after snapshots for all mutations

---

## 15. Open Questions

- **License:** AGPL (like Gitea/GitLab CE) vs MIT (like Forgejo) vs BSL (like Sentry). AGPL protects against cloud providers offering it as SaaS without contributing back.
- **Federation:** Should Gitwise support ActivityPub federation (like Forgejo) for cross-instance collaboration? Defer to V2 or design data model to support it?
- **CI/CD:** Build a built-in CI system (like Gitea Actions) or integrate with external CI providers via webhooks? V1 ships webhooks; CI is a V2 decision.
- **Plugin system:** Should the AI integration points be exposed as a plugin API so third parties can build integrations? Architecture should support this even if V1 does not ship plugins.

---

*End of Specification*
