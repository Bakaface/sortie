package workflow

import (
	"strings"
)

// DefaultSystemPrompt is used when no system_prompt is configured in .sortie.yml.
const DefaultSystemPrompt = `You are an autonomous coding agent. Work autonomously without asking for user input.
Make decisions and implement them directly. If something is ambiguous, pick the best option and proceed.`

// BuildSystemPrompt constructs the system prompt string from the configured preamble,
// resolved task prompt, and optional image paths.
// systemPrompt controls the preamble; if empty, DefaultSystemPrompt is used.
// imageRelPaths are worktree-relative paths to attached images (may be nil).
func BuildSystemPrompt(resolvedPrompt, systemPrompt string, imageRelPaths []string) string {
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}

	var sb strings.Builder

	sb.WriteString(systemPrompt)
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("# Task\n\n")
	sb.WriteString(resolvedPrompt)
	sb.WriteString("\n\n")

	// Include attached images section if present
	if len(imageRelPaths) > 0 {
		sb.WriteString("## Attached Images\n\n")
		sb.WriteString("The following images were attached to this task. Use your file reading tool to view them:\n\n")
		for _, imgPath := range imageRelPaths {
			sb.WriteString("- `" + imgPath + "`\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
