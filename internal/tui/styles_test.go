package tui

import (
	"strings"
	"testing"
)

func TestAppTitleContainsCrossedSwords(t *testing.T) {
	if !strings.Contains(AppTitle, "⚔") {
		t.Errorf("AppTitle should contain crossed swords (⚔), got %q", AppTitle)
	}
}

func TestAppTitleContainsSortie(t *testing.T) {
	if !strings.Contains(AppTitle, "Sortie") {
		t.Errorf("AppTitle should contain 'Sortie', got %q", AppTitle)
	}
}
