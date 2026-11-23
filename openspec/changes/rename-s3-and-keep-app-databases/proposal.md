## Why

The Slack desk's mark-read key waited about 3.5 s, and up to 9 s, on a synchronous `nebula s3 put` of the whole database inside the desk's write path. That put was three SSH commands in a row, each about 1 s over wstunnel and Cloudflare. c3po also ran `s3 make bucket` (two more commands) before every publish, and leaving the Slack screen waited about 6 s on a publish. Seven applications each carried their own copy of backup and restore code, with different timeouts, sync or async behaviour, and different guards against an empty file replacing a good backup. `nebula s3 list` over SSH printed nothing because the remote login shell stripped the backslashes in `find -printf`.

The object store package is `s3` and the remote directory is `/var/lib/nebula/s3`.

## What Changes

- The object store package is `nebula/s3`. The remote directory is `/var/lib/nebula/s3`. Reads, writes, and deletes use that directory only. `S3_ROOT` and `S3_SSH` are the env names.
- Every `s3` operation over SSH is one remote command. `put` checks the bucket, writes a temporary file, and renames it in one command; a missing bucket exits 3 and is `ErrNotFound`.
- `nebula s3 list` and `list s3://<bucket>` quote their `find` format for the remote shell and work again.
- New `nebula keep`, inside the existing backup component (`internal/backup`), owns one fact per application database: whether its local sqlite file has a durable copy at `s3://backups/<name>.sqlite`. `mark` records a change and wakes a detached per-database worker and never touches the network. The worker uploads a consistent snapshot (sqlite backup API), coalesces bursts, retries with backoff, takes the same `<db>.publish.lock` c3po's older publisher takes, and refuses to replace a backup with an empty database or with a file that did not come from `keep restore`. `restore`, `adopt`, `status [--json]`, `flush [--timeout]`, and `sweep` complete it. `adopt` is the operator's one-time vouch for a current file that an application restored itself before keep existed. `nebula-keep.timer` runs `sweep` every 10 minutes; `nebula-keep-flush.service` runs `flush` at logout or shutdown.
- nebula's own tunnel records and the nebula sql runtime cache stop uploading the live file themselves: tunnel records call keep, and the cache publish makes the bucket only when a put finds it missing.

## Non-goals

- No in-memory database manager. Applications keep their schema, rules, and single write path; keep never runs their SQL.
- No move of any live object or runtime file in this change. Object keys and formats are unchanged. The code does not read any other directory.
- Direct SSH instead of wstunnel (plan N5) is the operator's call.

## Capabilities

### Modified Capabilities

- `nebula`: the object store is renamed and made one round trip per operation; the keep backup component is added.

## Impact

r2d2 health imports `github.com/ryangerardwilson/nebula/s3`. Install nebula before the apps that call `nebula keep`. After installing, run `nebula keep adopt <name> <path>` once for each application file that is current and was restored before keep existed (c3po, the desk, musica, crusade); until then keep refuses to upload it and status says so. After the next reboot every file comes back through `nebula keep restore` and carries its own marker.

Rollback is reinstalling the previous nebula binary and moving objects into the directory that binary uses. Object keys are unchanged.
