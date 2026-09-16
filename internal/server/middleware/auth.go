package middleware

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// TokenAuth returns middleware that validates auth on every request.
// Accepts both "Bearer <token>" and "Basic <base64(user:token)>" formats.
// If token is empty, auth is disabled (pass-through). OPTIONS requests are
// always allowed through for CORS preflight support.
func TokenAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			if matchesToken(r.Header.Get("Authorization"), token) {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		})
	}
}

func matchesToken(auth, token string) bool {
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
