# Changelog

## Phase 1 — Security Hardening (2026-04-04)

Four code review passes identified and fixed ~30 issues across the full stack.

### Security Fixes

- **Path traversal** in git HTTP handler, git SSH handler, and SPA static file server — all three entry points now validate path components and verify resolved paths stay within their root directory
- **Command injection** via `?service=` query parameter — git service names whitelisted to exactly `upload-pack` and `receive-pack`
- **Private repo visibility** — `GetByOwnerAndName` and `ListByOwner` now enforce visibility based on the authenticated viewer; private repos return 404 for non-owners
- **CORS misconfiguration** — replaced `Access-Control-Allow-Origin: *` with reflected Origin (wildcard + credentials is invalid per spec)
- **Request body size limit** — all JSON endpoints capped at 1MB via `MaxBytesReader`; oversized requests return 413
- **Branch name validation** — repo create and update validate `default_branch` against a safe regex, preventing refspec injection in `AutoInit`
- **Session cookie validation** — reject non-hex or wrong-length session IDs before Redis lookup
- **Ref resolution restricted** — `ResolveRef` only accepts branches, tags, and 40-char hex SHAs; arbitrary revision syntax (`HEAD~3`, `^`, `@{}`) rejected
- **Password max length** — capped at 128 chars to prevent argon2id DoS
- **Docker** — container runs as non-root `gitwise` user; Postgres/Redis ports bound to `127.0.0.1`

### Bug Fixes

- **`owner_name` field mismatch** — frontend used `owner` but API returns `owner_name`; fixed in DashboardPage, NewRepoPage, RepoPage
- **`[object Object]` error display** — API errors are `{code, message}` objects, not strings; fixed `api.ts` type and all error render sites
- **`repo.Update` returned empty `owner_name`** — RETURNING clause didn't join users table; added subquery
- **`AutoInit` silent failure** — rewrote from shell-out to go-git library calls; errors now logged
- **Frontend query race** — tree/blob/commit queries fired with hardcoded `"main"` before repo metadata loaded; gated on `repoLoaded`
- **Blob view fell through to tree** — added explicit loading/error states
- **Logout resilience** — store clears auth state unconditionally; Layout catches errors and clears query cache
- **`res.json()` crash on non-JSON responses** — wrapped in try/catch
- **Git HTTP streaming** — `cmd.Output()` buffered entire repo in memory; now streams `cmd.Stdout` directly to `ResponseWriter`
- **`rows.Err()` missing** — added after all row iteration loops
- **`Sscanf` unchecked** — password verification now checks parse return value
- **`repo.Update` validation** — visibility and default_branch now validated (matching Create)
- **Commit ref URL-encoding** — `encodeURIComponent` on ref query parameter

---

## Phase 1 — Core Platform (2026-04-04)

The foundational layer: a functional self-hosted Git platform with authentication,
repository management, code browsing, and a complete web frontend.

### Backend (Go)

**Authentication & Users**
- Local account registration and login with argon2id password hashing
- Redis-backed session management with secure HTTP-only cookies
- Scoped API token system (create, list, revoke) with SHA-256 hashed storage
- Auth middleware supporting both session cookies and `Bearer` token headers
- User profile endpoint (`GET /api/v1/users/:username`)

**Repository Management**
- Full CRUD: create, read, update, delete repositories
- Per-user repository listing with visibility controls (public/private)
- Auto-init option creates a bare repo with initial README commit
- Shared issue/PR number sequence per repository via `next_repo_number()` SQL function
- On-disk bare git repository lifecycle tied to database records

**Git Protocol**
- Smart HTTP protocol handler for push/pull over HTTPS
  - `info/refs`, `git-upload-pack`, `git-receive-pack` endpoints
  - HTTP Basic Auth required for push operations (password or API token)
  - Gzip request decompression support
- SSH server via gliderlabs/ssh for push/pull over SSH
  - Public key authentication (infrastructure ready, key management is Phase 2)
  - Repository path parsing and command routing

**Code Browsing API**
- File tree listing at any ref and path (`GET /repos/:owner/:repo/tree/:ref/*`)
- File content retrieval with binary detection (`GET /repos/:owner/:repo/blob/:ref/*`)
- Raw file download (`GET /repos/:owner/:repo/raw/:ref/*`)
- Branch listing with HEAD indicator (`GET /repos/:owner/:repo/branches`)
- Paginated commit log (`GET /repos/:owner/:repo/commits`)
- Single commit detail with full diff and file stats (`GET /repos/:owner/:repo/commits/:sha`)

