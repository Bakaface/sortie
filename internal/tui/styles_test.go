package tui

import (
	"strings"
	"testing"
)

func TestAppTitleContainsAirplane(t *testing.T) {
	if !strings.Contains(AppTitle, "✈") {
		t.Errorf("AppTitle should contain airplane (✈), got %q", AppTitle)
	}
}

func TestAppTitleContainsSortie(t *testing.T) {
	if !strings.Contains(AppTitle, "Sortie") {
		t.Errorf("AppTitle should contain 'Sortie', got %q", AppTitle)
	}
}

func TestPromptPrefixContainsAirplane(t *testing.T) {
	if !strings.Contains(PromptPrefix, "✈") {
		t.Errorf("PromptPrefix should contain airplane (✈), got %q", PromptPrefix)
	}
}
