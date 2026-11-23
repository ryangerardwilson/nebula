# nebula Specification

## Purpose

Define the public nebula command: one binary whose features are sql, s3, run, and tunnel.
## Requirements
### Requirement: run hosts a site without migrating
`nebula run <directory>` SHALL build on the laptop when `nebula.toml` sets `build`, copy that directory to the SSH host, start `start` on `127.0.0.1` using `PORT`, and publish `hostname` only when that name is free. It MUST NOT open Postgres and MUST NOT run a migration. `nebula run rollback <name>` SHALL point the current release at the previous release and restart that process. It MUST NOT change the database. An edge config change MUST keep the ingress rule for `ssh.ryangerardwilson.com` and `127.0.0.1:8022`. The published hostname is the site origin. An OAuth callback is a path on `https://` plus that hostname. `nebula run` MUST NOT sign a user in and MUST NOT read a Google client.

#### Scenario: Rollback leaves the schema alone
- **GIVEN** a site with two releases
- **WHEN** the operator runs `nebula run rollback site`
- **THEN** the current release SHALL move to the earlier release
- **AND** the command MUST NOT send SQL

#### Scenario: The ssh ingress stays
- **GIVEN** an edge config that forwards `ssh.ryangerardwilson.com` to `127.0.0.1:8022` and then a catch-all
- **WHEN** a site hostname is inserted
- **THEN** the ssh rule SHALL be unchanged
- **AND** the new rule SHALL be before the catch-all

#### Scenario: The hostname is the callback origin
- **GIVEN** a site whose hostname is `app.ryangerardwilson.com`
- **WHEN** the operator publishes that site
- **THEN** the public origin SHALL be `https://app.ryangerardwilson.com`
- **AND** the command MUST NOT read a Google client

### Requirement: tunnel is the laptop publisher
`nebula tunnel` SHALL publish laptop ports as names under `ryangerardwilson.com`. Routes SHALL stay in the c3po sqlite record whose app is `tunnel`, kind is `routes`, and key is `routes`. Adding a name SHALL create a proxied CNAME only when that hostname has no DNS. A name that already points somewhere else MUST be left unchanged and MUST NOT be saved. Removing a route MUST NOT delete DNS. `nebula tunnel screen` SHALL open the route screen. `nebula tunnel open` MUST NOT be a command. `j` and `k` on that screen SHALL wrap the route list. On the last route, `j` SHALL move to the first route. On the first route, `k` SHALL move to the last route. An empty route list SHALL leave the cursor in place. While the shortcut list is open, `j` and `k` SHALL NOT move. Removing a route SHALL keep the cursor at the same index, clamped to the remaining routes.

#### Scenario: Foreign DNS is left alone
- **GIVEN** a hostname that already has DNS pointing somewhere else
- **WHEN** the operator runs `nebula tunnel add notes on 8080`
- **THEN** the command SHALL fail
- **AND** the route MUST NOT be saved

#### Scenario: The first list keeps the apex
- **GIVEN** no saved route list
- **WHEN** the list is created
- **THEN** it SHALL include `@` on port 8080

#### Scenario: j and k wrap the route list
- **WHEN** the route list has two routes and the cursor is on the last
- **AND** the operator presses `j`
- **THEN** the cursor SHALL be on the first route
- **AND** `k` SHALL move it to the last route

#### Scenario: Question mark blocks route motion
- **WHEN** the shortcut list is open
- **AND** the operator presses `j`
- **THEN** the cursor SHALL stay on the same route

### Requirement: crypt reads and writes 1Password
`nebula crypt` SHALL use the 1Password CLI and the service-account token. `nebula crypt list <vault>` SHALL print titles and Login usernames and MUST NOT print a password. `nebula crypt get <vault> <name>` SHALL copy one match with wl-copy when stdout is a terminal and SHALL print that secret when stdout is not a terminal. `nebula crypt describe <vault> <name>` SHALL print the title, id, category, and field labels and MUST NOT print a concealed value. `nebula crypt create` and `nebula crypt put` SHALL read a new secret from stdin and MUST NOT print it. `nebula crypt delete <vault> <name>` SHALL delete one exact title. `nebula crypt delete <vault> item <id>` SHALL delete that item id. `nebula crypt vaults` SHALL print vault names. `nebula crypt items <vault>` SHALL print item ids, titles, categories, and usernames and MUST NOT print a concealed value. `nebula crypt copy <vault> <id>` SHALL copy that item with wl-copy and MUST NOT print the secret. `nebula crypt load <vault>` SHALL, when stdout is not a terminal, print one `export TITLE='value'` line for each Login or API credential whose title is a shell identifier, sorted by title. A repeated title SHALL be exported once. A title that is not a shell identifier, and an item that is neither a Login nor an API credential, SHALL be skipped and its secret MUST NOT be read. The command MUST NOT print a secret on stderr. When stdout is a terminal, `nebula crypt load` SHALL fail before reading a secret and MUST NOT copy a secret. An empty vault, and a vault with no exportable title, SHALL fail and MUST leave stdout empty. Help SHALL name `nebula crypt load <vault>`. `nebula crypt` MUST NOT open an item screen. `r2d2 pass` SHALL reach 1Password only through these commands and MUST NOT call `op`. The argument forms `<vault> -l`, `<vault> <search>`, `--show`, `--create`, `--edit`, and `--delete` SHALL keep working.

