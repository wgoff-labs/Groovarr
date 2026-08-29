package clients

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/groovarr/groovarr/backend/internal/config"
)

type LidarrClient struct {
	baseURL   string
	apiKey    string
	client    *http.Client
	qProfileID int
	metaProfileCache map[string]int
}

type LidarrArtist struct {
	ID                int64  `json:"id"`
	ArtistName        string `json:"artistName"`
	ForeignArtistID   string `json:"foreignArtistId"`
	Monitored         bool   `json:"monitored"`
	RootFolderPath    string `json:"rootFolderPath"`
	QualityProfileID  int    `json:"qualityProfileId"`
	MetadataProfileID int    `json:"metadataProfileId"`
	SortName          string `json:"sortName"`
	Monitor           string `json:"monitor"` // "none", "albums", "all"
	Genres            []string `json:"genres"`
}

type LidarrAlbum struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Monitored bool   `json:"monitored"`
}

type LidarrTrack struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Monitored   bool   `json:"monitored"`
	HasFile     bool   `json:"hasFile"`
	TrackFileID int64  `json:"trackFileId"`
	AlbumID     int64  `json:"albumId"`
}

type LidarrRootFolder struct {
	ID   int64  `json:"id"`
	Path string `json:"path"`
}

type LidarrProfile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func NewLidarrClient() (*LidarrClient, error) {
	cfg := config.Load()
	return NewLidarrClientWith(cfg.LidarrURL, cfg.LidarrAPIKey)
}

// NewLidarrClientWith creates a Lidarr client from explicit URL + API key,
// useful for the connection manager which may not want to depend on global config.
func NewLidarrClientWith(url, apiKey string) (*LidarrClient, error) {
	if url == "" {
		return nil, fmt.Errorf("LIDARR_URL not set")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("LIDARR_API_KEY not set")
	}
	return &LidarrClient{
		baseURL:          strings.TrimSuffix(url, "/"),
		apiKey:           apiKey,
		client:           &http.Client{Timeout: 30 * time.Second},
		metaProfileCache: make(map[string]int),
	}, nil
}

