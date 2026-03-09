package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectGoUsesBaseName(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "go.mod")
	content := "module github.com/Bakaface/sortie\n\ngo 1.23\n"
	if err := os.WriteFile(goMod, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p := DetectProject(dir)
	if p.Type != ProjectTypeGo {
		t.Fatalf("expected Go project, got %s", p.Type)
	}
	if p.Name != "sortie" {
		t.Errorf("expected name 'sortie', got %q", p.Name)
	}
}

func TestDetectGoSimpleModuleName(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "go.mod")
	content := "module myapp\n\ngo 1.23\n"
	if err := os.WriteFile(goMod, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p := DetectProject(dir)
	if p.Type != ProjectTypeGo {
		t.Fatalf("expected Go project, got %s", p.Type)
	}
	if p.Name != "myapp" {
		t.Errorf("expected name 'myapp', got %q", p.Name)
	}
}

func TestApplyDetectedProjectSetsName(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "go.mod")
	content := "module github.com/example/coolapp\n\ngo 1.23\n"
	if err := os.WriteFile(goMod, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	cfg.syncCompat() // sets AutoDetect = true
	cfg.ApplyDetectedProject(dir)

	if cfg.Project.Name != "coolapp" {
		t.Errorf("expected project name 'coolapp', got %q", cfg.Project.Name)
	}
}

func TestApplyDetectedProjectDoesNotOverrideExplicitName(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "go.mod")
	content := "module github.com/example/coolapp\n\ngo 1.23\n"
	if err := os.WriteFile(goMod, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	cfg.syncCompat() // sets AutoDetect = true
	cfg.Project.Name = "my-custom-name"
	cfg.ApplyDetectedProject(dir)

	if cfg.Project.Name != "my-custom-name" {
		t.Errorf("expected project name 'my-custom-name', got %q", cfg.Project.Name)
	}
}

func TestDetectNodeProject(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := filepath.Join(dir, "package.json")
	content := `{"name": "my-node-app", "scripts": {"test": "jest"}}`
	if err := os.WriteFile(pkgJSON, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p := DetectProject(dir)
	if p.Type != ProjectTypeNode {
		t.Fatalf("expected Node project, got %s", p.Type)
	}
	if p.Name != "my-node-app" {
		t.Errorf("expected name 'my-node-app', got %q", p.Name)
	}
}
