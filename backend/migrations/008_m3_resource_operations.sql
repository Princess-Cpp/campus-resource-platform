CREATE TABLE IF NOT EXISTS m3_resource_operations (
 id VARCHAR(64) COLLATE utf8mb4_bin PRIMARY KEY,
 workorder_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 source_version BIGINT NOT NULL,
 request JSON NOT NULL,
 actor_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 idem_key VARCHAR(128) COLLATE utf8mb4_bin NOT NULL,
 correlation_id VARCHAR(64) NOT NULL,
 state VARCHAR(16) NOT NULL DEFAULT 'PENDING',
 attempts INT NOT NULL DEFAULT 0,
 next_attempt DATETIME(6) NOT NULL,
 last_error VARCHAR(64) NOT NULL DEFAULT '',
 receipt JSON NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY m3_resource_version (workorder_id,source_version),
 KEY m3_resource_due (state,next_attempt,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
