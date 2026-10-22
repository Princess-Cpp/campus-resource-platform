ALTER TABLE m3_workorders
 ADD COLUMN assignee_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL DEFAULT '',
 ADD COLUMN assignee_name VARCHAR(128) NOT NULL DEFAULT '',
 ADD INDEX m3_assignee_state (assignee_id,state,created_at,id);
CREATE TABLE IF NOT EXISTS m3_assignment_history (
 id VARCHAR(64) COLLATE utf8mb4_bin PRIMARY KEY,
 workorder_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 business_version BIGINT NOT NULL,
 dispatcher_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 previous_assignee_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 assignee_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 assignee_name VARCHAR(128) NOT NULL,
 comment VARCHAR(1000) NOT NULL,
 assigned_at DATETIME(6) NOT NULL,
 UNIQUE KEY m3_assignment_version (workorder_id,business_version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE m3_events ADD COLUMN comment VARCHAR(1000) NOT NULL DEFAULT '';
ALTER TABLE m3_outbox DROP PRIMARY KEY, ADD PRIMARY KEY (event_id,recipient_id,kind);
