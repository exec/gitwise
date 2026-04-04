-- Gitwise initial schema
-- Requires: PostgreSQL 16+, pgvector, pg_trgm

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "vector";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ============================================================
-- Users
-- ============================================================
CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username    VARCHAR(255) NOT NULL UNIQUE,
    email       VARCHAR(255) NOT NULL UNIQUE,
    password    TEXT,                          -- argon2id hash; NULL for OAuth-only users
    full_name   VARCHAR(255) NOT NULL DEFAULT '',
    avatar_url  TEXT NOT NULL DEFAULT '',
    bio         TEXT NOT NULL DEFAULT '',
    is_admin    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_username_trgm ON users USING gin (username gin_trgm_ops);
CREATE INDEX idx_users_email ON users (email);

-- ============================================================
-- Organizations
-- ============================================================
CREATE TABLE organizations (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(255) NOT NULL UNIQUE,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    avatar_url  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE org_members (
    org_id  UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role    VARCHAR(50) NOT NULL DEFAULT 'member',  -- owner, member
    PRIMARY KEY (org_id, user_id)
);

-- ============================================================
-- Repositories
-- ============================================================
CREATE TABLE repositories (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id                UUID NOT NULL,    -- FK to users or organizations
    owner_type              VARCHAR(20) NOT NULL DEFAULT 'user',  -- user, org
    name                    VARCHAR(255) NOT NULL,
    description             TEXT NOT NULL DEFAULT '',
    description_embedding   VECTOR(1536),
    default_branch          VARCHAR(255) NOT NULL DEFAULT 'main',
    visibility              VARCHAR(20) NOT NULL DEFAULT 'public',  -- public, private, internal
    language_stats          JSONB NOT NULL DEFAULT '{}',
    topics                  TEXT[] NOT NULL DEFAULT '{}',
    metadata                JSONB NOT NULL DEFAULT '{}',
    fork_of                 UUID REFERENCES repositories(id) ON DELETE SET NULL,
    stars_count             INTEGER NOT NULL DEFAULT 0,
    forks_count             INTEGER NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_id, name)
);

CREATE INDEX idx_repos_owner ON repositories (owner_id, owner_type);
CREATE INDEX idx_repos_visibility ON repositories (visibility);
CREATE INDEX idx_repos_topics ON repositories USING gin (topics);
CREATE INDEX idx_repos_description_fts ON repositories USING gin (to_tsvector('english', description));

-- ============================================================
-- Repository collaborators
-- ============================================================
CREATE TABLE repo_collaborators (
    repo_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role    VARCHAR(50) NOT NULL DEFAULT 'read',  -- admin, write, triage, read
    PRIMARY KEY (repo_id, user_id)
);

-- ============================================================
-- SSH keys
-- ============================================================
CREATE TABLE ssh_keys (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    fingerprint VARCHAR(255) NOT NULL UNIQUE,
    public_key  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ssh_keys_user ON ssh_keys (user_id);

-- ============================================================
-- API tokens
-- ============================================================
CREATE TABLE api_tokens (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    token_hash  TEXT NOT NULL UNIQUE,
    scopes      TEXT[] NOT NULL DEFAULT '{}',
    expires_at  TIMESTAMPTZ,
    last_used   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_api_tokens_hash ON api_tokens (token_hash);

-- ============================================================
-- Issues
-- ============================================================
CREATE TABLE issues (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id             UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    number              INTEGER NOT NULL,
    author_id           UUID NOT NULL REFERENCES users(id),
    title               VARCHAR(500) NOT NULL,
    title_embedding     VECTOR(1536),
    body                TEXT NOT NULL DEFAULT '',
    body_embedding      VECTOR(1536),
    body_history        JSONB NOT NULL DEFAULT '[]',
    status              VARCHAR(20) NOT NULL DEFAULT 'open',  -- open, closed, duplicate
    labels              TEXT[] NOT NULL DEFAULT '{}',
    assignees           UUID[] NOT NULL DEFAULT '{}',
    milestone_id        UUID,
    linked_prs          UUID[] NOT NULL DEFAULT '{}',
    priority            VARCHAR(20) NOT NULL DEFAULT 'none',  -- critical, high, medium, low, none
    metadata            JSONB NOT NULL DEFAULT '{}',
    closed_at           TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repo_id, number)
);

CREATE INDEX idx_issues_repo_status ON issues (repo_id, status);
CREATE INDEX idx_issues_author ON issues (author_id);
CREATE INDEX idx_issues_labels ON issues USING gin (labels);
CREATE INDEX idx_issues_title_fts ON issues USING gin (to_tsvector('english', title));
CREATE INDEX idx_issues_body_fts ON issues USING gin (to_tsvector('english', body));

-- ============================================================
-- Pull Requests
-- ============================================================
CREATE TABLE pull_requests (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id             UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    number              INTEGER NOT NULL,
    author_id           UUID NOT NULL REFERENCES users(id),
    title               VARCHAR(500) NOT NULL,
    title_embedding     VECTOR(1536),
    body                TEXT NOT NULL DEFAULT '',
    body_embedding      VECTOR(1536),
    body_history        JSONB NOT NULL DEFAULT '[]',
    source_branch       VARCHAR(255) NOT NULL,
    target_branch       VARCHAR(255) NOT NULL,
    status              VARCHAR(20) NOT NULL DEFAULT 'open',  -- draft, open, merged, closed
    intent              JSONB NOT NULL DEFAULT '{}',
    diff_stats          JSONB NOT NULL DEFAULT '{}',
    review_summary      JSONB NOT NULL DEFAULT '{}',
    merge_strategy      VARCHAR(20),  -- merge, squash, rebase
    merged_by           UUID REFERENCES users(id),
    merged_at           TIMESTAMPTZ,
    closed_at           TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repo_id, number)
);

