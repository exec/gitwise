-- Activity event tracking for repo and user feeds
CREATE TABLE activity_events (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id    UUID REFERENCES repositories(id) ON DELETE CASCADE,
    actor_id   UUID NOT NULL REFERENCES users(id),
    event_type VARCHAR(50) NOT NULL,
    ref_type   VARCHAR(50) NOT NULL DEFAULT '',
    ref_id     UUID,
    ref_number INTEGER,
    payload    JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_activity_repo ON activity_events (repo_id, created_at DESC);
CREATE INDEX idx_activity_actor ON activity_events (actor_id, created_at DESC);
CREATE INDEX idx_activity_type ON activity_events (event_type);
