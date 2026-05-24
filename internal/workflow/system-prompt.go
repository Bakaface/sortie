package workflow

import (
	"strings"
)

// DefaultSystemPrompt is used when no system_prompt is configured in .sortie.yml.
const DefaultSystemPrompt = `You are an autonomous coding agent. Work autonomously without asking for user input.
Make decisions and implement them directly. If something is ambiguous, pick the best option and proceed.`

// verificationFooter is appended to every system prompt (regardless of whether
// the project supplies a custom SystemPrompt) to nudge agents toward discovering
// and running the project's own test/lint/build commands instead of inventing
// them. It is intentionally project-agnostic — it does not name go/npm/cargo/etc.
const verificationFooter = `

---

## Verification before declaring done

Before reporting a step complete:

1. Read this project's CLAUDE.md (root + any in subdirs you touched). It defines the canonical test/lint/build commands. If a "what to run after a change" rule is documented, follow it exactly.
2. Run the project's tests. Do not invent test commands — use what CLAUDE.md, README, or the project's task runner (Makefile / mise.toml / justfile / package.json scripts / etc.) defines. If none exist, say so explicitly rather than skipping verification.
3. Run the project's linter / formatter if one is configured. Same rule: use the defined command, do not invent one.
4. If a verification step fails, surface the failure plainly — do not silently skip layers or claim partial success.

If you cannot locate the test/lint commands, stop and ask rather than guessing.`

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
	sb.WriteString(verificationFooter)
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