CREATE INDEX idx_prs_repo_status ON pull_requests (repo_id, status);
CREATE INDEX idx_prs_author ON pull_requests (author_id);
CREATE INDEX idx_prs_title_fts ON pull_requests USING gin (to_tsvector('english', title));
CREATE INDEX idx_prs_body_fts ON pull_requests USING gin (to_tsvector('english', body));

-- ============================================================
-- Reviews
-- ============================================================
CREATE TABLE reviews (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pr_id           UUID NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    author_id       UUID NOT NULL REFERENCES users(id),
    type            VARCHAR(30) NOT NULL DEFAULT 'comment',  -- approval, changes_requested, comment, dismissal
    body            TEXT NOT NULL DEFAULT '',
    body_embedding  VECTOR(1536),
    comments        JSONB NOT NULL DEFAULT '[]',
    submitted_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_reviews_pr ON reviews (pr_id);
CREATE INDEX idx_reviews_author ON reviews (author_id);

-- ============================================================
-- Issue / PR comments (timeline events)
-- ============================================================
CREATE TABLE comments (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id         UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    issue_id        UUID REFERENCES issues(id) ON DELETE CASCADE,
    pr_id           UUID REFERENCES pull_requests(id) ON DELETE CASCADE,
    author_id       UUID NOT NULL REFERENCES users(id),
    body            TEXT NOT NULL,
    body_embedding  VECTOR(1536),
    body_history    JSONB NOT NULL DEFAULT '[]',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (issue_id IS NOT NULL OR pr_id IS NOT NULL)
);

CREATE INDEX idx_comments_issue ON comments (issue_id) WHERE issue_id IS NOT NULL;
CREATE INDEX idx_comments_pr ON comments (pr_id) WHERE pr_id IS NOT NULL;

-- ============================================================
-- Commit metadata (indexed beyond what git stores)
-- ============================================================
CREATE TABLE commit_metadata (
    sha                 CHAR(40) NOT NULL,
    repo_id             UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    message             TEXT NOT NULL,
    message_embedding   VECTOR(1536),
    author_email        VARCHAR(255) NOT NULL,
    author_id           UUID REFERENCES users(id),
    diff_stats          JSONB NOT NULL DEFAULT '{}',
    parent_shas         TEXT[] NOT NULL DEFAULT '{}',
    pr_id               UUID REFERENCES pull_requests(id) ON DELETE SET NULL,
    committed_at        TIMESTAMPTZ NOT NULL,
    indexed_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (repo_id, sha)
);

CREATE INDEX idx_commits_repo_date ON commit_metadata (repo_id, committed_at DESC);
CREATE INDEX idx_commits_author ON commit_metadata (author_id) WHERE author_id IS NOT NULL;
CREATE INDEX idx_commits_message_fts ON commit_metadata USING gin (to_tsvector('english', message));

-- ============================================================
-- Labels (per-repo)
-- ============================================================
CREATE TABLE labels (
    id      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    name    VARCHAR(255) NOT NULL,
    color   CHAR(7) NOT NULL DEFAULT '#888888',  -- hex color
    description TEXT NOT NULL DEFAULT '',
    UNIQUE (repo_id, name)
);

-- ============================================================
-- Milestones
-- ============================================================
CREATE TABLE milestones (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id     UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    title       VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    due_date    TIMESTAMPTZ,
    status      VARCHAR(20) NOT NULL DEFAULT 'open',  -- open, closed
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repo_id, title)
);

-- ============================================================
-- Webhooks
-- ============================================================
CREATE TABLE webhooks (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id     UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    url         TEXT NOT NULL,
    secret      TEXT NOT NULL DEFAULT '',
    events      TEXT[] NOT NULL DEFAULT '{}',
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- Embedding metadata (track model versions)
-- ============================================================
CREATE TABLE embedding_config (
    id              SERIAL PRIMARY KEY,
    provider        VARCHAR(50) NOT NULL,
    model           VARCHAR(100) NOT NULL,
    dimensions      INTEGER NOT NULL,
    model_version   VARCHAR(100) NOT NULL,
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- Sequence counters for repo-scoped numbers (issues + PRs share a sequence)
-- ============================================================
CREATE TABLE repo_counters (
    repo_id     UUID PRIMARY KEY REFERENCES repositories(id) ON DELETE CASCADE,
    next_number INTEGER NOT NULL DEFAULT 1
);

-- ============================================================
-- Helper function: increment and return the next issue/PR number for a repo
-- ============================================================
CREATE OR REPLACE FUNCTION next_repo_number(p_repo_id UUID) RETURNS INTEGER AS $$
DECLARE
    v_number INTEGER;
BEGIN
    INSERT INTO repo_counters (repo_id, next_number)
    VALUES (p_repo_id, 2)
    ON CONFLICT (repo_id) DO UPDATE SET next_number = repo_counters.next_number + 1
    RETURNING next_number - 1 INTO v_number;
    RETURN v_number;
END;
$$ LANGUAGE plpgsql;
