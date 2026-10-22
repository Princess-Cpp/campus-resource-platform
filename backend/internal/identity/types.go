package identity

import "time"

const CookieName = "campus_session"

type Principal struct {
	ID           string   `json:"id"`
	SchoolNumber string   `json:"schoolNumber"`
	Name         string   `json:"name"`
	Department   string   `json:"department"`
	Roles        []string `json:"roles"`
	Permissions  []string `json:"permissions"`
	Areas        []string `json:"areas"`
	ContactRefs  []string `json:"contactRefs"`
}

func (p Principal) Can(permission string) bool {
	for _, value := range p.Permissions {
		if value == permission {
			return true
		}
	}
	return false
}

type Session struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
	Principal Principal
}

type SessionData struct {
	Actor       Principal `json:"actor"`
	CSRFToken   string    `json:"csrfToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
	LandingPath string    `json:"landingPath"`
}

type Fault struct {
	Code   string
	Status int
}

func (f *Fault) Error() string { return f.Code }

func fault(code string, status int) error { return &Fault{Code: code, Status: status} }

func LandingPath(p Principal) string {
	if p.Can("approval.department") || p.Can("approval.final") {
		return "/m2/approvals"
	}
	if p.Can("classroom.stats") {
		return "/m2/stats"
	}
	if p.Can("booking.create") || p.Can("classroom.apply") {
		return "/m1/search"
	}
	return "/"
}
