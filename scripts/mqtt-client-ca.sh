#!/usr/bin/env bash
# Manages the private CA that signs MQTT subscriber client certificates.
#
# Usage:
#   scripts/mqtt-client-ca.sh init            create certs/ca.key + certs/ca.crt (10-year CA)
#   scripts/mqtt-client-ca.sh issue <name>     issue a client cert/key for <name>, signed by the CA
#
# Point mqtt.tls_client_ca (config.yaml) at certs/ca.crt. Give each
# subscribing device (e.g. the ESP32) its own certs/<name>.crt + <name>.key.
set -euo pipefail

cd "$(dirname "$0")/.."
CERT_DIR="certs"
CA_KEY="$CERT_DIR/ca.key"
CA_CRT="$CERT_DIR/ca.crt"
CA_SUBJECT="/CN=dasher-mqtt-client-ca"
CLIENT_DAYS=1825   # 5 years
CA_DAYS=3650        # 10 years

cmd="${1:-}"

case "$cmd" in
  init)
    if [[ -f "$CA_KEY" || -f "$CA_CRT" ]]; then
      echo "refusing to overwrite existing $CA_KEY / $CA_CRT" >&2
      exit 1
    fi
    mkdir -p "$CERT_DIR"
    openssl req -x509 -newkey rsa:4096 -sha256 -days "$CA_DAYS" -nodes \
      -keyout "$CA_KEY" -out "$CA_CRT" -subj "$CA_SUBJECT"
    chmod 600 "$CA_KEY"
    echo "wrote $CA_KEY (keep private, do not commit) and $CA_CRT (public, set as mqtt.tls_client_ca)"
    ;;

  issue)
    name="${2:?usage: $0 issue <name>}"
    key="$CERT_DIR/$name.key"
    csr="$CERT_DIR/$name.csr"
    crt="$CERT_DIR/$name.crt"
    if [[ ! -f "$CA_KEY" || ! -f "$CA_CRT" ]]; then
      echo "no CA found; run '$0 init' first" >&2
      exit 1
    fi
    if [[ -f "$key" || -f "$crt" ]]; then
      echo "refusing to overwrite existing $key / $crt" >&2
      exit 1
    fi
    openssl req -newkey rsa:2048 -sha256 -nodes \
      -keyout "$key" -out "$csr" -subj "/CN=$name"
    openssl x509 -req -in "$csr" -CA "$CA_CRT" -CAkey "$CA_KEY" -CAcreateserial \
      -out "$crt" -days "$CLIENT_DAYS" -sha256
    rm -f "$csr" "$CERT_DIR/ca.srl"
    chmod 600 "$key"
    echo "wrote $key (load onto the $name device) and $crt (matching client cert)"
    ;;

  *)
    echo "usage: $0 {init|issue <name>}" >&2
    exit 1
    ;;
esac
