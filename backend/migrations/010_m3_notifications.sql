ALTER TABLE m3_outbox
 ADD COLUMN read_at DATETIME(6) NULL,
 ADD COLUMN next_attempt DATETIME(6) NULL,
 ADD COLUMN lease_until DATETIME(6) NULL,
 ADD COLUMN lease_token VARCHAR(64) COLLATE utf8mb4_bin NOT NULL DEFAULT '',
 ADD COLUMN delivered_at DATETIME(6) NULL,
 ADD COLUMN last_error VARCHAR(64) NOT NULL DEFAULT '',
 ADD INDEX m3_inbox (recipient_id,created_at,event_id),
 ADD INDEX m3_notification_retry (status,next_attempt,lease_until);
