CREATE TABLE IF NOT EXISTS m3_workorders (
 id VARCHAR(64) COLLATE utf8mb4_bin PRIMARY KEY,
 reporter_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 room_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 area VARCHAR(128) NOT NULL,
 category VARCHAR(32) NOT NULL,
 state VARCHAR(32) NOT NULL,
 version BIGINT NOT NULL,
 snapshot JSON NOT NULL,
 seat_label VARCHAR(128) NOT NULL DEFAULT '',
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 INDEX m3_reporter_created (reporter_id,created_at,id),
 INDEX m3_area_state (area,state,created_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS m3_commands (
 actor_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 idem_key VARBINARY(128) NOT NULL,
 request_hash CHAR(64) NOT NULL,
 response JSON NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY (actor_id,idem_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS m3_events (
 event_id VARCHAR(64) COLLATE utf8mb4_bin PRIMARY KEY,
 workorder_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 business_version BIGINT NOT NULL,
 actor_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 action VARCHAR(32) NOT NULL,
 old_state VARCHAR(32) NOT NULL,
 new_state VARCHAR(32) NOT NULL,
 occurred_at DATETIME(6) NOT NULL,
 correlation_id VARCHAR(64) NOT NULL,
 UNIQUE KEY m3_order_event (workorder_id,business_version,action)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS m3_outbox (
 event_id VARCHAR(64) COLLATE utf8mb4_bin PRIMARY KEY,
 workorder_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 recipient_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 kind VARCHAR(64) NOT NULL,
 payload JSON NOT NULL,
 status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
 attempts INT NOT NULL DEFAULT 0,
 created_at DATETIME(6) NOT NULL,
 INDEX m3_delivery (status,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
