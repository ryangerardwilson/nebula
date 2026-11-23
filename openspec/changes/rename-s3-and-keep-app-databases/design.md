## Ownership

- Each application owns its database file, its schema, its migrations, and its one write path. A write commits locally and returns.
- keep owns one fact per database: the backup record in `$XDG_STATE_HOME/nebula/keep/<name>.json` (`marked`, `kept`, `dirty_since`, `kept_at`, `kept_size`, `source`, `error`, `error_at`, `failures`). The state shown by `status` (`ok`, `waiting`, `failing`) is derived from those fields and never stored. The record has one write path, `Keeper.update`, under `<name>.lock`.
- The restore marker `<runtime file>.keep` sits beside the runtime file on tmpfs, so it lives and dies with that file. `restore` writes `restored` or `fresh`.

## Where keep lives

keep is in `internal/backup`, beside the daily sql snapshot, because the operator asked for the backup component to be built where nebula already backs databases up rather than beside it. Both share the consistent snapshot (`store.BackupFile`) and the object store. The cost is that tunnel records, which call keep, also link the sql backup code into their import graph; it is one binary either way. Moving keep to `internal/keep` is a pure code move if that cost starts to matter.

## Why a CLI and an on-demand worker

Every app already reaches `s3` as a subprocess, and none needs a module dependency on nebula. An on-demand worker per database has no IPC protocol and no shared in-memory state; one database's failure does not block another. SSH reuse comes from `ControlPersist`.

## Failure behaviour

- Backup host slow or down: commits succeed; the record stays `waiting`, then `failing` with the error; the worker backs off 5 s, 30 s, 2 min, 10 min and exits; `sweep` and the next `mark` start again.
- Runtime file missing and host down: `restore` fails and makes no file.
- An empty database, or a file with no restore marker, is refused and recorded as the error. A file that predates keep is uploaded only after the operator runs `nebula keep adopt` for it; adoption is never automatic, so a file an application made after a failed restore cannot replace the backup.
- Logout or shutdown with a lagging copy: `nebula-keep-flush.service` waits up to 20 s. Writes since the last good copy are lost at power-off if the host is down; that is the explicit price of write-behind on tmpfs.
