package api

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/groovarr/groovarr/backend/internal/config"
)

// authMiddleware checks for Basic Auth credentials and validates them
// against the configured AUTH_USERNAME and AUTH_PASSWORD environment variables.
// AuthMiddleware is the exported version for wiring in main.go.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this request path starts with /api/
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Look for Authorization header
		auth := r.Header.Get("Authorization")
		if auth == "" {
			// No auth header — challenge with Basic Auth
			w.Header().Set("WWW-Authenticate", `Basic realm="Groovarr API"`)
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		// Parse Basic auth credentials
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Basic" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Groovarr API"`)
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		decoded, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="Groovarr API"`)
			http.Error(w, "invalid authorization header", http.StatusUnauthorized)
			return
		}

		cred := strings.SplitN(string(decoded), ":", 2)
		if len(cred) != 2 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Groovarr API"`)
			http.Error(w, "invalid authorization header", http.StatusUnauthorized)
			return
		}

		username := cred[0]
		password := cred[1]

		// Validate against config
		cfg := config.Load()
		if cfg.AuthUsername == "" || cfg.AuthPassword == "" {
			// No auth configured — allow through
			next.ServeHTTP(w, r)
			return
		}

		if username != cfg.AuthUsername || password != cfg.AuthPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="Groovarr API"`)
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}