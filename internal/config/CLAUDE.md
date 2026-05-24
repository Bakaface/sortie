# internal/config — Configuration

`.sortie.yml` parsing, project type detection, workflow definitions. Load `/config` skill before making substantial changes.

## Critical Invariants

- **Loop goto must reference an earlier step** — validated at parse time; forward jumps or self-references create infinite loops
- **Summarization strategy affects step context capture** — `last_message` uses Claude result event text; `summarize_chat` spawns background haiku summarization
- **Config merge hierarchy is strict** — `.sortie.yml` > `~/.sortie.yml` > global defaults; invalid configs halt the CLI
- **Project string refs fall back to a global workflow pool** — after `~/.sortie.yml` is loaded, its resolved workflows (both inline and file-based under `~/.sortie/workflows/<cat>/`) are snapshotted into `cfg.globalPool`. Project-level string refs (`workflows.<cat>: [name]`) try the local `.sortie/workflows/` pool first, then `cfg.globalPool`. A project may override a global workflow by defining it inline or by adding a local `.sortie/workflows/<cat>/<name>.yml`. Inline-vs-file collision is enforced only within a single config scope.
