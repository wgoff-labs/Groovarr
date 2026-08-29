package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/groovarr/groovarr/backend/internal/store"
)

// HitFallenResponse represents a hit-fallen log entry with enriched track details.
type HitFallenResponse struct {
	ID          int64  `json:"id"`
	ArtistID    int64  `json:"artistId"`
	ArtistName  string `json:"artistName"`
	TrackID     int64  `json:"trackId"`
	TrackTitle  string `json:"trackTitle"`
	AlbumTitle  string `json:"albumTitle"`
	ScoreAtFall int    `json:"scoreAtFall"`
	FallenAt    string `json:"fallenAt"`
}

// HitFallenListResponse is the wrapper for the list endpoint.
type HitFallenListResponse struct {
	Entries []HitFallenResponse `json:"entries"`
}

// HitFallenHandler handles GET /api/hit-fallen
// Returns tracks that previously had state=hit but whose score has dropped below threshold.
// Lidarr is optional — if not connected, we still return log entries but skip track detail enrichment.
func HitFallenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	rows, err := store.GetDB().Query(`
		SELECT hfl.id, hfl.score_at_fall, hfl.fallen_at,
		       hfl.artist_id, hfl.lidarr_track_id, hfl.track_name, a.name as artist_name
		FROM hit_fallen_log hfl
		JOIN artists a ON hfl.artist_id = a.id
		ORDER BY hfl.fallen_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		http.Error(w, "Failed to fetch hit-fallen logs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type rowData struct {
		id            int64
		scoreAtFall   int64
		fallenAt      string
		artistID      int64
		lidarrTrackID int64
		trackName     string
		artistName    string
	}

	var raw []rowData
	for rows.Next() {
		var r rowData
		if err := rows.Scan(&r.id, &r.scoreAtFall, &r.fallenAt, &r.artistID, &r.lidarrTrackID, &r.trackName, &r.artistName); err != nil {
			http.Error(w, "Failed to scan hit-fallen log row", http.StatusInternalServerError)
			return
		}
		raw = append(raw, r)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Error iterating hit-fallen logs", http.StatusInternalServerError)
		return
	}

	// No Lidarr enrichment available without GetTrackByID.
	entries := make([]HitFallenResponse, 0, len(raw))
	for _, r := range raw {
		entry := HitFallenResponse{
			ID:          r.id,
			ArtistID:    r.artistID,
			ArtistName:  r.artistName,
			TrackID:     r.lidarrTrackID,
			TrackTitle:  r.trackName,
			AlbumTitle:  "",
			ScoreAtFall: int(r.scoreAtFall),
			FallenAt:    r.fallenAt,
		}
		// Track detail enrichment skipped — LidarrClient has no GetTrackByID method;
		// track titles can be retrieved from the track_popularity table in a future iteration.
		entries = append(entries, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HitFallenListResponse{Entries: entries})
}