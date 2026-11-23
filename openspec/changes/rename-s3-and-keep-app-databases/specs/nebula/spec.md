## ADDED Requirements

### Requirement: s3 is one SSH command per operation
Over SSH, each `s3` operation (`make bucket`, `put`, `get`, `list`, `delete`, `copy`, `move`, `stat`) SHALL run exactly one remote command. `put` SHALL check that the bucket exists, write a temporary file with mode 600, and rename it onto the key in that one command; a missing bucket SHALL return `ErrNotFound`. Objects SHALL be written and read under `/var/lib/nebula/s3`. `delete` SHALL remove the object from that directory. The `find` format SHALL survive the remote login shell, so `nebula s3 list` and `nebula s3 list s3://<bucket>` print every bucket and object. `S3_ROOT` and `S3_SSH` SHALL name the local directory and the SSH host.

#### Scenario: A put is one command
- **GIVEN** the bucket `backups` exists on the host
- **WHEN** a caller puts a file to `s3://backups/c3po.sqlite`
- **THEN** exactly one SSH command SHALL run
- **AND** the object SHALL be under `/var/lib/nebula/s3/backups/`

#### Scenario: List survives the remote shell
- **GIVEN** a bucket holds two objects on the host
- **WHEN** the operator runs `nebula s3 list s3://<bucket>`
- **THEN** both objects SHALL be printed with their sizes

### Requirement: keep backs up each application database
`nebula keep mark <name> <path>` SHALL record that the database changed and SHALL return without network I/O. It SHALL start a detached worker for `<name>` unless one is running. The worker SHALL upload a consistent snapshot of the file, not the live file, to `s3://backups/<name>.sqlite`, SHALL coalesce marks that arrive while it uploads into one later upload, SHALL make the bucket only when a put finds it missing, SHALL retry a failed upload after 5 s, 30 s, 2 min, and 10 min and then exit, and SHALL hold `<path>.publish.lock` from the snapshot until the upload returns. It SHALL NOT upload a database in which no table other than `schema_migration`, `app_once`, or `sqlx_orm` has a row, and SHALL NOT upload a file without a restore marker. `nebula keep adopt <name> <path>` SHALL write that marker for a file that holds data, and SHALL refuse a file that holds none. `nebula keep restore <name> <path>` SHALL leave a present file alone without network I/O; for a missing file it SHALL download the copy and write a `restored` marker, or create an empty file with a `fresh` marker when no copy exists, and SHALL fail and create no file when the copy is unreachable. `nebula keep status [--json]` SHALL show each database as `ok`, `waiting`, or `failing` with the last good upload, the time the copy started lagging, and the last error. `nebula keep flush [--timeout <d>]` SHALL upload every lagging database and exit 1 when one is still lagging. `nebula keep sweep` SHALL mark a database whose file changed since its last upload. `nebula-keep.timer` SHALL run the sweep every 10 minutes, and `nebula-keep-flush.service` SHALL run the flush when the user session stops. keep MUST NOT run application SQL or hold application schema.

#### Scenario: A mark returns before the upload
- **GIVEN** the backup host is slow
- **WHEN** an application commits and runs `nebula keep mark <name> <path>`
- **THEN** the command SHALL return in milliseconds
- **AND** the copy SHALL be uploaded later by the worker

#### Scenario: A burst of marks is one upload
- **WHEN** five marks arrive before the worker uploads
- **THEN** the worker SHALL upload once
- **AND** the upload SHALL hold the data of all five commits

#### Scenario: An empty database never replaces the backup
- **GIVEN** keep holds a copy with data
- **WHEN** the runtime file is emptied and marked
- **THEN** the copy SHALL NOT be replaced
- **AND** status SHALL show the database `failing` with the reason

#### Scenario: An unreachable backup makes no file
- **GIVEN** the runtime file is missing and the host is unreachable
- **WHEN** `nebula keep restore <name> <path>` runs
- **THEN** it SHALL fail
- **AND** the runtime file SHALL stay missing

#### Scenario: An unrestored file waits for the operator
- **GIVEN** keep holds a copy with data and the runtime file did not come from `nebula keep restore`
- **WHEN** the file is marked
- **THEN** the copy SHALL NOT be replaced
- **AND** status SHALL name `nebula keep adopt`

#### Scenario: A failed upload stays visible
- **GIVEN** every upload fails
- **WHEN** the worker has used its retries
- **THEN** status SHALL show the database `failing` with the last error and the time it started lagging
