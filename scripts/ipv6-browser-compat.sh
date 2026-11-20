#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${VIRTMGR_IPV6_E2E_PORT:-18082}"
URL="http://[::1]:${PORT}"
LOG="${VIRTMGR_IPV6_E2E_LOG:-/tmp/virtmgr-ipv6-browser-compat.log}"
server_pid=""

cleanup() {
  if [[ -n "$server_pid" ]] && kill -0 "$server_pid" 2>/dev/null; then
    kill -- "-$server_pid" 2>/dev/null || kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT

export GOSUMDB="${GOSUMDB:-sum.golang.org}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

setsid env \
  VIRTMGR_E2E_PORT="$PORT" \
  VIRTMGR_E2E_LISTEN="[::1]:${PORT}" \
  VIRTMGR_E2E_DATABASE_PATH="/tmp/virtmgr-e2e-compat-${PORT}.db" \
  "$ROOT/scripts/run-e2e-server.sh" >"$LOG" 2>&1 &
server_pid=$!

for _ in $(seq 1 120); do
  if curl --noproxy '*' -gfsS "${URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$server_pid" 2>/dev/null; then
    printf 'IPv6 test server exited before becoming ready. Log: %s\n' "$LOG" >&2
    tail -100 "$LOG" >&2 || true
    exit 1
  fi
  sleep 1
done

if ! curl --noproxy '*' -gfsS "${URL}/healthz" >/dev/null; then
  printf 'IPv6 test server did not become ready. Log: %s\n' "$LOG" >&2
  tail -100 "$LOG" >&2 || true
  exit 1
fi

cd "$ROOT/web"
export PLAYWRIGHT_SKIP_WEBSERVER=1

printf '\nRunning local Chromium and Firefox IPv6 checks against %s...\n' "$URL"
PLAYWRIGHT_WEBKIT=0 npx playwright test --workers=1 -c playwright.compat.config.ts \
  --project=chromium-ipv6 --project=firefox-ipv6

version="$(node -p "require('./node_modules/@playwright/test/package.json').version")"
image="mcr.microsoft.com/playwright:v${version}-noble"

if ! command -v podman >/dev/null 2>&1; then
  printf '\nWebKit check requires podman and %s because the locally installed WebKit build targets Ubuntu.\n' "$image" >&2
  exit 1
fi

printf '\nRunning WebKit IPv6 check in %s...\n' "$image"
podman run --rm --network=host --init --shm-size=1g \
  -v "$ROOT:/work" \
  -w /work/web \
  -e PLAYWRIGHT_SKIP_WEBSERVER=1 \
  -e PLAYWRIGHT_WEBKIT=1 \
  -e PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
  "$image" \
  ./node_modules/.bin/playwright test --workers=1 -c playwright.compat.config.ts \
  --project=webkit-ipv6
