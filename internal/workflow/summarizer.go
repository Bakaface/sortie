package workflow

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	gitpkg "github.com/aface/sortie/internal/git"
	"github.com/aface/sortie/internal/config"
	"github.com/aface/sortie/internal/task"
)

// FinalizeTask runs the on_complete action, then the summarizer, then worktree cleanup.
// Used when finalizing a tmux-continued task.
func (e *Engine) FinalizeTask(ctx context.Context, t *task.Task) error {
	// Open finalize log file so TUI can show progress
	logDir := ProjectLogsDir(e.dataDir, t.ID)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Warning: failed to create log dir for task #%d: %v", t.ID, err)
	}
	logPath := ProjectLogPath(e.dataDir, t.ID, "finalize")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Warning: failed to open finalize log for task #%d: %v", t.ID, err)
	}
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	logFn := func(format string, args ...any) {
		msg := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
		log.Printf("Task #%d finalize: %s", t.ID, fmt.Sprintf(format, args...))
		if logFile != nil {
			logFile.WriteString(msg + "\n")
		}
	}

	logFn("=== Finalization started for task #%d: %s ===", t.ID, t.Title)

	// Resolve branch name if not set (may not have been persisted to DB)
	if t.Worktree && t.Branch == "" {
		t.Branch = e.cfg.ResolveBranchForTask(t.ID, t.Title, t.Slug, t.BranchName)
	}

	// Run on_complete action first (merge to unblock user)
	action := e.cfg.Git.OnComplete
	if action == "" {
		action = "none"
	}
	logFn("Running on_complete action: %s", action)
	if err := e.executeOnComplete(ctx, t, nil, logFn); err != nil {
		logFn("Error: on_complete failed: %v", err)
		return err
	}

	// Run summarizer after merge
	wf := e.cfg.GetWorkflow(t.Workflow)
	if wf != nil {
		if err := e.database.UpdateTaskStatus(t.ID, task.StatusSummarizing); err != nil {
			logFn("Warning: failed to set summarizing status: %v", err)
		}
		logFn("Running summarizer...")
		if err := e.runSummarizer(ctx, t, wf, logFn); err != nil {
			logFn("Warning: summarizer failed: %v", err)
		} else {
			logFn("Summarizer completed")
		}
	}

	// Clean up worktree after summarizer (if merge was performed)
	if e.cfg.Git.OnComplete == "merge" && t.Worktree {
		e.cleanupMergedWorktree(t, logFn)
	}

	logFn("=== Finalization completed ===")
	return nil
}

