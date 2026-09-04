package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/groovarr/groovarr/backend/internal/config"
)

// makeRequest creates a test request to the given path with no auth header.
func makeRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, path, nil)
}

// TestAuthMiddleware_NoAuthConfigured_AllowsThrough verifies that when no
// AUTH_USERNAME/AUTH_PASSWORD are configured, the middleware passes requests
// through without challenge.
func TestAuthMiddleware_NoAuthConfigured_AllowsThrough(t *testing.T) {
	// Check if auth is already configured from a previous test.
	cfg := config.Load()
	if cfg.AuthUsername != "" || cfg.AuthPassword != "" {
		t.Skip("auth already configured — skipping no-auth test in this process")
	}

	os.Unsetenv("AUTH_USERNAME")
	os.Unsetenv("AUTH_PASSWORD")
	config.Reset()

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(next)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, makeRequest(t, "/api/artists"))

	if !called {
		t.Error("next handler was not called when no auth is configured")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// TestAuthMiddleware_WithAuthConfigured_NoCredentials_Challenges verifies that
// when AUTH is configured, requests without credentials receive a 401.
func TestAuthMiddleware_WithAuthConfigured_NoCredentials_Challenges(t *testing.T) {
	os.Setenv("AUTH_USERNAME", "admin")
	os.Setenv("AUTH_PASSWORD", "secret")
	config.Reset()
	t.Cleanup(func() {
		os.Unsetenv("AUTH_USERNAME")
		os.Unsetenv("AUTH_PASSWORD")
		config.Reset()
	})

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(next)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, makeRequest(t, "/api/artists"))

	if called {
		t.Error("next handler should not be called when credentials are missing")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestAuthMiddleware_WithAuthConfigured_BadCredentials_Challenges verifies that
// invalid credentials result in a 401.
func TestAuthMiddleware_WithAuthConfigured_BadCredentials_Challenges(t *testing.T) {
	os.Setenv("AUTH_USERNAME", "admin")
	os.Setenv("AUTH_PASSWORD", "secret")
	config.Reset()
	t.Cleanup(func() {
		os.Unsetenv("AUTH_USERNAME")
		os.Unsetenv("AUTH_PASSWORD")
		config.Reset()
	})

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(next)
	rr := httptest.NewRecorder()
	req := makeRequest(t, "/api/artists")
	req.SetBasicAuth("wrong", "creds")
	handler.ServeHTTP(rr, req)

	if called {
		t.Error("next handler should not be called with bad credentials")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestAuthMiddleware_WithAuthConfigured_GoodCredentials_AllowsThrough
// verifies that valid credentials pass through to the next handler.
func TestAuthMiddleware_WithAuthConfigured_GoodCredentials_AllowsThrough(t *testing.T) {
	os.Setenv("AUTH_USERNAME", "admin")
	os.Setenv("AUTH_PASSWORD", "secret")
	config.Reset()
	t.Cleanup(func() {
		os.Unsetenv("AUTH_USERNAME")
		os.Unsetenv("AUTH_PASSWORD")
		config.Reset()
	})

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(next)
	rr := httptest.NewRecorder()
	req := makeRequest(t, "/api/artists")
	req.SetBasicAuth("admin", "secret")
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("next handler should be called with valid credentials")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// TestAuthMiddleware_NonAPIPath_Bypasses verifies that non-API routes
// (e.g. /) bypass the auth check entirely.
func TestAuthMiddleware_NonAPIPath_Bypasses(t *testing.T) {
	os.Setenv("AUTH_USERNAME", "admin")
	os.Setenv("AUTH_PASSWORD", "secret")
	config.Reset()
	t.Cleanup(func() {
		os.Unsetenv("AUTH_USERNAME")
		os.Unsetenv("AUTH_PASSWORD")
		config.Reset()
	})

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(next)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, makeRequest(t, "/"))

	if !called {
		t.Error("next handler should be called for non-API paths")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// TestAuthMiddleware_AppliesToAllHandlers verifies the middleware correctly
// guards the handlers listed in the P0 task: ArtistImport, ArtistManage,
// HitFallen, TrackState.
func TestAuthMiddleware_AppliesToAllHandlers(t *testing.T) {
	os.Setenv("AUTH_USERNAME", "admin")
	os.Setenv("AUTH_PASSWORD", "secret")
	config.Reset()
	t.Cleanup(func() {
		os.Unsetenv("AUTH_USERNAME")
		os.Unsetenv("AUTH_PASSWORD")
		config.Reset()
	})

	// These are the API paths from the listed handlers.
	apiPaths := []string{
		"/api/artists/import",         // ArtistImportHandler
		"/api/artist/1/manage",        // ArtistManageHandler
		"/api/hit-fallen",             // HitFallenHandler
		"/api/artist/1/track/2/state", // TrackStateHandler
		"/api/artists/import/bulk",    // ArtistImportBulkHandler
	}

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range apiPaths {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := makeRequest(t, path)
			// No auth header — should be challenged.
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Errorf("path %q: expected 401 without credentials, got %d", path, rr.Code)
			}

			// Now with valid credentials — should pass through.
			rr2 := httptest.NewRecorder()
			req2 := makeRequest(t, path)
			req2.SetBasicAuth("admin", "secret")
			handler.ServeHTTP(rr2, req2)
			if rr2.Code != http.StatusOK {
				t.Errorf("path %q: expected 200 with valid credentials, got %d", path, rr2.Code)
			}
		})
	}
}

// TestAuthMiddleware_InvalidAuthHeader_Challenges verifies that malformed
// Authorization headers (not Basic) get a 401.
func TestAuthMiddleware_InvalidAuthHeader_Challenges(t *testing.T) {
	os.Setenv("AUTH_USERNAME", "admin")
	os.Setenv("AUTH_PASSWORD", "secret")
	config.Reset()
	t.Cleanup(func() {
		os.Unsetenv("AUTH_USERNAME")
		os.Unsetenv("AUTH_PASSWORD")
		config.Reset()
	})

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name   string
		header string
	}{
		{"Bearer token", "Bearer some-token"},
		{"No scheme", "no-scheme-here"},
		{"Empty", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := makeRequest(t, "/api/artists")
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 for %q, got %d", tc.header, rr.Code)
			}
		})
	}
}
