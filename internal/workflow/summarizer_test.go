package workflow

import (
	"fmt"
	"strings"
	"testing"
)

func TestShouldSummarizeChat(t *testing.T) {
	bigChat := strings.Repeat("x", smallChatBytes)
	smallChat := strings.Repeat("x", smallChatBytes/4)

	tests := []struct {
		name       string
		chat       string
		resultText string
		useTmux    bool
		want       bool
	}{
		{"tmux always summarizes", smallChat, "irrelevant", true, true},
		{"tmux with no result text still summarizes", smallChat, "", true, true},
		{"non-tmux empty result text always summarizes", smallChat, "", false, true},
		{"non-tmux small chat with result text short-circuits", smallChat, "Done!", false, false},
		{"non-tmux large chat with result text summarizes", bigChat, "Done!", false, true},
		{"non-tmux whitespace-only result text counts as empty", smallChat, "   \n  ", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSummarizeChat(tt.chat, tt.resultText, tt.useTmux); got != tt.want {
				t.Errorf("shouldSummarizeChat(len=%d, result=%q, tmux=%v) = %v, want %v",
					len(tt.chat), tt.resultText, tt.useTmux, got, tt.want)
			}
		})
	}
}

func TestSplitOnLineBoundary(t *testing.T) {
	t.Run("under limit returns single chunk", func(t *testing.T) {
		input := "one\ntwo\nthree"
		got := splitOnLineBoundary(input, 1024)
		if len(got) != 1 || got[0] != input {
			t.Errorf("got %#v, want [%q]", got, input)
		}
	})

	t.Run("splits on line boundaries without breaking lines", func(t *testing.T) {
		// Each line is ~50 bytes; with maxBytes=100 we expect ~2 lines per chunk.
		var lines []string
		for i := 0; i < 10; i++ {
			lines = append(lines, fmt.Sprintf("line-%02d-%s", i, strings.Repeat("x", 40)))
		}
		input := strings.Join(lines, "\n")
		chunks := splitOnLineBoundary(input, 100)
		if len(chunks) < 2 {
			t.Fatalf("expected multiple chunks, got %d", len(chunks))
		}
		// Reassembling the chunks with newlines must recover the original content.
		if got := strings.Join(chunks, "\n"); got != input {
			t.Errorf("reassembled chunks != input\nwant: %q\ngot:  %q", input, got)
		}
		// Each chunk must end at a complete line.
		for i, ch := range chunks {
			if strings.HasSuffix(ch, "\n") {
				t.Errorf("chunk %d has trailing newline: %q", i, ch)
			}
		}
	})

	t.Run("oversized single line yields its own oversized chunk", func(t *testing.T) {
		long := strings.Repeat("a", 300)
		input := "short\n" + long + "\nshort"
		chunks := splitOnLineBoundary(input, 100)
		if len(chunks) < 2 {
			t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
		}
		// The long line must appear intact in exactly one chunk.
		found := false
		for _, ch := range chunks {
			if strings.Contains(ch, long) {
				if found {
					t.Errorf("long line appears in multiple chunks")
				}
				found = true
			}
		}
		if !found {
			t.Errorf("long line not found intact in any chunk: %#v", chunks)
		}
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		got := splitOnLineBoundary("", 100)
		if len(got) != 1 || got[0] != "" {
			t.Errorf("got %#v, want [\"\"]", got)
		}
	})
}

func TestMaxPromptBytesForModel(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"haiku", haikuPromptByteLimit},
		{"", haikuPromptByteLimit},                          // empty falls back to haiku limit
		{"claude-haiku-4-5-20251001", haikuPromptByteLimit}, // full id unknown -> defaults to haiku limit
		{"sonnet", sonnetPromptByteLimit},
		{"opus", opusPromptByteLimit},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := maxPromptBytesForModel(tt.model); got != tt.want {
				t.Errorf("maxPromptBytesForModel(%q) = %d, want %d", tt.model, got, tt.want)
			}
		})
	}

	// Empirically calibrated ordering — see scripts/measure-claude-limits.
	// Haiku < sonnet < opus must hold for chooseSummarizationModel to keep
	// preferring the cheapest fitting model.
	if !(haikuPromptByteLimit < sonnetPromptByteLimit && sonnetPromptByteLimit < opusPromptByteLimit) {
		t.Errorf("expected haiku(%d) < sonnet(%d) < opus(%d)", haikuPromptByteLimit, sonnetPromptByteLimit, opusPromptByteLimit)
	}
}

