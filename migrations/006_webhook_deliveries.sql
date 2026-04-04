-- Webhook delivery tracking
CREATE TABLE webhook_deliveries (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    webhook_id      UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_type      VARCHAR(100) NOT NULL,
    payload         JSONB NOT NULL,
    request_headers JSONB NOT NULL DEFAULT '{}',
    response_status INTEGER,
    response_body   TEXT,
    response_headers JSONB,
    duration_ms     INTEGER,
    success         BOOLEAN NOT NULL DEFAULT FALSE,
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_retry      TIMESTAMPTZ,
    delivered_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_webhook_deliveries_webhook ON webhook_deliveries (webhook_id, delivered_at DESC);
CREATE INDEX idx_webhook_deliveries_retry ON webhook_deliveries (next_retry) WHERE next_retry IS NOT NULL AND success = FALSE;
