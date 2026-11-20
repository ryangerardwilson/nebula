## MODIFIED Requirements

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
