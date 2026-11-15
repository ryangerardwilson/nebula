# Drop auth

## Problem

`nebula auth` keeps a second list of Google redirects and a sign-in flow. A site published by `nebula run` already has its public hostname, so the callback is a path on that origin.

## Outcome

`auth` is not a nebula feature. `nebula run` publishes the hostname, and that hostname is the site origin. The Google client stays in 1Password. `run` does not read it and does not sign a user in.

## Scope

- Remove the auth package, the argv0 dispatch, the help, and the install link.
- Say in `run` that the published hostname is the callback origin.

## Non-goals

- Do not add a sign-in command to `run`.
- Do not create or edit the Google client.
- Do not change `crypt`.
