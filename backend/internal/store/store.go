package store

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	_ "github.com/mattn/go-sqlite3"
)

// DB is the shared database handle.
var DB *sql.DB

// Artist represents an artist in the watchlist.
type Artist struct {
	ID          int64
	Name        string
	DeezerID    *string
	LidarrID    *int64
	RootFolder  *string
	AddedBy     string
	AddedAt     string
}

// Init opens the SQLite DB and creates tables if they don't exist.
// dbPath is the path to the SQLite database file.
func Init(dbPath string) error {
	path := dbPath
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating db dir: %w", err)
	}

	db, err := sql.Open("sqlite3", path+"?_journal=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("opening db: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enabling fk: %w", err)
	}

	DB = db
	log.Printf("Database opened at %s", path)

	return migrate()
}

// Close closes the database connection.
func Close() {
	if DB != nil {
		DB.Close()
	}
}

// migrate runs the database migrations.
func migrate() error {
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
			status          TEXT,
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
			status          TEXT    NOT NULL, -- pending, downloaded, pruned, skipped
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
	}

	for i, q := range migrations {
		if _, err := DB.Exec(q); err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}

	return nil
}

// ArtistList returns all artists from the database, ordered by name.
func ArtistList() ([]*Artist, error) {
	rows, err := DB.Query("SELECT id, name, deezer_id, lidarr_id, root_folder, added_by, added_at FROM artists ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artists []*Artist
	for rows.Next() {
		var id int64
		var name, deezerIDStr, lidarrIDStr, rootFolderStr, addedBy, addedAt string
		if err := rows.Scan(&id, &name, &deezerIDStr, &lidarrIDStr, &rootFolderStr, &addedBy, &addedAt); err != nil {
			return nil, err
		}
		var deezerID *string
		if deezerIDStr != "" {
			deezerID = &deezerIDStr
		}
		var lidarrID *int64
		if lidarrIDStr != "" {
			lidarrIDVal, _ := strconv.ParseInt(lidarrIDStr, 10, 64)
			lidarrID = &lidarrIDVal
		}
		var rootFolder *string
		if rootFolderStr != "" {
			rootFolder = &rootFolderStr
		}
		artists = append(artists, &Artist{
			ID:         id,
			Name:       name,
			DeezerID:   deezerID,
			LidarrID:   lidarrID,
			RootFolder: rootFolder,
			AddedBy:    addedBy,
			AddedAt:    addedAt,
		})
	}
	return artists, rows.Err()
}

// ArtistGet returns the artist with the given name.
func ArtistGet(name string) (*Artist, error) {
	var id int64
	var nameCol, deezerIDStr, lidarrIDStr, rootFolderStr, addedBy, addedAt string
	err := DB.QueryRow("SELECT id, name, deezer_id, lidarr_id, root_folder, added_by, added_at FROM artists WHERE name = ?", name).
		Scan(&id, &nameCol, &deezerIDStr, &lidarrIDStr, &rootFolderStr, &addedBy, &addedAt)
	if err != nil {
		return nil, err
	}
	var deezerID *string
	if deezerIDStr != "" {
		deezerID = &deezerIDStr
	}
	var lidarrID *int64
	if lidarrIDStr != "" {
		lidarrIDVal, _ := strconv.ParseInt(lidarrIDStr, 10, 64)
		lidarrID = &lidarrIDVal
	}
	var rootFolder *string
	if rootFolderStr != "" {
		rootFolder = &rootFolderStr
	}
	return &Artist{
		ID:         id,
		Name:       nameCol,
		DeezerID:   deezerID,
		LidarrID:   lidarrID,
		RootFolder: rootFolder,
		AddedBy:    addedBy,
		AddedAt:    addedAt,
	}, nil
}

// ArtistAdd inserts a new artist.
// Returns an error if the artist already exists (unique constraint on name).
func ArtistAdd(name, deezerID string, lidarrID int64, rootFolder, addedBy string) error {
	_, err := DB.Exec(`
		INSERT INTO artists (name, deezer_id, lidarr_id, root_folder, added_by)
		VALUES (?, ?, ?, ?, ?)
	`, name, deezerID, lidarrID, rootFolder, addedBy)
	return err
}

// ArtistUpdateDeezerID updates the Deezer ID for the artist with the given name.
func ArtistUpdateDeezerID(name, deezerID string) error {
	_, err := DB.Exec("UPDATE artists SET deezer_id = ? WHERE name = ?", deezerID, name)
	return err
}

// ArtistUpdateLidarrID updates the Lidarr ID for the artist with the given name.
func ArtistUpdateLidarrID(name string, lidarrID int64) error {
	_, err := DB.Exec("UPDATE artists SET lidarr_id = ? WHERE name = ?", lidarrID, name)
	return err
}

// ArtistUpdateRootFolder updates the root folder for the artist with the given name.
func ArtistUpdateRootFolder(name, rootFolder string) error {
	_, err := DB.Exec("UPDATE artists SET root_folder = ? WHERE name = ?", rootFolder, name)
	return err
}

// ArtistDelete deletes the artist with the given name.
func ArtistDelete(name string) error {
	_, err := DB.Exec("DELETE FROM artists WHERE name = ?", name)
	return err
}

// ArtistMarkChecked updates the added_at timestamp for the artist with the given ID to now.
func ArtistMarkChecked(id int64) error {
	_, err := DB.Exec("UPDATE artists SET added_at = datetime('now') WHERE id = ?", id)
	return err
}

// ArtistGetByID returns the artist with the given ID.
func ArtistGetByID(id int64) (*Artist, error) {
	var idCol int64
	var name, deezerIDStr, lidarrIDStr, rootFolderStr, addedBy, addedAt string
	err := DB.QueryRow("SELECT id, name, deezer_id, lidarr_id, root_folder, added_by, added_at FROM artists WHERE id = ?", id).
		Scan(&idCol, &name, &deezerIDStr, &lidarrIDStr, &rootFolderStr, &addedBy, &addedAt)
	if err != nil {
		return nil, err
	}
	var deezerID *string
	if deezerIDStr != "" {
		deezerID = &deezerIDStr
	}
	var lidarrID *int64
	if lidarrIDStr != "" {
		lidarrIDVal, _ := strconv.ParseInt(lidarrIDStr, 10, 64)
		lidarrID = &lidarrIDVal
	}
	var rootFolder *string
	if rootFolderStr != "" {
		rootFolder = &rootFolderStr
	}
	return &Artist{
		ID:         idCol,
		Name:       name,
		DeezerID:   deezerID,
		LidarrID:   lidarrID,
		RootFolder: rootFolder,
		AddedBy:    addedBy,
		AddedAt:    addedAt,
	}, nil
}

// SettingGet returns the value for the given key.
func SettingGet(key string) (string, error) {
	var value string
	err := DB.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	return value, err
}

// SettingSet sets the value for the given key, inserting if not present or updating if it is.
func SettingSet(key, value string) error {
	_, err := DB.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// CheckLogInsert inserts a record into the check_log table.
func CheckLogInsert(artistID int64, albumName string, deezerURL *string, avgPopularity float64, added bool) error {
	var status string
	if added {
		status = "added"
	} else {
		status = "skipped"
	}
	_, err := DB.Exec(`
		INSERT INTO check_log (artist_id, album_name, album_id, status)
		VALUES (?, ?, ?, ?)
	`, artistID, albumName, 0, status) // album_id is not used in the old code, we set to 0
	return err
}

// MonitoredTrackRecord records a monitored track.
func MonitoredTrackRecord(artistID int64, albumName, trackName string) error {
	_, err := DB.Exec(`
		INSERT OR REPLACE INTO monitored_tracks (artist_id, album_name, track_name)
		VALUES (?, ?, ?)
	`, artistID, albumName, trackName)
	return err
}

// AlbumStatusSet sets the status for an album.
func AlbumStatusSet(artistID int64, albumName, status string, lidarrAlbumID *int64) error {
	_, err := DB.Exec(`
		INSERT INTO album_status (artist_id, album_name, status, lidarr_album_id)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(artist_id, album_name) DO UPDATE SET
			status = excluded.status,
			lidarr_album_id = COALESCE(excluded.lidarr_album_id, album_status.lidarr_album_id),
			updated_at = datetime('now')
	`, artistID, albumName, status, lidarrAlbumID)
	return err
}

// AlbumStatusGet returns the status and Lidarr album ID for the given artist and album name.
func AlbumStatusGet(artistID int64, albumName string) (string, int64, error) {
	var status string
	var lidarrAlbumID int64
	err := DB.QueryRow("SELECT status, lidarr_album_id FROM album_status WHERE artist_id = ? AND album_name = ?", artistID, albumName).
		Scan(&status, &lidarrAlbumID)
	return status, lidarrAlbumID, err
}

// PendingAlbums returns a list of pending or downloading albums.
func PendingAlbums() ([]map[string]any, error) {
	rows, err := DB.Query(`
		SELECT a.id as artist_id, a.name as artist_name, s.album_name, s.lidarr_album_id
		FROM album_status s
		JOIN artists a ON s.artist_id = a.id
		WHERE s.status IN ('pending', 'downloading')
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var artistID, artistName, albumName string
		var lidarrAlbumID int64
		if err := rows.Scan(&artistID, &artistName, &albumName, &lidarrAlbumID); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"artist_id":     artistID,
			"artist_name":   artistName,
			"album_name":    albumName,
			"lidarr_album_id": lidarrAlbumID,
		})
	}
	return results, rows.Err()
}

// NeverPruneAdd adds a track to the never_prune table for the given artist and album.
func NeverPruneAdd(artistID int64, albumName, trackName string) error {
	_, err := DB.Exec(`
		INSERT OR IGNORE INTO never_prune (artist_id, album_name, track_name)
		VALUES (?, ?, ?)
	`, artistID, albumName, trackName)
	return err
}

// NeverPruneTracks returns the list of tracks that are never pruned for the given artist and album.
func NeverPruneTracks(artistID int64, albumName string) ([]string, error) {
	rows, err := DB.Query(`
		SELECT track_name FROM never_prune
		WHERE artist_id = ? AND album_name = ?
	`, artistID, albumName)
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

// NeverPruneDelete removes a track from the never_prune table for the given artist and album.
func NeverPruneDelete(artistID int64, albumName, trackName string) error {
	_, err := DB.Exec(`
		DELETE FROM never_prune
		WHERE artist_id = ? AND album_name = ? AND track_name = ?
	`, artistID, albumName, trackName)
	return err
}