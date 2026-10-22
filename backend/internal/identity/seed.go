package identity

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type seedUser struct {
	ID, SchoolNumber, Name, Department, Role, Area string
}

// SeedDevelopment is deliberately gated by the caller. It inserts only
// fictional local-development identities and never overwrites passwords.
func (s *Store) SeedDevelopment(ctx context.Context, password string) error {
	if !validPassword(password) {
		return fmt.Errorf("AUTH_DEV_PASSWORD must contain %d-%d bytes", passwordMinLength, passwordMaxLength)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := s.Now().UTC()
	if _, err = s.DB.ExecContext(ctx, `INSERT INTO d_student_roster(school_number,user_id,name,department,active)
		VALUES ('20260001','student-1','演示学生','CS',1)
		ON DUPLICATE KEY UPDATE user_id=VALUES(user_id),name=VALUES(name),department=VALUES(department),active=1`); err != nil {
		return err
	}
	users := []seedUser{
		{ID: "department-1", SchoolNumber: "DEV-DEP-001", Name: "院系审批员", Department: "CS", Role: "department_approver"},
		{ID: "venue-1", SchoolNumber: "DEV-VENUE-001", Name: "场地审批员", Department: "CS", Role: "venue_approver", Area: "A楼"},
		{ID: "operator-1", SchoolNumber: "DEV-OP-001", Name: "运营管理员", Department: "CS", Role: "operator", Area: "A楼"},
	}
	for _, user := range users {
		_, execErr := s.DB.ExecContext(ctx, `INSERT IGNORE INTO d_users
			(id,school_number,name,department,password_hash,active,auth_version,created_at,updated_at)
			VALUES (?,?,?,?,?,1,1,?,?)`, user.ID, user.SchoolNumber, user.Name, user.Department, string(hash), now, now)
		if execErr != nil {
			return execErr
		}
		if _, execErr = s.DB.ExecContext(ctx, "INSERT IGNORE INTO d_user_roles(user_id,role_id) VALUES (?,?)", user.ID, user.Role); execErr != nil {
			return execErr
		}
		if user.Area != "" {
			if _, execErr = s.DB.ExecContext(ctx, "INSERT IGNORE INTO d_user_areas(user_id,area) VALUES (?,?)", user.ID, user.Area); execErr != nil {
				return execErr
			}
		}
	}
	_, err = s.DB.ExecContext(ctx, "DELETE FROM d_sessions WHERE expires_at<? OR revoked_at IS NOT NULL", s.Now().UTC())
	return err
}
