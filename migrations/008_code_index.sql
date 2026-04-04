-- Code file index for code search (pg_trgm fuzzy matching)

CREATE TABLE code_files (
    id       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id  UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    ref      VARCHAR(255) NOT NULL DEFAULT 'HEAD',
    path     TEXT NOT NULL,
    content  TEXT NOT NULL,
    language VARCHAR(100) NOT NULL DEFAULT '',
    indexed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repo_id, ref, path)
);

CREATE INDEX idx_code_files_repo ON code_files (repo_id);
CREATE INDEX idx_code_files_content_trgm ON code_files USING gin (content gin_trgm_ops);
CREATE INDEX idx_code_files_path_trgm ON code_files USING gin (path gin_trgm_ops);
CREATE INDEX idx_code_files_language ON code_files (language);
