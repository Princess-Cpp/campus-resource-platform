package identity

import (
	"net/http"
	"strings"
)

// Credential returns a browser session cookie when present; otherwise it
// returns a Bearer credential used by explicitly authorized internal clients.
func Credential(request *http.Request) (string, bool) {
	if cookie, err := request.Cookie(CookieName); err == nil && cookie.Value != "" {
		return cookie.Value, true
	}
	header := request.Header.Get("Authorization")
	token := strings.TrimPrefix(header, "Bearer ")
	return token, token != "" && token != header
}
