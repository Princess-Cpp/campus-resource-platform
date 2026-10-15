CREATE TABLE IF NOT EXISTS d_student_roster (
 school_number VARCHAR(64) PRIMARY KEY,
 user_id VARCHAR(64) NOT NULL UNIQUE,
 name VARCHAR(128) NOT NULL,
 department VARCHAR(64) NOT NULL,
 active BOOLEAN NOT NULL DEFAULT TRUE,
 claimed_at DATETIME(6) NULL
);
CREATE TABLE IF NOT EXISTS d_users (
 id VARCHAR(64) PRIMARY KEY,
 school_number VARCHAR(64) NOT NULL UNIQUE,
 name VARCHAR(128) NOT NULL,
 department VARCHAR(64) NOT NULL,
 password_hash VARCHAR(255) NOT NULL,
 active BOOLEAN NOT NULL DEFAULT TRUE,
 auth_version BIGINT NOT NULL DEFAULT 1,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL
);
CREATE TABLE IF NOT EXISTS d_roles (
 id VARCHAR(64) PRIMARY KEY,
 name VARCHAR(128) NOT NULL
);
CREATE TABLE IF NOT EXISTS d_permissions (
 id VARCHAR(64) PRIMARY KEY,
 name VARCHAR(128) NOT NULL
);
CREATE TABLE IF NOT EXISTS d_user_roles (
 user_id VARCHAR(64) NOT NULL,
 role_id VARCHAR(64) NOT NULL,
 PRIMARY KEY (user_id,role_id)
);
CREATE TABLE IF NOT EXISTS d_role_permissions (
 role_id VARCHAR(64) NOT NULL,
 permission_id VARCHAR(64) NOT NULL,
 PRIMARY KEY (role_id,permission_id)
);
CREATE TABLE IF NOT EXISTS d_user_areas (
 user_id VARCHAR(64) NOT NULL,
 area VARCHAR(128) NOT NULL,
 PRIMARY KEY (user_id,area)
);
CREATE TABLE IF NOT EXISTS d_user_contacts (
 user_id VARCHAR(64) NOT NULL,
 contact_ref VARCHAR(128) NOT NULL,
 PRIMARY KEY (user_id,contact_ref)
);
CREATE TABLE IF NOT EXISTS d_sessions (
 token_hash CHAR(64) PRIMARY KEY,
 user_id VARCHAR(64) NOT NULL,
 auth_version BIGINT NOT NULL,
 csrf_token VARCHAR(64) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 expires_at DATETIME(6) NOT NULL,
 revoked_at DATETIME(6) NULL,
 INDEX session_user (user_id),
 INDEX session_expiry (expires_at)
);
CREATE TABLE IF NOT EXISTS d_login_attempts (
 scope_key CHAR(64) PRIMARY KEY,
 failures INT NOT NULL,
 window_started DATETIME(6) NOT NULL,
 blocked_until DATETIME(6) NULL
);
INSERT IGNORE INTO d_roles (id,name) VALUES
 ('student','学生/申请人'),
 ('department_approver','院系审批人'),
 ('venue_approver','场地审批人'),
 ('operator','运营管理员');
INSERT IGNORE INTO d_permissions (id,name) VALUES
 ('booking.create','创建座位预约'),
 ('classroom.apply','提交教室申请'),
 ('approval.department','院系审批'),
 ('approval.final','场地审批'),
 ('classroom.terminate','终止教室活动'),
 ('classroom.stats','查看教室申请统计');
INSERT IGNORE INTO d_role_permissions (role_id,permission_id) VALUES
 ('student','booking.create'),
 ('student','classroom.apply'),
 ('department_approver','approval.department'),
 ('venue_approver','approval.final'),
 ('venue_approver','classroom.terminate'),
 ('operator','classroom.stats');
