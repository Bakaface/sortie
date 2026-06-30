# Sortie Configuration Complete Reference

## System Prompt

```yaml
system_prompt: |
  You are an autonomous coding agent. Work autonomously without asking for user input.
  Make decisions and implement them directly. If something is ambiguous, pick the best option and proceed.
```

Controls the preamble written to each task worktree's `CLAUDE.md`. When omitted, a minimal default is used that instructs Claude to work autonomously. Override this to customize agent behavior across all tasks.

---

## Git Section

```yaml
git:
  base_branch: main                              # Base branch for worktrees (default: system default)
  branch_template: "sortie/{{task_id}}-{{task_slug}}"  # Branch naming template
```

> **Note:** `on_complete` is a **top-level** key (see "Finalization" below), not part of the `git:` section. The legacy `git.on_complete` location was removed — configs that still use it produce a migration error.

### Branch Template Variables

| Variable | Description |
|---|---|
| `{{task_id}}` | Numeric task ID |
| `{{task_slug}}` | URL-safe slug from title |
| `{{task.id}}` | Same as `{{task_id}}` |
| `{{task.title}}` | Raw task title |
| `{{task.slug}}` | Same as `{{task_slug}}` |

---

## Finalization (`on_complete`)

```yaml
on_complete: commit    # top-level: "commit", "merge", or "none" (default: "commit")
```

Controls what Sortie does after a task's workflow finishes:

- `"commit"` — Commits changes in the worktree (default)
- `"merge"` — Merges the task branch into base branch
- `"none"` — Leaves changes in the worktree branch without action

It can be overridden per-workflow via a workflow-level `on_complete:` key
(see the Workflows section). Resolution precedence: **workflow-level →
project-level → default (`commit`)**.

> Moved here from the former `git.on_complete`. The old location now errors.

---

## Verification Section

```yaml
verification:
  max_retries: 2             # int
  verify_summarizer: true    # bool
```

Both fields are accepted by the schema (and validated) but are **not currently read by any execution path** — the block is inert at runtime today. Keep it minimal; do not rely on it to change behavior.

---

## Claude Binary Override

```yaml
claude:
  command: claude            # Binary name or path (default: "claude")
  default_args:              # Extra args prepended to every invocation
    - --model
    - opus
```

`--dangerously-skip-permissions` is appended automatically when top-level `yolo: true`.

---

## Poll Interval

```yaml
poll_interval: 5s            # Daemon task-polling cadence (Go duration; default 5s)
```

Invalid duration strings are a hard load error. Rarely set per-project.

---

## Summarization Model Allowlist

```yaml
allowed_summarization_models:   # Subset of: haiku, sonnet, opus
  - haiku
  - sonnet
```

Restricts which models the summarizer auto-selects from. The summarizer picks the **cheapest** allowed model (`haiku` < `sonnet` < `opus`) whose prompt-size ceiling fits the transcript. When omitted, all three are allowed. Override per-step with `allowed_summarization_models` on a `StepConfig` (step-level takes precedence over this project-level list). Invalid entries are a hard load error.

---

## Options (TUI Display)

```yaml
options:
  number: true               # Show task numbers in the list
  branch: true               # Show branch column
  target: true               # Show target/base branch column
  branchview: false          # Group the list by branch
  animation:
    enabled: true            # Sortie (airplane) animation on task submission
    duration: 800            # Animation duration in milliseconds
```

Cosmetic-only; does not affect task execution.

---

## Notifications Section

```yaml
notifications:
  enabled: true              # Master toggle (default: true)
  on_complete: true          # Notify when task completes
  on_failed: true            # Notify when task fails
  on_waiting_input: true     # Notify when task awaits human input
```

---

## Worktree Sync Paths

```yaml
worktree-sync-paths:
  link:                      # Hard-linked into each worktree
    - .docs
    - .env.local
    - config/secrets.yml
  copy:                      # Copied into each worktree (independent files)
    - templates/starter.tpl
```

