# dasher

Lightweight Go backend that aggregates data from GitHub, Claude (Anthropic)
and Yr.no (MET Norway), and serves it to an ESP32 dashboard over an embedded
MQTT broker (mutual TLS, retained messages).

## Build

```sh
go build -o dasher .
# or for a Linux VM:
GOOS=linux GOARCH=amd64 go build -o dasher .
```

Single binary; the only runtime files needed are the YAML config and the TLS
certificate/key.

## Configuration

All configuration lives in a YAML file (default `config.yaml`, override with
`-config path/to/file.yaml`). Secret values may be written literally or as
`${ENV_VAR}` references, which are expanded from the environment at startup.

Secrets are best kept in a `.env` file in the working directory — dasher
loads it automatically at startup (`-env path` to override). Variables
already present in the process environment take precedence over `.env`.
Copy `.env.example` to `.env`, fill it in, and `chmod 600 .env`. The file
is git-ignored. On a systemd-managed VM, an `EnvironmentFile=` directive
in the unit achieves the same thing without the file living next to the
binary.
See `config.yaml` for the full annotated example:

```yaml
mqtt:
  listen_addr: ":8883"            # TLS listener address
  tls_cert: ${MQTT_TLS_CERT}       # server certificate (PEM path)
  tls_key: ${MQTT_TLS_KEY}         # server private key (PEM path)
  tls_client_ca: ${MQTT_CLIENT_CA} # CA that signs subscriber client certs (PEM path)
  topic: dashboard/data            # payload topic (retained)

github:
  token: ${GITHUB_TOKEN}       # classic PAT, notifications scope (fine-grained PATs don't work)
  interval: 5m

claude:
  refresh_token: ${CLAUDE_REFRESH_TOKEN}
  client_id: ${CLAUDE_CLIENT_ID}
  token_url: https://console.anthropic.com/v1/oauth/token
  url: https://api.anthropic.com/api/oauth/usage
  token_cache: claude-token.json
  interval: 5m

yr:
  lat: 59.9497                 # Norge/Oslo/Oslo/Nydalsdammen
  lon: 10.7665
  user_agent: "dasher/per.buer@gmail.com Norge/Oslo/Oslo/Nydalsdammen"
  interval: 15m

publish:
  interval: 30s              # heartbeat ceiling, not a fixed cadence
```

Durations use Go syntax (`30s`, `5m`, `1h`). Required values: `mqtt.tls_cert`,
`mqtt.tls_key`, `mqtt.tls_client_ca`, `github.token`, `claude.client_id`,
`claude.token_url`, `yr.lat`, `yr.lon`, `yr.user_agent`.

`claude.refresh_token` is required only until the token cache exists; after the
first rotation the cache is the authoritative copy.

The `yr.user_agent` must identify the app with a contact address —
MET Norway rejects placeholder User-Agents with `403`.

## Claude credentials

The usage endpoint authenticates with your Claude subscription's OAuth
credentials — the same ones Claude Code holds. **Copy the refresh token, not
the access token.** Access tokens expire within hours; a static one in `.env`
returns `401` forever after that. dasher stores the refresh token and mints
access tokens as it needs them.

Where Claude Code keeps credentials depends on the platform:

```sh
# macOS — Keychain
security find-generic-password -s "Claude Code-credentials" -w | jq -r '.claudeAiOauth.refreshToken'

# Linux — plain file
jq -r '.claudeAiOauth.refreshToken' ~/.claude/.credentials.json
```

Put that in `.env` as `CLAUDE_REFRESH_TOKEN=`. You also need Claude Code's
public OAuth client id in `CLAUDE_CLIENT_ID=`. At the time of writing it is
`9d1c250a-e61b-44d9-88ed-5944d1962f5e`; recover it from the CLI binary with:

```sh
strings "$(readlink -f "$(which claude)")" | grep -o 'CLIENT_ID:"[0-9a-f-]*"' | sort -u
```

Take `CLIENT_ID`, **not** `DESIGN_CLIENT_ID` — that is a different, also-valid
client, so picking it fails late and unhelpfully with `invalid_grant` rather
than "client not found". Don't try to guess by grepping for bare UUIDs; the
binary contains many, and the right one is not the most frequent.

### Refresh tokens rotate — give each machine its own login

Confirmed by testing: the token endpoint **consumes the refresh token on every
exchange** and issues a replacement. A grant therefore has exactly one valid
holder at a time.

