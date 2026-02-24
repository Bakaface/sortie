package tui

import (
	"os"
	"path/filepath"
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

func TestPromptView_TextareaHeight(t *testing.T) {
	p := newPromptView()

	// Normal terminal size: textarea should be compact (5 lines)
	p.SetSize(80, 40)
	if h := p.textarea.Height(); h != 5 {
		t.Errorf("expected textarea height 5 for large terminal, got %d", h)
	}

	// Small terminal: textarea should be clamped to minimum 3
	p.SetSize(80, 8)
	if h := p.textarea.Height(); h < 3 {
		t.Errorf("expected textarea height >= 3 for small terminal, got %d", h)
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
