## 1. Object store

- [x] 1.1 The object store package is `s3`, the env names are `S3_ROOT` and `S3_SSH`, and the remote directory is `/var/lib/nebula/s3`
- [x] 1.2 One SSH command per operation; `put` exits 3 for a missing bucket
- [x] 1.3 Quote the `find -printf` format for the remote shell; a fake `ssh` that re-parses through `sh -c` proves `list`

## 2. keep

- [x] 2.1 `mark`, the detached worker, `restore`, `status`, `flush`, `sweep` in `internal/backup`
- [x] 2.2 Units `nebula-keep.timer`, `nebula-keep.service`, `nebula-keep-flush.service`, and install lines
- [x] 2.3 Tunnel records mark and restore through keep; cache publish makes the bucket only on a missing bucket

## 3. Verification

- [x] 3.1 `c3po ast exec -- openspec validate rename-s3-and-keep-app-databases --strict --no-interactive`
- [x] 3.2 `GOWORK=off go test ./...`
