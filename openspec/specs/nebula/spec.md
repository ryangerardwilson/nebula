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
`nebula tunnel` SHALL publish laptop ports as names under `ryangerardwilson.com`. Routes SHALL stay in the c3po sqlite record whose app is `tunnel`, kind is `routes`, and key is `routes`. Adding a name SHALL create a proxied CNAME only when that hostname has no DNS. A name that already points somewhere else MUST be left unchanged and MUST NOT be saved. Removing a route MUST NOT delete DNS. `nebula tunnel screen` SHALL open the route screen. `nebula tunnel open` MUST NOT be a command.

#### Scenario: Foreign DNS is left alone
- **GIVEN** a hostname that already has DNS pointing somewhere else
- **WHEN** the operator runs `nebula tunnel add notes on 8080`
- **THEN** the command SHALL fail
- **AND** the route MUST NOT be saved

#### Scenario: The first list keeps the apex
- **GIVEN** no saved route list
- **WHEN** the list is created
- **THEN** it SHALL include `@` on port 8080

### Requirement: crypt reads and writes 1Password
`nebula crypt` SHALL use the 1Password CLI and the service-account token. `nebula crypt list <vault>` SHALL print titles and Login usernames and MUST NOT print a password. `nebula crypt get <vault> <name>` SHALL copy one match with wl-copy when stdout is a terminal and SHALL print that secret when stdout is not a terminal. `nebula crypt describe <vault> <name>` SHALL print the title, id, category, and field labels and MUST NOT print a concealed value. `nebula crypt create` and `nebula crypt put` SHALL read a new secret from stdin and MUST NOT print it. `nebula crypt delete <vault> <name>` SHALL delete one exact title. `nebula crypt delete <vault> item <id>` SHALL delete that item id. `nebula crypt vaults` SHALL print vault names. `nebula crypt items <vault>` SHALL print item ids, titles, categories, and usernames and MUST NOT print a concealed value. `nebula crypt copy <vault> <id>` SHALL copy that item with wl-copy and MUST NOT print the secret. `nebula crypt` MUST NOT open an item screen. `c3po tui pass` SHALL reach 1Password only through these commands and MUST NOT call `op`. The argument forms `<vault> -l`, `<vault> <search>`, `--show`, `--create`, `--edit`, and `--delete` SHALL keep working.

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

