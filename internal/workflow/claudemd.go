package workflow

import (
	"os"
	"path/filepath"
	"strings"
)

// InjectClaudeMD writes a CLAUDE.md file in the worktree with the resolved prompt
// and structured directives to ensure Claude actually implements changes.
// imageRelPaths are worktree-relative paths to attached images (may be nil).
func InjectClaudeMD(worktreePath, resolvedPrompt string, imageRelPaths []string) error {
	var sb strings.Builder

	sb.WriteString("# CRITICAL: Autonomous Execution Mode\n\n")
	sb.WriteString("You are an autonomous coding agent. Work autonomously: **Do NOT ask for user input.**\n")
	sb.WriteString("Do NOT describe what you would do — actually do it. Do NOT ask clarifying questions.\n")
	sb.WriteString("Make decisions and implement them. If something is ambiguous, pick the best option and proceed.\n\n")

	sb.WriteString("**You MUST make actual code changes. Do NOT just describe what you would do.**\n")
	sb.WriteString("**Do NOT exit without writing code. An empty output is a failure.**\n\n")

	sb.WriteString("---\n\n")
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

	sb.WriteString("---\n\n")
	sb.WriteString("# Workflow\n\n")
	sb.WriteString("Follow these phases in order:\n\n")
	sb.WriteString("## Phase 1: Analyze\n")
	sb.WriteString("Read the codebase to understand the architecture, patterns, and relevant files.\n\n")
	sb.WriteString("## Phase 2: Plan\n")
	sb.WriteString("Decide what changes to make. Identify which files to create or modify.\n\n")
	sb.WriteString("## Phase 3: Implement\n")
	sb.WriteString("Make the code changes. Follow existing code style and patterns.\n\n")
	sb.WriteString("## Phase 4: Verify\n")
	sb.WriteString("Run the build command (e.g. `go build ./...`) and fix any errors.\n")
	sb.WriteString("Run tests if they exist and are relevant.\n\n")
	sb.WriteString("## Phase 5: Commit\n")
	sb.WriteString("Stage and commit your changes with a clear commit message.\n\n")

	claudeMDPath := filepath.Join(worktreePath, "CLAUDE.md")
	return os.WriteFile(claudeMDPath, []byte(sb.String()), 0644)
}
