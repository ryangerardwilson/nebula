# tunnel-session Specification

## Purpose
TBD - created by archiving change keep-tunnel-session-through-c3po. Update Purpose after archive.
## Requirements
### Requirement: The tunnel screen keeps its session through c3po
The tunnel screen SHALL keep its resumable session through `c3po __session` and SHALL NOT write c3po's session tables. Opening SHALL run `c3po __session open tunnel <pid> [<id>]` and take the session id and any restored place from its output. Each change of place SHALL run `place` or `note` for that id and process, and closing SHALL run `close`. A failed c3po call SHALL be reported. A bad `--resume` id SHALL fail before c3po runs.

#### Scenario: A tunnel session round trip
- **WHEN** the tunnel screen opens, places its cursor, notes its args, and closes
- **THEN** it SHALL run c3po `open`, `place`, `note`, and `close` in that order for one session id

#### Scenario: c3po refuses the session
- **WHEN** `c3po __session open` fails because the session was not found
- **THEN** opening SHALL fail with c3po's message

### Requirement: A record write keeps its row
Writing a record that exists SHALL update its body and keep its row id. Writing a record that does not exist SHALL insert it.

#### Scenario: A second write keeps the id
- **WHEN** the tunnel routes record is written twice
- **THEN** one row SHALL remain with the same id

