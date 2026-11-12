#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
python3 scripts/verify-brand.py
python3 -m json.tool apps/web/public/manifest.webmanifest >/dev/null
(cd apps/api && go test ./... && go vet ./...)
if command -v npm >/dev/null 2>&1; then
  if [ -d apps/web/node_modules ]; then
    (cd apps/web && npm run build)
  else
    echo 'SKIP Vue build: node_modules missing; run cd apps/web && npm install then npm run build'
  fi
else
  echo 'SKIP Vue build: npm missing'
fi
echo 'Phase 0 static checks passed (see any SKIP above)'
