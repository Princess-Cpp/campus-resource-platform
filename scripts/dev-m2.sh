#!/usr/bin/env bash
set -euo pipefail
project_root=$(cd "$(dirname "$0")/.." && pwd)
: "${MYSQL_DSN:?Set MYSQL_DSN for an isolated MySQL database (include parseTime=true)}"
: "${M1_CODE_SECRET:?Set M1_CODE_SECRET (at least 32 bytes)}"
export M1_DEV_MODE=true
export M1_DEV_FIXTURE=${M1_DEV_FIXTURE:-$project_root/backend/dev-fixture.json}
export M2_ENABLED=true
: "${M2_SERVICE_TOKEN:?Set M2_SERVICE_TOKEN (at least 32 bytes)}"
export M2_SERVICE_TOKEN
export AUTH_DEV_SEED=${AUTH_DEV_SEED:-true}
: "${AUTH_DEV_PASSWORD:?Set AUTH_DEV_PASSWORD (10-72 bytes) for fictional local accounts}"
export LISTEN_ADDR=${LISTEN_ADDR:-127.0.0.1:8081}
export M2_RESOURCE_URL=${M2_RESOURCE_URL:-http://127.0.0.1:8081}
cd "$project_root/backend"
exec go run ./cmd/server
