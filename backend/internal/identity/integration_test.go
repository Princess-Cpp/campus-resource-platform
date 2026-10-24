package identity_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"testing"
	"time"

	"campus-resource-platform/backend/internal/booking"
	"campus-resource-platform/backend/internal/classroom"
	"campus-resource-platform/backend/internal/dev"
	"campus-resource-platform/backend/internal/identity"
	"campus-resource-platform/backend/migrations"
	_ "github.com/go-sql-driver/mysql"
)

// testM2ServiceToken is test-only; runtime tokens must come from M2_SERVICE_TOKEN.
const testM2ServiceToken = "m2-service"

type identityHarness struct {
	t       *testing.T
	db      *sql.DB
	store   *identity.Store
	server  *httptest.Server
	client  *http.Client
	baseURL *url.URL
	now     time.Time
}

type sqlDirectory struct {
	store   *identity.Store
	fixture *dev.ClassroomFixture
}

func (d *sqlDirectory) Authenticate(ctx context.Context, token string) (classroom.Actor, error) {
	session, err := d.store.Authenticate(ctx, token)
	if err != nil {
		return classroom.Actor{}, err
	}
	p := session.Principal
	return classroom.Actor{
		ID: p.ID, Department: p.Department, Permissions: p.Permissions,
		Areas: p.Areas, ContactRefs: p.ContactRefs,
	}, nil
}

func (d *sqlDirectory) Actors(ctx context.Context) ([]classroom.Actor, error) {
	principals, err := d.store.Principals(ctx)
	if err != nil {
		return nil, err
	}
	actors := make([]classroom.Actor, 0, len(principals))
	for _, p := range principals {
		actors = append(actors, classroom.Actor{
			ID: p.ID, Department: p.Department, Permissions: p.Permissions,
			Areas: p.Areas, ContactRefs: p.ContactRefs,
		})
	}
	return actors, nil
}

func (d *sqlDirectory) Rooms(ctx context.Context) ([]classroom.Room, error) {
	return d.fixture.Rooms(ctx)
}

func setupIdentity(t *testing.T) *identityHarness {
	t.Helper()
	dsn := os.Getenv("IDENTITY_TEST_DSN")
	if dsn == "" {
		t.Skip("IDENTITY_TEST_DSN not set; requires isolated MySQL database")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(20)
	t.Cleanup(func() { db.Close() })

	var database string
	if err = db.QueryRow("SELECT DATABASE()").Scan(&database); err != nil {
		t.Fatal(err)
	}
	if database != "campus_identity_check" {
		t.Fatal("identity tests only run against campus_identity_check")
	}
	if err = migrations.Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	tables := []string{
		"m2_commands", "m2_events", "m2_outbox", "m2_operations", "m2_applications",
		"m1_operations", "m1_sources", "m1_bookings", "m1_audit", "m1_outbox", "m1_violations",
		"d_login_attempts", "d_sessions", "d_user_contacts", "d_user_areas",
		"d_user_roles", "d_users", "d_student_roster",
	}
	for _, table := range tables {
		if _, err = db.Exec("DELETE FROM " + table); err != nil {
			t.Fatalf("clear %s: %v", table, err)
		}
	}
	if _, err = db.Exec(`INSERT INTO d_student_roster(school_number,user_id,name,department,active)
		VALUES ('20269999','identity-student','测试学生','CS',1)`); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2030, 1, 10, 0, 0, 0, 0, time.UTC)
	store := identity.New(db, false)
	store.Now = func() time.Time { return now }
	fixture, err := dev.Load("../../dev-fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture.Tokens[testM2ServiceToken] = booking.Actor{ID: testM2ServiceToken, Service: "M2"}
	m2Fixture, err := dev.LoadClassroom("../../dev-fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	auth := func(ctx context.Context, token string) (booking.Actor, error) {
		session, sessionErr := store.Authenticate(ctx, token)
		if sessionErr == nil {
			p := session.Principal
			role := ""
			if len(p.Roles) > 0 {
				role = p.Roles[0]
			}
			return booking.Actor{ID: p.ID, Department: p.Department, Role: role, Permissions: p.Permissions}, nil
		}
		service, serviceErr := fixture.Auth(ctx, token)
		if serviceErr != nil || service.Service == "" {
			return booking.Actor{}, sessionErr
		}
		return service, nil
	}
	resource := booking.New(db, fixture, []byte("test-only-signing-secret-at-least-32"))
	resource.Now = func() time.Time { return now }
	authHTTP := &identity.HTTP{Store: store}
	router := resource.Router(auth, authHTTP.CSRFMiddleware())
	authHTTP.Register(router)
	classrooms := classroom.New(db, &sqlDirectory{store: store, fixture: m2Fixture}, &classroom.HTTPResources{})
	classrooms.Now = func() time.Time { return now }
	classrooms.Register(router)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	classrooms.Resources = &classroom.HTTPResources{BaseURL: server.URL, Token: testM2ServiceToken}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	baseURL, _ := url.Parse(server.URL)
	return &identityHarness{
		t: t, db: db, store: store, server: server,
		client: &http.Client{Jar: jar}, baseURL: baseURL, now: now,
	}
}

func (h *identityHarness) request(method, path string, body any, headers map[string]string) (int, []byte, []*http.Cookie) {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, h.server.URL+path, reader)
	if err != nil {
		h.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := h.client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	return res.StatusCode, raw, res.Cookies()
}

func errorCode(raw []byte) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	return envelope.Error.Code
}

