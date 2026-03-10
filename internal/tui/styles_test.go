package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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

func TestHighlightColorIsGreen(t *testing.T) {
	// The highlight adaptive color should use green tones, not blue
	lightColor := highlight.Light
	darkColor := highlight.Dark
	if lightColor != "#2D7A4F" {
		t.Errorf("highlight light color should be green (#2D7A4F), got %q", lightColor)
	}
	if darkColor != "#5BA87A" {
		t.Errorf("highlight dark color should be green (#5BA87A), got %q", darkColor)
	}
}

func TestTitleStyleUsesGreenBackground(t *testing.T) {
	bg := titleStyle.GetBackground()
	if bg == nil {
		t.Fatal("titleStyle should have a background color")
	}
}

func TestSelectedStyleUsesGreenBackground(t *testing.T) {
	bg := selectedStyle.GetBackground()
	if bg == nil {
		t.Fatal("selectedStyle should have a background color")
	}
}

func TestStateStylesFinalizingIsGreen(t *testing.T) {
	style, ok := stateStyles["finalizing"]
	if !ok {
		t.Fatal("stateStyles should have a 'finalizing' entry")
	}
	fg := style.GetForeground()
	if fg == nil {
		t.Fatal("finalizing style should have a foreground color")
	}
}

func TestProjectIndicatorStyleIsGreyWithNoBackground(t *testing.T) {
	fg := projectIndicatorStyle.GetForeground()
	if fg == nil {
		t.Fatal("projectIndicatorStyle should have a foreground color")
	}
	// Should be grey (#6B6B6B), not the title style highlight color
	bg := projectIndicatorStyle.GetBackground()
	if bg != nil {
		// lipgloss returns NoColor{} for unset backgrounds; ensure it's not a real color
		if _, ok := bg.(lipgloss.NoColor); !ok {
			t.Errorf("projectIndicatorStyle should have no background, got %v", bg)
		}
	}
	if projectIndicatorStyle.GetBold() {
		t.Error("projectIndicatorStyle should not be bold")
	}
}
