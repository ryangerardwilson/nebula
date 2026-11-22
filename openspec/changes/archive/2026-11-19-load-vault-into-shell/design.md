## Context

`nebula crypt get` of a glob already prints `export TITLE='value'` lines when stdout is a pipe and every selected title is a shell identifier. A child process cannot change the parent shell, so the load is `eval` of those lines. One exact get prints the raw secret. `list` prints titles only.

## Goals / Non-Goals

**Goals:**

- `eval "$(nebula crypt load <vault>)"` exports every Login and API credential whose title is a shell identifier.
- Secrets stay off a terminal and off the clipboard for this command.
- Non-exportable items are not read.
- Export order is the sorted title. A repeated title exports once, the first item in the vault list.

**Non-Goals:**

- Changing wiomctl, geniectl, or monoctl to call nebula crypt.
- A pass screen that dumps a vault.
- Putting exports in `.bashrc`.
- Exporting Secure Notes, SSH keys, or titles that are not shell identifiers.

## Decisions

- The word `load` is reserved, like `list` and `get`. It translates to the runner flag `--load`.
- A terminal is refused before the token is used and before any secret is read. The error tells the operator to eval the command.
- Login uses the password field and API credential uses the credential field, the same read as get.
- A failed read of an exportable item fails the command and prints no export lines.
- Skipped items produce one stderr count and no titles or secrets.
- An empty vault fails. A vault whose items are all skipped fails. Stdout stays empty so eval does not run an error sentence.

## Risks / Trade-offs

- A Login whose title is a shell identifier exports its password under that name. That matches a glob get of the same title.
- A Secure Note or SSH key with a shell title is skipped, not exported.
- `eval` does not keep the command's exit status. A failed load leaves the shell unchanged when stdout is empty.

## Migration Plan

- No stored data changes. Existing globs keep working.