#### Scenario: Items hide a concealed value
- **GIVEN** a listed item whose field value is concealed
- **WHEN** the operator runs `nebula crypt items`
- **THEN** the output names the item
- **AND** the output MUST NOT contain the concealed value

#### Scenario: Describe hides the secret
- **GIVEN** an API credential whose credential field is concealed
- **WHEN** the operator runs `nebula crypt describe`
- **THEN** the output names the credential field
- **AND** the output MUST NOT contain the secret

#### Scenario: A shell get prints the secret
- **GIVEN** one matching item
- **AND** stdout is not a terminal
- **WHEN** the operator runs `nebula crypt get`
- **THEN** the command SHALL print the secret

#### Scenario: A shell load exports the vault
- **GIVEN** a vault with API credentials titled `WIOM_SNOWFLAKE_ACCOUNT` and `WIOM_SNOWFLAKE_USER` and a Login whose title is an email
- **AND** stdout is not a terminal
- **WHEN** the operator runs `nebula crypt load` for that vault
- **THEN** stdout SHALL contain one export line for each API credential
- **AND** stdout MUST NOT export the email title
- **AND** stderr MUST NOT contain either secret

#### Scenario: A terminal load prints no secret
- **GIVEN** a vault with an exportable item
- **AND** stdout is a terminal
- **WHEN** the operator runs `nebula crypt load`
- **THEN** the command SHALL fail
- **AND** the output MUST NOT contain the secret
- **AND** the secret MUST NOT be copied

#### Scenario: An empty vault load fails closed
- **GIVEN** a vault with no items
- **AND** stdout is not a terminal
- **WHEN** the operator runs `nebula crypt load`
- **THEN** the command SHALL fail
- **AND** stdout MUST be empty

### Requirement: sql, s3, run, tunnel, and crypt are features of one binary
The module SHALL build one command, `nebula`. Invoking it as `sql`, or as `nebula sql`, SHALL run the database feature. Invoking it as `s3`, or as `nebula s3`, SHALL run the object feature. Invoking it as `run`, or as `nebula run`, SHALL run application hosting. Invoking it as `tunnel`, or as `nebula tunnel`, SHALL run the laptop tunnel. Invoking it as `crypt`, or as `nebula crypt`, SHALL run the 1Password secret feature. The repository MUST NOT publish a separate sqlx product. `nebula auth` MUST NOT be a command.

#### Scenario: Help names the features
- **GIVEN** the nebula binary
- **WHEN** the operator runs `nebula help`
- **THEN** the help MUST show `nebula sql`, `nebula s3`, `nebula run`, `nebula tunnel`, and `nebula crypt`
- **AND** the help MUST NOT show `nebula auth`
- **AND** it MUST NOT contain a `commands:` section

#### Scenario: argv0 selects the feature
- **GIVEN** the nebula binary is invoked with argv0 `sql`
- **WHEN** the operator passes `help`
- **THEN** the help MUST describe the database feature
- **AND** the same binary invoked as `s3` MUST describe the object feature
- **AND** the same binary invoked as `tunnel` MUST describe the laptop tunnel
- **AND** the same binary invoked as `run` MUST describe application hosting
- **AND** the same binary invoked as `crypt` MUST describe the 1Password secret feature

#### Scenario: auth is not a feature
- **WHEN** the operator runs `nebula auth`
- **THEN** the command SHALL exit 2

### Requirement: s3 stats one object
The `s3` package SHALL provide `Stat(ctx, bucket, key)` that returns the object's modification time and size. With `S3_ROOT` set, Stat SHALL stat the object file under that root. Without it, Stat SHALL run one command over SSH that stats `/var/lib/nebula/s3/<bucket>/<key>`, the path `Exists` tests, and SHALL honor the context. Stat MUST NOT read or download the object body. A missing object SHALL return `ErrNotFound`.

#### Scenario: A stored object reports its time and size
- **GIVEN** `S3_ROOT` holds `s3://notes/today.txt` with six bytes
- **WHEN** a caller runs `s3.Stat` on that object
- **THEN** the size SHALL be 6
- **AND** the time SHALL be the file's modification time

#### Scenario: A missing object is not found
- **GIVEN** `S3_ROOT` has no `s3://notes/gone.txt`
- **WHEN** a caller runs `s3.Stat` on that object
- **THEN** the error SHALL match `ErrNotFound`

