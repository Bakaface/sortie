# internal/config — Configuration

`.sortie.yml` parsing, project type detection, workflow definitions. Load `/config` skill before making substantial changes.

## Critical Invariants

- **Loop goto must reference an earlier step** — validated at parse time; forward jumps or self-references create infinite loops
- **Summarization strategy affects step context capture** — `last_message` uses Claude result event text; `summarize_chat` spawns background haiku summarization
- **Config merge hierarchy is strict** — `.sortie.yml` > `~/.sortie.yml` > global defaults; invalid configs halt the CLI
