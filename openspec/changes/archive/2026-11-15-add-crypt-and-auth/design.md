# crypt and auth

## Boundary
`crypt` is the 1Password interface. A 1Password vault name, such as Root, stays an argument. `auth` asks crypt for the two Root items and keeps only the client id in `~/.config/nebula/auth`. A product session file is that product's Google token cache. Application rows stay in the product.

Google's consumer client cannot be created or edited through a public API. The operator adds each https redirect on the existing client. A loopback redirect can use any port when the client allows it.

## Commands
`crypt list <vault>`, `crypt get <vault> <name>`, `crypt describe <vault> <name>`, `crypt create <vault> <name>`, `crypt create <vault> <name> login <username>`, `crypt create <vault> template <file>`, `crypt put <vault> <name>`, `crypt delete <vault> <name>`, `crypt delete <vault> item <id>`, `crypt vaults`, `crypt items <vault>`, `crypt copy <vault> <id>`. There is no crypt screen. `c3po tui pass` stays in c3po and reaches 1Password through `vaults`, `items`, `copy`, and `delete <vault> item <id>`. It does not call `op`.

`auth setup`, `auth add <name> redirect <url>`, `auth add <name> scope <scope>`, `auth remove <name>`, `auth list`, `auth url <name>`, `auth login <name>`, `auth exchange <name> <code> <state>`.

## Failure
A missing token, a missing item, a state mismatch, or a Google error stops the command. Errors do not include the secret or the token. Files under the auth directory are mode `0600`. `NEBULA_AUTH_ROOT` overrides that directory for tests. Tests do not call 1Password, Google, or SSH.