Files and directories listed here are populated into each new worktree before any setup commands run. Paths are relative to the project root.

### `link:` vs `copy:`

| Mode | Mechanism | When edits sync | Cross-filesystem | Best for |
|---|---|---|---|---|
| `link` | hard-link (`hardLinkDir`) | Yes — shared inodes | Fails — both must be on same FS | Shared docs, configs, lockfiles agents read but rarely modify |
| `copy` | file copy | No — independent | Works | Per-task `.env` overrides, scratch templates, files agents will mutate |

### Symlinks are not supported

`link:` performs **hard-links**, not symbolic links. The Sortie binary's code path is `linkPath` → `hardLinkDir`. If you need a true symlink (e.g., to a path outside the project root, or across filesystems), create it from a setup command:

```yaml
worktree-setup-commands:
  - ln -s /shared/build-cache {{worktree_path}}/.cache
```

### Entries are plain path strings

Each `copy:` / `link:` entry is a **string path relative to the project root** — the destination inside the worktree always mirrors the source path. There is no per-entry `target`/rename form (the schema is `copy: []string` and `link: []string`). To place a synced file at a different path, use a `worktree-setup-command` to move or symlink it after sync.

A missing source path is skipped silently. A `link:` failure (e.g. cross-filesystem) is collected and reported but does not abort the other entries.

### Per-workflow override

`worktree-sync-paths` (and `worktree-setup-command`, `worktree-setup-commands`, `tmux-setup-command`) may also be set on an individual workflow. A non-empty workflow-level value fully replaces the project-level one for tasks running that workflow:

```yaml
workflows:
  tasks:
    - name: heavy
      worktree-sync-paths:
        link: [.docs, vendor/cache]
      steps:
        - name: implementing
          prompt: "..."
```

---

## Worktree Setup Commands

Run shell commands inside each worktree after creation (and after `worktree-sync-paths` is applied). Use for: installing dependencies, generating files, creating real symlinks, copying secrets from a vault, etc.

Two forms:

```yaml
# Single command
worktree-setup-command: |
  pnpm install --frozen-lockfile

# Multiple commands (run in order; preferred when more than one step is needed)
worktree-setup-commands:
  - pnpm install --frozen-lockfile
  - cp ~/.config/myproject/.env.local {{worktree_path}}/.env.local
  - mkdir -p {{worktree_path}}/.cache
```

If both are set, `worktree-setup-commands` takes precedence. Each command runs with the worktree as `cwd`.

Available variables: `{{worktree_path}}`, `{{session_name}}` (tmux only), `{{run_agent}}` (tmux only).

A non-zero exit from any command logs a warning but does not abort the task.

---

## Tmux Setup Command

Run when a step uses tmux mode. Customizes the tmux session layout (windows, panes, initial commands) before the agent starts.

```yaml
tmux-setup-command: |-
  tmux rename-window -t {{session_name}}:0 vim
  tmux new-window -t {{session_name}}:1 -n agent -c {{worktree_path}}
  tmux send-keys -t {{session_name}}:1 '{{run_agent}}' C-m
  tmux new-window -t {{session_name}}:9 -n bash -c {{worktree_path}}
  tmux select-window -t {{session_name}}:1
```

Variables:

| Variable | Description |
|---|---|
| `{{session_name}}` | Tmux session created for the task |
| `{{worktree_path}}` | Absolute path to the task's worktree |
| `{{run_agent}}` | Pre-built command string that launches the Claude agent |

If you do not set `tmux-setup-command`, Sortie uses a minimal default that just starts the agent.

---

## Step Configuration Details

### Timeout Format

Go duration strings: `"30m"`, `"1h"`, `"1h30m"`, `"45m"`, `"2h"`. Default: `"30m"`.

### Step Context Flow

After each step completes, the result from Claude's output is captured as step context and stored in the `task_steps` database table. This context is available to subsequent steps via `{{steps.<step_name>.context}}` (or the backward-compat alias `{{artifacts.<step_name>}}`).

### Step Summarization

