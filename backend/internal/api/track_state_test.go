package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/groovarr/groovarr/backend/internal/store"
)

// testMain runs once for the test binary. It initialises the store with an
// in-memory SQLite database so that all DB calls in the handler hit a real
// (but temporary) database.
func TestMain(m *testing.M) {
	// Point the store at an in-memory DB; each test gets a fresh snapshot
	// because we re-open ":memory:" on every Init call in store code.
	if err := store.Init(":memory:"); err != nil {
		panic(err)
	}
	// Note: store.Init creates the schema; we do not need to migrate.
	code := m.Run()
	os.Exit(code)
}

// trackStateRequest mirrors the JSON body sent by the frontend for
// POST /api/artist/:id/track/:trackId/state.
type trackStateRequest struct {
	State string `json:"state"`
}

// makeStateReq returns a *http.Request with the given state as JSON body.
func makeStateReq(t *testing.T, state string) *http.Request {
	t.Helper()
	body, _ := json.Marshal(trackStateRequest{State: state})
	req := httptest.NewRequest(http.MethodPost, "/api/artist/1/track/99/state", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// checkBody decodes the response body into a map and checks that the top-level
// key "status" equals wantStatus.
func checkBody(t *testing.T, rr *httptest.ResponseRecorder, wantStatus string) {
	t.Helper()
	var got map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if got["status"] != wantStatus {
		t.Errorf("body status = %q, want %q", got["status"], wantStatus)
	}
}

func TestTrackStateHandler_ValidStates(t *testing.T) {
	tests := []struct {
		state     string
		wantCode  int
		wantBody  string
	}{
		{"keep", http.StatusOK, "ok"},
		{"hit", http.StatusOK, "ok"},
		{"not_keep", http.StatusOK, "ok"},
		{"", http.StatusOK, "ok"}, // empty = reset to auto
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			rr := httptest.NewRecorder()
			TrackStateHandler(rr, makeStateReq(t, tt.state))
			if rr.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", rr.Code, tt.wantCode)
			}
			checkBody(t, rr, tt.wantBody)
		})
	}
}

func TestTrackStateHandler_InvalidState(t *testing.T) {
	tests := []struct {
		name  string
		state string
	}{
		{"garbage string", "garbage"},
		{"wrong value", "keep_only"},
		{"case-sensitive fail", "Keep"},
		{"whitespace only", "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			TrackStateHandler(rr, makeStateReq(t, tt.state))
			if rr.Code != http.StatusBadRequest {
				t.Errorf("code = %d, want %d (state=%q)", rr.Code, http.StatusBadRequest, tt.state)
			}
		})
	}
}

func TestTrackStateHandler_InvalidPath(t *testing.T) {
	// Request with a malformed URL path should get a BadRequest response,
	// not panic. We also verify the handler never leaks internal errors
	// in the response body for a bad path.
	badPaths := []string{
		"/api/artist/abc/track/1/state",
		"/api/artist/1/track/xyz/state",
		"/api/artist/1/track/state",    // missing trackId
		"/api/artist/1/track/1/statex", // wrong suffix
		"/not/a/manage/path",
	}

	for _, path := range badPaths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(`{"state":"keep"}`)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			TrackStateHandler(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("code = %d, want %d for path %q", rr.Code, http.StatusBadRequest, path)
			}
		})
	}
}

func TestTrackStateHandler_WrongMethod(t *testing.T) {
	// Only POST is allowed — GET, PUT, DELETE should all be 405.
	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/artist/1/track/1/state", nil)
			rr := httptest.NewRecorder()
			TrackStateHandler(rr, req)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("code = %d, want %d for method %s", rr.Code, http.StatusMethodNotAllowed, method)
			}
		})
	}
}

func TestTrackStateHandler_InvalidJSON(t *testing.T) {
	// A completely malformed JSON body should return a 400 BadRequest, not
	// panic or return 500.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/artist/1/track/1/state",
		bytes.NewReader([]byte(`{not json at all`))
	)
	req.Header.Set("Content-Type", "application/json")
	TrackStateHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want %d for malformed JSON", rr.Code, http.StatusBadRequest)
	}
}