ALTER TABLE delivery_attempts DROP CONSTRAINT delivery_attempts_pkey;
ALTER TABLE delivery_attempts DROP COLUMN id;
ALTER TABLE delivery_attempts DROP CONSTRAINT delivery_attempts_notification_id_key;
ALTER TABLE delivery_attempts ADD PRIMARY KEY (notification_id);
