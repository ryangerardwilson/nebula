# Drop auth

`nebula run` chooses the public hostname and publishes it. An OAuth callback is `https://` plus that hostname plus a path the site serves. A separate feature that records redirects and exchanges codes duplicates that origin, and it still cannot create the Google client.

`nebula auth` becomes an unknown command and exits 2. Install removes `~/.local/bin/auth` when that path is a symlink to `nebula`.

`crypt` stays the 1Password interface. The Root Google items stay there for a site that reads them itself. `run` does not call `crypt` for those items, does not open Postgres, and does not store a session.
