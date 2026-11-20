## Context

The route screen increments the cursor while it is inside the route list. Help returns before that path. Removing a route clamps the cursor to the remaining routes.

## Goals / Non-Goals

**Goals:**

- `j` and `k`, and the paired down and up keys, wrap the route list.
- An empty list does not move.
- Help still swallows `j` and `k`.
- A remove still clamps the cursor.

**Non-Goals:**

- Changing add, remove, or DNS.
- The r2d2 menu and the c3po screens.

## Decisions

- Wrap with `(cursor + delta%n + n) % n` when the list has routes. Go's remainder keeps the sign of the dividend, so the added `n` keeps the index non-negative.
- Only the `j` and `k` path wraps. The clamp after a remove stays a clamp.

## Risks / Trade-offs

- `k` on the first route now selects the last route. That used to stay put.

## Migration Plan

- No stored data changes. An already-open route screen keeps its current binary until it quits.