By default (`summarization_strategy` unset), the step's context is produced by **`summarize_chat`** — a second Claude call summarizes the full transcript. `last_message` and `none` are the alternatives:

```yaml
- name: grilling
  # print omitted → tmux (the default)
  summarization_strategy: summarize_chat
  summarization_prompt: |
    Extract the durable design decisions reached in this Q&A.

    Format:
    - Numbered list, each item: question + paraphrased user answer.
    - Skip small-talk and detours.

    <chat>
    {{chat}}
    </chat>
  prompt: |
    Interview the user until shared understanding is reached...
```

| Strategy | What gets captured |
|---|---|
| (unset) | **Defaults to `summarize_chat`** |
| `summarize_chat` | A second Claude call summarizes the full transcript using `summarization_prompt` (the default) |
| `last_message` | The agent's final output message only (no extra Claude call; unusable for tmux steps) |
| `none` | Nothing — no step context is captured; later `{{steps.<name>.context}}` references resolve to empty |

`summarize_chat` is essential for tmux/grilling steps where the meaningful output is the dialogue, not a final message. The summarizer step also unlocks the `step_context_empty` loop exit pattern: instruct the summarizer to emit empty output when "no issues found", and the loop will terminate.

Inside `summarization_prompt`, the variable `{{chat}}` expands to the full transcript. All standard task variables (`{{task.id}}`, `{{steps.<name>.context}}`, etc.) are also available.

Example multi-step with step context:

```yaml
steps:
  - name: analyzing
    prompt: |
      Analyze the requirements:
      <task-description>
      {{task.description}}
      </task-description>
  - name: implementing
    prompt: |
      Implement based on the analysis:
      <step-context name="analyzing">
      {{steps.analyzing.context}}
      </step-context>
  - name: reviewing
    prompt: |
      Review the implementation:
      <step-context name="implementing">
      {{steps.implementing.context}}
      </step-context>
    human: true
```

### Human Approval Steps

When `human: true`, the task pauses at `awaiting-approval` status. The user reviews in the TUI and approves to continue. Use for review gates.

### Tmux Steps (the default) vs. headless `print`

Tmux is the **default** execution mode — a step runs in tmux unless `print: true` is set. When a step runs in tmux:
- Claude runs inside an interactive tmux session
- User can attach to watch/interact
- Task shows `tmux` status in TUI
- The daemon auto-advances on turn-end (or press `c` to finalize manually)

Execution-mode resolution order:
1. Step-level `print` field (if set: `true` = headless, `false` = tmux)
2. Workflow-level `print` field (default for all steps)
3. Falls back to `false` (tmux)

> The removed `tmux:` field is a **hard load error** — use `print:` (inverted). See the [`print` section in SKILL.md](../SKILL.md) for the full `print` × `human` behavior table.

### Loop Configuration

Loops allow iterative refinement (e.g., implement → review → fix → review again).

```yaml
steps:
  - name: implementing
    prompt: |
      Implement the following:
      <task-description>
      {{task.description}}
      </task-description>
  - name: reviewing
    prompt: |
      Review the implementation:
      <step-context name="implementing">
      {{steps.implementing.context}}
      </step-context>
    human: true
  - name: fixing
    print: true            # required: loop steps cannot run in tmux
    prompt: |
      Fix the issues found during review:
      <step-context name="reviewing">
      {{steps.reviewing.context}}
      </step-context>
    loop:
      goto: reviewing
      max_iterations: 3
      exit_condition:
        step_context_empty: reviewing
```

**Validation rules:**
- `goto` must reference a step that appears BEFORE the loop step
- No self-reference
- `max_iterations` must be >= 1
- Loop steps cannot have `human: true`
- Loop steps cannot run in tmux — set `print: true` on the loop step (or its workflow)
- Loop ranges cannot overlap with other loops

---

## Task States

