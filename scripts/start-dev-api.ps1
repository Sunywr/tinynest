param([switch]$ConfigureOnly)

$ErrorActionPreference = 'Stop'
$apiDir = Join-Path $PSScriptRoot '../apps/api'
$dataDir = Join-Path $apiDir 'data'
$tokenPath = Join-Path $dataDir '.env.setup-token'
New-Item -ItemType Directory -Force $dataDir | Out-Null

if (-not $env:TINYNEST_SETUP_TOKEN) {
    if (-not (Test-Path -LiteralPath $tokenPath)) {
        $bytes = New-Object byte[] 32
        $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
        try {
            $rng.GetBytes($bytes)
        } finally {
            $rng.Dispose()
        }
        $token = [BitConverter]::ToString($bytes).Replace('-', '').ToLowerInvariant()
        Set-Content -LiteralPath $tokenPath -Value $token -Encoding ascii -NoNewline
    }
    $env:TINYNEST_SETUP_TOKEN = (Get-Content -LiteralPath $tokenPath -Raw).Trim()
    if ($env:TINYNEST_SETUP_TOKEN -cnotmatch '^[0-9a-f]{64}$') {
        throw "Invalid local setup token in $tokenPath"
    }
}

if ($ConfigureOnly) { return }

$env:TINYNEST_LISTEN_ADDR = '127.0.0.1:8787'
$env:TINYNEST_COOKIE_SECURE = 'false'
$env:TINYNEST_DATA_DIR = $dataDir
$env:TINYNEST_DB_PATH = Join-Path $dataDir 'tinynest.sqlite3'

Push-Location $apiDir
try {
    go run ./cmd/server
    if ($LASTEXITCODE) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}
