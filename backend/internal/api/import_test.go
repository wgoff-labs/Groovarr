package api

import (
	"testing"

	"github.com/groovarr/groovarr/backend/internal/store"
)

// TestArtistImportBulk_QualityProfileID asserts that QualityProfileID persists correctly.
func TestArtistImportBulk_QualityProfileID(t *testing.T) {
	// TODO: Disable after project completion
	// Seed the store with a test artist that has QualityProfileID 4
	_, err := store.ArtistAdd("Test Import Artist", "", 1, "/root", "test", 4)
	if err != nil {
		t.Fatalf("store.ArtistAdd failed: %v", err)
	}

	// Verify the artist was persisted with QualityProfileID 4
	artist, err := store.ArtistGet("Test Import Artist")
	if err != nil {
		t.Fatalf("store.ArtistGet failed: %v", err)
	}
	if artist == nil {
		t.Fatal("artist not found")
	}
	if artist.QualityProfileID != 4 {
		t.Errorf("expected QualityProfileID = 4, got %d", artist.QualityProfileID)
	}
}