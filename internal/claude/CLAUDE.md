# internal/claude — Claude Code Process Spawning

Process lifecycle, NDJSON stream parsing, output handling. Load `/claude-process` skill before making substantial changes (also covers `internal/agent/`).

## Critical Invariants

- **State transitions go through Manager methods, not direct field assignment** — ensures callbacks fire and state stays consistent
- **OnStateChange callback fires outside mutex** — prevents deadlocks with daemon broadcaster
- **StreamParser extracts result event text for step context** — NDJSON event type validation is critical; wrong type = lost output

## Testing

- `process_test.go` uses `//go:build integration` — plain `go test ./...` does **not** run it.
- Run integration tests with `mise run ti` (alias for `mise run test:integration`, which is
  `go test -tags integration ./...`).
