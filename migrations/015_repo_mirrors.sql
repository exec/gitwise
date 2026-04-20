-- Migration 015: Repository mirrors.
-- One-mirror-per-repo config table plus append-only run history.

CREATE TABLE repo_mirrors (
    repo_id           UUID PRIMARY KEY REFERENCES repositories(id) ON DELETE CASCADE,
    direction         TEXT NOT NULL CHECK (direction IN ('push','pull')),
    github_owner      TEXT NOT NULL,
    github_repo       TEXT NOT NULL,
    pat_ciphertext    BYTEA,
    pat_nonce         BYTEA,
    interval_seconds  INTEGER NOT NULL DEFAULT 900,
    auto_push         BOOLEAN NOT NULL DEFAULT TRUE,
    last_status       TEXT NOT NULL DEFAULT 'pending'
                      CHECK (last_status IN ('pending','running','success','failed')),
    last_error        TEXT,
    last_synced_at    TIMESTAMPTZ,
    next_run_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_repo_mirrors_next_run ON repo_mirrors(next_run_at)
    WHERE interval_seconds > 0 AND last_status <> 'running';

CREATE TABLE repo_mirror_runs (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id       UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ,
    status        TEXT NOT NULL CHECK (status IN ('running','success','failed')),
    trigger       TEXT NOT NULL CHECK (trigger IN ('manual','scheduled','push_event','initial_clone')),
    refs_changed  INTEGER,
    error         TEXT,
    duration_ms   INTEGER
);

CREATE INDEX idx_repo_mirror_runs_repo_started ON repo_mirror_runs(repo_id, started_at DESC);
