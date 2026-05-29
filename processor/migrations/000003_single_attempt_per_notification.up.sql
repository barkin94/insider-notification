-- Replace the (notification_id, attempt_number) composite unique constraint with
-- a single unique constraint on notification_id, enforcing one entity per notification.
ALTER TABLE delivery_attempts
    DROP CONSTRAINT delivery_attempts_notification_id_attempt_number_key;

DROP INDEX IF EXISTS idx_delivery_attempts_notification_id;

ALTER TABLE delivery_attempts
    ADD CONSTRAINT delivery_attempts_notification_id_key UNIQUE (notification_id);