**Do not copy one refresh token to a second machine.** The first process to
refresh invalidates it for everyone else — including Claude Code itself, which
logs out on the machine the token came from. Separate logins produce separate
grants that coexist happily, so run `claude auth login` on each host instead.

dasher writes each rotated credential to `claude.token_cache`
(`claude-token.json`, mode `0600`, gitignored, written via temp-file rename)
and prefers it over `claude.refresh_token` from then on. **Preserve that file
across deploys** — losing it means re-authenticating, since the seed in `.env`
is dead the moment the first rotation happens.

### User-Agent

Send an honest `User-Agent`. The OAuth token endpoint returns **404** to
requests claiming to be `claude-code/<version>`, so impersonating the CLI
breaks refresh; the usage endpoint serves any UA, so it gains nothing either.

The token endpoint and client id come from Claude Code's private OAuth flow
rather than a documented API — treat both as liable to change without notice.

## GitHub token

Create a **classic** PAT with only the `notifications` scope at
github.com/settings/tokens ([pre-filled link](https://github.com/settings/tokens/new?scopes=notifications&description=dasher)).
Fine-grained PATs do not work with the notifications endpoint. Put it in
`.env` as `GITHUB_TOKEN=ghp_...`.

## Run

With secrets in `.env` (the usual case), just:

```sh
./dasher
```

Or pass them explicitly:

```sh
export MQTT_TLS_CERT=cert.pem MQTT_TLS_KEY=key.pem MQTT_CLIENT_CA=certs/ca.crt \
       GITHUB_TOKEN=ghp_... CLAUDE_CLIENT_ID=... CLAUDE_REFRESH_TOKEN=sk-ant-ort01-...
./dasher
```

Add `-debug` while developing to print every published payload to stdout
instead of publishing it, along with per-request timing. In this mode the
MQTT broker is not started at all, so no certificates or `mqtt.tls_*` values
are needed. Never run with `-debug` in production.

```sh
./dasher -debug
```

```
[yr] -> GET api.met.no/weatherapi/locationforecast/2.0/compact
[claude] access token valid for another 7h23m0s
[claude] -> GET api.anthropic.com/api/oauth/usage
[yr] <- api.met.no/weatherapi/locationforecast/2.0/compact 200 OK in 98ms
[yr] fetch ok in 100ms
[claude] <- api.anthropic.com/api/oauth/usage 200 OK in 477ms
[claude] fetch ok in 477ms
[publisher] changed: 838 bytes
{ ...payload... }
```

Only host and path are logged, never query strings or headers. Without
`-debug` the service is quiet: fetch failures (with their elapsed time, so a
timeout is distinguishable from a rejection) and token refreshes only.

### When payloads are published

Every successful fetch is published immediately, so a restart fills the
dashboard within about a second instead of leaving it blank. Sources that
land close together coalesce into one message. `publish.interval` is the
**heartbeat ceiling** — if nothing has been fetched in that long, the
retained message is republished anyway. In `-debug` each publish is tagged
`changed`, `heartbeat` or `shutdown`.

Note the trigger is a completed fetch, not a changed value: a fetch that
returns identical numbers still bumps `updated_at` and still publishes.

## Deploying to a Linux host

```sh
GOOS=linux GOARCH=amd64 go build -o dasher .
```

Copy `dasher`, `config.yaml`, `.env` and the certs to the host. **Do not copy
`claude-token.json`** — see below.

### Authenticate on the host itself

Refresh tokens rotate, so the credential cannot be shared with your laptop.
Log in on the host to get a grant of its own:

```sh
claude auth login --claudeai   # prints a URL if there is no browser
jq -r '.claudeAiOauth.refreshToken' ~/.claude/.credentials.json
```

Put that in the host's `.env` as `CLAUDE_REFRESH_TOKEN=`, alongside
`CLAUDE_CLIENT_ID=`. On the first refresh dasher writes `claude-token.json`
and stops consulting `.env` for the token.

Copying the laptop's token instead "works" until the first refresh, then the
two hosts start invalidating each other's credential at random — and log you
out of Claude Code wherever the token came from.

### Preserve the token cache across deploys

`claude-token.json` holds the only live credential once the first rotation has
happened. A deploy that wipes the working directory forces a fresh
`claude auth login`. Keep it outside the deploy path and point at it:

```yaml
claude:
  token_cache: /var/lib/dasher/claude-token.json
```

The file is written with mode `0600` via temp-file rename, so an interrupted
run cannot truncate it. dasher only overwrites it on a successful exchange.

### systemd

```ini
[Unit]
Description=dasher dashboard aggregator
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=dasher
WorkingDirectory=/opt/dasher
EnvironmentFile=/etc/dasher/dasher.env
ExecStart=/opt/dasher/dasher -config /etc/dasher/config.yaml
Restart=on-failure
RestartSec=10s
StateDirectory=dasher          # /var/lib/dasher for the token cache
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

`EnvironmentFile=` replaces `.env` (`chmod 600`, owned by root). Under
`ProtectSystem=strict` the filesystem is read-only apart from
`StateDirectory`, so `claude.token_cache` must live in `/var/lib/dasher` or
refresh fails with a write error.

### Checking it works

```sh
journalctl -u dasher -f
```

A healthy start logs three `fetcher started` lines and nothing else. Fetch
errors are logged and retried on the next tick — they never take the service
down, so watch the log rather than the exit code. Refresh happens five minutes
before expiry, roughly every six hours; `token endpoint returned 400` there
means the grant was invalidated elsewhere and the host needs a fresh login.

## MQTT auth model

Publishing is internal-only: `dasher` writes to the topic via the broker's
inline client, an in-process call that never touches the network listener
or any auth check. The TLS listener therefore only has one job: gatekeeping
*subscribers*. It requires mutual TLS — every connecting client must
present a certificate signed by `mqtt.tls_client_ca` — and the ACL
restricts every connection to read-only on `dashboard/data`; everything
else (publish, any other topic) is explicitly denied.

Manage the client CA and issue device certs with:

```sh
scripts/mqtt-client-ca.sh init          # once: creates certs/ca.key + certs/ca.crt
scripts/mqtt-client-ca.sh issue esp32   # per device: certs/esp32.key + certs/esp32.crt
```

`certs/ca.crt` is what `mqtt.tls_client_ca` points at. Everything under
`certs/` is git-ignored — `ca.key` in particular must stay private, since
anyone holding it can mint a valid subscriber cert.

ESP32 client: connect with mTLS on port 8883 (server cert + `certs/esp32.crt`/
`certs/esp32.key`), subscribe to `dashboard/data`; the retained message
arrives immediately on subscribe.

## Payload

Published to `dashboard/data` with `retain=true`, QoS 1:

```json
{
  "updated_at": "2026-08-05T17:54:25Z",
  "github": { "unread_notifications": 3 },
  "claude": {
    "five_hour_pct": 3,
    "five_hour_resets_at": "2026-08-07T22:30:00Z",
    "seven_day_pct": 48,
    "seven_day_resets_at": "2026-08-09T15:00:00Z"
  },
  "weather": [
    { "time": "2026-08-05T18:00:00Z", "temperature_c": 17.6, "precipitation_mm": 0.9, "symbol": "rain" }
  ]
}
```

`github`, `claude` and `weather` are omitted until the first successful
fetch of each source.

The usage endpoint returns roughly 1.8 KB — per-model breakdowns, spend, and
a number of unreleased feature flags. Only the four fields above are published
(128 bytes), to keep the ESP32's parse cheap. Dollar figures are deliberately
excluded: they are null on Team plans, so utilization is the only measure that
works across subscription types. If the endpoint renames its reset-time fields
the fetcher logs an error rather than publishing a misleading `0`.

## Notes

- The broker library is `github.com/mochi-mqtt/server/v2` — the renamed,
  maintained continuation of `mochi-co/mqtt/v2` (same project, v2.7.x).
- The Claude usage and token endpoints are part of Claude Code's private OAuth
  flow, not a documented public API. They work, but nothing guarantees they
  keep working; a schema change surfaces as a logged fetch error, never a
  crash.
- `claude setup-token` is **not** a shortcut here. The long-lived token it
  mints is scoped for inference and gets `403` from the usage endpoint.

## Layout

```
main.go                  wiring, signal handling
config.yaml              example configuration
internal/config/         YAML configuration
internal/broker/         embedded MQTT broker (mTLS + ACL)
internal/state/          shared aggregated payload
internal/fetchers/       github / claude / yr background tickers
internal/publisher/      periodic retained publish to the topic
```
