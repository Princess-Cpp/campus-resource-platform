#!/usr/bin/env bash
set -euo pipefail
project_root=$(cd "$(dirname "$0")/.." && pwd)
cd "$project_root/frontend"
node --experimental-strip-types --test src/modules/m3/reporting.test.ts src/modules/m3/transport.test.ts src/modules/m3/actions.test.ts src/modules/common/session.test.ts
npm run build
