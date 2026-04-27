# ✈ Sortie

Sortie is a daemon that orchestrates [Claude Code](https://docs.anthropic.com/en/docs/claude-code) agents through long-lived, multi-step workflows. Each task runs in its own git worktree on its own branch, advances through whatever steps you define in config — anything from a single "implement" step to a full plan/implement/review/approve/merge chain with loops and human gates — and reports back to a terminal UI where you stay in the driver's seat.

You decide what runs, how many run at once, where the human gates go, and how finished work lands on your base branch. Sortie just keeps the agents on the rails.

```
┌─────────────┐    ┌────────────────┐    ┌─────────────────┐
│  sortie tui │ ←→ │ sortie daemon  │ ←→ │ Claude Code     │
│  (control)  │    │ (orchestrator) │    │ agents in       │
└─────────────┘    └────────────────┘    │ git worktrees   │
                          │              └─────────────────┘
                          ↓
                    ┌──────────┐
                    │ SQLite   │
                    │ tasks.db │
                    └──────────┘
```

## Why

- **You stay in control.** Human-approval gates pause any step until you sign off. Tmux steps drop you straight into the agent's session for back-and-forth.
- **Parallelism without conflicts.** Every task gets a dedicated git worktree and branch, so N agents can work concurrently on the same repo without stepping on each other.
- **Workflows, not one-shots.** Chain planning, implementation, review, and final-approval steps. Loop the review/implement cycle until it converges. Pass artifacts between steps.
- **It survives a reboot.** Tasks live in SQLite. Logs are persisted per step. Stop the daemon, restart it, pick up where you left off.
- **Local first.** No cloud, no telemetry. A Go binary, a Unix socket, a SQLite file under `~/.config/sortie/`.

## Quick Start

```bash
# Build
go build -o sortie ./cmd/sortie

# Inside any git repo:
sortie init               # writes .sortie.yml + .sortie/ data dir
sortie daemon start       # starts the background daemon (Unix socket)
sortie tui                # opens the TUI
```

In the TUI, press `n` to create a new task, pick a workflow, and watch it run. Press `enter` on a task to follow its live logs.

To run from the command line instead:

```bash
sortie create "Add a /healthz endpoint"          # creates a pending task
sortie start <id>                                # kicks off its workflow
sortie logs <id>                                 # tails its logs
sortie tasks                                     # list, or `sortie tasks <id>` for detail
```

## How a task runs

1. **You create a task** (TUI or `sortie create`) and pick a workflow.
2. **The daemon picks it up** when a worker slot is free (`max_workers` controls concurrency).
3. **A worktree is provisioned** at `.worktrees/<branch>` on a new branch derived from `git.branch_template`. `worktree-sync-paths` and `worktree-setup-commands` run here (e.g. copy `.env`, run `bun install`).
4. **Each workflow step spawns a Claude Code agent** in that worktree with the rendered prompt and a Sortie-built system prompt. Output is parsed live (NDJSON), persisted to per-step log files, and broadcast to the TUI.
5. **Step context is captured** at the end of each step (the agent's last message by default, or a Haiku summary when `summarization_strategy: summarize_chat`) and made available to later steps via `{{steps.<name>.context}}`.
6. **Human gates pause** the workflow on `human: true` steps; **tmux gates** suspend until you detach from the agent's tmux session. **Loops** jump back to an earlier step until an exit condition is met or `max_iterations` is reached.
7. **On completion**, depending on `git.on_complete`, Sortie either leaves the work as a `commit` on the branch or `merge`s it into the base branch.

## Workflow configuration

Workflows live in `.sortie.yml` at the repo root. Three categories:

- **`tasks:`** — workflows you assign to ad-hoc tasks (the default category).
- **`one-off:`** — workflows you trigger directly from the TUI (`x` key) without a task description, e.g. a "refactor pass" or "run tests".
- **`init:`** — workflows for project bootstrapping (`i` key in TUI).

Minimal `.sortie.yml`:

```yaml
max_workers: 3
yolo: false                       # pass --dangerously-skip-permissions to claude

git:
  base_branch: main
  branch_template: sortie/{{task_id}}-{{task_slug}}
  on_complete: merge              # commit | merge

workflows:
  tasks:
    - name: sensible workflow
      steps:
        - name: implementing
          prompt: |
            Implement task #{{task.id}}: {{task.title}}
            {{task.description}}
          timeout: 30m

        - name: reviewing
          prompt: |
            Review the implementation.
            ## Implementation summary
            {{steps.implementing.context}}
          timeout: 20m
```

### Step options

| Key | Type | Notes |
|---|---|---|
| `name` | string | Step ID, used in `{{steps.<name>.context}}` and loop targets. |
| `prompt` | string | Templated prompt sent to the agent. |
| `timeout` | duration | e.g. `30m`. Default: 30 minutes. |
| `human` | bool | Pause and wait for explicit approval in the TUI. |
| `tmux` | bool | Run inside a tmux session you can attach to (`t` in the TUI). Step-level overrides workflow-level. |
| `summarization_strategy` | enum | `last_message` (default) or `summarize_chat` (Haiku-summarized chat log). |
| `loop` | object | Jump back to an earlier step. See below. |

### Loops

```yaml
- name: reviewing
  prompt: |
    Review iteration {{loop.iteration}} of {{loop.max_iterations}}.
    {{steps.implementing.context}}
    If everything passes, output nothing.
  loop:
    goto: implementing
    max_iterations: 3
    exit_condition:
      step_context_empty: reviewing   # exit early when this step's output is empty
```

Loops must point to an earlier step, can't be `human:` or `tmux:`, and can't overlap with other loops.

### Template variables

Available in any step `prompt`:

- `{{task.id}}`, `{{task.title}}`, `{{task.description}}`, `{{task.context}}`, `{{task.slug}}`, `{{task.branch}}`
- `{{git.base_branch}}`
- `{{steps.<step_name>.context}}` — captured output of a prior step
- `{{loop.iteration}}`, `{{loop.max_iterations}}` (inside a loop body)

### Worktree provisioning

```yaml
worktree-sync-paths:
  copy: [".env", ".env.local"]      # copied into each worktree
  link: [".claude", "node_modules"] # symlinked

worktree-setup-commands:            # run sequentially after sync
  - bun install
  - bun run db:migrate

tmux-setup-command: |               # run once after tmux session creation
  tmux split-window -h "tail -f .sortie/logs/<id>/<step>.log"
```

## Project layout reset


```
cmd/sortie/         CLI entry points (daemon, tui, task CRUD)
internal/
  config/           .sortie.yml parsing, project type auto-detection
  daemon/           Background daemon: Unix socket server, scheduling, pub/sub
  workflow/         Step engine, prompt templating, summarizer, merge logic
  claude/           Claude Code process spawning, NDJSON stream parsing
  agent/            Agent state machine, concurrent worker manager
  task/             Task model, status state machine, priority, dependencies
  tui/              BubbleTea terminal UI (list, detail, prompt, animation)
  db/               SQLite persistence and migrations
  git/              Worktree, branch, merge, conflict-resolution operations
  tmux/             Tmux session lifecycle, capture, monitoring
  client/           IPC client (RPC + event subscription) for tui/cli
  notify/           Desktop notifications
claude-code-plugin/ Companion Claude Code plugin (sortie-config skill)
```

The daemon listens on a Unix socket at `~/.config/sortie/daemon.sock` (or `$XDG_CONFIG_HOME/sortie/`) and persists state to `tasks.db` next to it. Project-level data (logs, the `.worktrees/` directory) lives under `.sortie/` inside the repo.

## TUI

Launch with `sortie tui`. Add `-g` / `--global` to see tasks across every project Sortie has tracked.

Common keys (full help with `ctrl+h`):

| Key | Action |
|---|---|
| `j` / `k` / `↑↓` | Move selection |
| `enter` | Open task detail / follow live logs |
| `n` / `N` | New task / new blocking task |
| `x` | Run a one-off workflow (no task needed) |
| `i` | Run an init workflow |
| `c` | Continue an awaiting-approval / completed / failed task |
| `s` | Stop the running step |
| `r` / `R` | Retry / revert |
| `t` | Attach to the task's tmux session |
| `b` / `alt+b` | Branch a new task off this one / toggle branch tree view |
| `D` / `A` | Detach branch from worktree / reattach |
| `o` / `e` | Open / edit step context (artifact) |
| `dd` | Delete task (worktree + branch + logs) |
| `/`, `?`, `n`, `N` | Vim-style search and next/prev match |
| `gg`, `G`, `:N` | Jump to top, bottom, or line N |

In the detail view, `j/k/G/gg/ctrl+u/ctrl+d` scroll logs; `esc` toggles between follow and normal mode; `e` opens the log file in `$EDITOR`.

## Tmux integration

Setting `tmux: true` on a step (or workflow) runs the agent inside a named tmux session (`sortie/<project>/<task_id>/<step>`). This is what you want when:

- The agent might ask clarifying questions you'll answer interactively.
- You want to drop in, observe, and steer the conversation in real time.
- A step is intrinsically interactive (e.g. reviewing diffs side-by-side).

Press `t` in the TUI to attach. Sortie detects nested-tmux situations (you're already inside tmux) and either switches client or nests a session, controlled by `tmux_nested_attach_behavior` (`switch` / `nest`).

`sortie attach <task_id>` does the same from the shell.

## CLI reference

**Daemon**

```bash
sortie daemon start [-f]      # start (background, or -f for foreground)
sortie daemon stop            # graceful shutdown
sortie daemon status          # is it running, what PID
```

**Tasks**

```bash
sortie create <description> [--workflow w] [--priority high]
              [--branch tmpl] [--target main] [--checkout existing-branch]
              [--no-worktree]
sortie tasks [<id>]           # list, or detail for one
sortie edit <id> [--title T] [--description D] [--context C] [--priority P]
sortie delete <id> [-y]
sortie start <id>             # manually kick off a pending task
sortie stop <id>              # stop a running task
sortie retry <id>             # retry a failed task from its current step
sortie revert <id>            # revert all commits made by a completed task
sortie continue <id>          # resume an awaiting-approval / completed / failed task
sortie logs <id> [step] [-n N]
sortie cleanup [<id>]         # remove worktree + branch + logs for completed/failed
sortie agents                 # list running agents
```

**Worktree branch management**

```bash
sortie detach <id>            # detach branch so you can check it out elsewhere
sortie attach-branch <id>     # reattach after detach
sortie attach <id>            # attach to the task's tmux session
```

**TUI**

```bash
sortie tui [-g]               # -g for cross-project view
```

## Requirements

- Go 1.24+
- git (worktree support, ≥ 2.5)
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) CLI on `PATH` as `claude`
- tmux (only required if you use tmux steps or `sortie attach`)
