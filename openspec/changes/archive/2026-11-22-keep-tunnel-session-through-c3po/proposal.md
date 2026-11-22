## Why

The tunnel screen kept its resumable session by writing c3po's `tui` session and active records straight into the c3po sqlite file. Two programs then wrote one store. The active record repeated the session id, screen, and workspace that its key and the session already held. c3po now owns screen sessions in its own tables and exposes `c3po __session` as the one way to open, note, place, and close one.

## What Changes

- The tunnel screen opens, notes, places, and closes its session through `c3po __session`, passing its process id. It no longer writes c3po's tables.
- Opening with `--resume <id>` restores the place c3po prints. A missing session, a session of another screen, or a failed c3po call fails the open.
- A record write keeps the row's id: it updates the row when it exists and inserts it otherwise.

## Capabilities

### New Capabilities

- `tunnel-session`: the tunnel screen keeps its session through c3po.

### Modified Capabilities

None.

## Impact

- Code: `tunnel/internal/session/session.go` and its test, `tunnel/internal/records/sqlite.go`.
- Needs a c3po with `c3po __session` on PATH. Without it, opening a tunnel session reports the error.
