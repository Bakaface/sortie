package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeValidateFixture(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".sortie.yml")
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestValidateCmd_ExplicitPath_Valid(t *testing.T) {
	path := writeValidateFixture(t, `
workflows:
  tasks:
    - name: default
      steps:
        - name: implementing
          prompt: "x"
`)
	err := validateCmd.RunE(validateCmd, []string{path})
	if err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestValidateCmd_ExplicitPath_Invalid(t *testing.T) {
	path := writeValidateFixture(t, "on_complete: boom\n")
	err := validateCmd.RunE(validateCmd, []string{path})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "on_complete") {
		t.Errorf("expected on_complete error, got: %v", err)
	}
}

func TestValidateCmd_MissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yml")
	err := validateCmd.RunE(validateCmd, []string{missing})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestValidateCmd_DefaultPath_UsesCwd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".sortie.yml"), []byte(`
workflows:
  tasks:
    - name: default
      steps:
        - name: a
          prompt: "x"
`), 0644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	if err := validateCmd.RunE(validateCmd, nil); err != nil {
		t.Fatalf("expected default-path validation to pass, got: %v", err)
	}
}

func TestValidateCmd_DefaultPath_NoConfig(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	err := validateCmd.RunE(validateCmd, nil)
	if err == nil {
		t.Fatal("expected error when no .sortie.yml in cwd")
	}
	if !strings.Contains(err.Error(), "no .sortie.yml") {
		t.Errorf("expected 'no .sortie.yml' error, got: %v", err)
	}
}
