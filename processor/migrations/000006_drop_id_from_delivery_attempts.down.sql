ALTER TABLE delivery_attempts DROP CONSTRAINT delivery_attempts_pkey;
ALTER TABLE delivery_attempts ADD CONSTRAINT delivery_attempts_notification_id_key UNIQUE (notification_id);
ALTER TABLE delivery_attempts ADD COLUMN id UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE delivery_attempts ADD PRIMARY KEY (id);
