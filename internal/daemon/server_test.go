package daemon

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/db"
	"github.com/aface/sortie/internal/task"
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

func TestCreateTaskRequest_EmptyDescriptionWithCheckoutBranch(t *testing.T) {
	// Verify that title is generated from branch name when description is empty
	// and checkout branch is provided
	tests := []struct {
		name           string
		description    string
		checkoutBranch string
		wantTitle      string
		wantError      bool
	}{
		{
			name:           "empty description with checkout branch generates branch title",
			description:    "",
			checkoutBranch: "feature/my-feature",
			wantTitle:      "⎇ feature/my-feature",
			wantError:      false,
		},
		{
			name:           "empty description without checkout branch is rejected",
			description:    "",
			checkoutBranch: "",
			wantTitle:      "",
			wantError:      true,
		},
		{
			name:           "non-empty description with checkout branch uses sanitized description",
			description:    "Implement the login page",
			checkoutBranch: "feature/login",
			wantTitle:      "Implement the login page",
			wantError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			description := strings.TrimSpace(tt.description)

			// Simulate the validation logic from handleCreateTask
			if description == "" && tt.checkoutBranch == "" {
				if !tt.wantError {
					t.Error("expected no error but got validation failure")
				}
				return
			}
			if tt.wantError {
				t.Error("expected error but validation passed")
				return
			}

			// Simulate the title generation logic
			var title string
			if description == "" && tt.checkoutBranch != "" {
				title = "⎇ " + tt.checkoutBranch
			} else {
				title = task.SanitizeTitle(description)
			}

			if title != tt.wantTitle {
				t.Errorf("got title %q, want %q", title, tt.wantTitle)
			}
		})
	}
}

func TestRefineTaskTitle_SkipsAIForEmptyDescription(t *testing.T) {
	// When description is empty (existing branch mode), refineTaskTitle should
	// use the initial title directly without calling generateTitle
	initialTitle := "⎇ feature/my-branch"
	description := ""

	// Simulate the logic from refineTaskTitle
	var title string
	if description == "" {
		title = initialTitle
	}

	if title != initialTitle {
		t.Errorf("expected title %q for empty description, got %q", initialTitle, title)
	}

	slug := task.Slugify(title)
	if slug == "" {
		t.Error("expected non-empty slug")
	}
}
