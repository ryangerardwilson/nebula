# nebula

Nebula is the storage binary local tools share. One file, two features.

`sql` keeps each application database in a runtime sqlite file and can publish that file as an object. `s3` stores, retrieves, copies, and deletes objects with `s3://` URIs. Both use one SSH host. Neither opens a public port.

`sql` and `s3` on `PATH` are this same binary.

```
nebula sql create database app
nebula sql app migrate ~/app/models
nebula sql app query "select 1"
nebula s3 make bucket notes
nebula s3 put ./today.txt s3://notes/today.txt
nebula s3 get s3://notes/today.txt ./today.txt
```

A set `SQLX_ROOT` keeps `sql` on a local sqlite directory. `SQLX_ENGINE=postgres` keeps the Postgres path. `SQLX_ENGINE=s3` forces object snapshots. `S3X_ROOT` keeps `s3` on a local directory and does not open SSH. `SQLX_PG_SSH` names the SSH host.

## Install

From a checkout:

```sh
./install.sh from "$PWD"
```

That builds `~/.local/bin/nebula` and links `sql` and `s3` to it. The daily snapshot timer stays `sqlx-backup.timer`.

Public source is https://github.com/ryangerardwilson/nebula.
