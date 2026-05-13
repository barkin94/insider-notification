DROP INDEX IF EXISTS processor.idx_delivery_attempts_retry_after;

ALTER TABLE processor.delivery_attempts
    DROP COLUMN IF EXISTS retry_after,
    DROP COLUMN IF EXISTS priority;
