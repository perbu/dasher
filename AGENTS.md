# AGENTS.md

Guidance for AI agents working in this repository.

## What this is

`dasher` — a single-binary Go service that aggregates GitHub notifications,
Claude usage metrics, and Yr.no (MET Norway) weather, then publishes the
result as retained JSON on an embedded MQTT broker (mutual TLS, read-only
ACL on the data topic) for an ESP32 dashboard. Publishing is internal-only
via the broker's inline client. No tests exist yet; verification is done by
building and running the binary.

`firmware/` is the other half: a PlatformIO/Arduino project for the
Wireless-Tag **WT32-SC01 Plus** (ESP32-S3, 16MB flash, 2MB PSRAM, 480x320
ST7796UI on an 8-bit parallel bus, FT6336U touch) that subscribes to the topic
and renders the payload. It draws with LovyanGFX directly — there is no LVGL,
and adding it would be a significant change of direction rather than a
refactor.

The **Plus** matters: the plain WT32-SC01 is an ESP32-WROVER-B with the panel
on SPI. Documentation and example configs for the two are freely mixed up
online, including in search results that name one and describe the other.
Verify against the hardware (`esptool.py chip_id`, USB VID `303A:1001` for the
Plus) rather than trusting a page that merely has the right board name on it.

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

Firmware (needs `pio`; see README for the config.h / certs.h prerequisites):

```sh
scripts/gen-firmware-certs.sh esp32      # regenerate firmware/src/certs.h from certs/
pio run -d firmware                      # build
pio run -d firmware -t upload            # flash over USB-C
pio device monitor -b 115200             # serial log
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
- The publisher polls the store every `pollInterval` and publishes when
  `UpdatedAt` moves, so new data reaches the topic in about a second.
  `publish.interval` is the heartbeat ceiling, not a fixed cadence — don't
  reintroduce a plain `time.Ticker(p.interval)` loop.
- Fetchers route HTTP through `do()` in `fetchers.go` rather than calling
  `client.Do` directly; that is what produces the `-debug` request timings.
  Set `fetchers.Verbose` before starting any fetcher, never after.
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

### Firmware

- The broker's own certificate is issued by the same CA as the client certs
  (`scripts/mqtt-client-ca.sh server <host>`). Its subjectAltName must match
  `MQTT_HOST` in `firmware/include/config.h` — mbedTLS verifies the name, and
  a mismatch fails as a bare "certificate verify failed".
- TLS checks certificate validity against the clock, and the ESP32 boots at
  the epoch. `net.cpp` syncs SNTP before it will attempt MQTT; do not reorder
  that.
- PubSubClient drops messages larger than its buffer **silently** — no error,
  no callback. The buffer is 16KB because the notification list can push the
  payload to several KB; shrink it and busy inboxes stop rendering.
- `firmware/include/config.h` (wifi credentials) and `firmware/src/certs.h`
  (device private key) are generated and git-ignored. Never commit either,
  and never inline a fallback default for them.
- The frame is composed in a PSRAM sprite and pushed once. Drawing straight
  to the panel per widget would tear; keep `ui.cpp` rendering into the
  `LGFXBase&` it is handed.
- Layout constants live in `theme.h`, not scattered through `ui.cpp`.
- `Serial` only works with `-DARDUINO_USB_CDC_ON_BOOT=1` (native USB, no UART
  bridge). Dropping that flag makes a healthy board look bricked.
- `pio device monitor` needs a TTY; read `/dev/cu.usbmodem*` with pyserial
  when scripting, and set `dtr = True` — the USB-CDC stack discards output
  while the host looks disconnected, so a silent port usually means a missing
  DTR rather than a crashed board. PlatformIO's own python has pyserial
  (`/opt/homebrew/Cellar/platformio/*/libexec/bin/python`); the system one
  does not.
- Only `kMaxNotifications` entries are kept from the payload, but every entry
  is still scanned to count distinct repos. Do not "optimise" the loop by
  breaking out early once the array is full.
- The board's flash is 16MB with `default_16MB.csv`; the stock
  `esp32-s3-devkitc-1` definition assumes 8MB and octal PSRAM, so
  `board_upload.flash_size` and `board_build.arduino.memory_type=qio_qspi`
  are both load-bearing.
