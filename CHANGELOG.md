# Changelog

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
