---
name: sortie-configurer
description: >
  Generate and edit .sortie.yml project configuration files for the Sortie daemon.
  Sortie orchestrates Claude Code agents working on tasks in parallel using isolated
  git worktrees. Use when (1) creating a new .sortie.yml config, (2) adding or modifying
  workflows, tasks, one-off jobs, or init pipelines, (3) configuring git, tmux, notifications,
  or verification settings, (4) user mentions "sortie config", ".sortie.yml", or asks about
  sortie workflow/task configuration, (5) troubleshooting sortie config issues.
user_invocable: true
---

# Sortie Configuration Skill

Generate correct `.sortie.yml` project configuration files for the Sortie daemon.

## What is Sortie?

Sortie is a daemon that orchestrates multiple Claude Code agents working on tasks in parallel. Each task runs in an isolated git worktree. Configuration lives in `.sortie.yml` at the project root.

## Config Loading Order (later overrides earlier)

1. Built-in defaults (hardcoded)
2. Global daemon config: `~/.config/sortie/config.yaml` (subset of fields, no workflows)
3. Global sortie config: `~/.sortie.yml`
4. **Project config: `.sortie.yml`** (this is what you generate)

## Quick Start

Minimal working config:

```yaml
workflows:
  tasks:
    - name: default
      steps:
        - name: implementing
          prompt: |
            Implement task #{{task.id}}: {{task.title}}

            <task-description>
            {{task.description}}
            </task-description>
```

## Top-Level Fields

