package middleware

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type CORSConfig struct {
	AllowOrigins    []string
	AllowOriginFunc func(origin string) bool
	AllowMethods    []string
	AllowHeaders    []string
	MaxAge          int
}

// isLocalhostOrigin returns true if the origin is http://localhost or
// http://127.0.0.1 on any port.
func isLocalhostOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1"
}

func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOriginFunc: isLocalhostOrigin,
		AllowMethods:    []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:    []string{"Content-Type", "Authorization", "X-Request-ID"},
		MaxAge:          86400,
	}
}

// isOriginAllowed checks whether the given origin is permitted by the CORS config.
func isOriginAllowed(cfg CORSConfig, origin string) bool {
	if cfg.AllowOriginFunc != nil && cfg.AllowOriginFunc(origin) {
		return true
	}
	for _, o := range cfg.AllowOrigins {
		if o == "*" || o == origin {
			return true
		}
	}
	return false
}

func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	methods := strings.Join(cfg.AllowMethods, ", ")
	headers := strings.Join(cfg.AllowHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || !isOriginAllowed(cfg, origin) {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", methods)
			w.Header().Set("Access-Control-Allow-Headers", headers)
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if r.Method != http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			if cfg.MaxAge > 0 {
				w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", cfg.MaxAge))
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}
