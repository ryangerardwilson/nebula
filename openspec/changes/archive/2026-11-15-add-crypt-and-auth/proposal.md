# Add crypt and auth

## Why
1Password items are still reached through c3po pass. Google sign-in needs that same store, under a nebula command that is not named vault, because a 1Password vault is already the container those items live in.

## What Changes
- `nebula crypt` reads and writes 1Password items with the service-account token. `list`, `get`, `describe`, `create`, `put`, and `delete` are the commands. `vaults`, `items`, `copy`, and `delete <vault> item <id>` serve the item screen. The older argument forms still run. There is no crypt screen.
- `nebula auth` records one shared Google client id and signs a product in. The client id and secret stay in Root items `GOOGLE_OAUTH_CLIENT_ID` and `GOOGLE_OAUTH_CLIENT_SECRET`, read through crypt. The secret is not written to disk and is not printed.
- `c3po pass` runs `nebula crypt`. `c3po tui pass` stays the item screen and reaches 1Password through `nebula crypt`, not through `op`.
- auth does not create the Google client, open Postgres, or store application users.

## Impact
Capabilities: nebula. Classification: tooling. No new cloud instance. No secret is copied into this change.
