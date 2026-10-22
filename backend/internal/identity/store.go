package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	passwordMinLength = 10
	passwordMaxLength = 72
	maxFailures       = 5
	attemptWindow     = 15 * time.Minute
	blockDuration     = 15 * time.Minute
	defaultDepartment = "CS"
)

type Store struct {
	DB           *sql.DB
	Now          func() time.Time
	SessionTTL   time.Duration
	CookieSecure bool
	dummyHash    []byte
}

func New(db *sql.DB, cookieSecure bool) *Store {
	dummy, _ := bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)
	return &Store{DB: db, Now: time.Now, SessionTTL: 12 * time.Hour, CookieSecure: cookieSecure, dummyHash: dummy}
}

func normalizeSchoolNumber(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func attemptKey(action, schoolNumber, ip string) string {
	sum := sha256.Sum256([]byte(action + "|" + normalizeSchoolNumber(schoolNumber) + "|" + ip))
	return hex.EncodeToString(sum[:])
}

func validPassword(password string) bool {
	return len(password) >= passwordMinLength && len(password) <= passwordMaxLength
}

func (s *Store) checkRateLimit(ctx context.Context, key string) error {
	var blocked sql.NullTime
	err := s.DB.QueryRowContext(ctx, "SELECT blocked_until FROM d_login_attempts WHERE scope_key=?", key).Scan(&blocked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if blocked.Valid && s.Now().UTC().Before(blocked.Time) {
		return fault("RATE_LIMITED", http.StatusTooManyRequests)
	}
	return nil
}

func (s *Store) recordFailure(ctx context.Context, key string) error {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := s.Now().UTC()
	var failures int
	var started time.Time
	err = tx.QueryRowContext(ctx, "SELECT failures,window_started FROM d_login_attempts WHERE scope_key=? FOR UPDATE", key).Scan(&failures, &started)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, "INSERT INTO d_login_attempts(scope_key,failures,window_started) VALUES (?,1,?)", key, now)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if now.Sub(started) >= attemptWindow {
		failures = 0
		started = now
	}
	failures++
	var blocked any
	if failures >= maxFailures {
		blocked = now.Add(blockDuration)
	} else {
		blocked = nil
	}
	if _, err = tx.ExecContext(ctx, "UPDATE d_login_attempts SET failures=?,window_started=?,blocked_until=? WHERE scope_key=?", failures, started, blocked, key); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) clearFailures(ctx context.Context, key string) {
	_, _ = s.DB.ExecContext(ctx, "DELETE FROM d_login_attempts WHERE scope_key=?", key)
}

func (s *Store) Register(ctx context.Context, schoolNumber, name, password, ip string) (Session, error) {
	schoolNumber, name = normalizeSchoolNumber(schoolNumber), strings.TrimSpace(name)
	key := attemptKey("register", schoolNumber, ip)
	if err := s.checkRateLimit(ctx, key); err != nil {
		return Session{}, err
	}
	if schoolNumber == "" || name == "" || !validPassword(password) {
		_ = s.recordFailure(ctx, key)
		return Session{}, fault("INVALID_INPUT", http.StatusBadRequest)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Session{}, err
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()

	var existing string
	err = tx.QueryRowContext(ctx, "SELECT id FROM d_users WHERE school_number=? FOR UPDATE", schoolNumber).Scan(&existing)
	if err == nil {
		_ = s.recordFailure(ctx, key)
		return Session{}, fault("REGISTRATION_UNAVAILABLE", http.StatusBadRequest)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Session{}, err
	}
	now := s.Now().UTC()
	userID := schoolNumber
	_, err = tx.ExecContext(ctx, "INSERT INTO d_users(id,school_number,name,department,password_hash,created_at,updated_at) VALUES (?,?,?,?,?,?,?)", userID, schoolNumber, name, defaultDepartment, string(passwordHash), now, now)
	if err != nil {
		return Session{}, fault("REGISTRATION_UNAVAILABLE", http.StatusBadRequest)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO d_user_roles(user_id,role_id) VALUES (?,'student')", userID); err != nil {
		return Session{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO d_user_contacts(user_id,contact_ref) VALUES (?,'campus-email')", userID); err != nil {
		return Session{}, err
	}
	session, err := s.createSessionTx(ctx, tx, userID, 1)
	if err != nil {
		return Session{}, err
	}
	if err = tx.Commit(); err != nil {
		return Session{}, err
	}
	s.clearFailures(ctx, key)
	session.Principal, err = s.Principal(ctx, userID)
	return session, err
}

func (s *Store) Login(ctx context.Context, schoolNumber, password, ip string) (Session, error) {
	schoolNumber = normalizeSchoolNumber(schoolNumber)
	key := attemptKey("login", schoolNumber, ip)
	if err := s.checkRateLimit(ctx, key); err != nil {
		return Session{}, err
	}
	var userID, passwordHash string
	var active bool
	var authVersion int64
	err := s.DB.QueryRowContext(ctx, "SELECT id,password_hash,active,auth_version FROM d_users WHERE school_number=?", schoolNumber).Scan(&userID, &passwordHash, &active, &authVersion)
	compareHash := s.dummyHash
	if err == nil {
		compareHash = []byte(passwordHash)
	}
	passwordErr := bcrypt.CompareHashAndPassword(compareHash, []byte(password))
	if err != nil || passwordErr != nil || !active {
		_ = s.recordFailure(ctx, key)
		return Session{}, fault("INVALID_CREDENTIALS", http.StatusUnauthorized)
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()
	session, err := s.createSessionTx(ctx, tx, userID, authVersion)
	if err != nil {
		return Session{}, err
	}
	if err = tx.Commit(); err != nil {
		return Session{}, err
	}
	s.clearFailures(ctx, key)
	session.Principal, err = s.Principal(ctx, userID)
	return session, err
}

func (s *Store) createSessionTx(ctx context.Context, tx *sql.Tx, userID string, authVersion int64) (Session, error) {
	token, err := randomToken()
	if err != nil {
		return Session{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return Session{}, err
	}
	now := s.Now().UTC()
	session := Session{Token: token, CSRFToken: csrf, ExpiresAt: now.Add(s.SessionTTL)}
	_, err = tx.ExecContext(ctx, "INSERT INTO d_sessions(token_hash,user_id,auth_version,csrf_token,created_at,expires_at) VALUES (?,?,?,?,?,?)", tokenHash(token), userID, authVersion, csrf, now, session.ExpiresAt)
	return session, err
}

func (s *Store) Authenticate(ctx context.Context, token string) (Session, error) {
	var session Session
	var userID string
	var sessionVersion, userVersion int64
	var active bool
	var revoked sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT s.user_id,s.auth_version,s.csrf_token,s.expires_at,s.revoked_at,u.active,u.auth_version
		FROM d_sessions s JOIN d_users u ON u.id=s.user_id WHERE s.token_hash=?`, tokenHash(token)).
		Scan(&userID, &sessionVersion, &session.CSRFToken, &session.ExpiresAt, &revoked, &active, &userVersion)
	if err != nil || revoked.Valid || !active || sessionVersion != userVersion || !s.Now().UTC().Before(session.ExpiresAt) {
		return Session{}, fault("UNAUTHENTICATED", http.StatusUnauthorized)
	}
	session.Token = token
	session.Principal, err = s.Principal(ctx, userID)
	return session, err
}

func (s *Store) AuthenticateRequest(ctx context.Context, request *http.Request) (Session, error) {
	cookie, err := request.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		return Session{}, fault("UNAUTHENTICATED", http.StatusUnauthorized)
	}
	return s.Authenticate(ctx, cookie.Value)
}

func (s *Store) Logout(ctx context.Context, token string) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE d_sessions SET revoked_at=COALESCE(revoked_at,?) WHERE token_hash=?", s.Now().UTC(), tokenHash(token))
	return err
}

func (s *Store) Principal(ctx context.Context, userID string) (Principal, error) {
	var p Principal
	err := s.DB.QueryRowContext(ctx, "SELECT id,school_number,name,department FROM d_users WHERE id=? AND active=1", userID).Scan(&p.ID, &p.SchoolNumber, &p.Name, &p.Department)
	if err != nil {
		return p, err
	}
	if p.Roles, err = stringsFor(ctx, s.DB, "SELECT role_id FROM d_user_roles WHERE user_id=? ORDER BY role_id", userID); err != nil {
		return p, err
	}
	if p.Permissions, err = stringsFor(ctx, s.DB, `SELECT DISTINCT rp.permission_id FROM d_user_roles ur JOIN d_role_permissions rp ON rp.role_id=ur.role_id
		WHERE ur.user_id=? ORDER BY rp.permission_id`, userID); err != nil {
		return p, err
	}
	if p.Areas, err = stringsFor(ctx, s.DB, "SELECT area FROM d_user_areas WHERE user_id=? ORDER BY area", userID); err != nil {
		return p, err
	}
	p.ContactRefs, err = stringsFor(ctx, s.DB, "SELECT contact_ref FROM d_user_contacts WHERE user_id=? ORDER BY contact_ref", userID)
	return p, err
}

func (s *Store) Principals(ctx context.Context) ([]Principal, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id FROM d_users WHERE active=1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Principal, 0, len(ids))
	for _, id := range ids {
		principal, loadErr := s.Principal(ctx, id)
		if loadErr != nil {
			return nil, loadErr
		}
		out = append(out, principal)
	}
	return out, nil
}

func stringsFor(ctx context.Context, db *sql.DB, query, id string) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []string{}
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
