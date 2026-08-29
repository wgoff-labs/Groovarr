package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

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

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS artists (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			name            TEXT    NOT NULL UNIQUE,
			deezer_id       TEXT,
			lidarr_id       INTEGER,
			root_folder     TEXT,
			added_by        TEXT    NOT NULL,
			added_at        TEXT    NOT NULL DEFAULT (datetime('now'))
		);`,
		`CREATE TABLE IF NOT EXISTS check_log (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			artist_id       INTEGER NOT NULL,
			album_name      TEXT    NOT NULL,
			album_id        INTEGER,
			deezer_url      TEXT,
			popularity      INTEGER,
			processed       INTEGER NOT NULL DEFAULT 0,
			checked_at      TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS monitored_tracks (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			artist_id       INTEGER NOT NULL,
			album_name      TEXT    NOT NULL,
			track_name      TEXT    NOT NULL,
			added_at        TEXT    NOT NULL DEFAULT (datetime('now')),
			UNIQUE(artist_id, album_name, track_name),
			FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS album_status (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			artist_id       INTEGER NOT NULL,
			album_name      TEXT    NOT NULL,
			status          TEXT    NOT NULL,
			lidarr_album_id INTEGER,
			updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS never_prune (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			artist_id   INTEGER NOT NULL,
			album_name  TEXT    NOT NULL,
			track_name  TEXT    NOT NULL,
			UNIQUE(artist_id, album_name, track_name),
			FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS track_popularity (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			artist_id       INTEGER NOT NULL,
			lidarr_track_id INTEGER NOT NULL,
			track_name      TEXT    NOT NULL,
			album_name      TEXT    NOT NULL,
			score           INTEGER NOT NULL,
			source          TEXT    NOT NULL DEFAULT 'deezer',
			checked_at      TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE CASCADE,
			UNIQUE(artist_id, lidarr_track_id)
		);`,
		`CREATE TABLE IF NOT EXISTS track_preferences (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			artist_id           INTEGER NOT NULL,
			lidarr_track_id     INTEGER NOT NULL,
			state               TEXT    NOT NULL CHECK(state IN ('keep', 'hit', 'not_keep')),
			score_at_time       INTEGER,
			updated_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE CASCADE,
			UNIQUE(artist_id, lidarr_track_id)
		);`,
		`CREATE TABLE IF NOT EXISTS hit_fallen_log (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			artist_id           INTEGER NOT NULL,
			lidarr_track_id     INTEGER NOT NULL,
			track_name          TEXT    NOT NULL,
			score_at_fall       INTEGER NOT NULL,
			fallen_at           TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS track_actions (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			artist_id           INTEGER NOT NULL,
			lidarr_track_id     INTEGER NOT NULL,
			action              TEXT    NOT NULL CHECK(action IN ('unmonitored', 'monitored', 'score_updated')),
			performed_at        TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS pruning_log (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			artist_id           INTEGER NOT NULL,
			lidarr_track_id     INTEGER NOT NULL,
			score_at_prune      INTEGER NOT NULL,
			pruned_at           TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE CASCADE
		);`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, m)
		}
	}
	return nil
}

// --- Artist ---

type Artist struct {
	ID         int64
	Name       string
	DeezerID   *string
	LidarrID   *int64
	RootFolder *string
	AddedBy    string
	AddedAt    string
}

func ArtistList() ([]*Artist, error) {
	rows, err := db.Query(`SELECT id, name, deezer_id, lidarr_id, root_folder, added_by, added_at FROM artists ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artists []*Artist
	for rows.Next() {
		a := &Artist{}
		if err := rows.Scan(&a.ID, &a.Name, &a.DeezerID, &a.LidarrID, &a.RootFolder, &a.AddedBy, &a.AddedAt); err != nil {
			return nil, err
		}
		artists = append(artists, a)
	}
	return artists, nil
}

func ArtistGet(name string) (*Artist, error) {
	row := db.QueryRow(`SELECT id, name, deezer_id, lidarr_id, root_folder, added_by, added_at FROM artists WHERE name = ?`, name)
	a := &Artist{}
	err := row.Scan(&a.ID, &a.Name, &a.DeezerID, &a.LidarrID, &a.RootFolder, &a.AddedBy, &a.AddedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func ArtistGetByID(id int64) (*Artist, error) {
	row := db.QueryRow(`SELECT id, name, deezer_id, lidarr_id, root_folder, added_by, added_at FROM artists WHERE id = ?`, id)
	a := &Artist{}
	err := row.Scan(&a.ID, &a.Name, &a.DeezerID, &a.LidarrID, &a.RootFolder, &a.AddedBy, &a.AddedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func ArtistAdd(name, deezerID string, lidarrID int64, rootFolder, addedBy string) (int64, error) {
	// Empty deezerID/lidarrID/rootFolder are stored as NULL.
	var deezerVal interface{}
	if deezerID != "" {
		deezerVal = deezerID
	}
	var lidarrVal interface{}
	if lidarrID != 0 {
		lidarrVal = lidarrID
	}
	var rootVal interface{}
	if rootFolder != "" {
		rootVal = rootFolder
	}
	res, err := db.Exec(
		`INSERT INTO artists (name, deezer_id, lidarr_id, root_folder, added_by) VALUES (?, ?, ?, ?, ?)`,
		name, deezerVal, lidarrVal, rootVal, addedBy,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func ArtistDelete(id int64) error {
	_, err := db.Exec(`DELETE FROM artists WHERE id = ?`, id)
	return err
}

func ArtistUpdateDeezerID(name, deezerID string) error {
	_, err := db.Exec(`UPDATE artists SET deezer_id = ? WHERE name = ?`, deezerID, name)
	return err
}

func ArtistUpdateLidarrID(name string, lidarrID int64) error {
	_, err := db.Exec(`UPDATE artists SET lidarr_id = ? WHERE name = ?`, lidarrID, name)
	return err
}

func ArtistMarkChecked(id int64) error {
	_, err := db.Exec(`UPDATE artists SET last_checked = datetime('now') WHERE id = ?`, id)
	return err
}

// --- Settings ---

func SettingGet(key string) (string, error) {
	row := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key)
	var val string
	err := row.Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func SettingSet(key, value string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)`, key, value)
	return err
}

func SettingDelete(key string) error {
	_, err := db.Exec(`DELETE FROM settings WHERE key = ?`, key)
	return err
}

// --- Check Log ---

func CheckLogInsert(artistID int64, albumName string, deezerURL *string, popularity int, processed bool) error {
	deezerURLVal := ""
	if deezerURL != nil {
		deezerURLVal = *deezerURL
	}
	_, err := db.Exec(`INSERT INTO check_log (artist_id, album_name, deezer_url, popularity, processed) VALUES (?, ?, ?, ?, ?)`,
		artistID, albumName, deezerURLVal, popularity, processed)
	return err
}

func CheckLogGet(artistID int64, limit int) ([]map[string]interface{}, error) {
	rows, err := db.Query(`SELECT id, album_name, deezer_url, popularity, checked_at FROM check_log WHERE artist_id = ? ORDER BY checked_at DESC LIMIT ?`, artistID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id int
		var albumName, deezerURL string
		var popularity int
		var checkedAt string
		if err := rows.Scan(&id, &albumName, &deezerURL, &popularity, &checkedAt); err != nil {
			continue
		}
		logs = append(logs, map[string]interface{}{
			"id":          id,
			"album_name":   albumName,
			"deezer_url":  deezerURL,
			"popularity":  popularity,
			"checked_at":  checkedAt,
		})
	}
	return logs, nil
}

// --- Monitored Tracks ---

func MonitoredTrackAdd(artistID int64, albumName, trackName string) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO monitored_tracks (artist_id, album_name, track_name) VALUES (?, ?, ?)`,
		artistID, albumName, trackName)
	return err
}

func MonitoredTracks(artistID int64) ([]string, error) {
	rows, err := db.Query(`SELECT track_name FROM monitored_tracks WHERE artist_id = ?`, artistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// --- Never Prune ---

func NeverPruneAdd(artistID int64, albumName, trackName string) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO never_prune (artist_id, album_name, track_name) VALUES (?, ?, ?)`,
		artistID, albumName, trackName)
	return err
}

func NeverPruneRemove(artistID int64, albumName, trackName string) error {
	_, err := db.Exec(`DELETE FROM never_prune WHERE artist_id = ? AND album_name = ? AND track_name = ?`,
		artistID, albumName, trackName)
	return err
}

func NeverPruneTracks(artistID int64, albumName string) ([]string, error) {
	rows, err := db.Query(`SELECT track_name FROM never_prune WHERE artist_id = ? AND album_name = ?`, artistID, albumName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// --- Album Status ---

func AlbumStatusSet(artistID int64, albumName, status string, lidarrAlbumID *int64) error {
	albumID := 0
	if lidarrAlbumID != nil {
		albumID = int(*lidarrAlbumID)
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO album_status (artist_id, album_name, status, lidarr_album_id, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'))`,
		artistID, albumName, status, albumID)
	return err
}

func PendingAlbums() ([]map[string]interface{}, error) {
	rows, err := db.Query(`SELECT artist_id, album_name, lidarr_album_id FROM album_status WHERE status = 'pending'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var albums []map[string]interface{}
	for rows.Next() {
		var artistID, lidarrAlbumID int64
		var albumName string
		if err := rows.Scan(&artistID, &albumName, &lidarrAlbumID); err != nil {
			continue
		}
		albums = append(albums, map[string]interface{}{
			"artist_id":        artistID,
			"album_name":       albumName,
			"lidarr_album_id": lidarrAlbumID,
		})
	}
	return albums, nil
}

// --- Track Popularity ---

func TrackPopularityUpsert(artistID, lidarrTrackID int64, trackName, albumName string, score int, source string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO track_popularity (artist_id, lidarr_track_id, track_name, album_name, score, source, checked_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`,
		artistID, lidarrTrackID, trackName, albumName, score, source)
	return err
}

func TrackPopularityGet(artistID, lidarrTrackID int64) (int, error) {
	row := db.QueryRow(`SELECT score FROM track_popularity WHERE artist_id = ? AND lidarr_track_id = ?`, artistID, lidarrTrackID)
	var score int
	err := row.Scan(&score)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return score, err
}

// --- Track Preferences ---

func TrackPreferenceGet(artistID, lidarrTrackID int64) (string, error) {
	row := db.QueryRow(`SELECT state FROM track_preferences WHERE artist_id = ? AND lidarr_track_id = ?`, artistID, lidarrTrackID)
	var state string
	err := row.Scan(&state)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return state, err
}

func TrackPreferenceSet(artistID, lidarrTrackID int64, state string, scoreAtTime int) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO track_preferences (artist_id, lidarr_track_id, state, score_at_time, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'))`,
		artistID, lidarrTrackID, state, scoreAtTime)
	return err
}

// --- Hit Fallen Log ---

func HitFallenLogInsert(artistID, lidarrTrackID int64, trackName string, scoreAtFall int) error {
	_, err := db.Exec(`INSERT INTO hit_fallen_log (artist_id, lidarr_track_id, track_name, score_at_fall) VALUES (?, ?, ?, ?)`,
		artistID, lidarrTrackID, trackName, scoreAtFall)
	return err
}

func HitFallenLogGet(limit int) ([]map[string]interface{}, error) {
	rows, err := db.Query(`SELECT h.id, h.artist_id, a.name, h.lidarr_track_id, h.track_name, h.score_at_fall, h.fallen_at
		FROM hit_fallen_log h JOIN artists a ON h.artist_id = a.id
		ORDER BY h.fallen_at DESC LIMIT ?`, limit)
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
			"id":              id,
			"artist_id":       artistID,
			"artist_name":     artistName,
			"lidarr_track_id": lidarrTrackID,
			"track_name":      trackName,
			"score_at_fall":   scoreAtFall,
			"fallen_at":       fallenAt,
		})
	}
	return logs, nil
}

// --- Track Actions ---

func TrackActionLog(artistID, lidarrTrackID int64, action string) error {
	_, err := db.Exec(`INSERT INTO track_actions (artist_id, lidarr_track_id, action) VALUES (?, ?, ?)`,
		artistID, lidarrTrackID, action)
	return err
}

// --- Pruning Log ---

func PruningLogInsert(artistID, lidarrTrackID int64, scoreAtPrune int) error {
	_, err := db.Exec(`INSERT INTO pruning_log (artist_id, lidarr_track_id, score_at_prune) VALUES (?, ?, ?)`,
		artistID, lidarrTrackID, scoreAtPrune)
	return err
}