| Status | Description |
|---|---|
| `pending` | Waiting to be picked up by a worker |
| `init` | Initializing worktree and environment |
| `running` | Claude agent is executing |
| `awaiting-approval` | Paused at a `human: true` step |
| `tmux` | Running in interactive tmux session |
| `finalizing` | Running post-completion steps |
| `summarizing` | Generating task context summary |
| `merge-blocked` | Merge conflicts or merge failure |
| `completed` | Successfully finished |
| `failed` | Execution failed |

---

## Task Priorities

| Priority | Sort Value |
|---|---|
| `low` | 1 |
| `medium` | 2 (default) |
| `high` | 3 |
| `urgent` | 4 |

---

## Continue Workflow

Pressing `c` on a completed/failed task triggers continuation:
1. TUI shows workflow selection (task workflows)
2. User enters a prompt (enhanced with continuation context)
3. Task resets to `pending` with the selected workflow
4. Daemon picks up and re-executes

For tmux workflows, continuation creates/reuses a worktree with a `CLAUDE.md` containing previous context.

### Fast-Track Finalization

When finalizing a tmux task with no meaningful changes (ignoring `.claude-output.log` and `CLAUDE.md`), the task skips summarizer/on_complete and goes directly to `completed`.

---

## Legacy Config Formats (Backward Compatible)

### Legacy List Format

```yaml
workflows:
  - name: default
    steps:
      - name: implementing
        prompt: "Implement the task"
```

All workflows treated as task workflows.

### Ancient Singular Format (Deprecated)

```yaml
workflow:
  steps:
    - name: implementing
      prompt: "Implement the task"
```

**Always use the current structured format with `workflows.tasks`, `workflows.one-off`, and `workflows.init`.**

---

## Complete Example

```yaml
max_workers: 3
default_priority: medium
yolo: true
tmux_nested_attach_behavior: switch
system_prompt: |
  You are an autonomous coding agent. Work autonomously without asking for user input.
  Make decisions and implement them directly. If something is ambiguous, pick the best option and proceed.

verification:
  verify_summarizer: true

git:
  base_branch: main
  branch_template: "sortie/{{task_id}}-{{task_slug}}"

on_complete: merge

notifications:
  enabled: true
  on_complete: true
  on_failed: true
  on_waiting_input: true

workflows:
  tasks:
    - name: sensible
      summarizer_prompt: "Summarize what was implemented and any decisions made"
      on_complete: commit    # optional per-workflow override of the top-level on_complete
      steps:
        - name: implementing
          prompt: |
            Implement task #{{task.id}}: {{task.title}}

            <task-description>
            {{task.description}}
            </task-description>
          timeout: 45m
        - name: reviewing
          prompt: |
            Review the implementation for task #{{task.id}}.

            Implementation summary:
            <step-context name="implementing">
            {{steps.implementing.context}}
            </step-context>
          human: true
          timeout: 20m
        - name: fixing
          print: true        # required: loop steps cannot run in tmux
          prompt: |
            Fix the issues found during review:
            <step-context name="reviewing">
            {{steps.reviewing.context}}
            </step-context>
          timeout: 30m
          loop:
            goto: reviewing
            max_iterations: 3
            exit_condition:
              step_context_empty: reviewing

    - name: quick
      # tmux is the default — no `print` needed for an interactive session
      steps:
        - name: implementing
          prompt: |
            Implement task #{{task.id}}: {{task.title}}

            <task-description>
            {{task.description}}
            </task-description>

  one-off:
    - name: housekeeping
      description: "Run standard codebase maintenance: linting, dead code removal, dependency updates"
      steps:
        - name: auditing
          prompt: "Audit the codebase for code smells, unused dependencies, and dead code"
          timeout: 20m
        - name: cleaning
          prompt: |
            Apply the following cleanups:
            <step-context name="auditing">
            {{steps.auditing.context}}
            </step-context>
          timeout: 30m

  init:
    - name: from-prd
      description: "Analyze a PRD and create implementation tasks"
      steps:
        - name: analyzing
          prompt: |
            Analyze the PRD and break it into implementable tasks.
            Create sortie tasks for each piece of work.
          timeout: 30m
```
