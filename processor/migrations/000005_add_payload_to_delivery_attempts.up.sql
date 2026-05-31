ALTER TABLE delivery_attempts
    ADD COLUMN channel      VARCHAR(20)  NOT NULL DEFAULT '',
    ADD COLUMN recipient    VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN content      TEXT         NOT NULL DEFAULT '',
    ADD COLUMN max_attempts INT          NOT NULL DEFAULT 4,
    ADD COLUMN metadata     TEXT         NOT NULL DEFAULT '{}';
