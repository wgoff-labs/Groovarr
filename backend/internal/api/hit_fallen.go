package api

import (
	"encoding/json"
	"net/http"

	"github.com/wgoff-labs/Groovarr/backend/internal/connections"
	"github.com/wgoff-labs/Groovarr/backend/internal/store"
)

// HitFallenHandler handles GET /api/hit-fallen
func HitFallenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get connection manager to access Lidarr client
	mgr := connections.GetManager()
	lidarrClient := mgr.GetLidarrClient()
	if lidarrClient == nil {
		http.Error(w, "Lidarr not configured", http.StatusServiceUnavailable)
		return
	}

	// Fetch hit-fallen logs with artist and track popularity info
	rows, err := store.DB.Query(`
		SELECT hfl.id, hfl.score_at_fall, hfl.fallen_at, 
		       tp.artist_id, tp.lidarr_track_id, a.name as artist_name
		FROM hit_fallen_log hfl
		JOIN track_popularity tp ON hfl.track_popularity_id = tp.id
		JOIN artists a ON tp.artist_id = a.id
		ORDER BY hfl.fallen_at DESC
	`)
	if err != nil {
		http.Error(w, "Failed to fetch hit-fallen logs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []HitFallenResponse
	for rows.Next() {
		var id int64
		var scoreAtFall int64
		var fallenAt string
		var artistID int64
		var lidarrTrackID int64
		var artistName string
		if err := rows.Scan(&id, &scoreAtFall, &fallenAt, &artistID, &lidarrTrackID, &artistName); err != nil {
			http.Error(w, "Failed to scan hit-fallen log row", http.StatusInternalServerError)
			return
		}

		// Fetch track details from Lidarr
		track, err := lidarrClient.GetTrackByID(lidarrTrackID)
		if err != nil {
			// If we can't get the track, we still want to show the log entry but with empty track/album titles?
			// For now, we'll skip the entry if we can't get the track details.
			// Alternatively, we can log the error and continue with empty strings.
			continue
		}

		results = append(results, HitFallenResponse{
			ID:          id,
			ArtistName:  artistName,
			TrackTitle:  track.Title,
			AlbumTitle:  track.Album.Title,
			ScoreAtFall: int(scoreAtFall),
			FallenAt:    fallenAt,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Error iterating hit-fallen logs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// HitFallenResponse represents a hit-fallen log entry with enriched track details.
type HitFallenResponse struct {
	ID          int64  `json:"id"`
	ArtistName  string `json:"artistName"`
	TrackTitle  string `json:"trackTitle"`
	AlbumTitle  string `json:"albumTitle"`
	ScoreAtFall int    `json:"scoreAtFall"`
	FallenAt    string `json:"fallenAt"`
}