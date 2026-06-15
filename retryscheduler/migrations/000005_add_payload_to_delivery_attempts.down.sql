ALTER TABLE delivery_attempts
    DROP COLUMN IF EXISTS channel,
    DROP COLUMN IF EXISTS recipient,
    DROP COLUMN IF EXISTS content,
    DROP COLUMN IF EXISTS max_attempts,
    DROP COLUMN IF EXISTS metadata;
