package tui

import "testing"

func TestInputHistory_PushAndUp(t *testing.T) {
	h := newInputHistory(10)

	h.Push("first")
	h.Push("second")
	h.Push("third")

	// Up from draft should give most recent
	entry, ok := h.Up("current")
	if !ok || entry != "third" {
		t.Fatalf("expected 'third', got %q (ok=%v)", entry, ok)
	}
	entry, ok = h.Up("")
	if !ok || entry != "second" {
		t.Fatalf("expected 'second', got %q (ok=%v)", entry, ok)
	}
	entry, ok = h.Up("")
	if !ok || entry != "first" {
		t.Fatalf("expected 'first', got %q (ok=%v)", entry, ok)
	}
	// Already at oldest — should not move
	_, ok = h.Up("")
	if ok {
		t.Fatal("expected ok=false at oldest entry")
	}
}

func TestInputHistory_DownReturnsToDraft(t *testing.T) {
	h := newInputHistory(10)

	h.Push("alpha")
	h.Push("beta")

	// Navigate up
	h.Up("my draft")
	h.Up("")

	// Down should go back toward draft
	entry, ok := h.Down("")
	if !ok || entry != "beta" {
		t.Fatalf("expected 'beta', got %q (ok=%v)", entry, ok)
	}
	entry, ok = h.Down("")
	if !ok || entry != "my draft" {
		t.Fatalf("expected 'my draft', got %q (ok=%v)", entry, ok)
	}
	// Already at draft
	_, ok = h.Down("")
	if ok {
		t.Fatal("expected ok=false at draft")
	}
}

func TestInputHistory_Dedup(t *testing.T) {
	h := newInputHistory(10)

	h.Push("same")
	h.Push("same")

	if len(h.entries) != 1 {
		t.Fatalf("expected 1 entry after dedup, got %d", len(h.entries))
	}
}

func TestInputHistory_EmptyPush(t *testing.T) {
	h := newInputHistory(10)

	h.Push("")
	if len(h.entries) != 0 {
		t.Fatal("empty string should not be pushed")
	}
}

func TestInputHistory_MaxEntries(t *testing.T) {
	h := newInputHistory(3)

	h.Push("a")
	h.Push("b")
	h.Push("c")
	h.Push("d")

	if len(h.entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(h.entries))
	}
	if h.entries[0] != "b" {
		t.Fatalf("expected oldest to be 'b', got %q", h.entries[0])
	}
}

func TestInputHistory_Reset(t *testing.T) {
	h := newInputHistory(10)

	h.Push("x")
	h.Up("draft")

	h.Reset()

	if h.cursor != -1 {
		t.Fatal("cursor should be -1 after reset")
	}
	// History entries should still be there
	if len(h.entries) != 1 {
		t.Fatal("entries should be preserved after reset")
	}
}

func TestInputHistory_UpWithNoHistory(t *testing.T) {
	h := newInputHistory(10)

	_, ok := h.Up("something")
	if ok {
		t.Fatal("Up with no history should return ok=false")
	}
}

func TestInputHistory_NonConsecutiveDedup(t *testing.T) {
	h := newInputHistory(10)

	h.Push("a")
	h.Push("b")
	h.Push("a") // not consecutive with first "a", so should be kept

	if len(h.entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(h.entries))
	}
}
