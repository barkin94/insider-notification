DROP INDEX IF EXISTS idx_delivery_attempts_retry_after;

ALTER TABLE delivery_attempts
    DROP COLUMN IF EXISTS retry_after,
    DROP COLUMN IF EXISTS priority;
