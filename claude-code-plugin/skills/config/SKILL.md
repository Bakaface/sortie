---
name: sortie-config
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

            {{task.description}}
```

## Top-Level Fields

| Field | Type | Default | Description |
|---|---|---|---|
| `max_workers` | int | `3` | Max concurrent Claude agents |
| `default_priority` | string | `"medium"` | `low`, `medium`, `high`, `urgent` |
| `yolo` | bool | `false` | Pass `--dangerously-skip-permissions` to Claude |
| `validate_artifact` | bool | `false` | Validate artifact files are non-empty after steps |
| `verification` | object | — | Artifact retry and summarizer verification settings |
| `git` | object | — | Branch naming, base branch, completion action |
| `workflows` | object | — | **Primary config block** — defines all workflow pipelines |
| `notifications` | object | — | Desktop notification toggles |
| `tmux_nested_attach_behavior` | string | `"switch"` | `"switch"` or `"nest"` for tmux-in-tmux |

## Workflow Categories

Workflows are organized into three categories under `workflows:`:

| Category | Key | TUI Key | User Prompt? | Description |
|---|---|---|---|---|
| **Tasks** | `workflows.tasks` | `n` | Yes | For user-created tasks. User provides title + description. |
| **One-Off** | `workflows.one-off` | `r` | No | Predefined jobs with built-in descriptions. Run directly. |
| **Init** | `workflows.init` | `i` | No | Initialization pipelines (e.g., spin up from PRD). |

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
      artifact: false        # write summary to .sortie/artifacts/<step_name>.md
      loop:                  # optional: jump back to earlier step
        goto: "step-name"    # must reference an earlier step
        max_iterations: 3    # >= 1
        exit_condition:
          artifact_empty: "step-name"  # exit early if this step's artifact is empty
```

## Template Variables (for step prompts)

| Variable | Description |
|---|---|
| `{{task.id}}` | Numeric task ID |
| `{{task.title}}` | Task title |
| `{{task.description}}` | Full task description |
| `{{task.slug}}` | URL-safe slug from title |
| `{{task.branch}}` | Resolved branch name |
| `{{task.images}}` | Newline-joined attached image paths |
| `{{git.base_branch}}` | Configured base branch |
| `{{git.repo_root}}` | Repository root path |
| `{{loop.iteration}}` | Current loop iteration (in loops) |
| `{{loop.max_iterations}}` | Max loop iterations (in loops) |
| `{{artifacts.<step_name>}}` | Content of a prior step's artifact |

## Decision Tree

When the user describes what they want, follow this:

1. **"Just implement tasks"** → Single task workflow with an `implementing` step
2. **"Review before completing"** → Add a step with `human: true`
3. **"Interactive tmux session"** → Set `tmux: true` on workflow or step
4. **"Multi-step pipeline"** → Multiple steps with artifacts passing context
5. **"Iterative review loop"** → Use `loop` config on a fix step pointing back to review
6. **"Predefined maintenance job"** → Use `workflows.one-off`
7. **"Bootstrap from PRD"** → Use `workflows.init`

For complete field reference with validation rules and examples, read `references/config-reference.md`.

## Important Rules

- Step `name` values must be unique within a workflow
- Loop `goto` must reference an earlier step (no forward jumps, no self-reference)
- Loop steps cannot have `human: true` or `tmux: true`
- Loop ranges cannot overlap
- `git.on_complete` values: `"commit"`, `"merge"`, `"none"`
- `git.branch_template` supports: `{{task_id}}`, `{{task_slug}}`, `{{task.id}}`, `{{task.title}}`, `{{task.slug}}`
- The file goes at the project root as `.sortie.yml`
- For one-off and init workflows, the `description` field is used as the task description

## Output Instructions

When generating a `.sortie.yml`:
1. Ask what kind of workflows the user needs (or infer from context)
2. Generate a complete, valid YAML file
3. Write it to `.sortie.yml` in the project root
4. Explain the key choices made
