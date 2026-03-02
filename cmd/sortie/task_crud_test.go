package main

import (
	"bytes"
	"testing"
)

func TestCreateCmd_MissingDescription(t *testing.T) {
	// create with no args and no stdin should fail
	cmd := createCmd
	cmd.SetArgs([]string{})
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error for missing description")
	}
	if got := err.Error(); got != "description is required (provide as argument or via stdin)" {
		t.Errorf("unexpected error: %s", got)
	}
}

func TestCreateCmd_InvalidPriority(t *testing.T) {
	cmd := createCmd
	cmd.SetArgs([]string{"test task"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.Flags().Set("priority", "invalid")

	err := cmd.RunE(cmd, []string{"test task"})
	if err == nil {
		t.Fatal("expected error for invalid priority")
	}
	expected := `invalid priority "invalid" (valid: low, medium, high, urgent)`
	if got := err.Error(); got != expected {
		t.Errorf("unexpected error: %q, want %q", got, expected)
	}
	// Reset flag for other tests
	cmd.Flags().Set("priority", "")
}

func TestEditCmd_MissingTaskID(t *testing.T) {
	// edit with non-numeric task ID
	err := editCmd.RunE(editCmd, []string{"abc"})
	if err == nil {
		t.Fatal("expected error for invalid task ID")
	}
	if got := err.Error(); got != "invalid task ID: abc" {
		t.Errorf("unexpected error: %s", got)
	}
}

func TestEditCmd_NoFlags(t *testing.T) {
	cmd := editCmd
	// Ensure all field flags are empty
	cmd.Flags().Set("title", "")
	cmd.Flags().Set("description", "")
	cmd.Flags().Set("context", "")
	cmd.Flags().Set("priority", "")

	err := cmd.RunE(cmd, []string{"1"})
	if err == nil {
		t.Fatal("expected error when no field flags provided")
	}
	expected := "at least one field flag is required (--title, --description, --context, --priority)"
	if got := err.Error(); got != expected {
		t.Errorf("unexpected error: %q, want %q", got, expected)
	}
}

func TestEditCmd_InvalidPriority(t *testing.T) {
	cmd := editCmd
	cmd.Flags().Set("priority", "banana")

	err := cmd.RunE(cmd, []string{"1"})
	if err == nil {
		t.Fatal("expected error for invalid priority")
	}
	expected := `invalid priority "banana" (valid: low, medium, high, urgent)`
	if got := err.Error(); got != expected {
		t.Errorf("unexpected error: %q, want %q", got, expected)
	}
	cmd.Flags().Set("priority", "")
}

func TestDeleteCmd_InvalidTaskID(t *testing.T) {
	err := deleteCmd.RunE(deleteCmd, []string{"notanumber"})
	if err == nil {
		t.Fatal("expected error for invalid task ID")
	}
	if got := err.Error(); got != "invalid task ID: notanumber" {
		t.Errorf("unexpected error: %s", got)
	}
}
