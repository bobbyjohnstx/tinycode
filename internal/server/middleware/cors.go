package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

type CORSConfig struct {
	AllowOrigins    []string
	AllowOriginFunc func(origin string) bool
	AllowMethods    []string
	AllowHeaders    []string
	MaxAge          int
}

// serverOrigins returns the exact CORS origins for a server bound to listenAddr
// (host:port). It includes both localhost and 127.0.0.1 variants so the web UI
// works regardless of which name the browser uses.
func serverOrigins(listenAddr string) []string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return nil
	}
	origins := []string{fmt.Sprintf("http://%s:%s", host, port)}
	if host == "127.0.0.1" {
		origins = append(origins, fmt.Sprintf("http://localhost:%s", port))
	} else if host == "localhost" {
		origins = append(origins, fmt.Sprintf("http://127.0.0.1:%s", port))
	}
	return origins
}

// DefaultCORSConfig returns a CORS configuration that allows only the exact
// origin matching the server's listen address. This prevents localhost CSRF
// where a malicious page on another port makes credentialed requests.
func DefaultCORSConfig(listenAddr string) CORSConfig {
	return CORSConfig{
		AllowOrigins: serverOrigins(listenAddr),
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Content-Type", "Authorization", "X-Request-ID"},
		MaxAge:       86400,
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
