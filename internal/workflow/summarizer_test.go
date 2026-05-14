package workflow

import (
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
