package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPromptView_HasAirplanePrompt(t *testing.T) {
	p := newPromptView()
	if p.textarea.Prompt != PromptPrefix {
		t.Errorf("expected textarea prompt to be %q, got %q", PromptPrefix, p.textarea.Prompt)
	}
}

func TestPromptView_DetectImages(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a test image file
	testImagePath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(testImagePath, []byte("fake image data"), 0644); err != nil {
		t.Fatalf("failed to create test image: %v", err)
	}

	testCases := []struct {
		name           string
		input          string
		expectedImages int
		expectedText   string
	}{
		{
			name:           "detects valid image path",
			input:          testImagePath,
			expectedImages: 1,
			expectedText:   "",
		},
		{
			name:           "ignores non-existent image",
			input:          "/nonexistent/image.png",
			expectedImages: 0,
			expectedText:   "/nonexistent/image.png",
		},
		{
			name:           "detects image with text before",
			input:          "Add this feature\n" + testImagePath,
			expectedImages: 1,
			expectedText:   "Add this feature",
		},
		{
			name:           "detects image with text after",
			input:          testImagePath + "\nAdd this feature",
			expectedImages: 1,
			expectedText:   "Add this feature",
		},
		{
			name:           "preserves regular text",
			input:          "This is a task description",
			expectedImages: 0,
			expectedText:   "This is a task description",
		},
		{
			name:           "detects multiple images",
			input:          testImagePath + "\n" + filepath.Join(tmpDir, "test2.jpg"),
			expectedImages: 1, // Only one exists
			expectedText:   filepath.Join(tmpDir, "test2.jpg"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := newPromptView()
			p.SetSize(80, 24)

			// Set the textarea value
			p.textarea.SetValue(tc.input)

			// Trigger image detection
			p.detectImages()

			// Check number of images
			if len(p.images) != tc.expectedImages {
				t.Errorf("expected %d images, got %d", tc.expectedImages, len(p.images))
			}

			// Check remaining text
			remainingText := p.Value()
			if remainingText != tc.expectedText {
				t.Errorf("expected text %q, got %q", tc.expectedText, remainingText)
			}
		})
	}
}

func TestPromptView_RemoveLastImage(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test image files
	img1 := filepath.Join(tmpDir, "test1.png")
	img2 := filepath.Join(tmpDir, "test2.png")
	os.WriteFile(img1, []byte("fake"), 0644)
	os.WriteFile(img2, []byte("fake"), 0644)

	p := newPromptView()
	p.SetSize(80, 24)

	// Add two images
	p.textarea.SetValue(img1 + "\n" + img2)
	p.detectImages()

	if len(p.images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(p.images))
	}

	// Remove last image
	p.RemoveLastImage()

	if len(p.images) != 1 {
		t.Errorf("expected 1 image after removal, got %d", len(p.images))
	}

	if p.images[0] != img1 {
		t.Errorf("expected first image to remain, got %s", p.images[0])
	}
}

func TestPromptView_Update(t *testing.T) {
	tmpDir := t.TempDir()
	testImage := filepath.Join(tmpDir, "test.png")
	os.WriteFile(testImage, []byte("fake"), 0644)

	p := newPromptView()
	p.SetSize(80, 24)

	// Simulate typing a path
	for _, char := range testImage {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}}
		p.Update(msg)
	}

	// Simulate newline to trigger detection
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	p.Update(msg)

	if len(p.images) != 1 {
		t.Errorf("expected 1 image after typing path, got %d", len(p.images))
	}
}

func TestPromptView_VisualLineCount(t *testing.T) {
	p := newPromptView()
	p.SetSize(80, 40)

	// Empty textarea should show 1 visual line
	if n := p.visualLineCount(); n != 1 {
		t.Errorf("expected 1 visual line for empty content, got %d", n)
	}

	// With content, visual lines should grow
	p.textarea.SetValue("line 1\nline 2\nline 3")
	if n := p.visualLineCount(); n != 3 {
		t.Errorf("expected 3 visual lines for 3 lines, got %d", n)
	}
}