// runSummarizer generates a summary of all artifacts and stores it as the task's context.
// logFn is optional; when provided, progress messages are written to it (e.g. finalize log).
func (e *Engine) runSummarizer(ctx context.Context, t *task.Task, wf *config.WorkflowConfig, logFn func(string, ...any)) error {
	logMsg := func(format string, args ...any) {
		log.Printf(format, args...)
		if logFn != nil {
			logFn(format, args...)
		}
	}
	// Collect step names
	var stepNames []string
	for _, s := range wf.Steps {
		stepNames = append(stepNames, s.Name)
	}

	// Collect step contexts from DB
	stepContexts, err := e.database.GetTaskStepContexts(t.ID, stepNames)
	if err != nil {
		logMsg("Warning: failed to get step contexts for task #%d: %v", t.ID, err)
		stepContexts = make(map[string]string)
	}

	// Get git diff stat as fallback context when no step contexts are available
	var diffStat string
	if len(stepContexts) == 0 {
		baseBranch := e.cfg.Git.BaseBranch
		if baseBranch == "" {
			baseBranch = "main"
		}
		var diffErr error
		diffStat, diffErr = gitpkg.DiffStat(t.WorktreePath, baseBranch)
		if diffErr != nil {
			logMsg("Warning: failed to get diff stat for task #%d: %v", t.ID, diffErr)
		}
		if diffStat == "" {
			logMsg("No step contexts or changes found for task #%d, skipping summarizer", t.ID)
			return nil
		}
	}

	// Build the prompt and log the summarization approach
	var prompt string
	if wf.SummarizerPrompt != "" {
		// Use the configured summarizer prompt with template resolution
		tmplCtx := &TemplateContext{
			Task: TaskVars{
				ID:          t.ID,
				Title:       t.Title,
				Description: t.Description,
				Slug:        t.Slug,
				Branch:      t.Branch,
			},
			Steps: stepContexts,
			Git: GitVars{
				BaseBranch: e.cfg.Git.BaseBranch,
				RepoRoot:   e.repoRoot,
			},
		}
		prompt = ResolveTemplate(wf.SummarizerPrompt, tmplCtx)
		var names []string
		for name := range stepContexts {
			names = append(names, name)
		}
		logMsg("%s", summarizationDescription(t.ID, true, names, false))
	} else if len(stepContexts) > 0 {
		// Build default prompt with all step contexts
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Summarize the progress made on task #%d: %s\n\n", t.ID, t.Title))
		sb.WriteString("Use the context from the following task step contexts:\n\n")
		var contextNames []string
		for _, name := range stepNames {
			if content, ok := stepContexts[name]; ok {
				sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", name, content))
				contextNames = append(contextNames, name)
			}
		}
		sb.WriteString("Provide a concise but comprehensive summary of what was accomplished, ")
		sb.WriteString("any decisions made, and the current state of the implementation. ")
		sb.WriteString("This summary will be used as context for future work on this task.")
		prompt = sb.String()
		logMsg("%s", summarizationDescription(t.ID, false, contextNames, false))
	} else {
		// No artifacts — use git diff stat and instruct Claude to read the actual changes
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Summarize the progress made on task #%d: %s\n\n", t.ID, t.Title))
		sb.WriteString("The task description was:\n")
		sb.WriteString(t.Description)
		sb.WriteString("\n\nThe following files were changed:\n\n```\n")
		sb.WriteString(diffStat)
		sb.WriteString("\n```\n\n")
		sb.WriteString("Read the changed files listed above and review the actual code to understand what was implemented. ")
		sb.WriteString("Do NOT guess or assume — base your summary on the actual file contents and git changes in this repository. ")
		sb.WriteString("Provide a concise summary of what was accomplished. ")
		sb.WriteString("This summary will be used as context for future work on this task.")
		prompt = sb.String()
		logMsg("%s", summarizationDescription(t.ID, false, nil, true))
	}

	logMsg("Running summarizer for task #%d", t.ID)

	// Run Claude synchronously to capture the summary text. The final task
	// summarizer uses the project-level summarization_model (per-step overrides
	// only apply to step-level summarize_chat passes).
	summary, err := e.runClaudeSync(ctx, prompt, t.WorktreePath, "summarize", e.cfg.SummarizationModel)
	if err != nil {
		return fmt.Errorf("summarizer claude invocation failed: %w", err)
	}

	summary = strings.TrimSpace(summary)
	if summary == "" {
		logMsg("Summarizer produced empty output for task #%d", t.ID)
		return nil
	}

	// Store the context in the database
	if err := e.database.UpdateTaskContext(t.ID, summary); err != nil {
		return fmt.Errorf("failed to store task context: %w", err)
	}

	t.Context = summary
	logMsg("Summarizer completed for task #%d (%d chars)", t.ID, len(summary))
	return nil
}

// encodeClaudeProjectDir encodes a workdir path to the directory name format used by
// Claude Code under ~/.claude/projects/. The encoding replaces both '/' and '.' with '-'.
func encodeClaudeProjectDir(workdir string) string {
	r := strings.NewReplacer("/", "-", ".", "-")
	return r.Replace(workdir)
}

