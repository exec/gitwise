CREATE TABLE branch_protection_rules (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id          UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    branch_pattern   VARCHAR(255) NOT NULL,
    required_reviews INTEGER NOT NULL DEFAULT 0,
    require_linear   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repo_id, branch_pattern)
);

CREATE INDEX idx_branch_protection_repo ON branch_protection_rules (repo_id);