func TestPromptView_AutoGrow(t *testing.T) {
	p := newPromptView()
	p.SetSize(80, 40)

	// Empty: view shows 1 line of textarea content
	view := p.View()
	taLines := countTextareaLines(view)
	if taLines != 1 {
		t.Errorf("expected 1 visible textarea line for empty content, got %d", taLines)
	}

	// 2 lines of content
	p.textarea.SetValue("line 1\nline 2")
	view = p.View()
	taLines = countTextareaLines(view)
	if taLines != 2 {
		t.Errorf("expected 2 visible textarea lines, got %d", taLines)
	}

	// 5 lines of content
	p.textarea.SetValue("line 1\nline 2\nline 3\nline 4\nline 5")
	view = p.View()
	taLines = countTextareaLines(view)
	if taLines != 5 {
		t.Errorf("expected 5 visible textarea lines, got %d", taLines)
	}

	// Shrinks back after reset
	p.Reset()
	view = p.View()
	taLines = countTextareaLines(view)
	if taLines != 1 {
		t.Errorf("expected 1 visible textarea line after reset, got %d", taLines)
	}
}

func TestPromptView_AutoGrowWrapping(t *testing.T) {
	p := newPromptView()
	// Set narrow width: content width = 30 - 4 - promptWidth
	p.SetSize(30, 40)

	// Content width is 30 - 4 - 2 = 24 chars (assuming prompt "✈ " is 2 wide)
	// A line of 48 chars should wrap to 2 visual lines
	p.textarea.SetValue("abcdefghijklmnopqrstuvwxabcdefghijklmnopqrstuvwx")
	if n := p.visualLineCount(); n < 2 {
		t.Errorf("expected visual line count >= 2 for long wrapped line, got %d", n)
	}
}

// countTextareaLines counts the number of textarea lines in the prompt view output.
// It counts lines between the title and the help text that contain the prompt character.
func countTextareaLines(view string) int {
	lines := strings.Split(view, "\n")
	count := 0
	for _, line := range lines {
		if strings.Contains(line, PromptPrefix) || strings.Contains(line, "✈") {
			count++
		}
	}
	return count
}

func TestVisualLineCount(t *testing.T) {
	p := newPromptView()
	p.SetSize(80, 40)

	tests := []struct {
		name     string
		value    string
		expected int
	}{
		{"empty", "", 1},
		{"single line", "hello", 1},
		{"two lines", "hello\nworld", 2},
		{"trailing newline", "hello\n", 2},
		{"empty lines", "\n\n\n", 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p.textarea.SetValue(tc.value)
			got := p.visualLineCount()
			if got != tc.expected {
				t.Errorf("visualLineCount() = %d, want %d", got, tc.expected)
			}
		})
	}
}

func TestPromptView_NewlinePreservesFirstLine(t *testing.T) {
	p := newPromptView()
	p.SetSize(80, 40)

	// Type "hello" one character at a time via Update, calling View after each
	// (like bubbletea does: Update → View → Update → View → ...)
	for _, ch := range "hello" {
		p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		p.View() // side-effect: sets viewport content
	}

	// Verify "hello" is visible before the newline
	view := p.View()
	if !strings.Contains(view, "hello") {
		t.Fatalf("expected 'hello' in view before newline, got:\n%s", view)
	}

	// Press ctrl+j to insert a newline
	p.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})

	// After the newline, the first line ("hello") must still be visible
	view = p.View()
	if !strings.Contains(view, "hello") {
		t.Fatalf("first line disappeared after newline! view:\n%s", view)
	}
}

// TestPromptView_NewlineViaParentModel tests the exact flow through the parent
// Model, mimicking how bubbletea routes messages.
func TestPromptView_NewlineViaParentModel(t *testing.T) {
	m := NewModel(nil, 0, "/tmp/test", false)
	// Simulate window size
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = result.(Model)

	// Switch to prompt view
	m.view = viewPrompt
	m.prompt.Focus()

	// Type "hello" through the parent model, calling View after each
	for _, ch := range "hello" {
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = result.(Model)
		m.View()
	}

	// Verify "hello" is in the view
	view := m.View()
	if !strings.Contains(view, "hello") {
		t.Fatalf("expected 'hello' in view, got:\n%s", view)
	}

	// Press ctrl+j to insert newline
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m = result.(Model)

	view = m.View()
	if !strings.Contains(view, "hello") {
		t.Fatalf("first line disappeared after newline via parent model! view:\n%s", view)
	}
}

