# Self-Hosting Guide

This guide covers deploying Gitwise on your own infrastructure. Gitwise is a single Go binary that serves both the API and the React frontend, backed by PostgreSQL and Redis.

## Requirements

| Component | Minimum Version | Notes |
|-----------|----------------|-------|
| Docker + Docker Compose | 24.x / 2.20+ | Recommended deployment method |
| PostgreSQL | 16+ | With pgvector, pg_trgm extensions |
| Redis | 7+ | Used for sessions and caching |
| Go | 1.24+ | Only needed for building from source |
| Node.js | 22+ | Only needed for building from source |
| Git | 2.x | Required at runtime for git operations |

The PostgreSQL image used in the provided Docker Compose files is `pgvector/pgvector:pg16`, which includes the `pgvector` extension pre-installed. The `pg_trgm` extension ships with PostgreSQL by default.

## Quick Start with Docker Compose

### 1. Clone the repository

```bash
git clone https://github.com/gitwise-io/gitwise.git
cd gitwise
```

### 2. Create an environment file

```bash
cp .env.example .env  # or create one manually
```

At minimum, set these values in your `.env` file:

```bash
# REQUIRED: Change these for production
GITWISE_SECRET=your-random-secret-string-at-least-32-chars
GITWISE_DB_PASSWORD=a-strong-database-password
GITWISE_BASE_URL=https://git.example.com

# OPTIONAL: GitHub OAuth (enables "Sign in with GitHub")
GITWISE_GITHUB_CLIENT_ID=your-github-oauth-app-client-id
GITWISE_GITHUB_CLIENT_SECRET=your-github-oauth-app-client-secret

# OPTIONAL: 2FA support (hex-encoded 32-byte AES-256 key)
# Generate with: openssl rand -hex 32
GITWISE_TOTP_KEY=your-64-char-hex-string
```

### 3. Start the services

For production with Caddy (automatic HTTPS):

```bash
docker compose -f docker-compose.prod.yml up -d
```

For local development:

```bash
docker compose up -d
```

### 4. Verify

```bash
curl http://localhost:3000/healthz
# {"data":{"status":"ok"}}
```

