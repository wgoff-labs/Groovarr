package clients

import (
	"reflect"
	"testing"
)

// ── TrackNumberInt ─────────────────────────────────────────────────────────────
//
// Lidarr has shipped trackNumber as both a JSON number and a JSON string across
// different versions, and the field is sometimes missing or null. The whole point
// of TrackNumberInt() is to accept any of those shapes and return a safe int.
// These cases are the ones that have caused real unmarshal errors in production.

func TestTrackNumberInt(t *testing.T) {
	tests := []struct {
		name string
		in   LidarrTrack
		want int
	}{
		// JSON number (Lidarr v3+ default)
		{"float64 number", LidarrTrack{TrackNumber: float64(7)}, 7},
		{"float64 zero", LidarrTrack{TrackNumber: float64(0)}, 0},
		{"int", LidarrTrack{TrackNumber: 3}, 3},
		{"int64", LidarrTrack{TrackNumber: int64(12)}, 12},

		// JSON string (older Lidarr versions)
		{"string with value", LidarrTrack{TrackNumber: "4"}, 4},
		{"string with whitespace", LidarrTrack{TrackNumber: "  9  "}, 9},
		{"string that is not a number", LidarrTrack{TrackNumber: "abc"}, 0},
		{"empty string", LidarrTrack{TrackNumber: ""}, 0},

		// JSON null or missing — `nil` after unmarshal, no fallback info available
		{"nil", LidarrTrack{TrackNumber: nil}, 0},

		// Unexpected types (defensive — should not crash, should return 0)
		{"bool true", LidarrTrack{TrackNumber: true}, 0},
		{"bool false", LidarrTrack{TrackNumber: false}, 0},
		{"map", LidarrTrack{TrackNumber: map[string]int{"k": 1}}, 0},
		{"slice", LidarrTrack{TrackNumber: []int{1, 2}}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TrackNumberInt(tt.in)
			if got != tt.want {
				t.Errorf("TrackNumberInt(%#v) = %d, want %d", tt.in.TrackNumber, got, tt.want)
			}
		})
	}
}

// ── LidarrAlbum.ReleaseYear ────────────────────────────────────────────────────

func TestLidarrAlbumReleaseYear(t *testing.T) {
	tests := []struct {
		name string
		in   LidarrAlbum
		want int
	}{
		{"normal ISO date", LidarrAlbum{ReleaseDate: "2024-03-15"}, 2024},
		{"ISO datetime with T", LidarrAlbum{ReleaseDate: "1999-12-31T00:00:00Z"}, 1999},
		{"with leading whitespace", LidarrAlbum{ReleaseDate: "  2010-06-01"}, 2010},
		{"empty string", LidarrAlbum{ReleaseDate: ""}, 0},
		{"too short", LidarrAlbum{ReleaseDate: "20"}, 0},
		{"non-numeric year", LidarrAlbum{ReleaseDate: "abcd-01-01"}, 0},
		{"untrimmed garbage", LidarrAlbum{ReleaseDate: "----"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.ReleaseYear()
			if got != tt.want {
				t.Errorf("ReleaseYear(%q) = %d, want %d", tt.in.ReleaseDate, got, tt.want)
			}
		})
	}
}

// ── GetAlbumTracks fallback stamping ──────────────────────────────────────────
//
// The HTTP handler in api/manage.go depends on GetAlbumTracks() stamping the
// 1-based array position as TrackNumber whenever the field is zero/missing.
// This test exercises the fallback by building tracks in-process (no live
// HTTP) and verifying the stamp. It avoids touching the network by using a
// zero-value LidarrClient and a httptest.Server. The httptest server proves
// the full request/response path works.

func TestGetAlbumTracksStampsFallback(t *testing.T) {
	const albumID = 42

	// Lidarr returns these three tracks. The first two have valid trackNumbers
	// (one as a JSON number, one as a JSON string — both versions we see in
	// the wild). The third has trackNumber explicitly set to 0 to force the
	// fallback path. We assert that the third gets stamped with position 3.
	lidarrJSON := `[
		{"id": 1, "title": "Intro",  "trackNumber": 1,  "monitored": true, "hasFile": false, "trackFileId": 0, "albumId": 42},
		{"id": 2, "title": "Verse",  "trackNumber": "2", "monitored": true, "hasFile": false, "trackFileId": 0, "albumId": 42},
		{"id": 3, "title": "Outro",  "trackNumber": 0,  "monitored": true, "hasFile": false, "trackFileId": 0, "albumId": 42}
	]`

	server := newTestLidarrServer(t, "/api/v1/track?albumId=42", lidarrJSON)
	defer server.Close()

	c, err := NewLidarrClientWith(server.URL, "test-key")
	if err != nil {
		t.Fatalf("NewLidarrClientWith: %v", err)
	}

	tracks, err := c.GetAlbumTracks(albumID)
	if err != nil {
		t.Fatalf("GetAlbumTracks: %v", err)
	}
	if len(tracks) != 3 {
		t.Fatalf("expected 3 tracks, got %d", len(tracks))
	}

	// First two should be unchanged (their JSON values parse cleanly).
	if got := TrackNumberInt(tracks[0]); got != 1 {
		t.Errorf("track 0 TrackNumber = %d, want 1", got)
	}
	if got := TrackNumberInt(tracks[1]); got != 2 {
		t.Errorf("track 1 TrackNumber = %d, want 2 (string parsed)", got)
	}
	// Third had trackNumber=0 — should have been stamped with its 1-based index.
	if got := TrackNumberInt(tracks[2]); got != 3 {
		t.Errorf("track 2 TrackNumber = %d, want 3 (fallback stamp)", got)
	}

	// Sanity: titles are decoded in the right order.
	wantTitles := []string{"Intro", "Verse", "Outro"}
	gotTitles := make([]string, len(tracks))
	for i, tr := range tracks {
		gotTitles[i] = tr.Title
	}
	if !reflect.DeepEqual(gotTitles, wantTitles) {
		t.Errorf("titles = %v, want %v", gotTitles, wantTitles)
	}
}

// TestGetAlbumTracksEmptyResponse covers the (degenerate) case of Lidarr
// returning an empty track list. We must still return an empty slice, not nil
// in a way the JSON encoder would treat as `null` — `make([]LidarrTrack, 0)`
// is the typical pattern. The handler in api/manage.go guards on
// `len(albumTracks) > 0` for the Year column, so a fully empty album must
// still produce sensible output downstream.
func TestGetAlbumTracksEmptyResponse(t *testing.T) {
	server := newTestLidarrServer(t, "/api/v1/track?albumId=99", `[]`)
	defer server.Close()

	c, err := NewLidarrClientWith(server.URL, "test-key")
	if err != nil {
		t.Fatalf("NewLidarrClientWith: %v", err)
	}

	tracks, err := c.GetAlbumTracks(99)
	if err != nil {
		t.Fatalf("GetAlbumTracks: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("expected 0 tracks, got %d", len(tracks))
	}
}
