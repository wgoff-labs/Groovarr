package api

import (
	"encoding/json"
	"net/http"

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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(artists)

	case http.MethodPost:
		var req struct {
			Name       string `json:"name"`
			RootFolder string `json:"root_folder"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		addedBy := "manual"
		id, err := store.ArtistAdd(req.Name, "", 0, req.RootFolder, addedBy)
		if err != nil {
			http.Error(w, "failed to add artist: "+err.Error(), http.StatusInternalServerError)
			return
		}
		artist, err := store.ArtistGetByID(id)
		if err != nil {
			http.Error(w, "artist added but failed to fetch: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(artist)

	case http.MethodDelete:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		artist, err := store.ArtistGet(req.Name)
		if err != nil || artist == nil {
			http.Error(w, "artist not found", http.StatusNotFound)
			return
		}
		if err := store.ArtistDelete(artist.ID); err != nil {
			http.Error(w, "failed to remove artist: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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
		http.Error(w, "Lidarr not connected: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	folders, err := c.GetRootFolders()
	if err != nil {
		http.Error(w, "Lidarr unreachable: "+err.Error(), http.StatusBadGateway)
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
		http.Error(w, "Lidarr not connected: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	profiles, err := c.GetQualityProfiles()
	if err != nil {
		http.Error(w, "Lidarr unreachable: "+err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profiles)
}

// CheckHandler triggers a manual popularity check.
func CheckHandler(w http.ResponseWriter, r *http.Request) {
	artist := r.URL.Query().Get("artist")
	results, err := core.RunDailyCheck(artist, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// ScanHandler triggers a full catalog scan for an artist.
func ScanHandler(w http.ResponseWriter, r *http.Request) {
	artist := r.URL.Query().Get("artist")
	results, err := core.RunDailyCheck(artist, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.SettingUpdate(req.Key, req.Value); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		if track == "" {
			http.Error(w, "track required", http.StatusBadRequest)
			return
		}
		if err := store.NeverPruneDelete(a.ID, album, track); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
