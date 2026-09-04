package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/store"
)

func TestScheduler(t *testing.T) {
	// Create a temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_scheduler.db")

	// Clean up global config for testing
	config.Reset()

	// Initialize database
	if err := store.Init(dbPath); err != nil {
		t.Fatalf("Database init failed: %v", err)
	}
	defer store.Close()

	// Just verify the scheduler can be created without LoadPersistedSettings
	_ = New(func(string) {})
}