// loadStepChatContent returns the raw chat content for a step.
// For tmux steps, reads the Claude session JSONL via the session id recorded by UpsertChat.
// For headless steps, reads the per-step log file.
// Returns empty string (no error) if no content is available yet.
func (e *Engine) loadStepChatContent(t *task.Task, stepName string, useTmux bool) (string, error) {
	if useTmux {
		// Look up the session id recorded when the tmux step started
		chat, err := e.database.GetChatByStep(t.ID, stepName)
		if err != nil {
			return "", fmt.Errorf("failed to look up chat session for tmux step %q: %w", stepName, err)
		}
		if chat == nil || chat.SessionID == "" {
			// No session recorded yet — treat as no content available
			return "", nil
		}

		// Construct the JSONL path: ~/.claude/projects/<encoded-workdir>/<sessionid>.jsonl
		encoded := encodeClaudeProjectDir(t.WorktreePath)
		jsonlPath := filepath.Join(os.Getenv("HOME"), ".claude", "projects", encoded, chat.SessionID+".jsonl")
		data, err := os.ReadFile(jsonlPath)
		if err != nil {
			if os.IsNotExist(err) {
				return "", nil
			}
			return "", fmt.Errorf("failed to read claude session JSONL for step %q: %w", stepName, err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	// Headless step: read the per-step log file
	logPath := ProjectLogPath(e.dataDir, t.ID, stepName)
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read step log for step %q: %w", stepName, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// smallChatBytes is the threshold below which a non-tmux step skips the haiku
// summarization pass and keeps the Claude result-event text as step context.
// Roughly equates to "a handful of NDJSON events" — too short to be worth
// paying a haiku round trip for.
const smallChatBytes = 4096

// shouldSummarizeChat returns true when the chat log is worth running a haiku
// summarization pass over. For non-tmux steps with a non-empty result text and
// a tiny chat log, the result text is kept and haiku is skipped. Tmux steps
// always summarize because they have no result-event fallback.
func shouldSummarizeChat(chat, resultText string, useTmux bool) bool {
	if useTmux {
		return true
	}
	if strings.TrimSpace(resultText) == "" {
		return true
	}
	return len(chat) >= smallChatBytes
}

// haikuPromptByteLimit / largeModelPromptByteLimit are the safe per-invocation
// prompt-size ceilings for `claude -p`. Empirically haiku rejects prompts
// around ~200 KB with "Prompt is too long". Larger-context models (sonnet,
// opus) tolerate much more; we cap at 800 KB to stay clear of the macOS
// ARG_MAX (1 MB) for the entire argv vector.
const (
	haikuPromptByteLimit      = 180 * 1024
	largeModelPromptByteLimit = 800 * 1024
)

// maxPromptBytesForModel returns the safe upper bound for a single `claude -p`
// invocation when using the given model.
func maxPromptBytesForModel(model string) int {
	switch model {
	case "sonnet", "opus":
		return largeModelPromptByteLimit
	default:
		return haikuPromptByteLimit
	}
}

// chunkBytesForModel returns the target chunk size for map-reduce summarization
// when using the given model, leaving headroom for the surrounding instruction
// prompt.
func chunkBytesForModel(model string) int {
	const headroom = 30 * 1024
	return maxPromptBytesForModel(model) - headroom
}

// summarizeChatLog runs Claude haiku to summarise the given chat content.
// customPrompt is a template that may reference the chat via a {{chat}} placeholder;
// task template variables ({{task.id}}, {{task.title}}, etc.) are also resolved.
// If customPrompt is empty, the default summarization prompt is used.
//
// When the resolved prompt exceeds the model's prompt byte limit the chat is
// summarised via map-reduce: it is split on line boundaries into chunks sized
// to fit the model, each chunk is summarised with a generic extraction prompt,
// and the chunk summaries are then fed back through the original (customPrompt
// or default) final-summary prompt.
//
// model is the Claude model alias (e.g. "haiku", "sonnet", "opus") or full
// model id to use for the haiku-style summarization passes. Empty falls back
// to DefaultSummarizationModel.
func (e *Engine) summarizeChatLog(ctx context.Context, t *task.Task, stepName, customPrompt, chatContent, model string) (string, error) {
	if strings.TrimSpace(chatContent) == "" {
		return "", nil
	}
	if model == "" {
		model = config.DefaultSummarizationModel
	}

	prompt := e.buildSummarizePrompt(t, stepName, customPrompt, chatContent)
	maxBytes := maxPromptBytesForModel(model)

	if len(prompt) > maxBytes {
		log.Printf("summarize_chat: prompt %d bytes exceeds %s limit %d for step %q of task #%d; running map-reduce", len(prompt), model, maxBytes, stepName, t.ID)
		chunkSummaries, err := e.summarizeChatChunks(ctx, t, stepName, chatContent, model)
		if err != nil {
			return "", err
		}
		reduced := strings.Join(chunkSummaries, "\n\n--- CHUNK BOUNDARY ---\n\n")
		prompt = e.buildSummarizePrompt(t, stepName, customPrompt, reduced)
		log.Printf("summarize_chat: map-reduce reduce step for step %q of task #%d (%d chunk summaries, %d chars)", stepName, t.ID, len(chunkSummaries), len(reduced))
	}

	log.Printf("Running summarize_chat for step %q of task #%d (model=%s, prompt %d bytes)", stepName, t.ID, model, len(prompt))
	summary, err := e.runClaudeSync(ctx, prompt, t.WorktreePath, "summarize_chat", model)
	if err != nil {
		return "", fmt.Errorf("summarize_chat claude invocation failed: %w", err)
	}
	summary = strings.TrimSpace(summary)
	log.Printf("summarize_chat completed for step %q of task #%d (%d chars)", stepName, t.ID, len(summary))
	return summary, nil
}

// buildSummarizePrompt resolves the summarization prompt for the given chat content,
// using customPrompt (template, with optional {{chat}} placeholder) if non-empty,
// or a sensible default otherwise.
func (e *Engine) buildSummarizePrompt(t *task.Task, stepName, customPrompt, chatContent string) string {
	if customPrompt != "" {
		tmplCtx := &TemplateContext{
			Task: TaskVars{
				ID:          t.ID,
				Title:       t.Title,
				Description: t.Description,
				Slug:        t.Slug,
				Branch:      t.Branch,
			},
			Git: GitVars{
				BaseBranch: e.cfg.Git.BaseBranch,
				RepoRoot:   e.repoRoot,
			},
		}
		resolved := ResolveTemplate(customPrompt, tmplCtx)

		if strings.Contains(resolved, "{{chat}}") {
			return strings.ReplaceAll(resolved, "{{chat}}", chatContent)
		}
		return resolved + "\n\n--- CONVERSATION LOG ---\n" + chatContent
	}

	return fmt.Sprintf(
		"Summarize the following Claude Code conversation log from step %q of task #%d: %s\n\n"+
			"Output requirements:\n"+
			"- Under 200 words.\n"+
			"- Preserve file paths, function/symbol names, command lines, and error strings VERBATIM — do not paraphrase identifiers.\n"+
			"- Cover what was accomplished, key decisions, files changed, and any blockers or unresolved issues.\n"+
			"- Prioritise actionable detail over narrative; this summary becomes context for later workflow steps.\n\n"+
			"--- CONVERSATION LOG ---\n%s",
		stepName, t.ID, t.Title, chatContent,
	)
}

// summarizeChatChunks splits chatContent on line boundaries and runs an
// extraction pass over each chunk with the given model, returning the per-chunk
// summaries.
func (e *Engine) summarizeChatChunks(ctx context.Context, t *task.Task, stepName, chatContent, model string) ([]string, error) {
	chunks := splitOnLineBoundary(chatContent, chunkBytesForModel(model))
	summaries := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		log.Printf("summarize_chat: map step %d/%d for step %q of task #%d (model=%s, %d chars)", i+1, len(chunks), stepName, t.ID, model, len(chunk))
		mapPrompt := fmt.Sprintf(
			"This is chunk %d of %d from a Claude Code conversation log (step %q of task #%d: %s).\n"+
				"Extract the key information from this chunk: decisions made, file paths, function/symbol names, "+
				"commands run, errors hit, blockers, and unresolved questions. Preserve identifiers VERBATIM. "+
				"Under 300 words. This is a partial slice — a later pass will combine all chunk extracts into a final summary.\n\n"+
				"--- CHUNK ---\n%s",
			i+1, len(chunks), stepName, t.ID, t.Title, chunk,
		)
		s, err := e.runClaudeSync(ctx, mapPrompt, t.WorktreePath, "summarize_chat_chunk", model)
		if err != nil {
			return nil, fmt.Errorf("summarize_chat map step %d/%d failed: %w", i+1, len(chunks), err)
		}
		summaries = append(summaries, strings.TrimSpace(s))
	}
	return summaries, nil
}

// splitOnLineBoundary splits content into chunks no larger than maxBytes, breaking
// only on newline boundaries so that line-delimited formats (e.g. JSONL) stay intact.
// A single line longer than maxBytes becomes its own (oversized) chunk.
func splitOnLineBoundary(content string, maxBytes int) []string {
	if len(content) <= maxBytes {
		return []string{content}
	}
	var chunks []string
	var cur strings.Builder
	for _, line := range strings.Split(content, "\n") {
		// +1 accounts for the newline that will be re-added before this line.
		if cur.Len() > 0 && cur.Len()+1+len(line) > maxBytes {
			chunks = append(chunks, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte('\n')
		}
		cur.WriteString(line)
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	return chunks
}

// summarizationDescription returns a human-readable description of the summarization
// approach being used for a task, suitable for logging.
func summarizationDescription(taskID int64, hasCustomPrompt bool, artifactNames []string, useDiffStat bool) string {
	if hasCustomPrompt {
		if len(artifactNames) > 0 {
			return fmt.Sprintf("Summarizing task #%d with custom prompt and artifacts: %s", taskID, strings.Join(artifactNames, ", "))
		}
		return fmt.Sprintf("Summarizing task #%d with custom prompt", taskID)
	}
	if len(artifactNames) > 0 {
		return fmt.Sprintf("Summarizing task #%d with artifacts: %s", taskID, strings.Join(artifactNames, ", "))
	}
	if useDiffStat {
		return fmt.Sprintf("Summarizing task #%d via git diff", taskID)
	}
	return fmt.Sprintf("Summarizing task #%d", taskID)
}

// RunWorktreeSetupCommand runs the configured worktree setup command, if any.
// The command is executed with the project root as the working directory.
// {{worktree_path}} in the command string is replaced with the actual worktree path.
func RunWorktreeSetupCommand(ctx context.Context, projectRoot, worktreePath, command string) error {
	if command == "" {
		return nil
	}

	// Replace template variable
	resolved := strings.ReplaceAll(command, "{{worktree_path}}", worktreePath)

	log.Printf("Running worktree setup command: %s", resolved)

	cmd := exec.CommandContext(ctx, "sh", "-c", resolved)
	cmd.Dir = projectRoot

	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		log.Printf("Worktree setup output:\n%s", string(output))
	}
	if err != nil {
		return fmt.Errorf("worktree setup command failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// RunWorktreeSetupCommands runs multiple worktree setup commands sequentially.
// Each command is executed with the project root as the working directory.
// {{worktree_path}} in command strings is replaced with the actual worktree path.
// Execution stops at the first failure.
func RunWorktreeSetupCommands(ctx context.Context, projectRoot, worktreePath string, commands []string) error {
	for i, command := range commands {
		if command == "" {
			continue
		}
		log.Printf("Running worktree setup command [%d/%d]: %s", i+1, len(commands), command)
		if err := RunWorktreeSetupCommand(ctx, projectRoot, worktreePath, command); err != nil {
			return fmt.Errorf("worktree setup command [%d/%d] failed: %w", i+1, len(commands), err)
		}
	}
	return nil
}

// runClaudeSync runs Claude Code synchronously and captures its stdout output.
// workDir sets the working directory for the Claude process so it can access
// the task's worktree files. purpose tags the invocation via SORTIE_PURPOSE so
// stub claude binaries can route the response without parsing prompt text.
// model is the Claude model alias (e.g. "haiku", "sonnet", "opus") or full
// model id; an empty string falls back to DefaultSummarizationModel.
func (e *Engine) runClaudeSync(ctx context.Context, prompt string, workDir string, purpose string, model string) (string, error) {
	if model == "" {
		model = config.DefaultSummarizationModel
	}
	args := []string{"-p", prompt, "--output-format", "text", "--model", model}
	args = append(args, e.cfg.Claude.Args()...)

	cmd := exec.CommandContext(ctx, e.cfg.Claude.Command, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	if purpose != "" {
		cmd.Env = append(os.Environ(), "SORTIE_PURPOSE="+purpose)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// claude prints user-facing errors (e.g. "Prompt is too long") to stdout
		// rather than stderr, so surface both streams for diagnosis.
		return "", fmt.Errorf("claude command failed: %w (stdout: %s, stderr: %s)", err, truncateForLog(stdout.String()), truncateForLog(stderr.String()))
	}

	return stdout.String(), nil
}

// truncateForLog clips a string to a sensible length for inclusion in error
// messages so a multi-megabyte stdout dump cannot drown a log line.
func truncateForLog(s string) string {
	const maxLen = 500
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + fmt.Sprintf("... (truncated, %d total bytes)", len(s))
}
