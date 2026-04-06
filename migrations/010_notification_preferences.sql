-- Notification preferences: per-user toggle for each notification type
CREATE TABLE notification_preferences (
    user_id         UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    pr_review       BOOLEAN NOT NULL DEFAULT TRUE,
    pr_merged       BOOLEAN NOT NULL DEFAULT TRUE,
    pr_comment      BOOLEAN NOT NULL DEFAULT TRUE,
    issue_comment   BOOLEAN NOT NULL DEFAULT TRUE,
    mention         BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Repository watchers: users subscribe to all activity on a repo
CREATE TABLE repo_watchers (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repo_id    UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, repo_id)
);

CREATE INDEX idx_repo_watchers_repo ON repo_watchers (repo_id);
