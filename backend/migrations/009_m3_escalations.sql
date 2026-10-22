CREATE TABLE IF NOT EXISTS m3_escalations (
 id VARCHAR(64) COLLATE utf8mb4_bin PRIMARY KEY,
 workorder_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 stage_number BIGINT NOT NULL,
 level INT NOT NULL,
 policy_version VARCHAR(64) NOT NULL,
 stage_state VARCHAR(32) NOT NULL,
 deadline_at DATETIME(6) NOT NULL,
 detected_at DATETIME(6) NOT NULL,
 routing_state VARCHAR(32) NOT NULL,
 next_route_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY m3_stage_escalation (workorder_id,stage_number,level),
 KEY m3_escalation_routing (routing_state,next_route_at,id),
 KEY m3_escalation_time (detected_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
