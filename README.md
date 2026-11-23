# nebula

Nebula is one binary. `sql` talks to Postgres on an SSH host. `s3` stores objects. `keep` keeps a durable copy of each application's sqlite file. `run` builds a site on this laptop and serves it on that host. `tunnel` publishes a laptop port on a hostname. `crypt` reads and writes 1Password items. The hostname `run` publishes is the site origin.

`sql` does not hold application data. Each tool keeps its own sqlite file in memory (`$XDG_RUNTIME_DIR`) and owns its schema and writes. After a commit the tool runs `nebula keep mark <name> <path>`, which returns in milliseconds; a detached worker uploads a consistent snapshot to `s3://backups/<name>.sqlite`, coalesces bursts, retries with backoff, and never lets an empty file replace a backup. At startup a missing file comes back with `nebula keep restore <name> <path>`. A file with no restore marker is not uploaded; `nebula keep adopt <name> <path>` vouches once for a current file that an app restored before keep existed. `nebula keep status` shows how far each copy lags. `nebula-keep.timer` runs `nebula keep sweep` every 10 minutes for a missed mark, and `nebula-keep-flush.service` runs `nebula keep flush` at logout or shutdown. `run` does not migrate a database. `tunnel` is the laptop connector, and the host connector stays separate. None of these features opens a public port on the host.

`sql`, `s3`, `run`, `tunnel`, and `crypt` on `PATH` are this same binary. `crypt` uses the 1Password service account. `c3po pass` runs `nebula crypt`. `r2d2 pass` stays the item screen and reaches 1Password through `nebula crypt`.

```
nebula sql create database app
nebula sql app migrate ~/app/models
nebula sql app query "select 1"
nebula s3 make bucket notes
nebula s3 put ./today.txt s3://notes/today.txt
nebula s3 get s3://notes/today.txt ./today.txt
nebula keep mark c3po /run/user/1000/c3po/c3po.sqlite
nebula keep status
nebula run ./site
nebula tunnel list
nebula crypt list Root
```

With nothing else set, `sql` uses Postgres on the host named by `SQLX_PG_SSH`, or `del-vc2`. A set `SQLX_ROOT` keeps `sql` on a local sqlite directory. `SQLX_ENGINE` can force `postgres`, `sqlite`, or `s3`. `S3_ROOT` keeps `s3` on a local directory and does not open SSH, and `S3_SSH` names the host. Objects live under `/var/lib/nebula/s3` on that host.

## Why this shape

A prototype needs a database, object storage, a public URL, and secrets without standing up cloud infrastructure or opening ports on the host.

`nebula` gives each tool its own SQLite file (owned by the tool, not by `sql`). After a commit the tool marks the file; a background worker uploads a consistent snapshot to S3. At startup a missing file is restored. The same binary also publishes a laptop port behind a hostname and reads and writes 1Password items.

The result is a first version that can be inspected, backed up, and iterated the same day it is written — without a separate platform team or a public attack surface.

## Install

From a checkout:

```sh
./install.sh from "$PWD"
```

That builds `~/.local/bin/nebula` and links `sql`, `s3`, `run`, `tunnel`, and `crypt` to it. A leftover `auth` name that points at `nebula` is removed. The daily backup timer is `nebula-sql-backup.timer`. Install also enables `nebula-keep.timer` and `nebula-keep-flush.service`. Install retires the old `sqlx-backup.timer`. It does not restart `tunnel.service`.

Public source is https://github.com/ryangerardwilson/nebula.

Part of an open-source harness for shipping the smallest inspectable system that agents and operators can run and improve. See also [nebula](https://github.com/ryangerardwilson/nebula), [c3po](https://github.com/ryangerardwilson/c3po), [r2d2](https://github.com/ryangerardwilson/r2d2), [crusade](https://github.com/ryangerardwilson/crusade), [tesseract](https://github.com/ryangerardwilson/tesseract).
