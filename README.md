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
  token: ${CLAUDE_TOKEN}       # Anthropic OAuth token
  url: https://api.anthropic.com/api/oauth/usage
  interval: 5m

yr:
  lat: 59.9497                 # Norge/Oslo/Oslo/Nydalsdammen
  lon: 10.7665
  user_agent: "dasher/per.buer@gmail.com Norge/Oslo/Oslo/Nydalsdammen"
  interval: 15m

publish:
  interval: 30s
```

Durations use Go syntax (`30s`, `5m`, `1h`). Required values: `mqtt.tls_cert`,
`mqtt.tls_key`, `mqtt.tls_client_ca`, `github.token`, `claude.token`,
`yr.lat`, `yr.lon`, `yr.user_agent`.

The `yr.user_agent` must identify the app with a contact address —
MET Norway rejects placeholder User-Agents with `403`.

## Getting a Claude token

The usage endpoint uses your Claude subscription's OAuth token (the same one
Claude Code holds). If you're logged into Claude Code, extract it with:

```sh
jq -r '.claudeAiOauth.accessToken' ~/.claude/.credentials.json
```

Put the result in `.env` as `CLAUDE_TOKEN=sk-ant-oat01-...`. Note the token
has an expiry — when requests start failing with `401`, re-extract it.
(Alternatively, `claude setup-token` prints a long-lived token.)

## GitHub token

Create a **classic** PAT with only the `notifications` scope at
github.com/settings/tokens ([pre-filled link](https://github.com/settings/tokens/new?scopes=notifications&description=dasher)).
Fine-grained PATs do not work with the notifications endpoint. Put it in
`.env` as `GITHUB_TOKEN=ghp_...`.

## Run

```sh
export MQTT_TLS_CERT=cert.pem MQTT_TLS_KEY=key.pem MQTT_CLIENT_CA=certs/ca.crt \
       GITHUB_TOKEN=ghp_... CLAUDE_TOKEN=sk-ant-oat01-...
./dasher
```

Add `-debug` while developing to print every published payload to stdout
instead of publishing it. In this mode the MQTT broker is not started at
all, so no certificates or `mqtt.tls_*` values are needed. Never run with
`-debug` in production.

```sh
./dasher -debug
```

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
  "claude": { "...": "opaque usage JSON from the Anthropic API" },
  "weather": [
    { "time": "2026-08-05T18:00:00Z", "temperature_c": 17.6, "precipitation_mm": 0.9, "symbol": "rain" }
  ]
}
```

`github`, `claude` and `weather` are omitted until the first successful
fetch of each source. The Claude document is passed through unmodified, so
the schema follows whatever the configured usage endpoint returns.

## Notes

- The broker library is `github.com/mochi-mqtt/server/v2` — the renamed,
  maintained continuation of `mochi-co/mqtt/v2` (same project, v2.7.x).
- The Claude fetcher targets the OAuth usage endpoint by default; adjust
  `claude.url` if your token type targets a different endpoint.

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
