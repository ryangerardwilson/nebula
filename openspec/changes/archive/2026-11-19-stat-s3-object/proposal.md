## Why

A health check needs to know when a backup object was last written. `s3x.Object` carries a key and a size only, and the SSH listing has no mtime. Downloading the body to learn its age is slow and reads backup data the check does not need.

## What Changes

- `s3x.Stat(ctx, bucket, key)` returns the modification time and size of one object.
- On a local root (`S3X_ROOT`) it stats the object file.
- On the SSH host it runs one `stat` on the same remote path that `Exists` tests. It honors the context and does not read the body.
- A missing object returns `ErrNotFound`.

## Capabilities

### New Capabilities

### Modified Capabilities

- `nebula`: s3x can stat one object for its mtime and size.

## Impact

- Spec: `openspec/specs/nebula/spec.md`.
- Code: `s3x/`.
- Non-goals: `List`, the `nebula s3` CLI, and the SSH command used by other calls stay as they are.
