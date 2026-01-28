package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InjectClaudeMD writes a CLAUDE.md file in the worktree with the resolved prompt
// and instructions for writing artifacts.
func InjectClaudeMD(worktreePath, resolvedPrompt, stepName, artifactsDir string) error {
	var sb strings.Builder

	sb.WriteString("# Task Instructions\n\n")
	sb.WriteString(resolvedPrompt)
	sb.WriteString("\n\n")

	sb.WriteString("## Artifact Output\n\n")
	sb.WriteString("When you complete this step, write a summary of what you did to:\n")
	fmt.Fprintf(&sb, "`%s/%s.md`\n\n", artifactsDir, stepName)
	sb.WriteString("This file will be available to subsequent workflow steps.\n\n")

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("1. Implement the task described above\n")
	sb.WriteString("2. Write tests if appropriate\n")
	sb.WriteString("3. Follow existing code style and patterns\n")
	sb.WriteString("4. Commit your changes when done\n")

	claudeMDPath := filepath.Join(worktreePath, "CLAUDE.md")
	return os.WriteFile(claudeMDPath, []byte(sb.String()), 0644)
}
