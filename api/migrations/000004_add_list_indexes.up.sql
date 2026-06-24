CREATE INDEX idx_notifications_status_created_at
    ON notifications(status, created_at DESC);

CREATE INDEX idx_notifications_batch_id_created_at
    ON notifications(batch_id, created_at DESC) WHERE batch_id IS NOT NULL;
