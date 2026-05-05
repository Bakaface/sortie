package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aface/sortie/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPromptView_HasAirplanePrompt(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")
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
			p := newPromptView(true, branchModeNew, "")
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

	p := newPromptView(true, branchModeNew, "")
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

	p := newPromptView(true, branchModeNew, "")
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
	p := newPromptView(true, branchModeNew, "")
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
	p := newPromptView(true, branchModeNew, "")
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
	p := newPromptView(true, branchModeNew, "")
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
	p := newPromptView(true, branchModeNew, "")
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
	p := newPromptView(true, branchModeNew, "")
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
	m := NewModel(nil, 0, "/tmp/test", "", false, true, 0, "")
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
			p := newPromptView(true, branchModeNew, "")
			p.SetSize(80, termHeight)

			for _, ch := range "hello world" {
				p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
				p.View()
			}

			p.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
			view := p.View()

			maxH := termHeight - 14 // must match promptView.maxHeight() reserved space
			if maxH >= 2 {          // Only check if terminal is big enough for 2 lines
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
	m := NewModel(nil, 0, "/tmp/test", "", false, true, 0, "")
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
	p := newPromptView(true, branchModeNew, "")
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
	p := newPromptView(true, branchModeNew, "")
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
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)

	view := p.View()

	// The view should contain the title and textarea — verify it renders
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestPromptView_DefaultWorktreeTrue(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")
	if !p.Worktree() {
		t.Error("expected worktree to be true when initialized with true")
	}
}

func TestPromptView_DefaultWorktreeFalse(t *testing.T) {
	p := newPromptView(false, branchModeNew, "")
	if p.Worktree() {
		t.Error("expected worktree to be false when initialized with false")
	}
}

func TestPromptView_ResetPreservesWorktreeState(t *testing.T) {
	// Start with worktree on, toggle off, then reset — should stay off
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)
	p.ToggleWorktree()
	if p.Worktree() {
		t.Error("expected worktree to be false after toggle")
	}

	p.textarea.SetValue("some task")
	p.Reset()

	if p.Worktree() {
		t.Error("expected worktree to remain false after Reset()")
	}
	if p.Value() != "" {
		t.Error("expected textarea to be cleared after Reset()")
	}
}

func TestPromptView_ResetPreservesWorktreeOn(t *testing.T) {
	// Start with worktree off, toggle on, then reset — should stay on
	p := newPromptView(false, branchModeNew, "")
	p.SetSize(80, 24)
	p.ToggleWorktree()
	if !p.Worktree() {
		t.Error("expected worktree to be true after toggle")
	}

	p.Reset()

	if !p.Worktree() {
		t.Error("expected worktree to remain true after Reset()")
	}
}

func TestPromptView_WorkflowPaneShownForMultiple(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)
	p.workflowName = "deploy"
	p.workflows = []string{"deploy", "review", "test"}
	p.workflowCursor = 0

	view := p.View()

	if !strings.Contains(view, "Workflow") {
		t.Errorf("expected Workflow pane in view, got:\n%s", view)
	}
	if !strings.Contains(view, "1. deploy") {
		t.Errorf("expected numbered workflow list, got:\n%s", view)
	}
}

func TestPromptView_WorkflowPaneHiddenForSingle(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)
	p.workflowName = "default"
	p.workflows = []string{"default"}

	view := p.View()

	if strings.Contains(view, "Workflow") {
		t.Errorf("expected no Workflow pane for single workflow, got:\n%s", view)
	}
}

func TestPromptView_NoWorkflowIndicatorInTitleBar(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)
	p.workflowName = "review"

	view := p.View()
	lines := strings.Split(view, "\n")
	titleLine := lines[0]

	if strings.Contains(titleLine, "[review]") {
		t.Errorf("expected no workflow indicator in title bar, got:\n%s", titleLine)
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

func TestPromptView_TitleInput(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)

	// Title input should start empty
	if got := p.TitleValue(); got != "" {
		t.Errorf("expected empty TitleValue initially, got %q", got)
	}

	// Set a value and verify TitleValue returns it trimmed
	p.titleInput.SetValue("  My Task  ")
	if got := p.TitleValue(); got != "My Task" {
		t.Errorf("expected TitleValue() = %q, got %q", "My Task", got)
	}
}

