-- User profile enhancements: pinned repos
CREATE TABLE pinned_repos (
    user_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repo_id  UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    position INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, repo_id)
);
CREATE INDEX idx_pinned_repos_user ON pinned_repos (user_id, position);
