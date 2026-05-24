# tests/e2e — End-to-End Tests

Tests in this directory are gated by the `e2e` build tag. They drive sortie against a stubbed
`claude` binary (`stub-claude.sh`) under per-test isolated `XDG_CONFIG_HOME` + git repos.

## Running

```bash
go test -tags=e2e ./tests/e2e/...
```

`go test ./...` will **NOT** compile or run these tests. Always pass `-tags=e2e` here.

See [README.md](README.md) for prerequisites (Go 1.24+, `git`, optionally `tmux`),
`KEEP_E2E_TMPDIR=1` for forensic tmpdir paths on failure, and the per-scenario `testdata/`
layout.

## When to run

Any change to `internal/workflow/`, `internal/daemon/`, `internal/merge/`, `cmd/sortie/`,
or step-execution plumbing should be followed by an e2e run.
