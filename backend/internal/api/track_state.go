package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"strconv"

	"github.com/groovarr/groovarr/backend/internal/store"
)

// TrackStateHandler handles POST /api/artist/:artistId/track/:lidarrTrackId/state
func TrackStateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get artist ID and track ID from URL: /api/artist/{artistId}/track/{lidarrTrackId}/state
	path := r.URL.Path
	// Expected format: /api/artist/{artistId}/track/{lidarrTrackId}/state
	const prefix = "/api/artist/"
	const middle = "/track/"
	const suffix = "/state"
	if !hasPrefixTS(path, prefix) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	// Use splitPath to extract parts

	parts := splitPath(path)
	// Expected: ["api", "artist", artistId, "track", lidarrTrackId, "state"]
	if len(parts) != 6 || parts[0] != "api" || parts[1] != "artist" || parts[3] != "track" || parts[5] != "state" {
		http.Error(w, "Invalid artist track state path", http.StatusBadRequest)
		return
	}
	artistID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		http.Error(w, "Invalid artist ID", http.StatusBadRequest)
		return
	}
	lidarrTrackID, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil {
		http.Error(w, "Invalid track ID", http.StatusBadRequest)
		return
	}

	// Decode request body
	var req struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	// Validate state — allow empty string to reset to auto (auto = no preference)
	validStates := map[string]bool{"keep": true, "hit": true, "not_keep": true, "": true}
	if !validStates[req.State] {
		http.Error(w, "Invalid state. Must be 'keep', 'hit', 'not_keep', or empty (auto)", http.StatusBadRequest)
		return
	}

	// Reset to auto: delete the preference row entirely
	if req.State == "" {
		if err := store.DeleteTrackPreference(artistID, lidarrTrackID); err != nil {
			http.Error(w, "Failed to clear track preference", http.StatusInternalServerError)
			return
		}
	} else {
		if err := store.UpsertTrackPreference(artistID, lidarrTrackID, req.State, 0); err != nil {
			http.Error(w, "Failed to set track preference", http.StatusInternalServerError)
			return
		}
	}

	// Return success
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// splitPath splits a path by '/' and returns the segments.
func splitPath(path string) []string {
	return filter(strings.Split(path, "/"), func(s string) bool { return s != "" })
}

// filter returns a slice containing only the elements of s that satisfy f.
func filter(s []string, f func(string) bool) []string {
	var r []string
	for _, v := range s {
		if f(v) {
			r = append(r, v)
		}
	}
	return r
}

// hasPrefix and hasSuffix are simple helper functions.
func hasPrefixTS(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
func hasSuffixTS(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}