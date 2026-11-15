#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${VIRTMGR_E2E_PORT:-18080}"
DB_PATH="${VIRTMGR_E2E_DATABASE_PATH:-/tmp/virtmgr-e2e-${PORT}.db}"

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

export VIRTMGR_LISTEN="127.0.0.1:${PORT}"
export VIRTMGR_DEMO_MODE="true"
export VIRTMGR_DATABASE_PATH="$DB_PATH"
export VIRTMGR_ADMIN_USERNAME="admin"
export VIRTMGR_ADMIN_PASSWORD="secret"
export VIRTMGR_READ_ONLY="${VIRTMGR_E2E_READ_ONLY:-true}"

exec go run ./cmd/virtmgr
