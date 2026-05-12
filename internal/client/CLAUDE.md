# internal/client — Daemon Client

Unix socket IPC client with RPC and event streaming. Load `/client` skill before making substantial changes.

## Critical Invariants

- **All RPC methods are synchronous** — every daemon request (including `Subscribe`/`Unsubscribe`) waits on `respChan`. Fire-and-forget leaves the daemon's `MsgOK` reply orphaned in `respChan`, where it ambushes the next caller's response.
- **Background reader routes by message type** — broadcasts → `subChan`, others → `respChan`; wrong routing = hung RPCs or lost events
- **`Close()` uses `sync.Once`** — prevents double-close panics; do not add separate close paths
