#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${VIRTMGR_E2E_PORT:-18080}"
LISTEN="${VIRTMGR_E2E_LISTEN:-127.0.0.1:${PORT}}"
DB_PATH="${VIRTMGR_E2E_DATABASE_PATH:-/tmp/virtmgr-e2e-${PORT}.db}"
CONFIG_PATH="${VIRTMGR_E2E_CONFIG_PATH:-/tmp/virtmgr-e2e-${PORT}.yaml}"
export GOSUMDB="${GOSUMDB:-sum.golang.org}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

python3 - "$DB_PATH" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
for candidate in (path, pathlib.Path(str(path) + "-shm"), pathlib.Path(str(path) + "-wal")):
    candidate.unlink(missing_ok=True)
PY
cd "$ROOT/web"
npm run build >/dev/null
cd "$ROOT"

cat >"$CONFIG_PATH" <<EOF
listen: "${LISTEN}"
libvirt_uri: qemu:///system
database_path: "${DB_PATH}"
host_name: e2e-host
admin_username: admin
admin_password: secret
session_ttl: 8h
cookie_secure: false
trusted_proxies: []
read_only: ${VIRTMGR_E2E_READ_ONLY:-true}
demo_mode: true
EOF

exec go run ./cmd/virtmgr --config "$CONFIG_PATH"
