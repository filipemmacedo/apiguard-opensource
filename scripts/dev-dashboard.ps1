$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$dashboardPath = Join-Path (Split-Path -Parent $PSScriptRoot) "dashboard"
Set-Location $dashboardPath

try {
    $host.UI.RawUI.WindowTitle = "API Guard Dashboard"
} catch {
}

Write-Host "Starting dashboard on http://localhost:3000"
Write-Host "Press Ctrl+C in this window to stop the dashboard."

& npm run dev

if ($LASTEXITCODE -ne $null) {
    exit $LASTEXITCODE
}
