CREATE SCHEMA IF NOT EXISTS processor;

CREATE TABLE processor.delivery_attempts (
    id              UUID        PRIMARY KEY,
    notification_id UUID        NOT NULL,
    attempt_number  INT         NOT NULL,
    status          VARCHAR(20) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (notification_id, attempt_number)
);

CREATE INDEX idx_delivery_attempts_notification_id
    ON processor.delivery_attempts(notification_id);
CREATE INDEX idx_delivery_attempts_created_at
    ON processor.delivery_attempts(created_at DESC);
