# nebula

Nebula is one binary with two features.

`sql` talks to Postgres on an SSH host. It does not hold application data. Each tool keeps its own in-memory sqlite. `s3` stores, retrieves, copies, and deletes objects with `s3://` URIs, and that is how those tools back the sqlite file up. Neither feature opens a public port.

`sql` and `s3` on `PATH` are this same binary.

```
nebula sql create database app
nebula sql app migrate ~/app/models
nebula sql app query "select 1"
nebula s3 make bucket notes
nebula s3 put ./today.txt s3://notes/today.txt
nebula s3 get s3://notes/today.txt ./today.txt
```

With nothing else set, `sql` uses Postgres on the host named by `SQLX_PG_SSH`, or `del-vc2`. A set `SQLX_ROOT` keeps `sql` on a local sqlite directory. `SQLX_ENGINE` can force `postgres`, `sqlite`, or `s3`. `S3X_ROOT` keeps `s3` on a local directory and does not open SSH.

## Install

From a checkout:

```sh
./install.sh from "$PWD"
```

That builds `~/.local/bin/nebula` and links `sql` and `s3` to it. The daily snapshot timer stays `sqlx-backup.timer`.

Public source is https://github.com/ryangerardwilson/nebula.