| Field | Type | Default | Description |
|---|---|---|---|
| `max_workers` | int | `3` | Max concurrent Claude agents |
| `default_priority` | string | `"medium"` | `low`, `medium`, `high`, `urgent` |
| `yolo` | bool | `false` | Pass `--dangerously-skip-permissions` to Claude |
| `system_prompt` | string | minimal default | Preamble written to each worktree's `CLAUDE.md` |
| `verification` | object | — | Summarizer verification settings |
| `git` | object | — | Branch naming, base branch, completion action |
| `workflows` | object | — | **Primary config block** — defines all workflow pipelines |
| `notifications` | object | — | Desktop notification toggles |
| `tmux_nested_attach_behavior` | string | `"switch"` | `"switch"` or `"nest"` for tmux-in-tmux |
| `worktree-sync-paths` | object | — | Hard-link or copy paths from main checkout into each worktree (e.g., `.docs`, `.env`). See [Sharing files into worktrees](#sharing-files-into-worktrees). |
| `worktree-setup-command` | string | — | Single shell command run after worktree creation (`{{worktree_path}}` available) |
| `worktree-setup-commands` | list[string] | — | Multiple setup commands run in order; preferred over the singular form when more than one step is needed |
| `tmux-setup-command` | string | — | Shell command run when launching a tmux step. Variables: `{{session_name}}`, `{{worktree_path}}`, `{{run_agent}}` |

### Field-name convention

Top-level field names mix two casing styles — **don't guess, copy exactly**:

- **kebab-case:** `worktree-sync-paths`, `worktree-setup-command`, `worktree-setup-commands`, `tmux-setup-command`
- **snake_case:** `max_workers`, `default_priority`, `system_prompt`, `tmux_nested_attach_behavior`, `base_branch`, `branch_template`, `on_complete`

If you author an unrecognized variant (`worktree_sync_paths`, `tmux_setup_command`, etc.), Sortie will silently ignore it.

### Sharing files into worktrees

`worktree-sync-paths` shape:

```yaml
worktree-sync-paths:
  link:                 # hard-linked (NOT symlinked — see caveat below)
    - .docs
    - .env.local
  copy:                 # copied (independent per worktree)
    - some/template.tpl
```

**Important:** `link:` performs **hard-links**, not symbolic links. Sortie's binary calls `hardLinkDir` under the hood. Implications:

- For source/text trees (markdown, configs), hard-links behave like the symlinks users typically expect — files appear in the worktree, edits sync via shared inodes.
- Hard-links cannot cross filesystems. If `.sortie/worktrees/` lives on a different filesystem from the main checkout, `link:` will fail; use `copy:` instead.
- For files you want **isolated per worktree** (build output, generated code, per-task `.env` overrides), use `copy:` not `link:`.
- Symbolic links are **not supported** as a `worktree-sync-paths` mode. If you genuinely need symlinks, create them in `worktree-setup-command` (e.g., `ln -s ...`).

## Workflow Categories

Workflows are organized into three categories under `workflows:`:

| Category | Key | TUI Key | User Prompt? | Description |
|---|---|---|---|---|
| **Tasks** | `workflows.tasks` | `n` (`:RunTask`) | Yes | For user-created tasks. User provides title + description. `:RunTask` opens a picker, then the new-task prompt with the workflow preselected. |
| **One-Off** | `workflows.one-off` | `x` (`:RunOneOff`) | No | Predefined jobs with built-in descriptions. Run directly. |
| **Init** | `workflows.init` | `i` (`:RunInit`) | No | Initialization pipelines (e.g., spin up from PRD). |

## Inline vs. File-Based Workflows

Workflows can be defined two ways. The `workflows:` section in `.sortie.yml`
controls which workflows are **active** (visible in TUI menus) and what order
they appear in. Each category's list accepts a mix of:

- **String refs** → resolved against `.sortie/workflows/<category>/<name>.yml`
- **Inline maps** → full workflow definition embedded directly in `.sortie.yml`

```yaml
workflows:
  tasks:
    - implement            # → .sortie/workflows/tasks/implement.yml (file-based)
    - review               # → .sortie/workflows/tasks/review.yml (file-based)
    - name: quick-fix      # inline
      steps:
        - name: do
          prompt: "fix it"
```

A workflow file at `.sortie/workflows/<category>/<name>.yml` contains the same
fields as an inline workflow body — minus the `name:` field, which is always
the filename. Use kebab-case filenames (`[a-z0-9-]+\.yml`).

**Files not referenced from `.sortie.yml` are loaded as hidden.** Hidden
workflows are:

- **Not** shown in TUI menus (the `n` / `x` / `i` shortcuts)
- **Reachable** via `:RunTask <name>`, `:RunOneOff <name>`, `:RunInit <name>` (and tab completion)
- **Reachable** via CLI: `sortie create -w <name>` accepts hidden workflows
- **Returned** by the MCP `list_workflows` tool with `"hidden": true`

### When to split a workflow into a file

Default to inline. Split when any of the following holds:

- The resulting `.sortie.yml` would exceed ~200 lines
- A single workflow body exceeds ~40 lines
- The category has more than five workflows

Splitting trades single-file readability for per-workflow editability. For tiny
projects, inline beats file-sprawl.

### Hard errors at config load

- String ref points to a missing file (`.sortie/workflows/<cat>/<name>.yml`).
- Same `<category>/<name>` is both inlined in `.sortie.yml` and present as a file.
- A file-based workflow sets a `name:` field (filename is authoritative).
- A workflow file uses a non-kebab-case filename or lives in a subdirectory of `.sortie/workflows/<cat>/`.

### Warnings (non-fatal — surfaced by `sortie validate`)

- File present under `.sortie/workflows/<cat>/` but not referenced in `.sortie.yml` (it's hidden).
- A category has files on disk but no `<cat>:` listing in `.sortie.yml` (everything in that category is hidden).

## Workflow Structure

```yaml
- name: my-workflow          # unique name (required)
  description: "..."         # human-readable (used as task desc for one-off/init)
  tmux: false                # default tmux mode for all steps
  summarizer_prompt: "..."   # custom prompt for post-completion summarizer
  steps:                     # ordered list of steps (required)
    - name: step-name        # unique step identifier (required)
      prompt: "..."          # template string sent to Claude (required)
      mode: ""               # execution mode (e.g., "automatic")
      tmux: true/false       # per-step override (omit to inherit workflow default)
      timeout: "30m"         # Go duration string
      human: false           # pause for human approval
      summarization_strategy: summarize_chat   # how this step's context is captured (see below)
      summarization_prompt: "..."              # prompt fed to the summarizer for THIS step's context
      loop:                  # optional: jump back to earlier step
        goto: "step-name"    # must reference an earlier step
        max_iterations: 3    # >= 1
        exit_condition:
          step_context_empty: "step-name"  # exit early if this step's context is empty
```

### Step summarization

By default, the agent's final output becomes the step's context. Set `summarization_strategy: summarize_chat` to instead summarize the entire transcript via a second Claude call using `summarization_prompt`. Inside `summarization_prompt`, the variable `{{chat}}` expands to the full transcript. This is essential for tmux/grilling steps where the meaningful output is the conversation, not a final message.

Set `summarization_strategy: none` to skip context capture entirely for the step — no last-message text is stored and no summarization pass is run. Useful for steps whose output is not meaningful to later steps (`{{steps.<name>.context}}` will resolve to empty).

### Prompt formatting

Prompt fields (`prompt`, `summarization_prompt`, `summarizer_prompt`, `system_prompt`) are LLM input, not human reading. Do not hard-wrap prose at ~80 columns — block scalars (`|`) preserve every newline as a token. Keep only the structural newlines: blank lines between paragraphs, one line per list item (continuation text stays on the item line), code fences verbatim. Reflow on contact when editing existing prompts.

### Wrapping multi-line interpolations

Several template variables expand to **multi-line** content at render time (a step's full output, a transcript, a task description). When inlined raw, the boundary between fixed prompt text and interpolated content vanishes — paragraphs of step context blend into the next instruction, and the receiving agent cannot tell where one ends and the other begins.

**Rule: wrap every multi-line interpolation in a semantic XML-style tag named after the variable.** Place the opening tag, the variable, and the closing tag each on their own line so the captured content sits between two clean boundaries:

```yaml
prompt: |
  Implement the following:
  <task-description>
  {{task.description}}
  </task-description>

  Earlier review feedback:
  <step-context name="reviewing">
  {{steps.reviewing.context}}
  </step-context>
```

Canonical tag for each multi-line variable:

| Variable | Wrapping tag |
|---|---|
| `{{task.description}}` | `<task-description>...</task-description>` |
| `{{task.images}}` | `<task-images>...</task-images>` |
| `{{steps.<name>.context}}` | `<step-context name="<name>">...</step-context>` |
| `{{artifacts.<name>}}` | `<step-context name="<name>">...</step-context>` (alias of the above) |
| `{{chat}}` | `<chat>...</chat>` |

Single-line variables (`{{task.id}}`, `{{task.title}}`, `{{task.slug}}`, `{{task.branch}}`, `{{git.base_branch}}`, `{{git.repo_root}}`, `{{loop.iteration}}`, `{{loop.max_iterations}}`) are inlined into surrounding prose **without** wrapping — they fit on one line and a tag would only add noise.

Do **not** use triple-backtick fences for this. Interpolated content (especially `{{chat}}` and summarized step contexts) routinely contains its own code fences, which would break the outer fence. XML-style tags survive arbitrary nested content.

## Template Variables

**Step prompts** (`prompt:`) and **summarizer prompts** (`summarizer_prompt:`):

Variables marked **multi-line** must be wrapped in a semantic tag — see [Wrapping multi-line interpolations](#wrapping-multi-line-interpolations).

| Variable | Description |
|---|---|
| `{{task.id}}` | Numeric task ID |
| `{{task.title}}` | Task title |
| `{{task.description}}` | Full task description **(multi-line — wrap in `<task-description>`)** |
| `{{task.slug}}` | URL-safe slug from title |
| `{{task.branch}}` | Resolved branch name |
| `{{task.images}}` | Newline-joined attached image paths **(multi-line — wrap in `<task-images>`)** |
| `{{git.base_branch}}` | Configured base branch |
| `{{git.repo_root}}` | Repository root path |
| `{{loop.iteration}}` | Current loop iteration (in loops) |
| `{{loop.max_iterations}}` | Max loop iterations (in loops) |
| `{{steps.<step_name>.context}}` | Context captured from a prior step's result **(multi-line — wrap in `<step-context name="<step_name>">`)** |
| `{{artifacts.<step_name>}}` | Backward compat alias for `{{steps.<step_name>.context}}` **(multi-line — same wrapping)** |

**Step `summarization_prompt:`** — same variables as above, plus:

| Variable | Description |
|---|---|
| `{{chat}}` | Full transcript of the step being summarized **(multi-line — wrap in `<chat>`)**. Only valid inside `summarization_prompt`. |

**`tmux-setup-command:`**:

| Variable | Description |
|---|---|
| `{{session_name}}` | Tmux session name created for the task |
| `{{worktree_path}}` | Absolute path to the task's worktree |
| `{{run_agent}}` | Pre-built command string that launches the Claude agent inside the worktree |

## Decision Tree

When the user describes what they want, follow this:

1. **"Just implement tasks"** → Single task workflow with an `implementing` step
2. **"Review before completing"** → Add a step with `human: true`
3. **"Interactive tmux session"** → Set `tmux: true` on workflow or step
4. **"Multi-step pipeline"** → Multiple steps with step context passing results between steps
5. **"Iterative review loop"** → Use `loop` config on a fix step pointing back to review
6. **"Predefined maintenance job"** → Use `workflows.one-off`
7. **"Bootstrap from PRD"** → Use `workflows.init`
8. **"Share files/dirs across worktrees"** ("symlink X into worktrees", ".env should be available", "docs/configs visible to agents") → Use `worktree-sync-paths` (`link:` for shared/synced files, `copy:` for per-worktree isolated copies). Note this is hard-link, not symlink.
9. **"Run something after worktree creation"** (install deps, generate files, create symlinks) → Use `worktree-setup-command` (single) or `worktree-setup-commands` (multiple)
10. **"Summarize a tmux/conversational step"** → Set `summarization_strategy: summarize_chat` and provide a `summarization_prompt` using `{{chat}}`

For complete field reference with validation rules and examples, read `references/config-reference.md`.

## Discovering undocumented fields

If you encounter a field used in an existing config that this skill doesn't document, or if the user asks about a feature not covered here, the binary itself is the authoritative source. Run:

```bash
strings $(which sortie) | grep 'yaml:"' | sort -u
```

This lists every YAML field name the binary will accept. Cross-reference unknown fields against the function names exposed in the binary (`strings $(which sortie) | grep 'aface/sortie/internal'`) to infer behavior. Update this skill when you confirm new fields work.

## Important Rules

- Step `name` values must be unique within a workflow
- Loop `goto` must reference an earlier step (no forward jumps, no self-reference)
- Loop steps cannot have `human: true` or `tmux: true`
- Loop ranges cannot overlap
- `git.on_complete` values: `"commit"`, `"merge"`, `"none"`
- `git.branch_template` supports: `{{task_id}}`, `{{task_slug}}`, `{{task.id}}`, `{{task.title}}`, `{{task.slug}}`
- The file goes at the project root as `.sortie.yml`
- For one-off and init workflows, the `description` field is used as the task description

## Validating a config

After **every** write or edit to a `.sortie.yml`, validate it with the built-in CLI:

```bash
sortie validate           # validates ./.sortie.yml
sortie validate path/to/.sortie.yml   # validates an explicit file
```

`sortie validate` runs the same checks the daemon performs at load time, plus a few that the runtime silently tolerates:

- YAML syntax errors
- **Unknown top-level fields** (catches typos like `worktree_sync_paths` for `worktree-sync-paths`)
- Workflow loop validity (forward `goto`, self-reference, missing target step, `max_iterations < 1`, overlapping ranges, `human`/`tmux` on a loop step)
- Invalid `summarization_strategy` values
- Invalid `git.on_complete` (must be `commit`, `merge`, or `none`)
- Invalid `default_priority` (must be `low`, `medium`, `high`, or `urgent`)
- Invalid `tmux_nested_attach_behavior` (must be `switch` or `nest`)
- Duplicate workflow names within a category and duplicate step names within a workflow
- File-based workflow errors: missing string ref, inline+file collision, invalid filename, `name:` field in file
- File-based workflow warnings: unreferenced files (hidden), category with files but no listing

Exit code is `0` on success and non-zero on the first error. Run it before reporting completion — never declare a config "done" until `sortie validate` exits cleanly.

## Output Instructions

When generating a `.sortie.yml`:
1. Ask what kind of workflows the user needs (or infer from context)
2. Generate a complete, valid YAML file
3. Write it to `.sortie.yml` in the project root
4. **Run `sortie validate`** and fix any reported errors before finishing
5. Explain the key choices made
