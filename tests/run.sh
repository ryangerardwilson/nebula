#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

grep -q 'module github.com/ryangerardwilson/nebula' go.mod || fail "module path"
grep -q 'replace ' go.mod && fail "go.mod still replaces a module" || true
grep -q 'install_from_source' install.sh || fail "install.sh missing from-source"
grep -q 'sqlx-backup.timer' install.sh || fail "install.sh missing timer"
grep -q 'Persistent=true' sqlx-backup.timer || fail "timer is not persistent"
grep -F -q 'OnCalendar=*-*-* 03:15:00' sqlx-backup.timer || fail "timer is not daily at 03:15"
grep -q 'sql backup' sqlx-backup.service || fail "backup unit missing sql backup"

bin=$(mktemp)
trap 'rm -f "$bin"' EXIT
( cd "$root" && GOWORK=off CGO_ENABLED=0 go build -o "$bin" ./cmd/nebula )

help=$("$bin" help)
printf '%s\n' "$help" | grep -q 'nebula sql create database app' || fail "help missing sql"
printf '%s\n' "$help" | grep -q 'nebula s3 put ./today.txt s3://notes/today.txt' || fail "help missing s3"
printf '%s\n' "$help" | grep -q 'commands:' && fail "help has a commands section" || true
[ "$("$bin" version)" = "0.1.0" ] || fail "nebula version"
[ "$("$bin" sql version)" = "0.1.0" ] || fail "sql version"
s3_help=$("$bin" s3 help)
printf '%s\n' "$s3_help" | grep -q 'nebula s3 make bucket notes' || fail "s3 help"
printf '%s\n' "$s3_help" | grep -q 'commands:' && fail "s3 help has a commands section" || true
printf '%s\n' "$help" | grep -q 'nebula run ./site' || fail "help missing run"
printf '%s\n' "$help" | grep -q 'nebula tunnel list' || fail "help missing tunnel"
printf '%s\n' "$help" | grep -q 'nebula crypt list Root' || fail "help missing crypt"
printf '%s\n' "$help" | grep -q 'nebula auth setup' || fail "help missing auth"
(
  d=$(mktemp -d)
  cp "$bin" "$d/tunnel"
  cp "$bin" "$d/run"
  cp "$bin" "$d/crypt"
  cp "$bin" "$d/auth"
  "$d/tunnel" help | grep -q 'nebula tunnel add notes on 8080' || exit 1
  "$d/run" help | grep -q 'nebula run ./site' || exit 1
  "$d/crypt" help | grep -q 'nebula crypt list Root' || exit 1
  "$d/auth" help | grep -q 'nebula auth setup' || exit 1
  "$d/crypt" help | grep -q 'commands:' && exit 1 || true
  "$d/run" help | grep -q 'commands:' && exit 1 || true
  rm -rf "$d"
) || fail "argv0 feature help"

work=$(mktemp -d)
trap 'rm -rf "$work"; rm -f "$bin"' EXIT
(
  export S3X_ROOT=$work/s3
  unset SQLX_ENGINE S3X_SSH || true
  file=$work/today.txt
  printf 'hello\n' > "$file"
  "$bin" s3 make bucket notes | grep -qx 'bucket notes'
  "$bin" s3 put "$file" s3://notes/today.txt | grep -qx 'put s3://notes/today.txt'
  "$bin" s3 get s3://notes/today.txt "$work/out.txt" >/dev/null
  grep -qx 'hello' "$work/out.txt" || fail "object body"
  "$bin" s3 list s3://notes | grep -q 'today.txt' || fail "list"
  "$bin" s3 delete s3://notes/today.txt >/dev/null
  "$bin" s3 remove bucket notes | grep -qx 'removed notes'

  export SQLX_ENGINE=s3
  export SQLX_RUNTIME=$work/runtime
  export SQLX_ROOT=
  "$bin" sql create database app >/dev/null
  schema=$work/schema
  printf 'model item\n  id integer primary key\n  name text not null\n' > "$schema"
  "$bin" sql app migrate "$schema" | grep -q 'created item'
  "$bin" sql app exec "insert into item (name) values ('kept')"
  rm -f "$SQLX_RUNTIME/app.sqlite" "$SQLX_RUNTIME/app.sqlite-wal" "$SQLX_RUNTIME/app.sqlite-shm"
  "$bin" sql app query "select name from item" | grep -qx 'kept' || fail "restore"
)

