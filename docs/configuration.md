# Configuration Reference

All Gitwise configuration is done via environment variables. No configuration files are used.

## Server

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `GITWISE_PORT` | HTTP server port | `3000` | No | `8080` |
| `GITWISE_HOST` | HTTP server bind address | `0.0.0.0` | No | `127.0.0.1` |
| `GITWISE_SECRET` | Secret key for session signing. **Must be changed in production.** | `change-me-in-production` | Yes | `a1b2c3d4e5f6...` |
| `GITWISE_BASE_URL` | Public URL of the Gitwise instance. Used for OAuth callbacks and generated URLs. | `http://localhost:3000` | Yes | `https://git.example.com` |
| `GITWISE_SSH_PORT` | SSH server port for git-over-SSH | `2222` | No | `22` |

## Database (PostgreSQL)

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `GITWISE_DB_HOST` | PostgreSQL hostname | `localhost` | No | `postgres` |
| `GITWISE_DB_PORT` | PostgreSQL port | `5432` | No | `5432` |
| `GITWISE_DB_USER` | PostgreSQL username | `gitwise` | No | `gitwise` |
| `GITWISE_DB_PASSWORD` | PostgreSQL password | `gitwise` | Yes | `s3cur3-p@ssw0rd` |
| `GITWISE_DB_NAME` | PostgreSQL database name | `gitwise` | No | `gitwise` |
| `GITWISE_DB_SSLMODE` | PostgreSQL SSL mode | `disable` | No | `require` |

The DSN is constructed as: `postgres://USER:PASSWORD@HOST:PORT/NAME?sslmode=SSLMODE`

## Redis

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `GITWISE_REDIS_HOST` | Redis hostname | `localhost` | No | `redis` |
| `GITWISE_REDIS_PORT` | Redis port | `6379` | No | `6379` |
| `GITWISE_REDIS_PASSWORD` | Redis password (if AUTH is enabled) | _(empty)_ | No | `redis-secret` |
| `GITWISE_REDIS_DB` | Redis database number | `0` | No | `1` |

## Git

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `GITWISE_REPOS_PATH` | Filesystem path where bare git repositories are stored | `./data/repos` | No | `/data/repos` |

## GitHub OAuth

Setting both `GITWISE_GITHUB_CLIENT_ID` and `GITWISE_GITHUB_CLIENT_SECRET` enables "Sign in with GitHub". Leave both empty to disable.

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `GITWISE_GITHUB_CLIENT_ID` | GitHub OAuth App client ID | _(empty)_ | No | `Iv1.abc123def456` |
| `GITWISE_GITHUB_CLIENT_SECRET` | GitHub OAuth App client secret | _(empty)_ | No | `abcdef0123456789` |

To create a GitHub OAuth App:
1. Go to GitHub Settings > Developer settings > OAuth Apps > New OAuth App
2. Set the Authorization callback URL to: `{GITWISE_BASE_URL}/api/v1/auth/github/callback`
3. Copy the Client ID and Client Secret

## Two-Factor Authentication (TOTP)

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `GITWISE_TOTP_KEY` | Hex-encoded 32-byte AES-256 key for encrypting TOTP secrets at rest. If empty, 2FA is disabled server-wide. | _(empty)_ | No | `0123456789abcdef...` (64 hex chars) |

Generate a key with:
```bash
openssl rand -hex 32
```

## Embedding / Semantic Search

The embedding pipeline is optional. When disabled, search falls back to PostgreSQL full-text search only.

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `GITWISE_EMBEDDING_PROVIDER` | Embedding provider: `openai`, `ollama`, or `none`/empty to disable | _(empty)_ | No | `openai` |
| `GITWISE_EMBEDDING_API_KEY` | API key for the embedding provider (required for `openai`) | _(empty)_ | Conditional | `sk-abc123...` |
| `GITWISE_EMBEDDING_MODEL` | Model name for OpenAI embeddings | `text-embedding-3-small` | No | `text-embedding-3-large` |
| `GITWISE_EMBEDDING_DIMENSIONS` | Vector dimensions for embeddings | `1536` | No | `768` |
| `GITWISE_EMBEDDING_WORKER_INTERVAL` | How often the background embedding worker runs | `5m` | No | `10m` |
| `GITWISE_OLLAMA_URL` | Ollama server URL (for `ollama` provider) | `http://localhost:11434` | No | `http://ollama:11434` |
| `GITWISE_OLLAMA_MODEL` | Ollama model name | `nomic-embed-text` | No | `mxbai-embed-large` |

Legacy aliases (without `GITWISE_` prefix) are also supported: `EMBEDDING_PROVIDER`, `EMBEDDING_API_KEY`, `EMBEDDING_MODEL`, `EMBEDDING_DIMENSIONS`, `EMBEDDING_WORKER_INTERVAL`. The `GITWISE_`-prefixed versions take precedence.

## Frontend

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `GITWISE_FRONTEND_DIST` | Path to the built frontend assets directory | `./web/dist` | No | `/app/web/dist` |

In Docker deployments, the Dockerfile copies the built frontend to `/app/web/dist`.

## Example `.env` File

```bash
# ── Required for production ──────────────────────────────────────────
GITWISE_SECRET=change-this-to-a-long-random-string
GITWISE_DB_PASSWORD=strong-database-password
GITWISE_BASE_URL=https://git.example.com

# ── Optional: GitHub OAuth ───────────────────────────────────────────
# GITWISE_GITHUB_CLIENT_ID=
# GITWISE_GITHUB_CLIENT_SECRET=

# ── Optional: 2FA ────────────────────────────────────────────────────
# GITWISE_TOTP_KEY=

# ── Optional: Semantic search ────────────────────────────────────────
# GITWISE_EMBEDDING_PROVIDER=openai
# GITWISE_EMBEDDING_API_KEY=sk-...

# ── Rarely changed ───────────────────────────────────────────────────
# GITWISE_PORT=3000
# GITWISE_HOST=0.0.0.0
# GITWISE_SSH_PORT=2222
# GITWISE_DB_HOST=localhost
# GITWISE_DB_PORT=5432
# GITWISE_DB_USER=gitwise
# GITWISE_DB_NAME=gitwise
# GITWISE_DB_SSLMODE=disable
# GITWISE_REDIS_HOST=localhost
# GITWISE_REDIS_PORT=6379
# GITWISE_REPOS_PATH=./data/repos
# GITWISE_FRONTEND_DIST=./web/dist
```
