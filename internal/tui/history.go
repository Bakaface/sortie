package tui

// inputHistory provides a simple ring-buffer history for command/search inputs,
// with vim-style up/down navigation and a draft line for the current unsaved input.
type inputHistory struct {
	entries []string // past entries, oldest first
	max     int      // max entries to keep
	cursor  int      // position in history (-1 = draft line)
	draft   string   // in-progress input before navigating history
}

func newInputHistory(maxEntries int) inputHistory {
	return inputHistory{
		max:    maxEntries,
		cursor: -1,
	}
}

// Push appends a non-empty, non-duplicate entry and resets the cursor.
func (h *inputHistory) Push(entry string) {
	if entry == "" {
		return
	}
	// Deduplicate: remove if already the most recent entry
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == entry {
		h.cursor = -1
		return
	}
	h.entries = append(h.entries, entry)
	if len(h.entries) > h.max {
		h.entries = h.entries[len(h.entries)-h.max:]
	}
	h.cursor = -1
}

// Up moves to the previous (older) history entry.
// On first call, it saves the current input as the draft.
// Returns the history entry to display, and ok=true if the cursor moved.
func (h *inputHistory) Up(currentInput string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.cursor == -1 {
		// Save draft before entering history
		h.draft = currentInput
		h.cursor = len(h.entries) - 1
	} else if h.cursor > 0 {
		h.cursor--
	} else {
		return h.entries[h.cursor], false // already at oldest
	}
	return h.entries[h.cursor], true
}

// Down moves to the next (newer) history entry, or back to the draft.
// Returns the entry/draft to display, and ok=true if the cursor moved.
func (h *inputHistory) Down(currentInput string) (string, bool) {
	if h.cursor == -1 {
		return "", false // already at draft
	}
	h.cursor++
	if h.cursor >= len(h.entries) {
		// Back to draft
		h.cursor = -1
		return h.draft, true
	}
	return h.entries[h.cursor], true
}

// Reset clears the navigation state (cursor and draft) without clearing history.
func (h *inputHistory) Reset() {
	h.cursor = -1
	h.draft = ""
}
