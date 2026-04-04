# Gitwise — Claude Code Instructions

## What This Is

Gitwise is an AI-native self-hosted code collaboration platform (Git hosting, PRs, issues, code review). See `SPEC.md` for the full product specification.

## Stack

| Layer | Tech |
|-------|------|
| Backend | Go (stdlib + chi router) |
| Database | PostgreSQL 16+ (pgvector, tsvector, pg_trgm) |
| Cache/Queue | Redis 7+ |
| Frontend | React 18 + TypeScript (Vite, TanStack Query, Zustand) |
| Git ops | go-git + shell to git CLI |
| Real-time | WebSocket (gorilla/websocket) |

## Project Layout

```
cmd/gitwise/          — Main binary entrypoint
internal/
  config/             — Environment-based configuration
  server/             — HTTP server, middleware, route setup
  database/           — PostgreSQL connection pool
  models/             — Data types / domain structs
  services/           — Business logic (repo, user, pull, issue, review, search)
  api/handlers/       — HTTP handler functions
  api/routes/         — Route registration
  middleware/         — Auth, logging, etc.
  git/                — Git backend operations
migrations/           — SQL migration files (numbered, applied in order)
web/                  — React frontend (Vite)
scripts/              — Build/deploy helpers
docs/                 — Architecture documentation
```

## Conventions

- **Go style:** Follow standard Go conventions. Use `slog` for logging. Error wrapping with `fmt.Errorf("context: %w", err)`.
- **API responses:** JSON envelope `{data, meta, errors}` on all endpoints.
- **Database:** Use pgx directly (no ORM). Migrations are numbered SQL files.
- **Frontend:** Functional components only. TanStack Query for server state. Zustand for client state. React Router for routing.
- **Naming:** Go files `snake_case.go`, React components `PascalCase.tsx`, CSS modules `Component.module.css`.
- **Tests:** Table-driven tests in Go. Vitest for frontend.

## Running

```bash
# Start dependencies
docker compose up -d postgres redis

# Run backend
go run ./cmd/gitwise

# Run frontend dev server
cd web && npm run dev

# Run all tests
make test
```

## Key Design Decisions

1. **PostgreSQL for everything** — relational data, FTS (tsvector), fuzzy search (pg_trgm), vector similarity (pgvector). No Elasticsearch.
2. **Embedding pipeline is optional** — can be disabled entirely. Semantic search falls back to keyword-only.
3. **Issues and PRs share a number sequence per repo** — like GitHub. Use `next_repo_number()` SQL function.
4. **Single binary deployment** — frontend embedded via `go:embed` in production builds.
