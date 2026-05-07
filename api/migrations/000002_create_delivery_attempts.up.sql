CREATE TABLE delivery_attempts (
    id                UUID        PRIMARY KEY,
    notification_id   UUID        NOT NULL REFERENCES notifications(id),
    attempt_number    INT         NOT NULL,
    status            VARCHAR(20) NOT NULL,
    http_status_code  INT,
    provider_response JSONB,
    error_message     TEXT,
    latency_ms        INT,
    attempted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_delivery_attempts_unique
    ON delivery_attempts(notification_id, attempt_number);

CREATE INDEX idx_delivery_attempts_attempted_at
    ON delivery_attempts(attempted_at DESC);
