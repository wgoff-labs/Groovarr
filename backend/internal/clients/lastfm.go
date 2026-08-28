package clients

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

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
		client: config.DefaultHTTPClient,
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

// GetArtistTopTracksScored returns track scores normalized to 0–100.
func (c *LastFMClient) GetArtistTopTracksScored(artistName string) (map[string]int, error) {
	result, err := c.get(map[string]string{
		"method": "artist.gettoptracks",
		"artist": artistName,
		"limit":  "50",
	})
	if err != nil {
		return nil, err
	}

	toptracks, ok := result["toptracks"].(map[string]any)
	if !ok {
		return nil, nil
	}

	tracksAny, ok := toSlice(toptracks["track"])
	if !ok {
		return nil, nil
	}

	tracks, ok := tracksAny.([]any)
	if !ok || len(tracks) == 0 {
		return nil, nil
	}

	type trackInfo struct {
		Name      string
		PlayCount int
	}
	var tracksParsed []trackInfo

	for _, item := range tracks {
		if m, ok := item.(map[string]any); ok {
			tracksParsed = append(tracksParsed, trackInfo{
				Name:      strings.ToLower(strings.TrimSpace(lastfmStrVal(m, "name"))),
				PlayCount: int(lastfmInt64Val(m, "playcount")),
			})
		}
	}

	maxCount := 0
	for _, t := range tracksParsed {
		if t.PlayCount > maxCount {
			maxCount = t.PlayCount
		}
	}
	if maxCount == 0 {
		maxCount = 1
	}

	scores := make(map[string]int, len(tracksParsed))
	for _, t := range tracksParsed {
		if t.Name == "" {
			continue
		}
		score := max(10, min(100, int((float64(t.PlayCount)/float64(maxCount))*100)))
		scores[t.Name] = score
	}

	log.Printf("Last.fm: got %d track scores for '%s'", len(scores), artistName)
	return scores, nil
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