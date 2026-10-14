CREATE TABLE IF NOT EXISTS m2_applications (
 id VARCHAR(64) PRIMARY KEY, user_id VARCHAR(64) NOT NULL, department VARCHAR(64) NOT NULL,
 room_id VARCHAR(64) NOT NULL, room_name VARCHAR(128) NOT NULL, area VARCHAR(128) NOT NULL,
 start_at DATETIME(6) NOT NULL, end_at DATETIME(6) NOT NULL, created_at DATETIME(6) NOT NULL,
 deadline_at DATETIME(6) NOT NULL, status VARCHAR(32) NOT NULL, version BIGINT NOT NULL,
 sync_status VARCHAR(16) NOT NULL, operation_id VARCHAR(64) NOT NULL DEFAULT '', source_version BIGINT NOT NULL DEFAULT 0,
 last_error VARCHAR(64) NOT NULL DEFAULT '', payload JSON NOT NULL,
 idem_key VARCHAR(128) NOT NULL, request_hash CHAR(64) NOT NULL,
 decided_at DATETIME(6) NULL, decision VARCHAR(16) NOT NULL DEFAULT '',
 UNIQUE KEY user_request (user_id,idem_key), INDEX due (status,deadline_at), INDEX owner_date (user_id,created_at), INDEX department_status (department,status)
);
CREATE TABLE IF NOT EXISTS m2_commands (
 application_id VARCHAR(64) NOT NULL, actor_id VARCHAR(64) NOT NULL, idem_key VARCHAR(128) NOT NULL,
 request_hash CHAR(64) NOT NULL, PRIMARY KEY(application_id,actor_id,idem_key)
);
CREATE TABLE IF NOT EXISTS m2_events (
 id VARCHAR(64) PRIMARY KEY, application_id VARCHAR(64) NOT NULL, actor_id VARCHAR(64) NOT NULL,
 action VARCHAR(48) NOT NULL, from_status VARCHAR(32) NOT NULL, to_status VARCHAR(32) NOT NULL,
 comment VARCHAR(2000) NOT NULL, correlation_id VARCHAR(64) NOT NULL, created_at DATETIME(6) NOT NULL,
 INDEX application_time (application_id,created_at,id)
);
CREATE TABLE IF NOT EXISTS m2_outbox (
 event_id VARCHAR(128) PRIMARY KEY, application_id VARCHAR(64) NOT NULL, recipient_id VARCHAR(64) NOT NULL,
 kind VARCHAR(48) NOT NULL, created_at DATETIME(6) NOT NULL, status VARCHAR(16) NOT NULL DEFAULT 'PENDING'
);
CREATE TABLE IF NOT EXISTS m2_operations (
 id VARCHAR(64) PRIMARY KEY, application_id VARCHAR(64) NOT NULL, request JSON NOT NULL,
 target_status VARCHAR(32) NOT NULL, actor_id VARCHAR(64) NOT NULL, correlation_id VARCHAR(64) NOT NULL,
 state VARCHAR(16) NOT NULL, attempts INT NOT NULL DEFAULT 0, next_attempt DATETIME(6) NOT NULL,
 last_error VARCHAR(64) NOT NULL DEFAULT '', INDEX pending (state,next_attempt)
);
