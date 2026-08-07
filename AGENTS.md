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

## Gotchas (learned the hard way)

- The broker module was renamed: use `github.com/mochi-mqtt/server/v2`, NOT
  `mochi-co/mqtt/v2` (the old path no longer resolves). TLS is configured by
  passing a `TLSConfig` to `listeners.NewTCP` — there is no `NewTLS`.
- GitHub `/notifications` requires a **classic** PAT with the `notifications`
  scope; fine-grained PATs are rejected by the endpoint. `per_page` maxes at
  50, so the unread count saturates at 50.
- MET Norway (Yr.no) 403s placeholder User-Agents. `yr.user_agent` must
  contain a real contact, e.g. `dasher/per.buer@gmail.com <location>`.
- The Claude document is passed through as opaque `json.RawMessage`; the
  usage endpoint's schema is not a stable contract, so don't model it.
- Verify changes by running the service against a self-signed cert and a
  real MQTT client, not just by compiling.
