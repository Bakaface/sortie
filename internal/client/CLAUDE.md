# internal/client — Daemon Client

Unix socket IPC client with RPC and event streaming. Load `/client` skill before making substantial changes.

## Critical Invariants

- **RPC methods are synchronous; `Unsubscribe` is fire-and-forget** — most RPCs wait on `respChan`; mixing this up causes deadlocks
- **Background reader routes by message type** — broadcasts → `subChan`, others → `respChan`; wrong routing = hung RPCs or lost events
- **`Close()` uses `sync.Once`** — prevents double-close panics; do not add separate close paths
