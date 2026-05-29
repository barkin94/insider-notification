ALTER TABLE delivery_attempts
    DROP CONSTRAINT delivery_attempts_notification_id_key;

CREATE INDEX idx_delivery_attempts_notification_id
    ON delivery_attempts(notification_id);

ALTER TABLE delivery_attempts
    ADD CONSTRAINT delivery_attempts_notification_id_attempt_number_key
    UNIQUE (notification_id, attempt_number);
