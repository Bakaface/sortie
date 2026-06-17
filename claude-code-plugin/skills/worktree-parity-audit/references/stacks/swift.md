# Swift — Worktree Parity Recipe

Marker file: `Package.swift`.

## Typically gitignored

- `.build/`, `Packages/`
- `*.xcodeproj/xcuserdata/`

## Standard install

- `swift package resolve`
- `swift build`

## Gotchas

- **macOS-only stack.** Sortie on Linux won't help; flag and stop.