func (c *LidarrClient) do(method, path string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = strings.NewReader(string(b))
	}
	url := fmt.Sprintf("%s/api/v1%s", c.baseURL, path)
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("lidarr error %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}

func (c *LidarrClient) get(path string, out any) error {
	data, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (c *LidarrClient) post(path string, body, out any) error {
	data, err := c.do(http.MethodPost, path, body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (c *LidarrClient) put(path string, body any) error {
	_, err := c.do(http.MethodPut, path, body)
	return err
}

func (c *LidarrClient) delete(path string) error {
	_, err := c.do(http.MethodDelete, path, nil)
	return err
}

// ── Root Folders ──────────────────────────────────────────────────────────────

func (c *LidarrClient) GetRootFolders() ([]LidarrRootFolder, error) {
	var folders []LidarrRootFolder
	err := c.get("/rootfolder", &folders)
	return folders, err
}

func (c *LidarrClient) ResolveRootFolder(nameOrPath string) (string, error) {
	folders, err := c.GetRootFolders()
	if err != nil {
		return "", err
	}
	search := strings.ToLower(strings.TrimSpace(nameOrPath))

	for _, f := range folders {
		fPath := strings.TrimSuffix(f.Path, "/")
		if strings.ToLower(fPath) == search {
			return f.Path, nil
		}
	}
	for _, f := range folders {
		name := strings.Split(strings.TrimSuffix(f.Path, "/"), "/")
		if strings.ToLower(name[len(name)-1]) == search {
			return f.Path, nil
		}
	}
	for _, f := range folders {
		if strings.Contains(strings.ToLower(f.Path), search) {
			return f.Path, nil
		}
	}

	// Return first folder as fallback
	if len(folders) > 0 {
		return folders[0].Path, nil
	}
	return "", fmt.Errorf("no root folders found")
}

// ── Profiles ────────────────────────────────────────────────────────────────

func (c *LidarrClient) GetQualityProfiles() ([]LidarrProfile, error) {
	var profiles []LidarrProfile
	err := c.get("/qualityprofile", &profiles)
	return profiles, err
}

func (c *LidarrClient) getQualityProfileID(name string) (int, error) {
	if c.qProfileID != 0 {
		return c.qProfileID, nil
	}
	profiles, err := c.GetQualityProfiles()
	if err != nil {
		return 0, err
	}
	cfg := config.Load()
	search := strings.ToLower(cfg.LidarrQualityProfile)
	for _, p := range profiles {
		if strings.ToLower(p.Name) == search {
			c.qProfileID = p.ID
			return p.ID, nil
		}
	}
	if len(profiles) > 0 {
		c.qProfileID = profiles[0].ID
		return profiles[0].ID, nil
	}
	return 0, fmt.Errorf("no quality profiles found")
}

func (c *LidarrClient) GetMetadataProfiles() ([]LidarrProfile, error) {
	var profiles []LidarrProfile
	err := c.get("/metadataprofile", &profiles)
	return profiles, err
}

func (c *LidarrClient) resolveMetadataProfile(folderName string) (int, error) {
	if id, ok := c.metaProfileCache[folderName]; ok {
		return id, nil
	}

	profiles, err := c.GetMetadataProfiles()
	if err != nil {
		return 0, err
	}
	if len(profiles) == 0 {
		return 1, nil
	}

	folderLower := strings.ToLower(folderName)

	for _, p := range profiles {
		pName := strings.ToLower(p.Name)
		if strings.Contains(folderLower, "comedy") && strings.Contains(pName, "comedy") {
			c.metaProfileCache[folderName] = p.ID
			return p.ID, nil
		}
		if strings.Contains(folderLower, "soundtrack") && strings.Contains(pName, "soundtrack") {
			c.metaProfileCache[folderName] = p.ID
			return p.ID, nil
		}
	}

	for _, p := range profiles {
		if strings.Contains(p.Name, "99") || strings.Contains(strings.ToLower(p.Name), "everything") {
			c.metaProfileCache[folderName] = p.ID
			return p.ID, nil
		}
	}

	id := profiles[0].ID
	c.metaProfileCache[folderName] = id
	return id, nil
}

// ── Artist ────────────────────────────────────────────────────────────────

func (c *LidarrClient) LookupArtist(name string) (*LidarrArtist, error) {
	var results []LidarrArtist
	err := c.get("/artist/lookup?term="+urlEncode(name), &results)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	nameLower := strings.ToLower(strings.TrimSpace(name))
	for _, r := range results {
		if strings.ToLower(r.ArtistName) == nameLower {
			return &r, nil
		}
	}
	return &results[0], nil
}

func (c *LidarrClient) GetArtist(id int64) (*LidarrArtist, error) {
	var artist LidarrArtist
	err := c.get(fmt.Sprintf("/artist/%d", id), &artist)
	if err != nil {
		return nil, err
	}
	return &artist, nil
}

func (c *LidarrClient) UpdateArtist(artist *LidarrArtist) error {
	return c.put(fmt.Sprintf("/artist/%d", artist.ID), artist)
}

func (c *LidarrClient) GetAllArtists() ([]LidarrArtist, error) {
	var artists []LidarrArtist
	err := c.get("/artist", &artists)
	return artists, err
}

func (c *LidarrClient) AddArtist(foreignArtistID, rootFolder string, metadataProfileID int) (*LidarrArtist, error) {
	lookup, err := c.LookupArtist(foreignArtistID)
	if err != nil || lookup == nil {
		return nil, fmt.Errorf("artist lookup failed: %v", err)
	}

	// Check if already exists
	all, err := c.GetAllArtists()
	if err == nil {
		for _, a := range all {
			if a.ForeignArtistID == lookup.ForeignArtistID {
				return nil, nil // already exists
			}
		}
	}

	qid, err := c.getQualityProfileID(config.Load().LidarrQualityProfile)
	if err != nil {
		return nil, err
	}

	resolvedFolder, err := c.ResolveRootFolder(rootFolder)
	if err != nil {
		return nil, err
	}

	metaID := metadataProfileID
	if metaID == 0 {
		folderName := ""
		if slashIdx := strings.LastIndex(resolvedFolder, "/"); slashIdx >= 0 {
			folderName = resolvedFolder[slashIdx+1:]
		}
		metaID, err = c.resolveMetadataProfile(folderName)
		if err != nil {
			return nil, err
		}
	}

	artistData := map[string]any{
		"foreignArtistId": lookup.ForeignArtistID,
		"artistName":      lookup.ArtistName,
		"monitored":       true,
		"qualityProfileId": qid,
		"metadataProfileId": metaID,
		"rootFolderPath":   resolvedFolder,
		"addOptions": map[string]any{
			"monitor":              "none",
			"searchForMissingAlbums": false,
		},
	}

	var result LidarrArtist
	err = c.post("/artist", artistData, &result)
	if err != nil {
		return nil, err
	}

	log.Printf("Added artist %s to Lidarr (ID %d)", result.ArtistName, result.ID)

	// Trigger refresh
	c.post("/command", map[string]any{"name": "RefreshArtist", "artistIds": []int64{result.ID}}, nil)

	return &result, nil
}

func (c *LidarrClient) MoveArtist(lidarrArtistID int64, newRootFolder string) error {
	artist, err := c.GetArtist(lidarrArtistID)
	if err != nil {
		return err
	}
	resolved, err := c.ResolveRootFolder(newRootFolder)
	if err != nil {
		return err
	}
	artist.RootFolderPath = resolved
	if err := c.put(fmt.Sprintf("/artist/%d", lidarrArtistID), artist); err != nil {
		return err
	}
	c.post("/command", map[string]any{
		"name":               "MoveArtist",
		"artistIds":          []int64{lidarrArtistID},
		"destinationRootFolder": resolved,
	}, nil)
	return nil
}

// ── Albums ────────────────────────────────────────────────────────────────

func (c *LidarrClient) GetArtistAlbums(lidarrArtistID int64) ([]LidarrAlbum, error) {
	var albums []LidarrAlbum
	err := c.get(fmt.Sprintf("/album?artistId=%d", lidarrArtistID), &albums)
	return albums, err
}

func (c *LidarrClient) GetAlbum(id int64) (*LidarrAlbum, error) {
	var album LidarrAlbum
	err := c.get(fmt.Sprintf("/album/%d", id), &album)
	if err != nil {
		return nil, err
	}
	return &album, nil
}

func (c *LidarrClient) SetAlbumMonitored(id int64, monitored bool) error {
	album, err := c.GetAlbum(id)
	if err != nil {
		return err
	}
	album.Monitored = monitored
	return c.put(fmt.Sprintf("/album/%d", id), album)
}

func (c *LidarrClient) SearchAlbum(id int64) error {
	err := c.post("/command", map[string]any{"name": "AlbumSearch", "albumIds": []int64{id}}, nil)
	return err
}

func (c *LidarrClient) MonitorAndSearchAlbum(id int64) error {
	if err := c.SetAlbumMonitored(id, true); err != nil {
		return err
	}
	return c.SearchAlbum(id)
}

// ── Tracks ────────────────────────────────────────────────────────────────

func (c *LidarrClient) GetAlbumTracks(albumID int64) ([]LidarrTrack, error) {
	var tracks []LidarrTrack
	err := c.get(fmt.Sprintf("/track?albumId=%d", albumID), &tracks)
	return tracks, err
}

func (c *LidarrClient) SetTrackMonitored(trackID int64, monitored bool) error {
	var track LidarrTrack
	err := c.get(fmt.Sprintf("/track/%d", trackID), &track)
	if err != nil {
		return err
	}
	track.Monitored = monitored
	return c.put("/track", []LidarrTrack{track})
}

func (c *LidarrClient) SetTracksMonitored(albumID int64, idsToMonitor map[int64]bool) (int, int, error) {
	tracks, err := c.GetAlbumTracks(albumID)
	if err != nil {
		return 0, 0, err
	}

	var toUpdate []LidarrTrack
	for _, t := range tracks {
		want, ok := idsToMonitor[t.ID]
		if !ok || t.Monitored == want {
			continue
		}
		t.Monitored = want
		toUpdate = append(toUpdate, t)
	}

	if len(toUpdate) == 0 {
		return 0, 0, nil
	}

	if err := c.put("/track", toUpdate); err != nil {
		return 0, 0, err
	}

	// Re-fetch to verify
	after, err := c.GetAlbumTracks(albumID)
	if err != nil {
		return 0, 0, err
	}
	verified := 0
	for _, t := range after {
		if idsToMonitor[t.ID] == t.Monitored {
			verified++
		}
	}
	return verified, len(toUpdate) - verified, nil
}

// ── Track Files ────────────────────────────────────────────────────────────

func (c *LidarrClient) DeleteTrackFile(fileID int64) error {
	// Verify deletion
	if err := c.delete(fmt.Sprintf("/trackfile/%d", fileID)); err != nil {
		return err
	}
	// GET should 404
	var dummy map[string]any
	err := c.get(fmt.Sprintf("/trackfile/%d", fileID), &dummy)
	if err == nil {
		return fmt.Errorf("track file %d still accessible after delete", fileID)
	}
	log.Printf("Verified track file %d deleted", fileID)
	return nil
}

// ── Utils ────────────────────────────────────────────────────────────────────

func urlEncode(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", "%20"), "#", "%23")
}