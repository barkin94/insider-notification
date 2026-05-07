CREATE TABLE notifications (
    id               UUID        PRIMARY KEY,
    batch_id         UUID,
    recipient        VARCHAR(255) NOT NULL,
    channel          VARCHAR(20)  NOT NULL,
    content          TEXT         NOT NULL,
    priority         VARCHAR(20)  NOT NULL DEFAULT 'normal',
    status           VARCHAR(20)  NOT NULL DEFAULT 'pending',
    deliver_after    TIMESTAMPTZ,
    attempts         INT          NOT NULL DEFAULT 0,
    max_attempts     INT          NOT NULL DEFAULT 4,
    metadata         JSONB,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_batch_id
    ON notifications(batch_id) WHERE batch_id IS NOT NULL;

CREATE INDEX idx_notifications_status
    ON notifications(status);

CREATE INDEX idx_notifications_channel
    ON notifications(channel);

CREATE INDEX idx_notifications_created_at
    ON notifications(created_at DESC);

CREATE INDEX idx_notifications_deliver_after_status
    ON notifications(deliver_after, status);

CREATE INDEX idx_notifications_status_updated_at
    ON notifications(status, updated_at);