func TestPromptView_TitleInputPlaceholder(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")

	if p.titleInput.Placeholder != "auto-generated if left blank" {
		t.Errorf("expected placeholder %q, got %q", "auto-generated if left blank", p.titleInput.Placeholder)
	}
}

func TestPromptView_TitleInputInView(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)

	view := p.View()
	if !strings.Contains(view, "Title:") {
		t.Errorf("expected 'Title:' label in view, got:\n%s", view)
	}
}

func TestPromptView_ResetClearsTitleInput(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)

	p.titleInput.SetValue("some title")
	p.Reset()

	if got := p.TitleValue(); got != "" {
		t.Errorf("expected empty TitleValue after Reset, got %q", got)
	}
}

func TestUnderlineDInPlaceholder(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "wraps the first D in CSI 4m / CSI 24m",
			in:   "prompt Describe the task...",
			want: "prompt \x1b[4mD\x1b[24mescribe the task...",
		},
		{
			name: "preserves surrounding ANSI codes (cursor visible)",
			in:   "\x1b[40m\x1b[7mD\x1b[0m\x1b[40m\x1b[90mescribe the task...\x1b[0m",
			want: "\x1b[40m\x1b[7m\x1b[4mD\x1b[24m\x1b[0m\x1b[40m\x1b[90mescribe the task...\x1b[0m",
		},
		{
			name: "preserves surrounding ANSI codes (cursor hidden)",
			in:   "\x1b[40m\x1b[90mD\x1b[0m\x1b[40m\x1b[90mescribe the task...\x1b[0m",
			want: "\x1b[40m\x1b[90m\x1b[4mD\x1b[24m\x1b[0m\x1b[40m\x1b[90mescribe the task...\x1b[0m",
		},
		{
			name: "leaves the line untouched when no D is present",
			in:   "no capital here",
			want: "no capital here",
		},
		{
			name: "wraps only the first D",
			in:   "Done DD",
			want: "\x1b[4mD\x1b[24mone DD",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := underlineDInPlaceholder(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPromptView_PlaceholderDIsUnderlined(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)

	// Default focus is description, value is empty — placeholder is showing.
	view := p.View()

	// The view should embed the underline-on / underline-off ANSI sequence
	// around the leading "D" of "Describe the task...".
	if !strings.Contains(view, "\x1b[4mD\x1b[24m") {
		t.Errorf("expected underline ANSI around D in placeholder, view:\n%q", view)
	}
}

func TestPromptView_PlaceholderUnderlineOnlyWhenEmpty(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)

	// Type a "D" — placeholder is hidden, no underline injection should happen.
	p.textarea.SetValue("Done")
	view := p.View()

	if strings.Contains(view, "\x1b[4mD\x1b[24m") {
		t.Errorf("expected no underline ANSI around D when value is non-empty, view:\n%q", view)
	}
}

func TestPromptView_PlaceholderUnderlineOnlyWhenDescriptionFocused(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)

	// Move focus to the title field — placeholder is still showing on the
	// blurred textarea but shouldn't be decorated with the alt+d hint.
	p.focusInput(promptFieldTitle)
	view := p.View()

	if strings.Contains(view, "\x1b[4mD\x1b[24m") {
		t.Errorf("expected no underline ANSI around D when description is not focused, view:\n%q", view)
	}
}

func TestPromptView_DefaultFocusIsDescription(t *testing.T) {
	p := newPromptView(true, branchModeNew, "")

	if p.focusField != promptFieldDescription {
		t.Errorf("expected default focus to be promptFieldDescription, got %v", p.focusField)
	}
}

func TestPromptView_SwitchFocusForward(t *testing.T) {
	// Without worktree: title → description → title
	p := newPromptView(false, branchModeNew, "")
	p.SetSize(80, 24)

	// Start on description (default)
	if p.focusField != promptFieldDescription {
		t.Fatalf("expected starting focus on description, got %v", p.focusField)
	}

	p.SwitchFocus(true)
	if p.focusField != promptFieldTitle {
		t.Errorf("expected focus on title after forward switch, got %v", p.focusField)
	}

	p.SwitchFocus(true)
	if p.focusField != promptFieldDescription {
		t.Errorf("expected focus back on description after second forward switch, got %v", p.focusField)
	}
}