sqlbin=$work/sql
cp "$bin" "$sqlbin"
chmod 0755 "$sqlbin"
(
  unset SQLX_ENGINE S3X_ROOT S3X_SSH || true
  export SQLX_ROOT=$work/root
  log=$work/gdrive.log
  fake=$work/gdrive
  cat > "$fake" << EOF
#!/bin/sh
printf '%s\n' "\$*" >> "$log"
exit 0
EOF
  chmod 0755 "$fake"
  export SQLX_GDRIVE=$fake
  export SQLX_GDRIVE_PRESET=1
  export SQLX_DRIVE_PATH=sqlx
  mkdir -p "$SQLX_ROOT/backups/2020-01-01"

  help=$("$sqlbin" help)
  printf '%s\n' "$help" | grep -q 'sql app query' || fail "help missing query"
  printf '%s\n' "$help" | grep -q 'sql app inspect' || fail "help missing inspect"
  printf '%s\n' "$help" | grep -q 'sql app schema' || fail "help missing schema"
  printf '%s\n' "$help" | grep -q 'sql app migrate' || fail "help missing migrate"
  printf '%s\n' "$help" | grep -q 'model <name>' || fail "help missing model grammar"
  printf '%s\n' "$help" | grep -q 'sql backup' || fail "help missing backup"
  printf '%s\n' "$help" | grep -q 'sql create database app' || fail "help missing create"
  printf '%s\n' "$help" | grep -q 'commands:' && fail "help has a commands section" || true
  ver=$("$sqlbin" version)
  [ "$ver" = "0.1.0" ] || fail "version $ver"
  set +e
  "$sqlbin" create database >/dev/null 2>&1
  rc=$?
  set -e
  [ "$rc" -eq 2 ] || fail "bad create rc $rc"

  "$sqlbin" create database app >/dev/null
  schema=$work/schema
  printf 'model item\n  id integer primary key\n  name text not null\n' > "$schema"
  "$sqlbin" app migrate "$schema" | grep -q 'created item' || fail "model was not created"
  "$sqlbin" app query "insert into item (id, name) values (7, 'a')" >/dev/null
  "$sqlbin" app query "select id from item" | grep -qx '7' || fail "query did not return 7"
  set +e
  "$sqlbin" app query "drop table item" >"$work/drop.out" 2>"$work/drop.err"
  drc=$?
  set -e
  [ "$drc" -ne 0 ] || fail "schema sql was accepted"
  grep -q 'orm' "$work/drop.err" || fail "schema refusal did not name the orm"
  "$sqlbin" app query "select id from item" | grep -qx '7' || fail "row disappeared after a refused drop"
  inspect=$("$sqlbin" app inspect)
  printf '%s\n' "$inspect" | grep -q 'orm match' || fail "inspect missed the orm"
  printf '%s\n' "$inspect" | grep -q $'item\tid integer primary key, name text not null\t1' || fail "inspect row count drifted: $inspect"
  "$sqlbin" app schema | grep -q 'model item' || fail "schema did not print the model"
  /usr/bin/sqlite3 "$SQLX_ROOT/data/app.sqlite" "create table rogue (id integer);"
  set +e
  "$sqlbin" app query "select 1" >"$work/rogue.out" 2>"$work/rogue.err"
  rrc=$?
  set -e
  [ "$rrc" -ne 0 ] || fail "a table outside the orm was accepted"
  grep -q 'rogue' "$work/rogue.err" || fail "drift did not name rogue"

  "$sqlbin" backup >"$work/backup.out"
  today=$(date +%F)
  snap="$SQLX_ROOT/backups/$today/data/app.sqlite"
  [ -f "$snap" ] || fail "today's snapshot is missing"
  [ ! -d "$SQLX_ROOT/backups/2020-01-01" ] || fail "old snapshot was kept"
  id=$(/usr/bin/sqlite3 "$snap" "select id from item")
  [ "$id" = "7" ] || fail "snapshot row $id"
  grep -q '1 register ' "$log" || fail "backup did not register the directory"
  grep -q 'sync run' "$log" || fail "backup did not sync"
)

( cd "$root" && GOWORK=off go test ./... ) || fail "go tests failed"
echo PASS
