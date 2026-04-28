$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

try {
    $host.UI.RawUI.WindowTitle = "API Guard API"
} catch {
}

Write-Host "Starting API Guard backend on http://localhost:8080"
Write-Host "Press Ctrl+C in this window to stop the backend."

& go run ./cmd/apiguard

if ($LASTEXITCODE -ne $null) {
    exit $LASTEXITCODE
}
