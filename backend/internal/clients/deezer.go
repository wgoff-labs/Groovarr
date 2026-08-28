package clients

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/groovarr/groovarr/backend/internal/config"
)

const DeezerBase = "https://api.deezer.com"

type DeezerArtist struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	NbFan     int64  `json:"nb_fan"`
	Picture   string `json:"picture_medium"`
}

type DeezerTrack struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Duration int    `json:"duration"`
	Rank     int64  `json:"rank"`
}

type DeezerAlbum struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	RecordType   string `json:"record_type"`
	ReleaseDate  string `json:"release_date"`
	Link         string `json:"link"`
	NbTracks     int    `json:"nb_tracks"`
	CoverMedium  string `json:"cover_medium"`
}

type TrackPopularity struct {
	Name       string
	Popularity int
}

type AlbumInfo struct {
	Name              string
	DeezerID          string
	DeezerURL         string
	ReleaseDate       string
	AlbumType         string
	TotalTracks       int
	AvgPopularity     float64
	TopTrackNames     []string
	TrackPopularities []TrackPopularity
}

type DeezerClient struct {
	client *http.Client
}

func NewDeezerClient() *DeezerClient {
	return &DeezerClient{client: config.DefaultHTTPClient}
}

func (c *DeezerClient) get(path string) (any, error) {
	url := DeezerBase + path
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *DeezerClient) SearchArtist(name string) (*DeezerArtist, error) {
	result, err := c.get(fmt.Sprintf("/search/artist?q=%s&limit=5", urlEncode(name)))
	if err != nil {
		return nil, err
	}

	m, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected deezer response type")
	}

	data, ok := m["data"].([]any)
	if !ok || len(data) == 0 {
		return nil, nil
	}

	nameLower := strings.ToLower(strings.TrimSpace(name))
	for _, item := range data {
		a, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.ToLower(strVal(a, "name")) == nameLower {
			return toArtist(a), nil
		}
	}
	return toArtist(data[0].(map[string]any)), nil
}

func (c *DeezerClient) GetArtistTopTracks(artistID string) ([]DeezerTrack, error) {
	result, err := c.get(fmt.Sprintf("/artist/%s/top?limit=50", artistID))
	if err != nil {
		return nil, err
	}
	m, ok := result.(map[string]any)
	if !ok {
		return nil, nil
	}
	data, ok := m["data"].([]any)
	if !ok {
		return nil, nil
	}
	tracks := make([]DeezerTrack, 0, len(data))
	for _, item := range data {
		if t, ok := item.(map[string]any); ok {
			tracks = append(tracks, DeezerTrack{
				ID:       int64Val(t, "id"),
				Title:    strVal(t, "title"),
				Duration: intVal(t, "duration"),
				Rank:     int64Val(t, "rank"),
			})
		}
	}
	return tracks, nil
}

func (c *DeezerClient) GetNewReleases(artistID string, lookbackDays int) ([]AlbumInfo, error) {
	cfg := config.Load()
	threshold := cfg.PopularityThreshold

	// Get top tracks for scoring
	topTracks, err := c.GetArtistTopTracks(artistID)
	if err != nil {
		return nil, err
	}

	// Build ID and name score maps
	totalTop := len(topTracks)
	topIDs := make(map[int64]int, totalTop)
	for i, t := range topTracks {
		topIDs[t.ID] = i
	}

	// Fetch all albums
	albums, err := c.getArtistAlbums(artistID)
	if err != nil {
		return nil, err
	}

	cutoffDays := lookbackDays
	if cutoffDays == 0 {
		cutoffDays = 90
	}

	results := []AlbumInfo{}
	for _, album := range albums {
		if !inDateRange(album.ReleaseDate, cutoffDays) {
			continue
		}

		tracks, err := c.getAlbumTracks(album.ID)
		if err != nil || len(tracks) == 0 {
			continue
		}

		var pops []TrackPopularity
		var topNames []string
		var totalScore float64

		for _, t := range tracks {
			score := c.calcScore(t, topIDs, totalTop)
			pops = append(pops, TrackPopularity{Name: t.Title, Popularity: score})
			totalScore += float64(score)
			if score >= threshold {
				topNames = append(topNames, t.Title)
			}
		}

		avgPop := 0.0
		if len(pops) > 0 {
			avgPop = totalScore / float64(len(pops))
		}

		recordType := album.RecordType
		if recordType == "" {
			recordType = "album"
		}

		results = append(results, AlbumInfo{
			Name:              album.Title,
			DeezerID:          fmt.Sprintf("%d", album.ID),
			DeezerURL:         album.Link,
			ReleaseDate:       album.ReleaseDate,
			AlbumType:         recordType,
			TotalTracks:       album.NbTracks,
			AvgPopularity:     avgPop,
			TopTrackNames:     topNames,
			TrackPopularities: pops,
		})
	}

	return results, nil
}