func sessionData(t *testing.T, raw []byte) identity.SessionData {
	t.Helper()
	var envelope struct {
		Data identity.SessionData `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}

func (h *identityHarness) csrf() string {
	h.t.Helper()
	status, raw, _ := h.request(http.MethodGet, "/api/v1/auth/session", nil, nil)
	if status != http.StatusOK {
		h.t.Fatalf("session: %d %s", status, raw)
	}
	return sessionData(h.t, raw).CSRFToken
}

func TestSQLIdentityHTTPAndModuleIsolation(t *testing.T) {
	h := setupIdentity(t)

	status, raw, _ := h.request(http.MethodPost, "/api/v1/auth/register", map[string]any{
		"schoolNumber": "20269999", "name": "测试学生",
		"password": "short", "confirmPassword": "short",
	}, nil)
	if status != http.StatusBadRequest || errorCode(raw) != "INVALID_INPUT" {
		t.Fatalf("weak password: %d %s", status, raw)
	}

	status, raw, cookies := h.request(http.MethodPost, "/api/v1/auth/register", map[string]any{
		"schoolNumber": "not-on-roster", "name": "未知学生", "role": "operator",
		"password": "student-pass-1", "confirmPassword": "student-pass-1",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("open registration: %d %s", status, raw)
	}
	session := sessionData(t, raw)
	if session.Actor.ID != "NOT-ON-ROSTER" || session.Actor.Name != "未知学生" ||
		session.Actor.Department != "CS" || !slices.Equal(session.Actor.Roles, []string{"student"}) ||
		session.LandingPath != "/m1/search" {
		t.Fatalf("server did not assign student identity: %+v", session)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == identity.CookieName {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteLaxMode ||
		sessionCookie.Path != "/" {
		t.Fatalf("unsafe session cookie: %+v", sessionCookie)
	}

	status, raw, _ = h.request(http.MethodPost, "/api/v1/auth/register", map[string]any{
		"schoolNumber": "not-on-roster", "name": "未知学生",
		"password": "student-pass-1", "confirmPassword": "student-pass-1",
	}, nil)
	if status != http.StatusBadRequest || errorCode(raw) != "REGISTRATION_UNAVAILABLE" {
		t.Fatalf("duplicate registration: %d %s", status, raw)
	}

	status, raw, _ = h.request(http.MethodGet, "/api/v1/auth/session", nil, nil)
	if status != http.StatusOK || sessionData(t, raw).Actor.ID != "NOT-ON-ROSTER" {
		t.Fatalf("session lookup: %d %s", status, raw)
	}

	booking := map[string]any{
		"roomId": "room-101", "seatId": "seat-101-01",
		"startAt": h.now.Add(2 * time.Hour), "endAt": h.now.Add(3 * time.Hour),
	}
	status, raw, _ = h.request(http.MethodPost, "/api/v1/m1/bookings", booking, map[string]string{"Idempotency-Key": "identity-booking-no-csrf"})
	if status != http.StatusForbidden || errorCode(raw) != "CSRF_INVALID" {
		t.Fatalf("write without csrf: %d %s", status, raw)
	}
	csrf := h.csrf()
	status, raw, _ = h.request(http.MethodPost, "/api/v1/m1/bookings", booking, map[string]string{
		"Idempotency-Key": "identity-booking", "X-CSRF-Token": csrf,
	})
	if status != http.StatusOK {
		t.Fatalf("SQL student M1 request: %d %s", status, raw)
	}
	if _, err := h.db.Exec("DELETE FROM d_user_roles WHERE user_id='NOT-ON-ROSTER'"); err != nil {
		t.Fatal(err)
	}
	revoked := map[string]any{
		"roomId": "room-101", "seatId": "seat-101-02",
		"startAt": h.now.Add(2 * time.Hour), "endAt": h.now.Add(3 * time.Hour),
	}
	status, raw, _ = h.request(http.MethodPost, "/api/v1/m1/bookings", revoked, map[string]string{
		"Idempotency-Key": "identity-booking-revoked", "X-CSRF-Token": h.csrf(),
	})
	if status != http.StatusForbidden || errorCode(raw) != "FORBIDDEN" {
		t.Fatalf("booking after role removal: %d %s", status, raw)
	}
	status, raw, _ = h.request(http.MethodGet, "/api/v1/m2/session", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("SQL student M2 request: %d %s", status, raw)
	}
	status, raw, _ = h.request(http.MethodPost, "/api/v1/internal/resources/operations", map[string]any{},
		map[string]string{"X-CSRF-Token": csrf, "Idempotency-Key": "browser-cannot-be-service"})
	if status != http.StatusForbidden || errorCode(raw) != "FORBIDDEN" {
		t.Fatalf("browser session reached service API: %d %s", status, raw)
	}

	status, raw, _ = h.request(http.MethodPost, "/api/v1/auth/logout", nil, nil)
	if status != http.StatusForbidden || errorCode(raw) != "CSRF_INVALID" {
		t.Fatalf("logout without csrf: %d %s", status, raw)
	}
	status, raw, _ = h.request(http.MethodPost, "/api/v1/auth/logout", nil, map[string]string{"X-CSRF-Token": csrf})
	if status != http.StatusOK {
		t.Fatalf("logout: %d %s", status, raw)
	}
	status, raw, _ = h.request(http.MethodGet, "/api/v1/auth/session", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("revoked session remained valid: %d %s", status, raw)
	}
	h.client.Jar.SetCookies(h.baseURL, []*http.Cookie{{Name: identity.CookieName, Value: testM2ServiceToken, Path: "/"}})
	status, raw, _ = h.request(http.MethodGet, "/api/v1/internal/resources/sources", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("service credential in browser cookie was accepted: %d %s", status, raw)
	}
}

func TestSessionInvalidationPermissionsAndRateLimit(t *testing.T) {
	h := setupIdentity(t)
	_, err := h.store.Register(context.Background(), "20269999", "测试学生", "student-pass-1", "setup")
	if err != nil {
		t.Fatal(err)
	}

	login := func(password string) (int, []byte) {
		status, raw, _ := h.request(http.MethodPost, "/api/v1/auth/login", map[string]string{
			"schoolNumber": "20269999", "password": password,
		}, nil)
		return status, raw
	}
	status, raw := login("student-pass-1")
	if status != http.StatusOK {
		t.Fatalf("login: %d %s", status, raw)
	}

	if _, err = h.db.Exec("UPDATE d_users SET active=0 WHERE id='20269999'"); err != nil {
		t.Fatal(err)
	}
	status, raw, _ = h.request(http.MethodGet, "/api/v1/auth/session", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("disabled user remained authenticated: %d %s", status, raw)
	}
	if _, err = h.db.Exec("UPDATE d_users SET active=1 WHERE id='20269999'"); err != nil {
		t.Fatal(err)
	}
	status, raw = login("student-pass-1")
	if status != http.StatusOK {
		t.Fatalf("login after re-enable: %d %s", status, raw)
	}
	if _, err = h.db.Exec("UPDATE d_users SET auth_version=auth_version+1 WHERE id='20269999'"); err != nil {
		t.Fatal(err)
	}
	status, raw, _ = h.request(http.MethodGet, "/api/v1/auth/session", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("old auth version remained valid: %d %s", status, raw)
	}

	status, raw = login("student-pass-1")
	if status != http.StatusOK {
		t.Fatalf("fresh login: %d %s", status, raw)
	}
	if _, err = h.db.Exec("DELETE FROM d_role_permissions WHERE role_id='student' AND permission_id='booking.create'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = h.db.Exec("INSERT IGNORE INTO d_role_permissions(role_id,permission_id) VALUES ('student','booking.create')")
	})
	status, raw, _ = h.request(http.MethodGet, "/api/v1/auth/session", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("session after permission removal: %d %s", status, raw)
	}
	if slices.Contains(sessionData(t, raw).Actor.Permissions, "booking.create") {
		t.Fatal("removed permission remained in the next session response")
	}
	status, raw, _ = h.request(http.MethodPost, "/api/v1/m1/bookings", map[string]any{
		"roomId": "room-101", "seatId": "seat-101-01",
		"startAt": h.now.Add(2 * time.Hour), "endAt": h.now.Add(3 * time.Hour),
	}, map[string]string{"Idempotency-Key": "revoked-write", "X-CSRF-Token": sessionData(t, raw).CSRFToken})
	if status != http.StatusForbidden || errorCode(raw) != "FORBIDDEN" {
		t.Fatalf("booking after permission removal: %d %s", status, raw)
	}

	if _, err = h.db.Exec("UPDATE d_sessions SET expires_at=? WHERE revoked_at IS NULL", h.now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	status, raw, _ = h.request(http.MethodGet, "/api/v1/auth/session", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("expired session remained valid: %d %s", status, raw)
	}

	for attempt := 1; attempt <= 5; attempt++ {
		status, raw = login("wrong-password")
		if status != http.StatusUnauthorized || errorCode(raw) != "INVALID_CREDENTIALS" {
			t.Fatalf("failed login %d: %d %s", attempt, status, raw)
		}
	}
	status, raw = login("wrong-password")
	if status != http.StatusTooManyRequests || errorCode(raw) != "RATE_LIMITED" {
		t.Fatalf("rate limit after five failures: %d %s", status, raw)
	}

	var fault *identity.Fault
	_, err = h.store.Authenticate(context.Background(), "not-a-session")
	if !errors.As(err, &fault) || fault.Code != "UNAUTHENTICATED" {
		t.Fatalf("invalid session error: %v", err)
	}
}

func TestDevelopmentSeedIsIdempotentAndRolesLandCorrectly(t *testing.T) {
	h := setupIdentity(t)
	const password = "local-seed-pass"
	if err := h.store.SeedDevelopment(context.Background(), password); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SeedDevelopment(context.Background(), password); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		schoolNumber string
		landingPath  string
	}{
		{"DEV-DEP-001", "/m2/approvals"},
		{"DEV-VENUE-001", "/m2/approvals"},
		{"DEV-OP-001", "/m2/stats"},
	}
	for _, test := range cases {
		session, err := h.store.Login(context.Background(), test.schoolNumber, password, "seed-test")
		if err != nil {
			t.Fatalf("%s login: %v", test.schoolNumber, err)
		}
		if got := identity.LandingPath(session.Principal); got != test.landingPath {
			t.Fatalf("%s landing path = %s, want %s", test.schoolNumber, got, test.landingPath)
		}
	}
	var users int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM d_users WHERE school_number LIKE 'DEV-%'").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != len(cases) {
		t.Fatalf("development seed created %d users, want %d", users, len(cases))
	}
}

func hasSessionCookie(cookies []*http.Cookie) bool {
	for _, cookie := range cookies {
		if cookie.Name == identity.CookieName && cookie.Value != "" && cookie.MaxAge != -1 {
			return true
		}
	}
	return false
}

func TestLoginRateLimitIgnoresForwardedFor(t *testing.T) {
	h := setupIdentity(t)
	if _, err := h.store.Register(context.Background(), "20269999", "测试学生", "student-pass-1", "setup"); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 5; attempt++ {
		status, raw, _ := h.request(http.MethodPost, "/api/v1/auth/login", map[string]string{
			"schoolNumber": "20269999", "password": "wrong-password",
		}, map[string]string{"X-Forwarded-For": fmt.Sprintf("203.0.113.%d", attempt)})
		if status != http.StatusUnauthorized || errorCode(raw) != "INVALID_CREDENTIALS" {
			t.Fatalf("failed login %d: %d %s", attempt, status, raw)
		}
	}
	status, raw, _ := h.request(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"schoolNumber": "20269999", "password": "wrong-password",
	}, map[string]string{"X-Forwarded-For": "203.0.113.99"})
	if status != http.StatusTooManyRequests || errorCode(raw) != "RATE_LIMITED" {
		t.Fatalf("forwarded-for bypassed rate limit: %d %s", status, raw)
	}
}

func TestLoginRegisterRejectCrossSiteAndNonJSON(t *testing.T) {
	h := setupIdentity(t)
	if _, err := h.store.Register(context.Background(), "20269999", "测试学生", "student-pass-1", "setup"); err != nil {
		t.Fatal(err)
	}
	status, raw, cookies := h.request(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"schoolNumber": "20269999", "password": "student-pass-1",
	}, map[string]string{"Content-Type": "text/plain"})
	if status != http.StatusBadRequest || errorCode(raw) != "INVALID_INPUT" || hasSessionCookie(cookies) {
		t.Fatalf("text/plain login: %d %s cookies=%v", status, raw, cookies)
	}
	status, raw, cookies = h.request(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"schoolNumber": "20269999", "password": "student-pass-1",
	}, map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"})
	if status != http.StatusForbidden || errorCode(raw) != "CSRF_INVALID" || hasSessionCookie(cookies) {
		t.Fatalf("cross-site login: %d %s cookies=%v", status, raw, cookies)
	}
	status, raw, cookies = h.request(http.MethodPost, "/api/v1/auth/register", map[string]any{
		"schoolNumber": "open-1", "name": "跨站学生",
		"password": "student-pass-1", "confirmPassword": "student-pass-1",
	}, map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"})
	if status != http.StatusForbidden || errorCode(raw) != "CSRF_INVALID" || hasSessionCookie(cookies) {
		t.Fatalf("cross-site register: %d %s cookies=%v", status, raw, cookies)
	}
	status, raw, cookies = h.request(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"schoolNumber": "20269999", "password": "student-pass-1",
	}, map[string]string{
		"Origin":         "http://127.0.0.1:5173",
		"Sec-Fetch-Site": "same-origin",
	})
	if status != http.StatusOK || !hasSessionCookie(cookies) {
		t.Fatalf("vite-proxied same-origin login: %d %s", status, raw)
	}
}
