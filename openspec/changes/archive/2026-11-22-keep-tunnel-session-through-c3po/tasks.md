## 1. Keep the session through c3po

- [x] 1.1 Run `c3po __session` for open, note, place, and close; stop writing c3po's tables
- [x] 1.2 Update a record in place before inserting it

## 2. Check

- [x] 2.1 Test the c3po calls and a refused open with a stubbed runner
- [x] 2.2 Run `GOWORK=off go test ./tunnel/...`
- [x] 2.3 Validate with `openspec validate keep-tunnel-session-through-c3po --strict --no-interactive`
