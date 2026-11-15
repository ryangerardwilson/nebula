#!/usr/bin/env sh
set -eu

install_from_source() {
  checkout=$1
  if [ ! -x /usr/bin/sqlite3 ]; then
    sudo pacman -S --needed --noconfirm sqlite
  fi
  dest=${HOME}/.local/bin
  unit_dir=${HOME}/.config/systemd/user
  mkdir -p "$dest" "$unit_dir"
  staged=${dest}/.nebula.tmp.$$
  (
    cd "$checkout"
    GOWORK=off CGO_ENABLED=0 go build -o "$staged" ./cmd/nebula
  )
  chmod 0755 "$staged"
  mv "$staged" "$dest/nebula"
  ln -sfn nebula "$dest/sql"
  ln -sfn nebula "$dest/s3"
  ln -sfn nebula "$dest/run"
  ln -sfn nebula "$dest/tunnel"
  cp "$checkout/sqlx-backup.service" "$unit_dir/sqlx-backup.service"
  cp "$checkout/sqlx-backup.timer" "$unit_dir/sqlx-backup.timer"
  systemctl --user daemon-reload
  loginctl enable-linger "$USER" >/dev/null 2>&1 || true
  systemctl --user enable --now sqlx-backup.timer
}

if [ "$#" -eq 2 ] && [ "$1" = "from" ] && [ -d "$2" ]; then
  install_from_source "$2"
  exit 0
fi

echo "usage: ./install.sh from <source-checkout>" >&2
exit 1
