# Script name is historical (Phase 0); it now runs local static checks against the V0.1 tree.
# Full gate: go mod tidy -diff, npm ci / typecheck / build, git diff --check (docs/acceptance.md).
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
python scripts/verify-brand.py
Push-Location apps/api
try { go test ./...; if ($LASTEXITCODE -ne 0) { throw "Go tests failed" }; go vet ./...; if ($LASTEXITCODE -ne 0) { throw "Go vet failed" } }
finally { Pop-Location }
if (Test-Path apps/web/node_modules) {
    Push-Location apps/web
    try { npm run build; if ($LASTEXITCODE -ne 0) { throw "Frontend build failed" } }
    finally { Pop-Location }
} else { Write-Host "SKIP: Install npm dependencies to run Vue build" }
