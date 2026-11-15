## REMOVED Requirements

### Requirement: sql, s3, run, tunnel, crypt, and auth are features of one binary

### Requirement: auth signs in with the Root Google client

## MODIFIED Requirements

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

## ADDED Requirements

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
