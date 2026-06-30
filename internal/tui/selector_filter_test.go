package tui

import (
	"strings"
	"testing"
)

func TestSelectorFilterable_FiltersOnTyping(t *testing.T) {
	items := []string{"alpha", "beta", "gamma", "alligator"}
	s := selector{
		kind:       selectorWorkflow,
		title:      "Pick",
		items:      append([]string(nil), items...),
		filterable: true,
		allItems:   items,
	}

	// Type "al" — should match "alpha" and "alligator"
	s.HandleKey("a")
	s.HandleKey("l")
	if len(s.items) != 2 {
		t.Fatalf("want 2 matches for 'al', got %d (%v)", len(s.items), s.items)
	}
	for _, name := range s.items {
		if !strings.Contains(strings.ToLower(name), "al") {
			t.Errorf("filtered item %q does not contain 'al'", name)
		}
	}

	// Esc with non-empty filter clears it; second esc cancels.
	r := s.HandleKey("esc")
	if r != selNone {
		t.Errorf("first esc with non-empty filter should not cancel, got %v", r)
	}
	if s.filter != "" {
		t.Errorf("filter should be cleared after esc, got %q", s.filter)
	}
	if len(s.items) != len(items) {
		t.Errorf("items should be restored to full list after clearing filter, got %d", len(s.items))
	}
	r = s.HandleKey("esc")
	if r != selCancelled {
		t.Errorf("second esc with empty filter should cancel, got %v", r)
	}
}

func TestSelectorFilterable_BackspaceShortens(t *testing.T) {
	items := []string{"foo", "bar"}
	s := selector{
		filterable: true,
		items:      append([]string(nil), items...),
		allItems:   items,
	}
	s.HandleKey("f")
	if s.filter != "f" || len(s.items) != 1 {
		t.Fatalf("after typing 'f' want filter=f, 1 item, got filter=%q items=%v", s.filter, s.items)
	}
	s.HandleKey("backspace")
	if s.filter != "" || len(s.items) != 2 {
		t.Errorf("after backspace want filter empty and full list, got filter=%q items=%v", s.filter, s.items)
	}
}

func TestSelectorFilterable_EnterChoosesFiltered(t *testing.T) {
	items := []string{"foo", "bar"}
	s := selector{
		filterable: true,
		items:      append([]string(nil), items...),
		allItems:   items,
	}
	s.HandleKey("b")
	if s.Selected() != "bar" {
		t.Fatalf("after 'b' filter, cursor 0 should select bar, got %q", s.Selected())
	}
	r := s.HandleKey("enter")
	if r != selChosen {
		t.Errorf("enter should choose, got %v", r)
	}
}

func TestSelectorView_FilterableShowsFilterLine(t *testing.T) {
	s := selector{
		kind:       selectorWorkflow,
		title:      "Pick One",
		items:      []string{"alpha", "beta"},
		filterable: true,
		allItems:   []string{"alpha", "beta"},
		filter:     "al",
	}
	// Force-apply filter so items match
	s.applyFilter()
	out := s.View()
	if !strings.Contains(out, "filter: al") {
		t.Errorf("view should show 'filter: al' line, got:\n%s", out)
	}
	if !strings.Contains(out, "type to filter") {
		t.Errorf("filterable hint missing, got:\n%s", out)
	}
}
