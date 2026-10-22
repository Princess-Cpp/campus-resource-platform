#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
export GOCACHE=${GOCACHE:-/tmp/campus-go-cache}
export GOPATH=${GOPATH:-/tmp/campus-go-path}
export GOMODCACHE=${GOMODCACHE:-/tmp/campus-go-mod-cache}
: "${MYSQL_DSN:?Set MYSQL_DSN (include parseTime=true&multiStatements=true)}"
export MYSQL_DSN
export M1_DEV_MODE=true
: "${M1_CODE_SECRET:?Set M1_CODE_SECRET (at least 32 bytes)}"
export M1_CODE_SECRET
export M1_DEV_FIXTURE=${M1_DEV_FIXTURE:-$root/backend/dev-fixture.json}
export AUTH_DEV_SEED=${AUTH_DEV_SEED:-true}
: "${AUTH_DEV_PASSWORD:?Set AUTH_DEV_PASSWORD (10-72 bytes) for fictional local accounts}"
export LISTEN_ADDR=${LISTEN_ADDR:-127.0.0.1:8080}
exec "$root/backend/server"