func (c *DeezerClient) GetTopAlbums(artistID string) ([]AlbumInfo, error) {
	cfg := config.Load()
	threshold := cfg.PopularityThreshold

	topTracks, err := c.GetArtistTopTracks(artistID)
	if err != nil || len(topTracks) == 0 {
		return nil, err
	}

	totalTop := len(topTracks)
	nameScores := make(map[string]int, totalTop)
	idScores := make(map[int64]int, totalTop)

	maxRank := int64(0)
	for _, t := range topTracks {
		if t.Rank > maxRank {
			maxRank = t.Rank
		}
	}
	if maxRank == 0 {
		maxRank = 1
	}

	for i, t := range topTracks {
		score := max(10, min(100, int(float64(t.Rank)/float64(maxRank)*100)))
		if score == 0 {
			score = max(50, 100-int((float64(i)/float64(totalTop))*50))
		}
		name := strings.ToLower(strings.TrimSpace(t.Title))
		nameScores[name] = score
		idScores[t.ID] = score
	}

	albums, err := c.getArtistAlbums(artistID)
	if err != nil {
		return nil, err
	}

	var results []AlbumInfo
	for _, album := range albums {
		tracks, err := c.getAlbumTracks(album.ID)
		if err != nil || len(tracks) == 0 {
			continue
		}

		var pops []TrackPopularity
		var topNames []string
		var totalScore float64

		for _, t := range tracks {
			score := c.scoreTrack(t, nameScores, idScores)
			pops = append(pops, TrackPopularity{Name: t.Title, Popularity: score})
			totalScore += float64(score)
			if score >= threshold {
				topNames = append(topNames, t.Title)
			}
		}

		avgPop := 0.0
		if len(pops) > 0 {
			avgPop = totalScore / float64(len(pops))
		}

		recordType := album.RecordType
		if recordType == "" {
			recordType = "album"
		}

		results = append(results, AlbumInfo{
			Name:              album.Title,
			DeezerID:          fmt.Sprintf("%d", album.ID),
			DeezerURL:         album.Link,
			ReleaseDate:       album.ReleaseDate,
			AlbumType:         recordType,
			TotalTracks:       album.NbTracks,
			AvgPopularity:     avgPop,
			TopTrackNames:     topNames,
			TrackPopularities: pops,
		})
	}

	// Sort by number of popular tracks
	// (bubble up most popular)
	sortAlbumsByPopularity(results)

	log.Printf("Deezer top albums for %s: %d albums scored", artistID, len(results))
	return results, nil
}

func (c *DeezerClient) calcScore(t DeezerTrack, topIDs map[int64]int, totalTop int) int {
	if pos, ok := topIDs[t.ID]; ok && totalTop > 0 {
		return max(50, 100-int((float64(pos)/float64(totalTop))*50))
	}
	return 10
}

func (c *DeezerClient) scoreTrack(t DeezerTrack, nameScores map[string]int, idScores map[int64]int) int {
	if score, ok := idScores[t.ID]; ok {
		return score
	}
	name := strings.ToLower(strings.TrimSpace(t.Title))
	if score, ok := nameScores[name]; ok {
		return score
	}
	for topName, score := range nameScores {
		if strings.Contains(name, topName) || strings.Contains(topName, name) {
			return score
		}
	}
	return 10
}

func (c *DeezerClient) getArtistAlbums(artistID string) ([]DeezerAlbum, error) {
	result, err := c.get(fmt.Sprintf("/artist/%s/albums?limit=100", artistID))
	if err != nil {
		return nil, err
	}
	m, ok := result.(map[string]any)
	if !ok {
		return nil, nil
	}
	data, ok := m["data"].([]any)
	if !ok {
		return nil, nil
	}
	albums := make([]DeezerAlbum, 0, len(data))
	for _, item := range data {
		if a, ok := item.(map[string]any); ok {
			albums = append(albums, DeezerAlbum{
				ID:          int64Val(a, "id"),
				Title:       strVal(a, "title"),
				RecordType:  strVal(a, "record_type"),
				ReleaseDate: strVal(a, "release_date"),
				Link:        strVal(a, "link"),
				NbTracks:    intVal(a, "nb_tracks"),
				CoverMedium: strVal(a, "cover_medium"),
			})
		}
	}
	return albums, nil
}

func (c *DeezerClient) getAlbumTracks(albumID int64) ([]DeezerTrack, error) {
	result, err := c.get(fmt.Sprintf("/album/%d/tracks", albumID))
	if err != nil {
		return nil, err
	}
	m, ok := result.(map[string]any)
	if !ok {
		return nil, nil
	}
	data, ok := m["data"].([]any)
	if !ok {
		return nil, nil
	}
	tracks := make([]DeezerTrack, 0, len(data))
	for _, item := range data {
		if t, ok := item.(map[string]any); ok {
			tracks = append(tracks, DeezerTrack{
				ID:       int64Val(t, "id"),
				Title:    strVal(t, "title"),
				Duration: intVal(t, "duration"),
				Rank:     int64Val(t, "rank"),
			})
		}
	}
	return tracks, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func toArtist(m map[string]any) *DeezerArtist {
	return &DeezerArtist{
		ID:      int64Val(m, "id"),
		Name:    strVal(m, "name"),
		NbFan:   int64Val(m, "nb_fan"),
		Picture: strVal(m, "picture_medium"),
	}
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func intVal(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

func int64Val(m map[string]any, key string) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	}
	return 0
}

func inDateRange(dateStr string, days int) bool {
	if dateStr == "" {
		return false
	}
	// Simple check: just require the year is 4 digits
	return len(dateStr) >= 4
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func sortAlbumsByPopularity(albums []AlbumInfo) {
	n := len(albums)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if len(albums[j].TopTrackNames) < len(albums[j+1].TopTrackNames) {
				albums[j], albums[j+1] = albums[j+1], albums[j]
			}
		}
	}
}
