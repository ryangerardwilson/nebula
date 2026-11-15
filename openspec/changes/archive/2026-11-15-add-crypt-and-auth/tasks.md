## Tasks

- [x] Validate this change with `openspec validate add-crypt-and-auth --strict --no-interactive`
- [x] Add `nebula crypt` over 1Password, including list, get, describe, create, put, and delete, with no screen. `vaults`, `items`, `copy`, and `delete <vault> item <id>` serve `c3po tui pass`
- [x] Add `nebula auth` that reads the Root Google client through crypt and does not store the secret
- [x] Point `c3po pass` at `nebula crypt`, and make `c3po tui pass` call `nebula crypt` instead of `op`
- [x] Retarget context docs under the home directory and Work from `c3po pass` to `nebula crypt`, and leave `c3po tui pass`
- [x] Run `GOWORK=off go test ./...` and `bash tests/run.sh`
