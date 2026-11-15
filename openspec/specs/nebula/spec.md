# nebula Specification

## Purpose

Define the public nebula command: one binary whose features are sql, s3, run, and tunnel.

## Requirements
### Requirement: sql, s3, run, and tunnel are features of one binary
The module SHALL build one command, `nebula`. Invoking it as `sql`, or as `nebula sql`, SHALL run the database feature. Invoking it as `s3`, or as `nebula s3`, SHALL run the object feature. Invoking it as `run`, or as `nebula run`, SHALL run application hosting. Invoking it as `tunnel`, or as `nebula tunnel`, SHALL run the laptop tunnel. The repository MUST NOT publish a separate sqlx product.

#### Scenario: Help names the features
- **GIVEN** the nebula binary
- **WHEN** the operator runs `nebula help`
- **THEN** the help MUST show `nebula sql`, `nebula s3`, `nebula run`, and `nebula tunnel`
- **AND** it MUST NOT contain a `commands:` section

#### Scenario: argv0 selects the feature
- **GIVEN** the nebula binary is invoked with argv0 `sql`
- **WHEN** the operator passes `help`
- **THEN** the help MUST describe the database feature
- **AND** the same binary invoked as `s3` MUST describe the object feature
- **AND** the same binary invoked as `tunnel` MUST describe the laptop tunnel
- **AND** the same binary invoked as `run` MUST describe application hosting

### Requirement: run hosts a site without migrating
`nebula run <directory>` SHALL build on the laptop when `nebula.toml` sets `build`, copy that directory to the SSH host, start `start` on `127.0.0.1` using `PORT`, and publish `hostname` only when that name is free. It MUST NOT open Postgres and MUST NOT run a migration. `nebula run rollback <name>` SHALL point the current release at the previous release and restart that process. It MUST NOT change the database. An edge config change MUST keep the ingress rule for `ssh.ryangerardwilson.com` and `127.0.0.1:8022`.

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

