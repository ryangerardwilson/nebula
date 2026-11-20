## 1. Stat

- [x] 1.1 `s3x.Stat` returns mtime and size on a local root and through one remote `stat`, and a missing object is `ErrNotFound`

## 2. Verification

- [x] 2.1 `openspec validate stat-s3-object --strict --no-interactive`
- [x] 2.2 `GOWORK=off go test ./s3x/...`
