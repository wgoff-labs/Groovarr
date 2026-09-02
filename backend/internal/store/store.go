package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

// GetDB returns the database connection for use by other packages.
func GetDB() *sql.DB {
	return db
}

func Init(dbPath string) error {
	var err error
	db, err = sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=1")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		return err
	}

	// Apply migrations
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// Close closes the database connection.
func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

var migrations = []string{
	// Artists table
	`CREATE TABLE IF NOT EXISTS artists (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		name            TEXT    NOT NULL UNIQUE,
		deezer_id       TEXT,
		lidarr_id       INTEGER,
		root_folder     TEXT,
		added_by        TEXT,
		added_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_checked    DATETIME
	);`,
	// Settings table (key-value store)
	`CREATE TABLE IF NOT EXISTS settings (
		key   TEXT PRIMARY KEY,
		value TEXT
	);`,
	// Check log
	`CREATE TABLE IF NOT EXISTS check_log (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		artist_id     INTEGER NOT NULL,
		album_name    TEXT    NOT NULL,
		deezer_url    TEXT,
		popularity    INTEGER,
		processed     BOOLEAN NOT NULL DEFAULT 0,
		checked_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (artist_id) REFERENCES artists(id)
	);`,
	// Monitored tracks
	`CREATE TABLE IF NOT EXISTS monitored_tracks (
		artist_id INTEGER NOT NULL,
		album_name TEXT NOT NULL,
		track_name TEXT NOT NULL,
		PRIMARY KEY (artist_id, album_name, track_name),
		FOREIGN KEY (artist_id) REFERENCES artists(id)
	);`,
	// Never prune list
	`CREATE TABLE IF NOT EXISTS never_prune (
		artist_id INTEGER NOT NULL,
		album_name TEXT NOT NULL,
		track_name TEXT NOT NULL,
		PRIMARY KEY (artist_id, album_name, track_name),
		FOREIGN KEY (artist_id) REFERENCES artists(id)
	);`,
	// Track preferences (3-state model)
	`CREATE TABLE IF NOT EXISTS track_preferences (
		artist_id     INTEGER NOT NULL,
		lidarr_track_id INTEGER NOT NULL,
		state         TEXT NOT NULL CHECK(state IN ('keep', 'hit', 'not_keep')),
		score_at_time INTEGER,
		created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (artist_id, lidarr_track_id),
		FOREIGN KEY (artist_id) REFERENCES artists(id)
	);`,
	// Hit-fallen log (tracks that were hit but fell below threshold)
	`CREATE TABLE IF NOT EXISTS hit_fallen_log (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		artist_id     INTEGER NOT NULL,
		lidarr_track_id INTEGER,
		track_name    TEXT NOT NULL,
		score_at_fall INTEGER NOT NULL,
		fallen_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (artist_id) REFERENCES artists(id)
	);`,
	// Track popularity (cached from Lidarr/Last.fm)
	`CREATE TABLE IF NOT EXISTS track_popularity (
		artist_id     INTEGER NOT NULL,
		lidarr_track_id INTEGER NOT NULL,
		play_count    INTEGER DEFAULT 0,
		last_played   DATETIME,
		UNIQUE(artist_id, lidarr_track_id),
		FOREIGN KEY (artist_id) REFERENCES artists(id)
	);`,
	// Per-artist popularity cache freshness (drives background refresh)
	`CREATE TABLE IF NOT EXISTS artist_popularity_cache (
		artist_id      INTEGER PRIMARY KEY,
		artist_name    TEXT NOT NULL,
		last_fetched   DATETIME NOT NULL,
		track_count    INTEGER DEFAULT 0,
		max_playcount  INTEGER DEFAULT 0,
		FOREIGN KEY (artist_id) REFERENCES artists(id)
	);`,
	// Track actions log (audit)
	`CREATE TABLE IF NOT EXISTS track_actions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		artist_id   INTEGER NOT NULL,
		lidarr_track_id INTEGER,
		action      TEXT NOT NULL,
		reason      TEXT,
		timestamp   DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (artist_id) REFERENCES artists(id)
	);`,
	// Pruning log
	`CREATE TABLE IF NOT EXISTS pruning_log (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		artist_id   INTEGER NOT NULL,
		album_name  TEXT NOT NULL,
		action      TEXT NOT NULL,
		reason      TEXT,
		count       INTEGER NOT NULL,
		timestamp   DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (artist_id) REFERENCES artists(id)
	);`,
}

