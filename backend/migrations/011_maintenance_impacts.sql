CREATE TABLE IF NOT EXISTS m1_maintenance_impacts (
 id VARCHAR(64) COLLATE utf8mb4_bin PRIMARY KEY,
 source_business_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 payload JSON NOT NULL,
 state VARCHAR(16) NOT NULL DEFAULT 'PENDING',
 attempts INT NOT NULL DEFAULT 0,
 next_attempt DATETIME(6) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 last_error VARCHAR(64) NOT NULL DEFAULT '',
 INDEX m1_impact_due (state,next_attempt,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS m2_maintenance_impacts (
 id VARCHAR(64) COLLATE utf8mb4_bin PRIMARY KEY,
 request_hash CHAR(64) NOT NULL,
 application_id VARCHAR(64) NOT NULL,
 payload JSON NOT NULL,
 state VARCHAR(16) NOT NULL DEFAULT 'PENDING',
 created_at DATETIME(6) NOT NULL,
 INDEX m2_impact_pending (state,application_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
