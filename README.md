# nebula

Nebula is one binary. `sql` talks to Postgres on an SSH host. `s3` stores objects. `run` builds a site on this laptop and serves it on that host. `tunnel` publishes a laptop port on a hostname. `crypt` reads and writes 1Password items. The hostname `run` publishes is the site origin.

`sql` does not hold application data. Each tool keeps its own in-memory sqlite and backs that file up with `s3`. `run` does not migrate a database. `tunnel` is the laptop connector, and the host connector stays separate. None of these features opens a public port on the host.

`sql`, `s3`, `run`, `tunnel`, and `crypt` on `PATH` are this same binary. `crypt` uses the 1Password service account. `c3po pass` runs `nebula crypt`. `r2d2 pass` stays the item screen and reaches 1Password through `nebula crypt`.

```
nebula sql create database app
nebula sql app migrate ~/app/models
nebula sql app query "select 1"
nebula s3 make bucket notes
nebula s3 put ./today.txt s3://notes/today.txt
nebula s3 get s3://notes/today.txt ./today.txt
nebula run ./site
nebula tunnel list
nebula crypt list Root
```

With nothing else set, `sql` uses Postgres on the host named by `SQLX_PG_SSH`, or `del-vc2`. A set `SQLX_ROOT` keeps `sql` on a local sqlite directory. `SQLX_ENGINE` can force `postgres`, `sqlite`, or `s3`. `S3X_ROOT` keeps `s3` on a local directory and does not open SSH.

## Install

From a checkout:

```sh
./install.sh from "$PWD"
```

That builds `~/.local/bin/nebula` and links `sql`, `s3`, `run`, `tunnel`, and `crypt` to it. A leftover `auth` name that points at `nebula` is removed. The daily snapshot timer stays `sqlx-backup.timer`. It does not restart `tunnel.service`.

Public source is https://github.com/ryangerardwilson/nebula.
