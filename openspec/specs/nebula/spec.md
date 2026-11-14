# nebula Specification

## Purpose

Define the public nebula command: one binary whose features are sql and s3.

## Requirements

### Requirement: sql and s3 are features of one binary

The module SHALL build one command, `nebula`. Invoking it as `sql`, or as `nebula sql`, SHALL run the database feature. Invoking it as `s3`, or as `nebula s3`, SHALL run the object feature. The repository MUST NOT publish a separate sqlx product.

#### Scenario: Help names both features

- **GIVEN** the nebula binary
- **WHEN** the operator runs `nebula help`
- **THEN** the help MUST show `nebula sql` and `nebula s3`
- **AND** it MUST NOT contain a `commands:` section

#### Scenario: argv0 selects the feature

- **GIVEN** the nebula binary is invoked with argv0 `sql`
- **WHEN** the operator passes `help`
- **THEN** the help MUST describe the database feature
- **AND** the same binary invoked as `s3` MUST describe the object feature
