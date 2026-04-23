-- Migration 020: embedding_failures table
--
-- Tracks rows where embedding generation has permanently failed after all
-- retries, so a background worker can surface and re-attempt them later.

BEGIN;

CREATE TABLE IF NOT EXISTS embedding_failures (
    id         UUID        NOT NULL,          -- source row ID (issue, PR, comment, etc.)
    table_name VARCHAR(100) NOT NULL,         -- table the source row lives in
    reason     TEXT        NOT NULL DEFAULT '',
    failed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, table_name)
);

CREATE INDEX IF NOT EXISTS idx_embedding_failures_failed_at
    ON embedding_failures (failed_at);

COMMIT;