The production compose file includes a Caddy reverse proxy that handles TLS automatically. See the [Reverse Proxy](#reverse-proxy-setup) section for details.

## Docker Compose Architecture

The production `docker-compose.prod.yml` runs four services:

| Service | Image | Purpose |
|---------|-------|---------|
| `gitwise` | Built from `Dockerfile` | Application server (HTTP :3000, SSH :22) |
| `postgres` | `pgvector/pgvector:pg16` | Database |
| `redis` | `redis:7-alpine` | Session store and cache |
| `caddy` | `caddy:2-alpine` | Reverse proxy with automatic HTTPS |

Data is persisted in named Docker volumes: `pg_data`, `redis_data`, `repo_data`, `caddy_data`, `caddy_config`.

## Configuration Reference

All configuration is done via environment variables. See [docs/configuration.md](configuration.md) for the complete reference table.

Key variable groups:

- **Server**: `GITWISE_PORT`, `GITWISE_HOST`, `GITWISE_SECRET`, `GITWISE_BASE_URL`
- **Database**: `GITWISE_DB_HOST`, `GITWISE_DB_PORT`, `GITWISE_DB_USER`, `GITWISE_DB_PASSWORD`, `GITWISE_DB_NAME`, `GITWISE_DB_SSLMODE`
- **Redis**: `GITWISE_REDIS_HOST`, `GITWISE_REDIS_PORT`, `GITWISE_REDIS_PASSWORD`, `GITWISE_REDIS_DB`
- **Git**: `GITWISE_REPOS_PATH`, `GITWISE_SSH_PORT`
- **OAuth**: `GITWISE_GITHUB_CLIENT_ID`, `GITWISE_GITHUB_CLIENT_SECRET`
- **2FA**: `GITWISE_TOTP_KEY`
- **Embedding**: `GITWISE_EMBEDDING_PROVIDER`, `GITWISE_EMBEDDING_API_KEY`, etc.
- **Frontend**: `GITWISE_FRONTEND_DIST`

## Reverse Proxy Setup

### Caddy (recommended)

The repository includes a `Caddyfile` used by the production Docker Compose:

```
git.example.com {
    reverse_proxy gitwise:3000

    request_body {
        max_size 1GB
    }
}
```

Caddy automatically provisions TLS certificates from Let's Encrypt. Replace `git.example.com` with your domain. The 1GB body limit is needed to support large git pushes.

If you are running Caddy outside Docker, point it to `localhost:3000`:

```
git.example.com {
    reverse_proxy localhost:3000

    request_body {
        max_size 1GB
    }
}
```

### nginx

```nginx
server {
    listen 80;
    server_name git.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name git.example.com;

    ssl_certificate     /etc/letsencrypt/live/git.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/git.example.com/privkey.pem;

    client_max_body_size 1G;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket support
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400;
    }
}
```

Make sure to set `client_max_body_size` to at least 1G to support large git pushes. The WebSocket headers are required for real-time notifications.

## SSL/TLS

### With Caddy (automatic)

Caddy handles certificate provisioning and renewal automatically via Let's Encrypt. Just set your domain in the Caddyfile and ensure ports 80 and 443 are accessible from the internet.

### With nginx + certbot

```bash
# Install certbot
sudo apt install certbot python3-certbot-nginx

# Obtain certificate
sudo certbot --nginx -d git.example.com

# Auto-renewal is configured automatically
sudo systemctl enable certbot.timer
```

## Git SSH Access

Gitwise includes a built-in SSH server for git-over-SSH access. By default it listens on port 2222. In the production Docker Compose, it is mapped to port 22 on the host.

Users authenticate by uploading their SSH public keys via the API (`POST /api/v1/user/ssh-keys`) or the web UI.

Clone URLs follow the pattern:
```
ssh://git@git.example.com:22/owner/repo.git
```

If you run the SSH server on a non-standard port, users must specify it:
```
ssh://git@git.example.com:2222/owner/repo.git
```

## Semantic Search (optional)

Gitwise supports semantic code search using vector embeddings. This is entirely optional -- when disabled, search falls back to keyword-only (PostgreSQL full-text search).

### OpenAI embeddings

```bash
GITWISE_EMBEDDING_PROVIDER=openai
GITWISE_EMBEDDING_API_KEY=sk-your-openai-api-key
GITWISE_EMBEDDING_MODEL=text-embedding-3-small
GITWISE_EMBEDDING_DIMENSIONS=1536
```

### Ollama (local, no API key needed)

```bash
GITWISE_EMBEDDING_PROVIDER=ollama
GITWISE_OLLAMA_URL=http://localhost:11434
GITWISE_OLLAMA_MODEL=nomic-embed-text
GITWISE_EMBEDDING_DIMENSIONS=768
```

### Disabled (default)

```bash
GITWISE_EMBEDDING_PROVIDER=none
```

## Database Setup

### Migrations

Migrations run automatically on startup. They are embedded in the binary from the `migrations/` directory.

### Manual PostgreSQL setup (without Docker)

If you prefer to manage PostgreSQL yourself:

```bash
# Create database and user
sudo -u postgres createuser gitwise
sudo -u postgres createdb -O gitwise gitwise

# Enable required extensions (connect to the gitwise database)
psql -U postgres -d gitwise -c "CREATE EXTENSION IF NOT EXISTS pgvector;"
psql -U postgres -d gitwise -c "CREATE EXTENSION IF NOT EXISTS pg_trgm;"
```

## Backup and Restore

### Database

```bash
# Backup
docker compose exec postgres pg_dump -U gitwise gitwise > backup.sql

# Or with compression
docker compose exec postgres pg_dump -U gitwise -Fc gitwise > backup.dump

# Restore
docker compose exec -T postgres psql -U gitwise gitwise < backup.sql

# Or from compressed backup
docker compose exec -T postgres pg_restore -U gitwise -d gitwise < backup.dump
```

### Git repositories

Git repositories are stored in the `repo_data` volume (or the path specified by `GITWISE_REPOS_PATH`).

```bash
# Backup
docker run --rm -v gitwise_repo_data:/data -v $(pwd):/backup alpine \
    tar czf /backup/repos-backup.tar.gz -C /data .

# Restore
docker run --rm -v gitwise_repo_data:/data -v $(pwd):/backup alpine \
    sh -c "cd /data && tar xzf /backup/repos-backup.tar.gz"
```

### Full backup script

```bash
#!/bin/bash
set -e
BACKUP_DIR="./backups/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"

# Database
docker compose exec -T postgres pg_dump -U gitwise -Fc gitwise > "$BACKUP_DIR/db.dump"

# Git repos
docker run --rm -v gitwise_repo_data:/data -v "$(pwd)/$BACKUP_DIR":/backup alpine \
    tar czf /backup/repos.tar.gz -C /data .

echo "Backup saved to $BACKUP_DIR"
```

## Updating

### Docker Compose deployment

```bash
# Pull latest code
git pull

# Rebuild and restart
docker compose -f docker-compose.prod.yml build
docker compose -f docker-compose.prod.yml up -d
```

Migrations run automatically on startup, so database schema changes are applied during the restart.

### GitHub Actions deployment

If you have set up CI/CD with GitHub Actions (e.g., the `deploy.yml` workflow), pushing to the main branch or triggering the manual deploy workflow will build and deploy automatically.

## Building from Source

```bash
# Build frontend
cd web && npm ci && npm run build && cd ..

# Build backend (embeds frontend assets)
CGO_ENABLED=0 go build -o gitwise ./cmd/gitwise

# Run
./gitwise
```

The binary serves the frontend from `./web/dist` by default (configurable via `GITWISE_FRONTEND_DIST`).

## Troubleshooting

### Port conflicts

**Symptom**: `bind: address already in use`

Check what is using the port:
```bash
sudo lsof -i :3000  # HTTP
sudo lsof -i :2222  # SSH
```

Change the port via `GITWISE_PORT` or `GITWISE_SSH_PORT`, or stop the conflicting service.

### Database connection failures

**Symptom**: `failed to connect to database` on startup

1. Verify PostgreSQL is running: `docker compose ps postgres`
2. Check the health status: `docker compose exec postgres pg_isready -U gitwise`
3. Verify credentials match between `GITWISE_DB_*` vars and PostgreSQL config
4. If using `GITWISE_DB_SSLMODE=require`, ensure your PostgreSQL has SSL configured

### Migrations fail

**Symptom**: `migration error` in logs

1. Check that the `pgvector` extension is installed: `psql -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';"`
2. Check that `pg_trgm` is available: `psql -c "SELECT extversion FROM pg_extension WHERE extname = 'pg_trgm';"`
3. If using the `pgvector/pgvector:pg16` Docker image, both extensions are available by default

### Frontend not loading

**Symptom**: `{"errors":[{"code":"no_frontend","message":"frontend not built"}]}`

The frontend assets are not found at the expected path. Either:
- Build the frontend: `cd web && npm ci && npm run build`
- Set `GITWISE_FRONTEND_DIST` to point to your built assets
- If using Docker, the Dockerfile builds the frontend automatically

### SSH connection refused

**Symptom**: `Connection refused` when trying to clone via SSH

1. Verify the SSH server is running on the expected port
2. Check that port 22 (or your configured SSH port) is exposed in Docker
3. Upload your SSH public key via the web UI or API before trying to connect
4. Test with: `ssh -T -p 2222 git@git.example.com`

### Redis connection failures

**Symptom**: `failed to connect to Redis` or sessions not persisting

1. Verify Redis is running: `docker compose ps redis`
2. Check health: `docker compose exec redis redis-cli ping`
3. If using Redis AUTH, set `GITWISE_REDIS_PASSWORD`

### Large git pushes fail

**Symptom**: `413 Request Entity Too Large` or push hangs

Ensure your reverse proxy allows large request bodies:
- Caddy: `request_body { max_size 1GB }` (included in the provided Caddyfile)
- nginx: `client_max_body_size 1G;`
- The Gitwise server itself allows up to 1GB request bodies
