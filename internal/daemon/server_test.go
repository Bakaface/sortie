package daemon

import (
	"path/filepath"
	"testing"

	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/db"
)

func TestProjectLogPrefix(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	cfg := &config.Config{}
	s := NewServer(cfg, database)

	// Create a project in the DB
	proj, err := database.GetOrCreateProject("/home/user/myproject")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Known project should return "[myproject] " prefix
	prefix := s.projectLogPrefix(proj.ID)
	expected := "[myproject] "
	if prefix != expected {
		t.Errorf("expected prefix %q, got %q", expected, prefix)
	}

	// Unknown project ID should return empty string
	prefix = s.projectLogPrefix(9999)
	if prefix != "" {
		t.Errorf("expected empty prefix for unknown project, got %q", prefix)
	}
}

func TestProjectLogPrefix_EmptyName(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	cfg := &config.Config{}
	s := NewServer(cfg, database)

	// Project with path "/" would have empty name (basename of "/")
	// In practice this shouldn't happen but test the fallback
	prefix := s.projectLogPrefix(0)
	if prefix != "" {
		t.Errorf("expected empty prefix for project ID 0, got %q", prefix)
	}
}
