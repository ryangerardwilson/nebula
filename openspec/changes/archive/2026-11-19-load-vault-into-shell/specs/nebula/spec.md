## MODIFIED Requirements

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