func TestChunkBytesForModel(t *testing.T) {
	if got := chunkBytesForModel("haiku"); got >= maxPromptBytesForModel("haiku") {
		t.Errorf("chunkBytesForModel(haiku)=%d should be strictly less than maxPromptBytesForModel(haiku)=%d to leave headroom for the instruction prompt", got, maxPromptBytesForModel("haiku"))
	}
	if got := chunkBytesForModel("opus"); got <= chunkBytesForModel("haiku") {
		t.Errorf("chunkBytesForModel(opus)=%d should exceed haiku's chunk size %d", got, chunkBytesForModel("haiku"))
	}
}

func TestChooseSummarizationModel(t *testing.T) {
	all := []string{"haiku", "sonnet", "opus"}

	tests := []struct {
		name        string
		promptBytes int
		allowed     []string
		wantModel   string
		wantFits    bool
	}{
		{
			name:        "tiny prompt with all allowed picks haiku",
			promptBytes: 1024,
			allowed:     all,
			wantModel:   "haiku",
			wantFits:    true,
		},
		{
			name:        "past haiku ceiling picks sonnet",
			promptBytes: haikuPromptByteLimit + 1,
			allowed:     all,
			wantModel:   "sonnet",
			wantFits:    true,
		},
		{
			name:        "past sonnet ceiling picks opus",
			promptBytes: sonnetPromptByteLimit + 1,
			allowed:     all,
			wantModel:   "opus",
			wantFits:    true,
		},
		{
			name:        "past opus ceiling falls back to opus with fits=false",
			promptBytes: opusPromptByteLimit + 1,
			allowed:     all,
			wantModel:   "opus",
			wantFits:    false,
		},
		{
			name:        "haiku disallowed skips to sonnet",
			promptBytes: 1024,
			allowed:     []string{"sonnet", "opus"},
			wantModel:   "sonnet",
			wantFits:    true,
		},
		{
			name:        "only opus allowed always picks opus",
			promptBytes: 1024,
			allowed:     []string{"opus"},
			wantModel:   "opus",
			wantFits:    true,
		},
		{
			name:        "only haiku allowed past haiku ceiling falls back to haiku with fits=false",
			promptBytes: haikuPromptByteLimit + 1,
			allowed:     []string{"haiku"},
			wantModel:   "haiku",
			wantFits:    false,
		},
		{
			name:        "empty allowed list uses default allowlist",
			promptBytes: 1024,
			allowed:     nil,
			wantModel:   "haiku",
			wantFits:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, fits := chooseSummarizationModel(tt.promptBytes, tt.allowed)
			if model != tt.wantModel || fits != tt.wantFits {
				t.Errorf("chooseSummarizationModel(%d, %v) = (%q, %v), want (%q, %v)",
					tt.promptBytes, tt.allowed, model, fits, tt.wantModel, tt.wantFits)
			}
		})
	}
}

func TestTruncateForLog(t *testing.T) {
	t.Run("short string passes through", func(t *testing.T) {
		if got := truncateForLog("hello"); got != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("trims surrounding whitespace", func(t *testing.T) {
		if got := truncateForLog("  hello\n"); got != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("long string is truncated with size suffix", func(t *testing.T) {
		input := strings.Repeat("x", 1000)
		got := truncateForLog(input)
		if !strings.HasPrefix(got, strings.Repeat("x", 500)) {
			t.Errorf("expected prefix of 500 x's")
		}
		if !strings.Contains(got, "1000 total bytes") {
			t.Errorf("expected total-bytes annotation, got %q", got)
		}
	})
}
