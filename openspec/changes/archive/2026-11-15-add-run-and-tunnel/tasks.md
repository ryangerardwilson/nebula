## Tasks

- [x] Validate this change with `openspec validate add-run-and-tunnel --strict --no-interactive`
- [x] Move the laptop tunnel into nebula as `nebula tunnel` and keep the c3po route record
- [x] Add `nebula run` with tests that do not dial the host or Cloudflare
- [x] Wire both features into the binary, help, and install symlinks
- [x] Run `GOWORK=off go test ./...` and `bash tests/run.sh`
