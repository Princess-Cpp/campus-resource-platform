CREATE TABLE IF NOT EXISTS m1_room_locks (room_id VARCHAR(64) PRIMARY KEY);
CREATE TABLE IF NOT EXISTS m1_config (id TINYINT PRIMARY KEY, max_hours INT NOT NULL);
INSERT IGNORE INTO m1_config VALUES (1,4);
CREATE TABLE IF NOT EXISTS m1_bookings (
 id VARCHAR(64) PRIMARY KEY, user_id VARCHAR(64) NOT NULL, room_id VARCHAR(64) NOT NULL,
 seat_id VARCHAR(64) NOT NULL, room_name VARCHAR(128) NOT NULL, seat_label VARCHAR(64) NOT NULL,
 start_at DATETIME(6) NOT NULL, end_at DATETIME(6) NOT NULL,
 status VARCHAR(32) NOT NULL, reason VARCHAR(64) NOT NULL DEFAULT '',
 checked_in_at DATETIME(6) NULL, created_at DATETIME(6) NOT NULL,
 idem_key VARCHAR(128) NOT NULL, request_hash CHAR(64) NOT NULL,
 UNIQUE KEY user_request (user_id,idem_key), INDEX due (status,start_at), INDEX room_time (room_id,start_at,end_at)
);
CREATE TABLE IF NOT EXISTS m1_sources (
 module VARCHAR(8) NOT NULL, business_id VARCHAR(64) NOT NULL, room_id VARCHAR(64) NOT NULL,
 seat_id VARCHAR(64) NOT NULL DEFAULT '', version BIGINT NOT NULL,
 active BOOLEAN NOT NULL, kind VARCHAR(32) NOT NULL,
 start_at DATETIME(6) NULL, end_at DATETIME(6) NULL,
 created_at DATETIME(6) NOT NULL, released_at DATETIME(6) NULL,
 PRIMARY KEY (module,business_id), INDEX room_active (room_id,active)
);
CREATE TABLE IF NOT EXISTS m1_operations (
 id VARCHAR(64) PRIMARY KEY, module VARCHAR(8) NOT NULL, idem_key VARCHAR(128) NOT NULL,
 request_hash CHAR(64) NOT NULL, response JSON NOT NULL,
 UNIQUE KEY module_key (module,idem_key)
);
CREATE TABLE IF NOT EXISTS m1_violations (booking_id VARCHAR(64) PRIMARY KEY, user_id VARCHAR(64) NOT NULL, created_at DATETIME(6) NOT NULL);
CREATE TABLE IF NOT EXISTS m1_outbox (
 event_id VARCHAR(128) PRIMARY KEY, user_id VARCHAR(64) NOT NULL, booking_id VARCHAR(64) NOT NULL,
 kind VARCHAR(32) NOT NULL, created_at DATETIME(6) NOT NULL, delivered_at DATETIME(6) NULL,
 attempts INT NOT NULL DEFAULT 0, next_attempt DATETIME(6) NULL, status VARCHAR(16) NOT NULL DEFAULT 'PENDING'
);
CREATE TABLE IF NOT EXISTS m1_audit (
 id VARCHAR(64) PRIMARY KEY, actor VARCHAR(64) NOT NULL, action VARCHAR(48) NOT NULL,
 business_id VARCHAR(64) NOT NULL, source_module VARCHAR(8) NOT NULL, correlation_id VARCHAR(64) NOT NULL,
 created_at DATETIME(6) NOT NULL
);
