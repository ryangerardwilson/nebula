## REMOVED Requirements

### Requirement: sql, s3, run, and tunnel are features of one binary

## ADDED Requirements

### Requirement: sql, s3, run, tunnel, crypt, and auth are features of one binary
The module SHALL build one command, `nebula`. Invoking it as `sql`, or as `nebula sql`, SHALL run the database feature. Invoking it as `s3`, or as `nebula s3`, SHALL run the object feature. Invoking it as `run`, or as `nebula run`, SHALL run application hosting. Invoking it as `tunnel`, or as `nebula tunnel`, SHALL run the laptop tunnel. Invoking it as `crypt`, or as `nebula crypt`, SHALL run the 1Password secret feature. Invoking it as `auth`, or as `nebula auth`, SHALL run the shared Google sign-in. The repository MUST NOT publish a separate sqlx product.

#### Scenario: Help names the features
- **GIVEN** the nebula binary
- **WHEN** the operator runs `nebula help`
- **THEN** the help MUST show `nebula sql`, `nebula s3`, `nebula run`, `nebula tunnel`, `nebula crypt`, and `nebula auth`
- **AND** it MUST NOT contain a `commands:` section

#### Scenario: argv0 selects the feature
- **GIVEN** the nebula binary is invoked with argv0 `sql`
- **WHEN** the operator passes `help`
- **THEN** the help MUST describe the database feature
- **AND** the same binary invoked as `s3` MUST describe the object feature
- **AND** the same binary invoked as `tunnel` MUST describe the laptop tunnel
- **AND** the same binary invoked as `run` MUST describe application hosting
- **AND** the same binary invoked as `crypt` MUST describe the 1Password secret feature
- **AND** the same binary invoked as `auth` MUST describe the shared Google sign-in

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

### Requirement: auth signs in with the Root Google client
`nebula auth setup` SHALL read Root items `GOOGLE_OAUTH_CLIENT_ID` and `GOOGLE_OAUTH_CLIENT_SECRET` through crypt and MUST NOT write the client secret. `nebula auth add <name> redirect <url>` SHALL record that redirect for the product. `nebula auth login <name>` on a `127.0.0.1` redirect SHALL exchange the code and write a session file for that product. The command output MUST NOT contain the access token, the refresh token, or the client secret. `nebula auth` MUST NOT open Postgres, MUST NOT create an application user table, and MUST NOT create or edit the Google client.

#### Scenario: Setup keeps the secret out of the client file
- **GIVEN** a client id and a client secret in the Root items
- **WHEN** the operator runs `nebula auth setup`
- **THEN** the saved client file contains the client id
- **AND** the saved client file MUST NOT contain the client secret

#### Scenario: Login does not print the token
- **GIVEN** a product whose redirect is on `127.0.0.1`
- **WHEN** login completes
- **THEN** the command reports that the product is signed in
- **AND** the output MUST NOT contain the access token