func TestPromptView_SwitchFocusForwardWithWorktree(t *testing.T) {
	// With worktree and branchModeNew: title → description → branch → targetBranch → title
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)

	// Start on description
	p.SwitchFocus(true)
	if p.focusField != promptFieldBranch {
		t.Errorf("expected focus on branch after forward from description, got %v", p.focusField)
	}

	p.SwitchFocus(true)
	if p.focusField != promptFieldTargetBranch {
		t.Errorf("expected focus on targetBranch, got %v", p.focusField)
	}

	p.SwitchFocus(true)
	if p.focusField != promptFieldTitle {
		t.Errorf("expected focus on title after wrapping, got %v", p.focusField)
	}

	p.SwitchFocus(true)
	if p.focusField != promptFieldDescription {
		t.Errorf("expected focus on description after title, got %v", p.focusField)
	}
}

func TestPromptView_SwitchFocusBackward(t *testing.T) {
	// Without worktree: backward from description → title → description
	p := newPromptView(false, branchModeNew, "")
	p.SetSize(80, 24)

	// Start on description
	p.SwitchFocus(false)
	if p.focusField != promptFieldTitle {
		t.Errorf("expected focus on title after backward switch, got %v", p.focusField)
	}

	p.SwitchFocus(false)
	if p.focusField != promptFieldDescription {
		t.Errorf("expected focus back on description after second backward switch, got %v", p.focusField)
	}
}

func TestPromptView_SwitchFocusBackwardWithWorktree(t *testing.T) {
	// With worktree: backward from description → title → targetBranch → branch → description
	p := newPromptView(true, branchModeNew, "")
	p.SetSize(80, 24)

	// Start on description, go backward
	p.SwitchFocus(false)
	if p.focusField != promptFieldTitle {
		t.Errorf("expected focus on title after backward from description, got %v", p.focusField)
	}

	p.SwitchFocus(false)
	if p.focusField != promptFieldTargetBranch {
		t.Errorf("expected focus on targetBranch, got %v", p.focusField)
	}

	p.SwitchFocus(false)
	if p.focusField != promptFieldBranch {
		t.Errorf("expected focus on branch, got %v", p.focusField)
	}

	p.SwitchFocus(false)
	if p.focusField != promptFieldDescription {
		t.Errorf("expected focus on description after wrapping backward, got %v", p.focusField)
	}
}

// TestModel_SelectedWorkflowAllowsEmptyDescription verifies that the prompt
// validation is bypassed when the chosen workflow's first step uses tmux.
func TestModel_SelectedWorkflowAllowsEmptyDescription(t *testing.T) {
	tr := true

	tmuxFirst := config.WorkflowConfig{
		Name: "interact",
		Steps: []config.StepConfig{
			{Name: "shell", Tmux: &tr},
			{Name: "review"},
		},
	}
	plain := config.WorkflowConfig{
		Name: "default",
		Steps: []config.StepConfig{
			{Name: "implement"},
		},
	}

	cfg := &config.Config{
		Workflows:     []config.WorkflowConfig{plain, tmuxFirst},
		TaskWorkflows: []config.WorkflowConfig{plain, tmuxFirst},
	}

	tests := []struct {
		name     string
		cfg      *config.Config
		workflow string
		want     bool
	}{
		{name: "nil cfg never allows empty", cfg: nil, workflow: "anything", want: false},
		{name: "plain workflow rejects empty", cfg: cfg, workflow: "default", want: false},
		{name: "tmux-first workflow allows empty", cfg: cfg, workflow: "interact", want: true},
		{name: "empty workflow resolves to first (plain)", cfg: cfg, workflow: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{cfg: tt.cfg, selectedWorkflow: tt.workflow}
			if got := m.selectedWorkflowAllowsEmptyDescription(); got != tt.want {
				t.Errorf("selectedWorkflowAllowsEmptyDescription() = %v, want %v", got, tt.want)
			}
		})
	}
}
