## 1. Command

- [x] 1.1 `nebula crypt load <vault>` prints export lines on a pipe, skips non-exportable items, and refuses a terminal without printing or copying a secret

## 2. Verification

- [x] 2.1 `openspec validate load-vault-into-shell --strict --no-interactive`
- [x] 2.2 `GOWORK=off go test ./crypt/...`
