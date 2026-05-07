CREATE TABLE idempotency_keys (
    key             VARCHAR(255) PRIMARY KEY,
    notification_id UUID         NOT NULL REFERENCES notifications(id),
    key_type        VARCHAR(20)  NOT NULL,
    expires_at      TIMESTAMPTZ  NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_idempotency_keys_expires_at
    ON idempotency_keys(expires_at);
