#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export CODEGRAPH_TELEMETRY="${CODEGRAPH_TELEMETRY:-0}"
if command -v codegraph >/dev/null 2>&1; then
  codegraph init
  codegraph status
else
  npx -y @colbymchenry/codegraph init
  npx -y @colbymchenry/codegraph status
fi