**Infrastructure**
- chi router with request ID, logging, recovery, CORS, and auth middleware
- PostgreSQL connection pool (pgx) with configurable pool size
- Redis client for session storage
- Environment-based configuration (all settings via `GITWISE_*` env vars)
- Graceful shutdown on SIGINT/SIGTERM
- Health check endpoint (`GET /healthz`)

### Database

- Initial migration (`001_initial.sql`) with full AI-native schema:
  - Users, organizations, org membership
  - Repositories with pgvector embedding columns, JSONB metadata, topics array
  - Issues and pull requests with embedding columns, body history, structured metadata
  - Reviews with inline comment JSONB, code review types
  - Comments (shared issue/PR timeline) with embedding columns and edit history
  - Commit metadata beyond what git stores (embeddings, diff stats, PR linkage)
  - Labels, milestones, webhooks
  - API tokens with SHA-256 hashed storage and scoped permissions
  - SSH keys per user
  - Embedding configuration table (provider-agnostic model tracking)
  - Full-text search indexes (tsvector) on all text fields
  - Trigram indexes (pg_trgm) for fuzzy username search
  - PostgreSQL extensions: uuid-ossp, vector, pg_trgm

### Frontend (React + TypeScript)

- Vite-based React 18 app with TypeScript strict mode
- Dark-mode native UI (no CSS framework, custom properties throughout)
- TanStack Query for all server state management
- Zustand store for auth state with automatic session restoration on load
- React Router with auth-guarded routes

**Pages:**
- Login and registration forms with error handling
- Dashboard showing user's repositories with "New Repository" button
- New repository creation form (name, description, visibility, auto-init)
- Repository view with:
  - Header (owner/name, description, visibility badge)
  - Code/Commits tab navigation
  - Branch selector dropdown
  - File tree table (directories first, navigable)
  - File content viewer (monospace `<pre>` display)
  - Breadcrumb path navigation
  - Paginated commit list (message, author, date, short SHA)

**App shell:**
- Sticky navigation bar with Gitwise branding
- Authenticated user menu with sign-out
- Unauthenticated sign-in / sign-up links

### DevOps

- Multi-stage Dockerfile (Node build → Go build → Alpine runtime)
- Docker Compose with pgvector/pgvector:pg16 + redis:7-alpine
- Makefile targets: build, run, dev, test, frontend, docker-up/down, clean
- `.env.example` with all configuration variables documented
- Vite dev proxy for `/api` and git protocol requests to backend
- `.gitignore` covering Go, Node, IDE, OS, and environment files

### API Design

All endpoints follow the `{data, meta, errors}` JSON envelope convention.
Cursor-based pagination on list endpoints. Consistent error codes across
all error responses.

| Endpoint | Method | Description |
|---|---|---|
| `/healthz` | GET | Health check |
| `/api/v1/auth/register` | POST | Create account |
| `/api/v1/auth/login` | POST | Login |
| `/api/v1/auth/logout` | POST | Logout |
| `/api/v1/auth/me` | GET | Current user |
| `/api/v1/auth/tokens` | POST/GET | Create/list API tokens |
| `/api/v1/auth/tokens/:id` | DELETE | Revoke token |
| `/api/v1/user/repos` | GET | Authenticated user's repos |
| `/api/v1/repos` | POST | Create repository |
| `/api/v1/repos/:owner/:repo` | GET/PATCH/DELETE | Repository CRUD |
| `/api/v1/repos/:owner/:repo/tree/:ref/*` | GET | File tree |
| `/api/v1/repos/:owner/:repo/blob/:ref/*` | GET | File content |
| `/api/v1/repos/:owner/:repo/raw/:ref/*` | GET | Raw file download |
| `/api/v1/repos/:owner/:repo/commits` | GET | Commit log |
| `/api/v1/repos/:owner/:repo/commits/:sha` | GET | Commit detail + diff |
| `/api/v1/repos/:owner/:repo/branches` | GET | Branch list |
| `/api/v1/users/:username` | GET | User profile |
| `/api/v1/users/:username/repos` | GET | User's repos |
| `/:owner/:repo.git/*` | * | Git smart HTTP protocol |

### Stub Endpoints (Phase 2)

The following endpoints are registered but return 501 Not Implemented:
- Pull request CRUD and merge
- Issue CRUD
- Unified search
