package clients

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestLidarrServer spins up a minimal HTTP server that mimics the Lidarr
// API surface we exercise from tests. It validates the X-Api-Key header
// (so we catch accidental auth regressions) and serves the provided JSON
// for the exact path it was registered with. All other paths 404 —
// deliberately, so a typo in a test's expected URL surfaces immediately
// instead of being silently answered by some unrelated handler.
func newTestLidarrServer(t *testing.T, path, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != path {
			t.Errorf("unexpected request URI: got %q, want %q", r.URL.RequestURI(), path)
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("X-Api-Key"); got != "test-key" {
			t.Errorf("missing or wrong X-Api-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
}
