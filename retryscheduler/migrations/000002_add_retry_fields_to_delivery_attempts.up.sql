ALTER TABLE delivery_attempts
    ADD COLUMN retry_after TIMESTAMPTZ,
    ADD COLUMN priority    VARCHAR(10);

CREATE INDEX idx_delivery_attempts_retry_after
    ON delivery_attempts(retry_after)
    WHERE retry_after IS NOT NULL;
