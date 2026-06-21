CREATE TABLE scheduled_notifications (
    notification_id UUID PRIMARY KEY,
    scheduled_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_scheduled_notifications_scheduled_at
    ON scheduled_notifications(scheduled_at ASC)
    WHERE scheduled_at IS NOT NULL;
