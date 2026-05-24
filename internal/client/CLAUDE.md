# internal/client — Daemon Client

Unix socket IPC client with RPC and event streaming. Load `/client` skill before making substantial changes.

## Critical Invariants

- **All RPC methods are synchronous** — every daemon request (including `Subscribe`/`Unsubscribe`) waits on `respChan`. Fire-and-forget leaves the daemon's `MsgOK` reply orphaned in `respChan`, where it ambushes the next caller's response.
- **Background reader routes by message type** — broadcasts → `subChan`, others → `respChan`; wrong routing = hung RPCs or lost events
- **`Close()` uses `sync.Once`** — prevents double-close panics; do not add separate close paths
- **Reconnect is bounded to ONE retry per call** — on wire failure (Write error or `errConnectionClosed` signaled by `readLoop`), `sendAndWait`/`send` close the conn, dial fresh, transparently re-subscribe if previously subscribed, and retry the original request exactly once. A second consecutive failure escalates to the caller — never spin-loop. Reconnect does NOT fire after explicit `Close()` (the `done` channel is the terminal signal). See `reconnectLocked` for details.
- **Subscription state is tracked on the `Client` struct** — `c.subscribed` flips inside the `c.mu`-protected critical section via `sendAndWaitWithHook`'s onSuccess callback, so a concurrent reconnect can't miss the state change. The reconnect path re-issues `MsgSubscribe` on the new conn before retrying the original request, so broadcast-dependent flows (e.g. `create_task --wait_for_ready`) survive a daemon hiccup.
