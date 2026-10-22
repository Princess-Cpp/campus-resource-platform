#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
export GOCACHE=${GOCACHE:-/tmp/campus-go-cache}
export GOPATH=${GOPATH:-/tmp/campus-go-path}
export GOMODCACHE=${GOMODCACHE:-/tmp/campus-go-mod-cache}
export MYSQL_DSN=${MYSQL_DSN:-campus:campus-dev-only@tcp(127.0.0.1:3306)/campus?parseTime=true&multiStatements=true}
export M1_DEV_MODE=true
export M1_CODE_SECRET=${M1_CODE_SECRET:-campus-development-secret-012345678901}
export M1_DEV_FIXTURE=${M1_DEV_FIXTURE:-$root/backend/dev-fixture.json}
export AUTH_DEV_SEED=${AUTH_DEV_SEED:-true}
: "${AUTH_DEV_PASSWORD:?Set AUTH_DEV_PASSWORD (10-72 bytes) for fictional local accounts}"
export LISTEN_ADDR=${LISTEN_ADDR:-127.0.0.1:8080}
exec "$root/backend/server"
