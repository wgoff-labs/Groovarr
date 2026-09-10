package api

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/connections"
	"github.com/groovarr/groovarr/backend/internal/core"
	"github.com/groovarr/groovarr/backend/internal/discord"
	"github.com/groovarr/groovarr/backend/internal/store"
)

// ArtistHandler handles artist list (GET), add (POST), and remove (DELETE).
func ArtistHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		artists, err := store.ArtistList()
		if err != nil {
			http.Error(w, config.SanitizeError(err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(artists)

	case http.MethodPost:
		var req struct {
			Name       string `json:"name"`
			RootFolder string `json:"root_folder"`
		}
		if err := ValidateJSON(r, &req); err != nil {
			BadRequest(w, "invalid request: "+config.SanitizeError(err.Error()))
			return
		}
		if req.Name == "" {
			BadRequest(w, "name is required")
			return
		}
		addedBy := "manual"
		id, err := store.ArtistAdd(req.Name, "", 0, req.RootFolder, addedBy, 0)
		if err != nil {
			http.Error(w, "failed to add artist: "+config.SanitizeError(err.Error()), http.StatusInternalServerError)
			return
		}
		artist, err := store.ArtistGetByID(id)
		if err != nil {
			http.Error(w, "artist added but failed to fetch: "+config.SanitizeError(err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(artist)

	case http.MethodDelete:
		var req struct {
			Name string `json:"name"`
		}
		if err := ValidateJSON(r, &req); err != nil {
			BadRequest(w, "invalid request: "+config.SanitizeError(err.Error()))
			return
		}
		if req.Name == "" {
			BadRequest(w, "name is required")
			return
		}
		artist, err := store.ArtistGet(req.Name)
		if err != nil || artist == nil {
			http.Error(w, "artist not found", http.StatusNotFound)
			return
		}
		if err := store.ArtistDelete(artist.ID); err != nil {
			http.Error(w, "failed to remove artist: "+config.SanitizeError(err.Error()), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// CheckStatusHandler returns whether a check is currently running.
func CheckStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"running": core.CheckRunning(),
	})
}

// StatusHandler returns basic service status.
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "groovarr",
	})
}

// FoldersHandler returns all Lidarr root folders (scans Lidarr on every call).
// Also includes the env-allowed subset if LIDARR_ROOT_FOLDERS is set.
func FoldersHandler(w http.ResponseWriter, r *http.Request) {
	cm := connections.New()
	c, err := cm.GetLidarrClient()
	if err != nil {
		if WriteLidarrUnavailable(w, cm) {
			return
		}
		http.Error(w, "Lidarr not connected: "+config.SanitizeError(err.Error()), http.StatusServiceUnavailable)
		return
	}
	folders, err := c.GetRootFolders()
	if err != nil {
		http.Error(w, "Lidarr unreachable: "+config.SanitizeError(err.Error()), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(folders)
}

// ProfilesHandler returns all Lidarr quality profiles (scans Lidarr on every call).
func ProfilesHandler(w http.ResponseWriter, r *http.Request) {
	cm := connections.New()
	c, err := cm.GetLidarrClient()
	if err != nil {
		if WriteLidarrUnavailable(w, cm) {
			return
		}
		http.Error(w, "Lidarr not connected: "+config.SanitizeError(err.Error()), http.StatusServiceUnavailable)
		return
	}
	profiles, err := c.GetQualityProfiles()
	if err != nil {
		http.Error(w, "Lidarr unreachable: "+config.SanitizeError(err.Error()), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profiles)
}

// CheckHandler triggers a manual popularity check.
// GET /api/check?artist=Name&force=kill
// force=kill: stop any in-progress check and start this one immediately.
func CheckHandler(w http.ResponseWriter, r *http.Request) {
	artist := r.URL.Query().Get("artist")
	debug := r.URL.Query().Get("debug") == "1"
	force := r.URL.Query().Get("force")

	// Warn if a check is already running and this call isn't forcing.
	if core.CheckRunning() && force != "kill" {
		// We'll let it block and wait — that's the behavior.
		// Could also return 409 Conflict here, but blocking is simpler.
	}

	results, err := core.RunDailyCheck(artist, false, force)
	if err != nil {
		http.Error(w, config.SanitizeError(err.Error()), http.StatusInternalServerError)
		return
	}

	// Debug payload: top scored tracks (descending) for the artist, and
	// the few lowest scores so we can see the actual distribution in the UI.
	var dbg *DebugResult
	if debug {
		cfg := config.Load()
		a, _ := store.ArtistGet(artist)
		var scored []ScoredTrack
		var top []ScoredTrack
		var bottom []ScoredTrack
		if a != nil {
			tracks, _ := store.GetTrackPopularity(a.ID)
			for _, t := range tracks {
				scored = append(scored, ScoredTrack{
					LidarrTrackID: t.LidarrTrackID,
					PlayCount:     int64(t.PlayCount),
					Score:         t.PlayCount,
					Source:        t.TrackKey,
				})
			}
			sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
			n := len(scored)
			if n > 10 {
				top = scored[:10]
			} else {
				top = scored
			}
			if n > 10 {
				bottom = scored[n-10:]
			}
		}
		dbg = &DebugResult{
			Threshold:   cfg.PopularityThreshold,
			Mode:        cfg.DownloadMode,
			TrackCount:  len(scored),
			TopScores:   top,
			LowScores:   bottom,
		}
	}

	resp := CheckResponse{Results: results, Debug: dbg}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// CheckResponse wraps the result list with optional debug info.
type CheckResponse struct {
	Results []core.CheckResult `json:"results"`
	Debug   *DebugResult       `json:"debug,omitempty"`
}

type DebugResult struct {
	Threshold  int           `json:"threshold"`
	Mode       string        `json:"mode"`
	TrackCount int           `json:"track_count"`
	TopScores  []ScoredTrack `json:"top_scores"`
	LowScores  []ScoredTrack `json:"low_scores"`
}

type ScoredTrack struct {
	LidarrTrackID int64  `json:"lidarr_track_id"`
	PlayCount     int64  `json:"play_count"`
	Score         int    `json:"score"`
	Source        string `json:"source"`
}

// ScanHandler triggers a full catalog scan for an artist.
func ScanHandler(w http.ResponseWriter, r *http.Request) {
	artist := r.URL.Query().Get("artist")
	results, err := core.RunDailyCheck(artist, true, "")
	if err != nil {
		http.Error(w, config.SanitizeError(err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// PruneHandler triggers a prune operation.
func PruneHandler(w http.ResponseWriter, r *http.Request) {
	artist := r.URL.Query().Get("artist")
	force := r.URL.Query().Get("force") == "true"
	results, err := core.PruneDownloadedAlbums(artist, force)
	if err != nil {
		http.Error(w, config.SanitizeError(err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// SetupHandler handles the initial setup page.
// GET /api/setup → returns the setup form HTML or redirects if already configured
// POST /api/setup → saves auth credentials to the database and returns JSON success
func SetupHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Check if auth is already configured
		cfg := config.Load()
		if cfg.AuthUsername != "" && cfg.AuthPassword != "" {
			// Already configured, redirect to dashboard
			w.Header().Set("Location", "/")
			w.WriteHeader(http.StatusFound)
			return
		}
		// Return setup form
		setupHTML := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8>
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Groovarr Initial Setup</title>
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 0; padding: 20px; background: #0d1117; color: #e8e8e8; }
		.max-w-2xl { max-width: 24rem; margin auto; }
		.card { background: #161b22; border: 1px solid #30363d; border-radius: 8px; padding: 2rem; margin-bottom: 1rem; }
		input { width: 100%; padding: 0.5rem; margin-bottom: 1rem; background: #21262d; border: 1px solid #30363d; border-radius: 4px; color: #e8e8e8; }
		button { width: 100%; padding: 0.75rem; background: #e8e8e8; color: #0d1117; border: none; border-radius: 4px; font-weight: bold; cursor: pointer; }
		button:hover { background: #f0f0f0; }
		.error { color: #f04747; margin-bottom: 1rem; }
	</style>
</head>
<body>
	<div class="max-w-2xl mx-auto">
		<h1>Groovarr Initial Setup</h1>
		<p class="text-sm text-gray-600 mb-6">Set up your admin credentials to secure the Groovarr API and UI.</p>
		<div class="card">
			<h2>Admin Credentials</h2>
			<form method="POST" action="/api/setup">
				<div class="mb-3">
					<label class="text-sm text-gray-400 mb-1">Username</label>
					<input type="text" name="username" required autocomplete="username" />
				</div>
				<div class="mb-3">
					<label class="text-sm text-gray-400 mb-1">Password</label>
					<input type="password" name="password" required autocomplete="new-password" />
				</div>
				<button type="submit">Save Credentials and Continue</button>
			</form>
		</div>
	</div>
</body>
</html>`;
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(setupHTML))
	case http.MethodPost:
		// Parse the form data
		if err := r.ParseForm(); err != nil {
			http.Error(w, "failed to parse form", http.StatusBadRequest)
			return
		}
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "" || password == "" {
			http.Error(w, "username and password are required", http.StatusBadRequest)
			return
		}

		// Save credentials to the database
		err := store.SettingUpdate("auth_username", username)
		if err != nil {
			http.Error(w, "failed to save username: "+config.SanitizeError(err.Error()), http.StatusInternalServerError)
			return
		}
		err = store.SettingUpdate("auth_password", password)
		if err != nil {
			http.Error(w, "failed to save password: "+config.SanitizeError(err.Error()), http.StatusInternalServerError)
			return
		}

		// Return JSON success so the frontend can navigate to / (dashboard) properly
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok": true}`))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// SettingsHandler gets or sets simple settings.
func SettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "missing key", http.StatusBadRequest)
			return
		}
		val, _ := store.SettingGet(key)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"key": key, "value": val})
	case http.MethodPost:
		var req struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := ValidateJSON(r, &req); err != nil {
			BadRequest(w, config.SanitizeError(err.Error()))
			return
		}
		if err := store.SettingUpdate(req.Key, req.Value); err != nil {
			http.Error(w, config.SanitizeError(err.Error()), http.StatusInternalServerError)
			return
		}
		// Reload Discord bot settings if a discord-related key was saved
		if bot := discord.GetBot(); bot != nil {
			switch req.Key {
			case "discord_token", "discord_home_channel", "discord_allow_users",
				"discord_auto_thread",
				"discord_allowed_channels", "discord_allowed_users":
				bot.ReloadSettings()
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"key": req.Key, "value": req.Value})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// KeepHandler manages never-prune tracks.
// GET  ?artist=X&album=Y → list protected tracks for album
// POST ?artist=X&album=Y&track=Z → protect a track
// DELETE ?artist=X&album=Y&track=Z → unprotect a track
func KeepHandler(w http.ResponseWriter, r *http.Request) {
	artist := r.URL.Query().Get("artist")
	album := r.URL.Query().Get("album")
	track := r.URL.Query().Get("track")

	if artist == "" || album == "" {
		http.Error(w, "artist and album required", http.StatusBadRequest)
		return
	}

	a, err := store.ArtistGet(artist)
	if err != nil || a == nil {
		http.Error(w, "artist not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		tracks, _ := store.NeverPruneTracks(a.ID, album)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"artist": artist,
			"album":  album,
			"tracks": tracks,
		})
	case http.MethodPost:
		if track == "" {
			http.Error(w, "track required", http.StatusBadRequest)
			return
		}
		if err := store.NeverPruneInsert(a.ID, album, track); err != nil {
			http.Error(w, config.SanitizeError(err.Error()), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		if track == "" {
			http.Error(w, "track required", http.StatusBadRequest)
			return
		}
		if err := store.NeverPruneDelete(a.ID, album, track); err != nil {
			http.Error(w, config.SanitizeError(err.Error()), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// DownloadStatusHandler checks for completed downloads and auto-prunes them.
func DownloadStatusHandler(w http.ResponseWriter, r *http.Request) {
	results, err := core.CheckDownloads()
	if err != nil {
		http.Error(w, config.SanitizeError(err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
