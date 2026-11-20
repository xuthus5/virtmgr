#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d /tmp/virtmgr-tls-drill.XXXXXX)"
PORT="${VIRTMGR_TLS_DRILL_PORT:-18443}"
URL="https://[::1]:${PORT}"
SERVER_LOG="${WORK}/server.log"
server_pid=""

cleanup() {
  if [[ -n "$server_pid" ]] && kill -0 "$server_pid" 2>/dev/null; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

export GOSUMDB="${GOSUMDB:-sum.golang.org}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

write_certificate() {
  local name="$1" common_name="$2"
  local directory="${WORK}/${name}"
  mkdir -p "$directory"
  cat >"${directory}/openssl.cnf" <<EOF
[req]
distinguished_name = subject
x509_extensions = extensions
prompt = no
[subject]
CN = ${common_name}
[extensions]
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = DNS:localhost,IP:::1
EOF
  openssl req -x509 -newkey rsa:2048 -nodes -days 30 \
    -keyout "${directory}/tls.key" -out "${directory}/tls.crt" \
    -config "${directory}/openssl.cnf" >/dev/null 2>&1
  chmod 0600 "${directory}/tls.key"
}

serve_certificate() {
  timeout 8 openssl s_client -connect "[::1]:${PORT}" -servername localhost \
    -showcerts </dev/null 2>/dev/null | openssl x509 -noout -fingerprint -sha256
}

wait_for_fingerprint() {
  local expected="$1" actual=""
  for _ in $(seq 1 20); do
    actual="$(serve_certificate || true)"
    if [[ "$actual" == "$expected" ]]; then
      return 0
    fi
    sleep 1
  done
  printf 'certificate did not rotate: expected %s, got %s\n' "$expected" "$actual" >&2
  return 1
}

for tool in openssl curl go; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    printf '%s is required for the TLS rotation drill\n' "$tool" >&2
    exit 1
  fi
done

write_certificate initial initial.example
write_certificate rotated rotated.example
cp "${WORK}/initial/tls.crt" "${WORK}/live.crt"
cp "${WORK}/initial/tls.key" "${WORK}/live.key"
chmod 0600 "${WORK}/live.key"

cat >"${WORK}/server.yaml" <<EOF
listen: "[::1]:${PORT}"
libvirt_uri: qemu:///system
database_path: "${WORK}/virtmgr.db"
host_name: tls-rotation-drill
admin_username: admin
admin_password: drill-only-password
session_ttl: 1h
cookie_secure: true
trusted_proxies: []
tls_cert_file: "${WORK}/live.crt"
tls_key_file: "${WORK}/live.key"
read_only: true
demo_mode: true
EOF

go build -o "${WORK}/virtmgr" ./cmd/virtmgr
"${WORK}/virtmgr" --config "${WORK}/server.yaml" >"$SERVER_LOG" 2>&1 &
server_pid=$!

for _ in $(seq 1 60); do
  if curl --noproxy '*' -gfsS --cacert "${WORK}/initial/tls.crt" "${URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$server_pid" 2>/dev/null; then
    printf 'TLS drill server exited early:\n' >&2
    cat "$SERVER_LOG" >&2
    exit 1
  fi
  sleep 1
done
curl --noproxy '*' -gfsS --cacert "${WORK}/initial/tls.crt" "${URL}/healthz" >/dev/null

initial_fingerprint="$(openssl x509 -in "${WORK}/initial/tls.crt" -noout -fingerprint -sha256)"
rotated_fingerprint="$(openssl x509 -in "${WORK}/rotated/tls.crt" -noout -fingerprint -sha256)"
if [[ "$(serve_certificate)" != "$initial_fingerprint" ]]; then
  printf 'server did not serve the initial certificate\n' >&2
  exit 1
fi

cp "${WORK}/rotated/tls.crt" "${WORK}/live.crt.new"
cp "${WORK}/rotated/tls.key" "${WORK}/live.key.new"
mv "${WORK}/live.crt.new" "${WORK}/live.crt"
mv "${WORK}/live.key.new" "${WORK}/live.key"
wait_for_fingerprint "$rotated_fingerprint"
curl --noproxy '*' -gfsS --cacert "${WORK}/rotated/tls.crt" "${URL}/healthz" >/dev/null

printf 'not a certificate\n' >"${WORK}/live.crt"
sleep 1
curl --noproxy '*' -gfsS --cacert "${WORK}/rotated/tls.crt" "${URL}/healthz" >/dev/null
if [[ "$(serve_certificate)" != "$rotated_fingerprint" ]]; then
  printf 'invalid replacement did not retain the previous certificate\n' >&2
  exit 1
fi

printf 'TLS rotation drill passed: IPv6 listener, live rotation, SAN trust and failed-reload rollback.\n'
