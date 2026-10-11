#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../frontend"
npm install --package-lock-only --ignore-scripts
npm run build
rm -rf ../backend/cmd/server/webdist
mkdir -p ../backend/cmd/server/webdist
cp -R dist/. ../backend/cmd/server/webdist/
cd ../backend
GOTOOLCHAIN=local go mod tidy
go build ./cmd/server
