# Design

## Two connectors
The laptop connector publishes names that already point at this machine, including the apex. The host connector on del-vc2 is the always-on edge for SSH and for `nebula run`. They stay separate processes. `nebula tunnel` does not edit the host connector. `nebula run` does not edit the laptop connector.

## run
`nebula.toml` names the app, the laptop build, the start command, and the hostname. The build runs locally. The release directory is `/var/lib/nebula/apps/<name>/releases/<id>`, and `current` is a symlink. The unit `nebula-<name>.service` sets `HOST=127.0.0.1` and `PORT` from 18100 through 18199. The app binds that address itself.

DNS is created only when the hostname has no records. The apex, the ssh name, and the existing mail and site names are refused before any write. The host cloudflared config gains one ingress rule before the catch-all. The ssh rule is compared before and after the edit and the write is refused if it changed. The connector restart is a detached system unit so a dropped SSH session does not kill the restore path. If the restart fails, the previous config is put back.

`rollback` points `current` at the previous release and restarts the unit. It does not send SQL and does not change ingress.

## tunnel
The command words stay help, version, start, stop, status, logs, run, list, add, and remove. `screen` opens the route editor. `open` is not a command. Routes stay in the c3po sqlite record `tunnel` / `routes` / `routes`. Config stays under `~/.config/c3po/tunnel`. The user unit name stays `tunnel.service`.

## Non-goals
No second Vultr instance. No provider API. No database migration inside `run`. No merge of the two connectors.