// Artist represents an artist in the watchlist.
type Artist struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DeezID      string `json:"deezer_id"`
	LidarrID    *int64 `json:"lidarr_id"`
	RootFolder  string `json:"root_folder"`
	AddedBy     string `json:"added_by"`
	AddedAt     string `json:"added_at"`
	LastChecked *string `json:"last_checked"`
}

// ArtistAdd adds a new artist to the watchlist.
// Returns the new artist ID and an error if any.
func ArtistAdd(name, deezerID string, lidarrID int64, rootFolder, addedBy string) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO artists (name, deezer_id, lidarr_id, root_folder, added_by) VALUES (?, ?, ?, ?, ?)`,
		name, deezerID, lidarrID, rootFolder, addedBy,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

// ArtistGet retrieves an artist by name.
// Returns the artist and an error if not found.
func ArtistGet(name string) (*Artist, error) {
	row := db.QueryRow(
		`SELECT id, name, deezer_id, lidarr_id, root_folder, added_by, added_at, last_checked FROM artists WHERE name = ?`,
		name,
	)
	var a Artist
	var lidarrID *int64
	var lastChecked *string
	err := row.Scan(&a.ID, &a.Name, &a.DeezID, &lidarrID, &a.RootFolder, &a.AddedBy, &a.AddedAt, &lastChecked)
	if err != nil {
		return nil, err
	}
	a.LidarrID = lidarrID
	a.LastChecked = lastChecked
	return &a, nil
}

// ArtistGetByID retrieves an artist by ID.
// Returns the artist and an error if not found.
func ArtistGetByID(id int64) (*Artist, error) {
	row := db.QueryRow(
		`SELECT id, name, deezer_id, lidarr_id, root_folder, added_by, added_at, last_checked FROM artists WHERE id = ?`,
		id,
	)
	var a Artist
	var lidarrID *int64
	var lastChecked *string
	err := row.Scan(&a.ID, &a.Name, &a.DeezID, &lidarrID, &a.RootFolder, &a.AddedBy, &a.AddedAt, &lastChecked)
	if err != nil {
		return nil, err
	}
	a.LidarrID = lidarrID
	a.LastChecked = lastChecked
	return &a, nil
}

// ArtistList returns all artists in the watchlist.
func ArtistList() ([]*Artist, error) {
	rows, err := db.Query(
		`SELECT id, name, deezer_id, lidarr_id, root_folder, added_by, added_at, last_checked FROM artists ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artists []*Artist
	for rows.Next() {
		var a Artist
		var lidarrID *int64
		var lastChecked *string
		if err := rows.Scan(&a.ID, &a.Name, &a.DeezID, &lidarrID, &a.RootFolder, &a.AddedBy, &a.AddedAt, &lastChecked); err != nil {
			return nil, err
		}
		a.LidarrID = lidarrID
		a.LastChecked = lastChecked
		artists = append(artists, &a)
	}
	return artists, rows.Err()
}

// ArtistDelete removes an artist by ID.
func ArtistDelete(id int64) error {
	_, err := db.Exec(`DELETE FROM artists WHERE id = ?`, id)
	return err
}

// ArtistUpdateDeezerID updates an artist's Deezer ID by name.
func ArtistUpdateDeezerID(name, deezerID string) error {
	_, err := db.Exec(`UPDATE artists SET deezer_id = ? WHERE name = ?`, deezerID, name)
	return err
}

// ArtistUpdateLidarrID updates an artist's Lidarr ID by name.
func ArtistUpdateLidarrID(name string, lidarrID int64) error {
	_, err := db.Exec(`UPDATE artists SET lidarr_id = ? WHERE name = ?`, lidarrID, name)
	return err
}

// ArtistMarkChecked marks an artist as checked (updates last_checked timestamp).
func ArtistMarkChecked(id int64) error {
	_, err := db.Exec(`UPDATE artists SET last_checked = datetime('now') WHERE id = ?`, id)
	return err
}

