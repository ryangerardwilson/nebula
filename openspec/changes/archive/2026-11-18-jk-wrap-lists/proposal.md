## Why

`j` and `k` stop at the ends of the tunnel route list. Moving past the last route should return to the first, and moving above the first should return to the last.

## What Changes

- On `nebula tunnel screen`, `j` and `k` wrap the route list.
- An empty list stays put. While `?` is open, `j` and `k` still do nothing.
- Removing a route still leaves the cursor at the same index, clamped to the remaining routes.

## Capabilities

### New Capabilities

### Modified Capabilities

- `nebula`: `j` and `k` wrap the tunnel route list.

## Impact

- Spec: `openspec/specs/nebula/spec.md`.
- Code: `tunnel/internal/tui`.
- DNS, the saved route record, and the remove clamp stay as they are.
