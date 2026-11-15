# Add run and tunnel

## Why
Nebula can talk to Postgres and store objects, but a site still needs a hand-built unit and a hostname. The laptop Cloudflare tunnel still lives in c3po, while publishing names belongs on the same cloud surface as the host.

## What Changes
- `nebula run` builds a site on the laptop, copies it to the existing SSH host, and serves it on 127.0.0.1 behind one free hostname.
- `nebula tunnel` is the laptop tunnel. It keeps the c3po route record, the apex on port 8080, and the rule that foreign DNS is left alone.
- `run` does not open Postgres, run a migration, or replace the laptop connector.
- An edge edit on the host keeps the ssh ingress for `ssh.ryangerardwilson.com` and `127.0.0.1:8022`.

## Impact
Capabilities: nebula. Classification: tooling. No new cloud instance. Rollback of a site is the previous release directory. The laptop tunnel unit stays `tunnel.service`. No secret is copied into this change.
