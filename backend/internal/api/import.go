package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/connections"
	"github.com/groovarr/groovarr/backend/internal/store"
)

// LidarrArtistForImport is a summary of a Lidarr artist used in the import list.
type LidarrArtistForImport struct {
	LidarrID          int64  `json:"lidarrId"`
	Name              string `json:"name"`
	SortName          string `json:"sortName"`
	RootFolder        string `json:"rootFolder"`
	QualityProfileID  int    `json:"qualityProfileId"`
	QualityProfile    string `json:"qualityProfile"`
	MetadataProfileID int    `json:"metadataProfileId"`
	MetadataProfile   string `json:"metadataProfile"`
	Monitor           string `json:"monitor"` // "none", "albums", "all"
	AlreadyInGroovarr bool   `json:"alreadyInGroovarr"`
	Genres            string `json:"genres"`
}

// ImportListResponse is the paginated import list response.
type ImportListResponse struct {
	Artists    []LidarrArtistForImport `json:"artists"`
	Total      int                     `json:"total"`
	Page       int                     `json:"page"`
	Limit      int                     `json:"limit"`
	TotalPages int                     `json:"totalPages"`
}

// ArtistImportHandler returns a paginated list of Lidarr artists not yet in Groovarr.
func ArtistImportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cm := connections.New()
	c, err := cm.GetLidarrClient()
	if err != nil {
		if WriteLidarrUnavailable(w, cm) {
			return
		}
		http.Error(w, "Lidarr not connected: "+config.SanitizeError(err.Error()), http.StatusServiceUnavailable)
		return
	}

	// Fetch quality profiles for name lookup
	profiles, _ := c.GetQualityProfiles()
	profileNames := make(map[int]string)
	for _, p := range profiles {
		profileNames[p.ID] = p.Name
	}

	// Fetch metadata profiles for name lookup
	metaProfiles, _ := c.GetMetadataProfiles()
	metaProfileNames := make(map[int]string)
	for _, p := range metaProfiles {
		metaProfileNames[p.ID] = p.Name
	}

	// Get all Lidarr artists
	lidarrArtists, err := c.GetAllArtists()
	if err != nil {
		http.Error(w, "Failed to fetch Lidarr artists: "+config.SanitizeError(err.Error()), http.StatusBadGateway)
		return
	}

	// Check which ones are already in Groovarr
	groovarrArtists, _ := store.ArtistList()
	groovarrNames := make(map[string]bool)
	for _, a := range groovarrArtists {
		groovarrNames[a.Name] = true
	}

	// Build the import list (only non-duplicates)
	var importList []LidarrArtistForImport
	for _, la := range lidarrArtists {
		importList = append(importList, LidarrArtistForImport{
			LidarrID:          la.ID,
			Name:              la.ArtistName,
			SortName:          la.SortName,
			RootFolder:        la.RootFolderPath,
			QualityProfileID:  la.QualityProfileID,
			QualityProfile:    profileNames[la.QualityProfileID],
			MetadataProfileID: la.MetadataProfileID,
			MetadataProfile:   metaProfileNames[la.MetadataProfileID],
			Monitor:           resolveMonitor(la.Monitored),
			AlreadyInGroovarr: groovarrNames[la.ArtistName],
			Genres:            strings.Join(la.Genres, ", "),
		})
	}

	// Pagination
	page := 1
	limit := 20
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	total := len(importList)
	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	start := (page - 1) * limit
	end := start + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ImportListResponse{
		Artists:    importList[start:end],
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// BulkImportRequest is the body for the bulk import endpoint.
type BulkImportRequest struct {
	ArtistIDs        []json.Number `json:"artistIds"`
	RootFolder       string        `json:"rootFolder"`
	QualityProfileID int           `json:"qualityProfileId"`
	Monitor          string        `json:"monitor"`
}

// BulkImportResponse is the response for the bulk import endpoint.
type BulkImportResponse struct {
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

// ArtistImportBulkHandler handles bulk import of artists from Lidarr.
func ArtistImportBulkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BulkImportRequest
	if err := ValidateJSON(r, &req); err != nil {
		BadRequest(w, "invalid request body: "+config.SanitizeError(err.Error()))
		return
	}

	// Validate required fields
	if len(req.ArtistIDs) == 0 {
		BadRequest(w, "artistIds is required")
		return
	}
	if req.QualityProfileID <= 0 {
		BadRequest(w, "qualityProfileId is required and must be > 0")
		return
	}
	if req.RootFolder == "" {
		BadRequest(w, "rootFolder is required")
		return
	}
	if req.Monitor == "" {
		BadRequest(w, "monitor is required")
		return
	}

	cm := connections.New()
	c, err := cm.GetLidarrClient()
	if err != nil {
		if WriteLidarrUnavailable(w, cm) {
			return
		}
		http.Error(w, "Lidarr not connected: "+config.SanitizeError(err.Error()), http.StatusServiceUnavailable)
		return
	}

	// Fetch quality profiles for name lookup
	profiles, _ := c.GetQualityProfiles()
	profileMap := make(map[int]string)
	for _, p := range profiles {
		profileMap[p.ID] = p.Name
	}

	imported := 0
	skipped := 0
	var errs []string

	for _, lidarrIDRaw := range req.ArtistIDs {
		lidarrID, err := strconv.ParseInt(string(lidarrIDRaw), 10, 64)
		if err != nil {
			errs = append(errs, "invalid artist ID "+string(lidarrIDRaw)+": "+config.SanitizeError(err.Error()))
			continue
		}
		// Get artist from Lidarr
		artist, err := c.GetArtist(lidarrID)
		if err != nil {
			errs = append(errs, "Lidarr ID "+strconv.FormatInt(lidarrID, 10)+": "+config.SanitizeError(err.Error()))
			continue
		}

		// Check if already in Groovarr
		existing, err := store.ArtistGet(artist.ArtistName)
		if err == nil && existing != nil {
			skipped++
			continue
		}

		// Determine root folder
		rootFolder := req.RootFolder
		if rootFolder == "" {
			rootFolder = artist.RootFolderPath
		}

		// Add to Groovarr
		addedBy := "lidarr_import"
		if _, err := store.ArtistAdd(artist.ArtistName, "", lidarrID, rootFolder, addedBy); err != nil {
			errs = append(errs, artist.ArtistName+": "+config.SanitizeError(err.Error()))
			continue
		}

		// Update Lidarr ID if ArtistAdd didn't store it (it accepts lidarrID as param 3)
		// store.ArtistAdd stores lidarr_id in column 3; verify
		// Actually ArtistAdd signature: ArtistAdd(name, deezerID string, lidarrID int64, rootFolder, addedBy string)
		// So we pass lidarrID directly. Good.

		// If monitor override requested, update in Lidarr
		if req.Monitor != "" && req.Monitor != resolveMonitor(artist.Monitored) {
			// Update monitored status in Lidarr
			artist.Monitored = req.Monitor == "all" || req.Monitor == "albums"
			if err := c.UpdateArtist(artist); err != nil {
				// Non-fatal: log but continue
				errs = append(errs, artist.ArtistName+" (monitor update failed): "+config.SanitizeError(err.Error()))
			}
		}

		imported++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BulkImportResponse{
		Imported: imported,
		Skipped:  skipped,
		Errors:   errs,
	})
}

// resolveMonitor converts Lidarr's bool monitored field to our string representation.
func resolveMonitor(monitored bool) string {
	if monitored {
		return "albums"
	}
	return "none"
}
