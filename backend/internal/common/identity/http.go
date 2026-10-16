package identity

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type HTTP struct {
	Store *Store
}

func (h *HTTP) Register(router *gin.Engine) {
	auth := router.Group("/api/v1/auth")
	auth.POST("/register", func(c *gin.Context) {
		if err := rejectCrossSiteAuth(c); err != nil {
			h.reply(c, nil, err)
			return
		}
		var input struct {
			SchoolNumber    string `json:"schoolNumber"`
			Name            string `json:"name"`
			Password        string `json:"password"`
			ConfirmPassword string `json:"confirmPassword"`
		}
		if c.ShouldBindJSON(&input) != nil || input.Password != input.ConfirmPassword {
			h.reply(c, nil, fault("INVALID_INPUT", http.StatusBadRequest))
			return
		}
		session, err := h.Store.Register(c.Request.Context(), input.SchoolNumber, input.Name, input.Password, c.ClientIP())
		if err != nil {
			h.reply(c, nil, err)
			return
		}
		h.setCookie(c, session.Token, session.ExpiresAt)
		h.reply(c, sessionData(session), nil)
	})
	auth.POST("/login", func(c *gin.Context) {
		if err := rejectCrossSiteAuth(c); err != nil {
			h.reply(c, nil, err)
			return
		}
		var input struct {
			SchoolNumber string `json:"schoolNumber"`
			Password     string `json:"password"`
		}
		if c.ShouldBindJSON(&input) != nil {
			h.reply(c, nil, fault("INVALID_INPUT", http.StatusBadRequest))
			return
		}
		session, err := h.Store.Login(c.Request.Context(), input.SchoolNumber, input.Password, c.ClientIP())
		if err != nil {
			h.reply(c, nil, err)
			return
		}
		h.setCookie(c, session.Token, session.ExpiresAt)
		h.reply(c, sessionData(session), nil)
	})
	auth.GET("/session", func(c *gin.Context) {
		session, err := h.Store.AuthenticateRequest(c.Request.Context(), c.Request)
		h.reply(c, sessionData(session), err)
	})
	auth.POST("/logout", func(c *gin.Context) {
		session, err := h.Store.AuthenticateRequest(c.Request.Context(), c.Request)
		if err != nil {
			h.reply(c, nil, err)
			return
		}
		if !csrfMatches(c.GetHeader("X-CSRF-Token"), session.CSRFToken) {
			h.reply(c, nil, fault("CSRF_INVALID", http.StatusForbidden))
			return
		}
		if err = h.Store.Logout(c.Request.Context(), session.Token); err != nil {
			h.reply(c, nil, err)
			return
		}
		h.clearCookie(c)
		h.reply(c, gin.H{"loggedOut": true}, nil)
	})
}

func sessionData(session Session) SessionData {
	return SessionData{
		Actor:       session.Principal,
		CSRFToken:   session.CSRFToken,
		ExpiresAt:   session.ExpiresAt,
		LandingPath: LandingPath(session.Principal),
	}
}

func (h *HTTP) setCookie(c *gin.Context, token string, expiresAt time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   max(0, int(time.Until(expiresAt).Seconds())),
		HttpOnly: true,
		Secure:   h.Store.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *HTTP) clearCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.Store.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func csrfMatches(got, want string) bool {
	return got != "" && len(got) == len(want) && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func jsonContentType(value string) bool {
	media := strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	return media == "application/json"
}

func hostOf(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	return strings.ToLower(parsed.Host), true
}

func rejectCrossSiteAuth(c *gin.Context) error {
	if !jsonContentType(c.GetHeader("Content-Type")) {
		return fault("INVALID_INPUT", http.StatusBadRequest)
	}
	site := strings.ToLower(c.GetHeader("Sec-Fetch-Site"))
	if site == "cross-site" {
		return fault("CSRF_INVALID", http.StatusForbidden)
	}
	// Vite 把浏览器同源 /api 代理到 Go 后 Host 变成后端端口；
	// 浏览器已标明 same-origin/same-site 时不再用 Host 对比 Origin。
	if site == "same-origin" || site == "same-site" || site == "none" {
		return nil
	}
	if origin := c.GetHeader("Origin"); origin != "" {
		host, ok := hostOf(origin)
		if !ok || host != strings.ToLower(c.Request.Host) {
			return fault("CSRF_INVALID", http.StatusForbidden)
		}
	}
	if referer := c.GetHeader("Referer"); referer != "" {
		host, ok := hostOf(referer)
		if !ok || host != strings.ToLower(c.Request.Host) {
			return fault("CSRF_INVALID", http.StatusForbidden)
		}
	}
	return nil
}

func (h *HTTP) CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions ||
			c.Request.URL.Path == "/api/v1/auth/login" || c.Request.URL.Path == "/api/v1/auth/register" {
			c.Next()
			return
		}
		cookie, err := c.Request.Cookie(CookieName)
		if err != nil || cookie.Value == "" {
			c.Next()
			return
		}
		session, err := h.Store.Authenticate(c.Request.Context(), cookie.Value)
		if err != nil {
			h.reply(c, nil, err)
			c.Abort()
			return
		}
		if !csrfMatches(c.GetHeader("X-CSRF-Token"), session.CSRFToken) {
			h.reply(c, nil, fault("CSRF_INVALID", http.StatusForbidden))
			c.Abort()
			return
		}
		c.Next()
	}
}

func (h *HTTP) reply(c *gin.Context, data any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"data": data, "correlationId": c.GetString("trace")})
		return
	}
	status, code := http.StatusInternalServerError, "INTERNAL_ERROR"
	var target *Fault
	if errors.As(err, &target) {
		status, code = target.Status, target.Code
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code}, "correlationId": c.GetString("trace")})
}
