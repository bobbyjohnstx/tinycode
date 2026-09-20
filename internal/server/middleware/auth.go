package middleware

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

const authCookieName = "tinycode_auth"

// TokenAuth returns middleware that validates auth on every request.
// Accepts "Bearer <token>", "Basic <base64(user:token)>", ?auth_token= query
// param, and a tinycode_auth cookie. When auth_token is in the URL, a
// session cookie is set so the browser stays authenticated on reload.
// If token is empty, auth is disabled (pass-through). OPTIONS requests are
// always allowed through for CORS preflight support.
func TokenAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// 1. Authorization header
			if MatchesToken(r.Header.Get("Authorization"), token) {
				next.ServeHTTP(w, r)
				return
			}

			// 2. ?auth_token= query param — set cookie for future requests
			if qt := r.URL.Query().Get("auth_token"); qt != "" {
				if MatchesToken("Basic "+qt, token) {
					http.SetCookie(w, &http.Cookie{
						Name:     authCookieName,
						Value:    qt,
						Path:     "/",
						HttpOnly: true,
						SameSite: http.SameSiteStrictMode,
					})
					next.ServeHTTP(w, r)
					return
				}
			}

			// 3. Cookie fallback
			if c, err := r.Cookie(authCookieName); err == nil {
				if MatchesToken("Basic "+c.Value, token) {
					next.ServeHTTP(w, r)
					return
				}
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		})
	}
}

func MatchesToken(auth, token string) bool {
	if auth == "Bearer "+token {
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
	return ok && password == token
}