func TestPromptView_NewlinePreservesFirstLine_SmallTerminal(t *testing.T) {
	for _, termHeight := range []int{8, 10, 12, 20, 40} {
		t.Run(fmt.Sprintf("height=%d", termHeight), func(t *testing.T) {
			p := newPromptView()
			p.SetSize(80, termHeight)

			for _, ch := range "hello world" {
				p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
				p.View()
			}

			p.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
			view := p.View()

			maxH := termHeight - 8 // must match promptView.maxHeight() reserved space
			if maxH >= 2 {         // Only check if terminal is big enough for 2 lines
				if !strings.Contains(view, "hello") {
					t.Errorf("first line disappeared at terminal height %d! view:\n%s", termHeight, view)
				}
			}
		})
	}
}

// TestPromptView_NewlineWithInterleaved tests with non-key messages between
// keystrokes, simulating cursor blink and tick messages in the real runtime.
func TestPromptView_NewlineWithInterleaved(t *testing.T) {
	m := NewModel(nil, 0, "/tmp/test", false)
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = result.(Model)

	m.view = viewPrompt
	m.prompt.Focus()

	// Type "hello" with interleaved non-key messages and View calls
	for _, ch := range "hello" {
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = result.(Model)
		m.View()

		// Simulate a tick message (happens between keystrokes)
		result, _ = m.Update(tickMsg{})
		m = result.(Model)
		m.View()
	}

	// Press ctrl+j with an interleaved tick before checking view
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m = result.(Model)
	m.View() // First view after newline

	// A tick fires
	result, _ = m.Update(tickMsg{})
	m = result.(Model)
	view := m.View() // Second view, after tick

	if !strings.Contains(view, "hello") {
		t.Fatalf("first line disappeared after newline + tick! view:\n%s", view)
	}
}

func TestPromptView_NewlineAfterLongLine(t *testing.T) {
	p := newPromptView()
	p.SetSize(40, 20) // Narrow terminal to force wrapping

	// Type a long line that will wrap
	longText := "this is a long line that should wrap in a narrow terminal"
	for _, ch := range longText {
		p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		p.View()
	}

	viewBefore := p.View()
	if !strings.Contains(viewBefore, "this") {
		t.Fatalf("expected start of text in view, got:\n%s", viewBefore)
	}

	// Press ctrl+j
	p.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	view := p.View()

	if !strings.Contains(view, "this") {
		t.Fatalf("first line start disappeared after newline! view:\n%s", view)
	}
}

func TestPromptView_MultipleNewlines(t *testing.T) {
	p := newPromptView()
	p.SetSize(80, 40)

	// Type first line
	for _, ch := range "line one" {
		p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		p.View()
	}

	// Insert first newline
	p.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	p.View()

	// Type second line
	for _, ch := range "line two" {
		p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		p.View()
	}

	// Insert second newline
	p.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	view := p.View()

	if !strings.Contains(view, "line one") {
		t.Fatalf("first line disappeared after multiple newlines! view:\n%s", view)
	}
	if !strings.Contains(view, "line two") {
		t.Fatalf("second line disappeared after multiple newlines! view:\n%s", view)
	}
}

func TestPromptView_ViewPadding(t *testing.T) {
	p := newPromptView()
	p.SetSize(80, 24)

	view := p.View()

	// The view should contain the title and textarea — verify it renders
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestIsImagePath(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"/path/to/image.png", true},
		{"/path/to/image.jpg", true},
		{"/path/to/image.jpeg", true},
		{"/path/to/image.gif", true},
		{"/path/to/image.webp", true},
		{"/path/to/image.PNG", true}, // Case insensitive
		{"~/image.png", true},
		{"./relative/image.png", true},
		{"../parent/image.jpg", true},
		{"/path/to/file.txt", false},
		{"not a path", false},
		{"image.png", false}, // Must start with /, ~, or .
		{"/path/to/dir/", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := isImagePath(tc.input)
			if result != tc.expected {
				t.Errorf("isImagePath(%q) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}
