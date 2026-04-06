-- OAuth account linking table
CREATE TABLE oauth_accounts (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider     VARCHAR(50) NOT NULL,   -- 'github'
    provider_id  VARCHAR(255) NOT NULL,  -- GitHub user ID (numeric string)
    access_token TEXT,                   -- OAuth access token
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_id)
);

CREATE INDEX idx_oauth_accounts_user ON oauth_accounts (user_id);
