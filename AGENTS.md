# AGENTS.md

Guidance for AI agents working in this repository.

## What this is

`dasher` — a single-binary Go service that aggregates GitHub notifications,
Claude usage metrics, and Yr.no (MET Norway) weather, then publishes the
result as retained JSON on an embedded MQTT broker (mutual TLS, read-only
ACL on the data topic) for an ESP32 dashboard. Publishing is internal-only
via the broker's inline client. No tests exist yet; verification is done by
building and running the binary.

## Commands

```sh
go build -o dasher .                     # build
GOOS=linux GOARCH=amd64 go build -o dasher .   # deploy target
go vet ./...                             # lint
gofmt -w .                               # format (must be clean)
./dasher                                 # run (needs config.yaml + secrets)
./dasher -debug                          # dev: payloads to stdout, MQTT broker disabled (no TLS/certs needed)
./dasher -config other.yaml -env prod.env     # override paths
```

## Conventions

- Standard library first. Only two external deps are acceptable in spirit:
  `github.com/mochi-mqtt/server/v2` (broker) and `gopkg.in/yaml.v3` (config).
  Do not add dependencies without a clear need.
- Configuration lives in `config.yaml`; secrets come from `.env`
  (auto-loaded, real env wins) via `${VAR}` expansion in the YAML. Never
  hardcode secrets, and never commit `.env` (see `.gitignore`).
- Fetchers live in `internal/fetchers/`, one file per source, and share the
  `runLoop` ticker helper. Fetch errors are logged and retried on the next
  tick — never fatal.
- Fetched data goes into `internal/state.Store` (mutex-guarded); the
  publisher snapshots it. Keep payloads small; fields use `omitempty` until
  their first successful fetch.
- Match existing style: short doc comments on packages/exports, no test
  scaffolding unless asked.
- Commit straight to `main` when asked to commit. This is a solo project with
  no review flow — do not create a feature branch or open a PR for it. Still
  commit only when asked, and never push unless asked.

## Gotchas (learned the hard way)

- The broker module was renamed: use `github.com/mochi-mqtt/server/v2`, NOT
  `mochi-co/mqtt/v2` (the old path no longer resolves). TLS is configured by
  passing a `TLSConfig` to `listeners.NewTCP` — there is no `NewTLS`.
- GitHub `/notifications` requires a **classic** PAT with the `notifications`
  scope; fine-grained PATs are rejected by the endpoint. `per_page` maxes at
  50, so the unread count saturates at 50.
- MET Norway (Yr.no) 403s placeholder User-Agents. `yr.user_agent` must
  contain a real contact, e.g. `dasher/per.buer@gmail.com <location>`.
- The Claude usage endpoint returns ~1.8KB; only the four fields the display
  needs are projected into `state.ClaudeData` (~128 bytes). Model no more than
  that — the rest of the schema is not a stable contract. Use `utilization`,
  not the dollar fields, which are null on Team plans.
- Claude OAuth **access** tokens expire within hours — a static one in `.env`
  yields a permanent `401`. The fetcher holds a **refresh** token and mints
  access tokens itself, persisting rotated credentials to `claude.token_cache`.
  That cache outranks `claude.refresh_token` once written; don't "fix" a stale
  login by editing `.env` alone.
- Refresh tokens **rotate**: each exchange consumes the old one. A grant has a
  single valid holder, so never share one between machines (or with Claude Code
  itself) — it logs the other holder out. One `claude auth login` per host.
- Use an honest `User-Agent`. The token endpoint 404s on `claude-code/<ver>`;
  the usage endpoint accepts anything. `claudeUserAgent` in `claude.go`.
- Claude Code's OAuth constants live in the CLI bundle as `CLIENT_ID` (the one
  you want) and `DESIGN_CLIENT_ID`. Both are valid clients, so a wrong pick
  fails late with `invalid_grant` rather than "client not found" — grep the
  binary for `CLIENT_ID:"` rather than guessing from a bare UUID list.
- Verify changes by running the service against a self-signed cert and a
  real MQTT client, not just by compiling.
