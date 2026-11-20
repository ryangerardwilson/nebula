## ADDED Requirements

### Requirement: s3x stats one object
The `s3x` package SHALL provide `Stat(ctx, bucket, key)` that returns the object's modification time and size. With `S3X_ROOT` set, Stat SHALL stat the object file under that root. Without it, Stat SHALL run one `stat` over SSH on `/var/lib/nebula/s3x/<bucket>/<key>`, the path `Exists` tests, and SHALL honor the context. Stat MUST NOT read or download the object body. A missing object SHALL return `ErrNotFound`. `List`, the `nebula s3` command, and the SSH command used by other calls SHALL NOT change.

#### Scenario: A stored object reports its time and size
- **GIVEN** `S3X_ROOT` holds `s3://notes/today.txt` with six bytes
- **WHEN** a caller runs `s3x.Stat` on that object
- **THEN** the size SHALL be 6
- **AND** the time SHALL be the file's modification time

#### Scenario: A missing object is not found
- **GIVEN** `S3X_ROOT` has no `s3://notes/gone.txt`
- **WHEN** a caller runs `s3x.Stat` on that object
- **THEN** the error SHALL match `ErrNotFound`
