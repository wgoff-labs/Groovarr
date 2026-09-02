package clients

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/groovarr/groovarr/backend/internal/config"
)

const LastFMBase = "https://ws.audioscrobbler.com/2.0/"

type LastFMClient struct {
	apiKey  string
	client  *http.Client
}

func NewLastFMClient() (*LastFMClient, error) {
	cfg := config.Load()
	if cfg.LastFMAPIKey == "" {
		return nil, fmt.Errorf("LASTFM_API_KEY not set")
	}
	return &LastFMClient{
		apiKey: cfg.LastFMAPIKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *LastFMClient) get(params map[string]string) (map[string]any, error) {
	v := url.Values{}
	v.Set("api_key", c.apiKey)
	v.Set("format", "json")
	for k, val := range params {
		v.Set(k, val)
	}
	req, err := http.NewRequest(http.MethodGet, LastFMBase+"?"+v.Encode(), nil)
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
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// TrackScore holds a track name (lowercase) → popularity score.
type TrackScore struct {
	Name  string
	Score int
}

// RawTrack holds the raw data returned by the Last.fm top-tracks endpoint.
type RawTrack struct {
	Name      string
	PlayCount int64
}

// GetArtistTopTracks fetches up to maxTracks tracks for an artist across multiple
// pages (Last.fm pages at 50). Returns tracks ordered by playcount descending.
func (c *LastFMClient) GetArtistTopTracks(artistName string, maxTracks int) ([]RawTrack, error) {
	pageSize := 50
	pages := (maxTracks + pageSize - 1) / pageSize
	if pages < 1 {
		pages = 1
	}
	var all []RawTrack

	for page := 1; page <= pages && len(all) < maxTracks; page++ {
		result, err := c.get(map[string]string{
			"method": "artist.gettoptracks",
			"artist": artistName,
			"limit":  fmt.Sprintf("%d", pageSize),
			"page":   fmt.Sprintf("%d", page),
		})
		if err != nil {
			return nil, err
		}

		toptracks, ok := result["toptracks"].(map[string]any)
		if !ok {
			continue
		}

		tracksAny, _ := toSlice(toptracks["track"])
		tracks, ok := tracksAny.([]any)
		if !ok || len(tracks) == 0 {
			continue
		}

		for _, item := range tracks {
			if m, ok := item.(map[string]any); ok {
				all = append(all, RawTrack{
					Name:      lastfmStrVal(m, "name"),
					PlayCount: lastfmInt64Val(m, "playcount"),
				})
			}
		}
	}

	// Trim to max.
	if len(all) > maxTracks {
		all = all[:maxTracks]
	}

	log.Printf("Last.fm: fetched %d raw tracks for '%s' across %d pages", len(all), artistName, pages)
	return all, nil
}

func toSlice(v any) (any, bool) {
	_, ok := v.([]any)
	return v, ok
}

// Helper functions to extract values from map[string]any
func lastfmStrVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func lastfmInt64Val(m map[string]any, key string) int64 {
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

func lastfmIntVal(m map[string]any, key string) int {
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