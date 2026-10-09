package middleware

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

const authCookieName = "tinycode_auth"

// AuthMode selects the HTML recovery copy for browser unauthorized responses.
type AuthMode int

const (
	// AuthModeAPI is headless serve — no SPA; point users at tinycode web.
	AuthModeAPI AuthMode = iota
	// AuthModeWeb is tinycode web — tell users to reopen the startup auth URL.
	AuthModeWeb
)

// TokenAuth returns middleware that validates auth (API-mode recovery HTML).
func TokenAuth(token string) func(http.Handler) http.Handler {
	return TokenAuthMode(token, AuthModeAPI)
}

// TokenAuthMode is like TokenAuth with an explicit recovery-page mode.
func TokenAuthMode(token string, mode AuthMode) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			if MatchesToken(r.Header.Get("Authorization"), token) {
				next.ServeHTTP(w, r)
				return
			}

			// ?auth_token= — set cookie, then redirect to strip token from URL.
			if qt := r.URL.Query().Get("auth_token"); qt != "" {
				if MatchesToken("Basic "+qt, token) {
					setAuthCookie(w, r, qt)
					cleanQuery := r.URL.Query()
					cleanQuery.Del("auth_token")
					cleanURL := *r.URL
					cleanURL.RawQuery = cleanQuery.Encode()
					http.Redirect(w, r, cleanURL.String(), http.StatusFound)
					return
				}
			}

			if c, err := r.Cookie(authCookieName); err == nil {
				if MatchesToken("Basic "+c.Value, token) {
					next.ServeHTTP(w, r)
					return
				}
			}

			WriteUnauthorized(w, r, mode)
		})
	}
}

// IsAuthenticated reports whether the request carries a valid token
// (Bearer, Basic, ?auth_token=, or cookie). Empty server token means auth is off.
func IsAuthenticated(r *http.Request, token string) bool {
	if token == "" {
		return true
	}
	if MatchesToken(r.Header.Get("Authorization"), token) {
		return true
	}
	if qt := r.URL.Query().Get("auth_token"); qt != "" && MatchesToken("Basic "+qt, token) {
		return true
	}
	if c, err := r.Cookie(authCookieName); err == nil && MatchesToken("Basic "+c.Value, token) {
		return true
	}
	return false
}

// SetAuthCookieIfValid sets the session cookie when ?auth_token= matches.
// Returns true when the query token was valid (cookie set).
func SetAuthCookieIfValid(w http.ResponseWriter, r *http.Request, token string) bool {
	if token == "" {
		return false
	}
	qt := r.URL.Query().Get("auth_token")
	if qt == "" || !MatchesToken("Basic "+qt, token) {
		return false
	}
	setAuthCookie(w, r, qt)
	return true
}

func setAuthCookie(w http.ResponseWriter, r *http.Request, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

// WriteUnauthorized writes JSON or HTML 401 depending on the client's Accept header.
func WriteUnauthorized(w http.ResponseWriter, r *http.Request, mode AuthMode) {
	if wantsHTML(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(unauthorizedHTML(mode)))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}

func wantsHTML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return false
	}
	htmlIdx := strings.Index(accept, "text/html")
	if htmlIdx < 0 {
		return false
	}
	jsonIdx := strings.Index(accept, "application/json")
	if jsonIdx >= 0 && jsonIdx < htmlIdx {
		return false
	}
	return true
}

func unauthorizedHTML(mode AuthMode) string {
	var body string
	switch mode {
	case AuthModeWeb:
		body = `<h1>Authentication required</h1>
<p>This is the tinycode web UI, but this browser is not authenticated.</p>
<ul>
<li>Run <code>tinycode web</code> again — it opens a URL that sets your session cookie.</li>
<li>Use the same host as the server (<code>127.0.0.1</code> vs <code>localhost</code> are different cookies).</li>
</ul>`
	default:
		body = `<h1>Authentication required</h1>
<p><code>tinycode serve</code> exposes a JSON API plus a small ops console (status, doctor, models, sessions).</p>
<ul>
<li>Open the <code>?auth_token=…</code> URL printed in the terminal when <code>tinycode serve</code> starts (sets a session cookie). The log file stores only a truncated token.</li>
<li>API clients: send <code>Authorization: Bearer &lt;token&gt;</code>.</li>
<li>Full chat UI: run <code>tinycode web</code> (separate command).</li>
</ul>`
	}
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>tinycode — authentication</title>
<style>
body{font-family:system-ui,sans-serif;max-width:36rem;margin:2rem auto;padding:0 1rem;line-height:1.5;color:#111;background:#fafafa}
code{background:#eee;padding:0.1em 0.35em;border-radius:3px}
h1{font-size:1.25rem}
</style>
</head>
<body>
` + body + `
</body>
</html>
`
}

// MatchesToken reports whether Authorization matches the expected token.
func MatchesToken(auth, token string) bool {
	if strings.HasPrefix(auth, "Bearer ") && subtle.ConstantTimeCompare([]byte(auth[7:]), []byte(token)) == 1 {
		return true
	}
	if !strings.HasPrefix(auth, "Basic ") {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(auth[6:])
	if err != nil {
		return false
	}
	_, password, ok := strings.Cut(string(decoded), ":")
	return ok && subtle.ConstantTimeCompare([]byte(password), []byte(token)) == 1
}