// GetTrackPreferences returns the track preference state for an artist.
// Returns a map[lidarrTrackID]state.
func GetTrackPreferences(artistID int64) (map[int64]string, error) {
	rows, err := db.Query(
		`SELECT lidarr_track_id, state FROM track_preferences WHERE artist_id = ?`,
		artistID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prefs := make(map[int64]string)
	for rows.Next() {
		var lidarrTrackID int64
		var state string
		if err := rows.Scan(&lidarrTrackID, &state); err != nil {
			return nil, err
		}
		prefs[lidarrTrackID] = state
	}
	return prefs, rows.Err()
}

// GetTrackPreferencesForArtist returns track preferences for an artist as a slice.
func GetTrackPreferencesForArtist(artistID int64) ([]TrackPreference, error) {
	rows, err := db.Query(
		`SELECT lidarr_track_id, state, score_at_time FROM track_preferences WHERE artist_id = ? ORDER BY lidarr_track_id`,
		artistID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prefs []TrackPreference
	for rows.Next() {
		var tp TrackPreference
		if err := rows.Scan(&tp.LidarrTrackID, &tp.State, &tp.ScoreAtTime); err != nil {
			return nil, err
		}
		prefs = append(prefs, tp)
	}
	return prefs, rows.Err()
}

// GetTrackPreference returns the track preference state for a specific track.
func GetTrackPreference(artistID, lidarrTrackID int64) (string, error) {
	row := db.QueryRow(
		`SELECT state FROM track_preferences WHERE artist_id = ? AND lidarr_track_id = ?`,
		artistID, lidarrTrackID,
	)
	var state string
	err := row.Scan(&state)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return state, nil
}

// UpsertTrackPreference inserts or updates a track preference.
func UpsertTrackPreference(artistID, lidarrTrackID int64, state string, scoreAtTime int) error {
	_, err := db.Exec(
		`INSERT INTO track_preferences (artist_id, lidarr_track_id, state, score_at_time, updated_at) 
		 VALUES (?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(artist_id, lidarr_track_id) DO UPDATE SET 
		 state=excluded.state, score_at_time=excluded.score_at_time, updated_at=datetime('now')`,
		artistID, lidarrTrackID, state, scoreAtTime,
	)
	return err
}

// DeleteTrackPreference removes a track preference.
func DeleteTrackPreference(artistID, lidarrTrackID int64) error {
	_, err := db.Exec(
		`DELETE FROM track_preferences WHERE artist_id = ? AND lidarr_track_id = ?`,
		artistID, lidarrTrackID,
	)
	return err
}

// HitFallenLogInsert logs a hit-fallen event.
func HitFallenLogInsert(artistID, lidarrTrackID int64, trackName string, scoreAtFall int) error {
	_, err := db.Exec(
		`INSERT INTO hit_fallen_log (artist_id, lidarr_track_id, track_name, score_at_fall) VALUES (?, ?, ?, ?)`,
		artistID, lidarrTrackID, trackName, scoreAtFall,
	)
	return err
}

// HitFallenLogGet returns hit-fallen log entries.
func HitFallenLogGet(limit int) ([]map[string]interface{}, error) {
	rows, err := db.Query(
		`SELECT h.id, h.artist_id, a.name, h.lidarr_track_id, h.track_name, h.score_at_fall, h.fallen_at
		 FROM hit_fallen_log h JOIN artists a ON h.artist_id = a.id
		 ORDER BY h.fallen_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id, artistID, lidarrTrackID, scoreAtFall int
		var artistName, trackName, fallenAt string
		if err := rows.Scan(&id, &artistID, &artistName, &lidarrTrackID, &trackName, &scoreAtFall, &fallenAt); err != nil {
			continue
		}
		logs = append(logs, map[string]interface{}{
			"id":           id,
			"artist_id":    artistID,
			"artist_name":  artistName,
			"lidarr_track_id": lidarrTrackID,
			"track_name":   trackName,
			"score_at_fall": scoreAtFall,
			"fallen_at":    fallenAt,
		})
	}
	return logs, rows.Err()
}

// TrackActionLog logs a track action (monitor/unmonitor/etc).
func TrackActionLog(artistID, lidarrTrackID int64, action string, reason string) error {
	_, err := db.Exec(
		`INSERT INTO track_actions (artist_id, lidarr_track_id, action, reason, timestamp) 
		 VALUES (?, ?, ?, ?, datetime('now'))`,
		artistID, lidarrTrackID, action, reason,
	)
	return err
}

// PruningLogInsert logs a pruning event.
func PruningLogInsert(artistID int64, albumName string, action string, reason string, count int) error {
	_, err := db.Exec(
		`INSERT INTO pruning_log (artist_id, album_name, action, reason, count, timestamp) 
		 VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		artistID, albumName, action, reason, count,
	)
	return err
}

// UpsertTrackPopularity inserts or updates track popularity.
func UpsertTrackPopularity(artistID, lidarrTrackID int64, score int, source string) error {
	_, err := db.Exec(
		`INSERT INTO track_popularity (artist_id, lidarr_track_id, play_count, last_played) 
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(artist_id, lidarr_track_id) DO UPDATE SET 
		 play_count=excluded.play_count, last_played=excluded.last_played`,
		artistID, lidarrTrackID, score, source,
	)
	return err
}

// GetTrackPopularity returns track popularity for an artist.
func GetTrackPopularity(artistID int64) ([]TrackPopularity, error) {
	rows, err := db.Query(
		`SELECT lidarr_track_id, play_count, last_played FROM track_popularity WHERE artist_id = ?`,
		artistID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pops []TrackPopularity
	for rows.Next() {
		var tp TrackPopularity
		if err := rows.Scan(&tp.LidarrTrackID, &tp.PlayCount, &tp.LastPlayed); err != nil {
			return nil, err
		}
		pops = append(pops, tp)
	}
	return pops, rows.Err()
}

// CacheFreshness returns when the popularity cache for an artist was last refreshed.
// ok=false means the cache is missing or stale (older than maxAge).
func CacheFreshness(artistID int64, maxAge time.Duration) (lastFetched time.Time, ok bool, err error) {
	var t time.Time
	err = db.QueryRow(
		`SELECT last_fetched FROM artist_popularity_cache WHERE artist_id = ?`,
		artistID,
	).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return t, time.Since(t) < maxAge, nil
}

// UpdateCacheFreshness records that an artist's popularity cache was just refreshed.
func UpdateCacheFreshness(artistID int64, artistName string, trackCount, maxPlaycount int) error {
	_, err := db.Exec(`
		INSERT INTO artist_popularity_cache (artist_id, artist_name, last_fetched, track_count, max_playcount)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(artist_id) DO UPDATE SET
			last_fetched=excluded.last_fetched,
			track_count=excluded.track_count,
			max_playcount=excluded.max_playcount`,
		artistID, artistName, time.Now(), trackCount, maxPlaycount)
	return err
}

// StaleArtistIDs returns up to `limit` artist IDs whose popularity cache is older than maxAge.
// Used by the background refresher to walk the list without hammering the DB.
func StaleArtistIDs(maxAge time.Duration, limit int) ([]int64, []string, error) {
	cutoff := time.Now().Add(-maxAge)
	rows, err := db.Query(`
		SELECT a.id, a.name
		FROM artists a
		LEFT JOIN artist_popularity_cache c ON a.id = c.artist_id
		WHERE c.last_fetched IS NULL OR c.last_fetched < ?
		ORDER BY c.last_fetched ASC NULLS FIRST
		LIMIT ?`,
		cutoff, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var ids []int64
	var names []string
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		names = append(names, name)
	}
	return ids, names, rows.Err()
}

// DeleteArtistPopularity removes cached scores for an artist (used when re-scoring).
func DeleteArtistPopularity(artistID int64) error {
	_, err := db.Exec(`DELETE FROM track_popularity WHERE artist_id = ?`, artistID)
	return err
}

// CheckLogInsert logs a check result for an album.
func CheckLogInsert(artistID int64, albumName string, deezerURL *string, popularity int, processed bool) error {
	_, err := db.Exec(
		`INSERT INTO check_log (artist_id, album_name, deezer_url, popularity, processed) 
		 VALUES (?, ?, ?, ?, ?)`,
		artistID, albumName, deezerURL, popularity, processed,
	)
	return err
}

// GetCheckLog returns check log entries for an artist.
func GetCheckLog(artistID int64, limit int) ([]CheckLogEntry, error) {
	rows, err := db.Query(
		`SELECT id, album_name, deezer_url, popularity, processed, checked_at 
		 FROM check_log WHERE artist_id = ? ORDER BY checked_at DESC LIMIT ?`,
		artistID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []CheckLogEntry
	for rows.Next() {
		var entry CheckLogEntry
		var deezerURL *string
		if err := rows.Scan(&entry.ID, &entry.AlbumName, &deezerURL, &entry.Popularity, &entry.Processed, &entry.CheckedAt); err != nil {
			return nil, err
		}
		entry.DeezURL = deezerURL
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// NeverPruneInsert adds an entry to the never_prune list.
func NeverPruneInsert(artistID int64, albumName string, trackName string) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO never_prune (artist_id, album_name, track_name) VALUES (?, ?, ?)`,
		artistID, albumName, trackName,
	)
	return err
}

// NeverPruneDelete removes an entry from the never_prune list.
func NeverPruneDelete(artistID int64, albumName string, trackName string) error {
	_, err := db.Exec(
		`DELETE FROM never_prune WHERE artist_id = ? AND album_name = ? AND track_name = ?`,
		artistID, albumName, trackName,
	)
	return err
}

// IsNeverPrune checks if a track is in the never_prune list.
func IsNeverPrune(artistID int64, albumName string, trackName string) (bool, error) {
	var exists int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM never_prune WHERE artist_id = ? AND album_name = ? AND track_name = ?`,
		artistID, albumName, trackName,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// NeverPruneTracks returns the track names that are never prune for the given artist and album.
func NeverPruneTracks(artistID int64, albumName string) ([]string, error) {
	rows, err := db.Query(
		`SELECT track_name FROM never_prune WHERE artist_id = ? AND album_name = ?`,
		artistID, albumName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []string
	for rows.Next() {
		var track string
		if err := rows.Scan(&track); err != nil {
			return nil, err
		}
		tracks = append(tracks, track)
	}
	return tracks, rows.Err()
}

// MonitoredTrackInsert adds a track to the monitored_tables list.
func MonitoredTrackInsert(artistID int64, albumName string, trackName string) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO monitored_tracks (artist_id, album_name, track_name) VALUES (?, ?, ?)`,
		artistID, albumName, trackName,
	)
	return err
}

// MonitoredTrackDelete removes a track from the monitored_tables list.
func MonitoredTrackDelete(artistID int64, albumName string, trackName string) error {
	_, err := db.Exec(
		`DELETE FROM monitored_tracks WHERE artist_id = ? AND album_name = ? AND track_name = ?`,
		artistID, albumName, trackName,
	)
	return err
}

// GetMonitoredTracks returns all monitored tracks for an artist.
func GetMonitoredTracks(artistID int64) ([]string, error) {
	rows, err := db.Query(
		`SELECT track_name FROM monitored_tracks WHERE artist_id = ?`,
		artistID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []string
	for rows.Next() {
		var track string
		if err := rows.Scan(&track); err != nil {
			return nil, err
		}
		tracks = append(tracks, track)
	}
	return tracks, rows.Err()
}

// SettingGet retrieves a setting value by key.
// Returns the value and an error if not found.
func SettingGet(key string) (string, error) {
	row := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key)
	var value string
	err := row.Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("setting not found: %s", key)
		}
		return "", err
	}
	return value, nil
}

// SettingUpdate sets a setting value by key (insert or update).
func SettingUpdate(key, value string) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)`,
		key, value,
	)
	return err
}

// SetAlbumStatus sets the album status for display in the UI.
func SetAlbumStatus(artistID int64, albumName string, status string, albumID int64) error {
	// We'll store the album status in the settings table with a key like "album_status_<artistID>_<albumName>"
	key := fmt.Sprintf("album_status_%d_%s", artistID, albumName)
	return SettingUpdate(key, status)
}

// GetAlbumStatus retrieves the album status for display in the UI.
func GetAlbumStatus(artistID int64, albumName string) (string, error) {
	key := fmt.Sprintf("album_status_%d_%s", artistID, albumName)
	return SettingGet(key)
}

// TrackPreference represents a track preference entry.
type TrackPreference struct {
	LidarrTrackID int64
	State         string // keep, hit, not_keep
	ScoreAtTime   int
}

// TrackPopularity represents track popularity data.
type TrackPopularity struct {
	LidarrTrackID int64
	PlayCount     int
	LastPlayed    *string // nullable
}

// CheckLogEntry represents a check log entry.
type CheckLogEntry struct {
	ID         int64
	AlbumName  string
	DeezURL    *string // nullable
	Popularity int
	Processed  bool
	CheckedAt  string
}