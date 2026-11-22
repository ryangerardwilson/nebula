## Why

Loading one vault into the current shell takes a glob per name prefix. `eval "$(nebula crypt Wiom 'WIOM_SNOWFLAKE_*')"` loads one group. The operator needs one command that exports every shell-named item in the vault.

## What Changes

- `nebula crypt load <vault>` prints `export` lines for eval when stdout is not a terminal.
- A terminal invocation fails and does not print or copy secrets.
- Titles that are not shell identifiers, and items that are not a Login or an API credential, are skipped and their secrets are not read.
- Help names the command. Existing get, list, and glob behavior stays.

## Capabilities

### New Capabilities

### Modified Capabilities

- `nebula`: crypt can load a vault into the shell as export lines.

## Impact

- Spec: `openspec/specs/nebula/spec.md`.
- Code: `crypt/`.
- `r2d2 pass` does not gain a screen. wiomctl still reads the process environment and does not call nebula crypt.
