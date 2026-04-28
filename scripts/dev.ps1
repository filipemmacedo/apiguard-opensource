param(
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$repoRoot = Split-Path -Parent $PSScriptRoot
$dashboardPath = Join-Path $repoRoot "dashboard"
$powershellExe = Join-Path $PSHOME "powershell.exe"

function Assert-CommandExists {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "$Name is not installed or not available on PATH."
    }
}

function Test-PortListening {
    param(
        [Parameter(Mandatory = $true)]
        [int]$Port
    )

    $listener = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
    return $null -ne $listener
}

function Start-DevWindow {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Title,
        [Parameter(Mandatory = $true)]
        [string]$WorkingDirectory,
        [Parameter(Mandatory = $true)]
        [string]$ScriptName
    )

    $scriptPath = Join-Path $PSScriptRoot $ScriptName

    if (-not (Test-Path $scriptPath)) {
        throw "Launcher script '$ScriptName' was not found."
    }

    $argumentList = "-NoExit -ExecutionPolicy Bypass -File `"$scriptPath`""

    if ($DryRun) {
        Write-Host "[$Title] $powershellExe $argumentList"
        return
    }

    Start-Process -FilePath $powershellExe -WorkingDirectory $WorkingDirectory -ArgumentList $argumentList | Out-Null
    Write-Host "Started $Title."
}

Assert-CommandExists -Name "go"
Assert-CommandExists -Name "npm"

if (-not (Test-Path (Join-Path $dashboardPath "package.json"))) {
    throw "dashboard/package.json was not found."
}

if (-not (Test-Path (Join-Path $dashboardPath "node_modules"))) {
    throw "Dashboard dependencies are missing. Run 'cd dashboard; npm install' once first."
}

$startedServices = @()

if (Test-PortListening -Port 8080) {
    Write-Warning "Port 8080 is already in use, so the backend window was not started."
} else {
    Start-DevWindow -Title "API Guard API" -WorkingDirectory $repoRoot -ScriptName "dev-api.ps1"
    $startedServices += "api"
}

if (Test-PortListening -Port 3000) {
    Write-Warning "Port 3000 is already in use, so the dashboard window was not started."
} else {
    Start-DevWindow -Title "API Guard Dashboard" -WorkingDirectory $dashboardPath -ScriptName "dev-dashboard.ps1"
    $startedServices += "dashboard"
}

if ($startedServices.Count -eq 0) {
    throw "Nothing was started because ports 8080 and 3000 are already in use."
}

if (-not $DryRun) {
    Write-Host "API Guard dev servers are running in separate PowerShell windows."
    Write-Host "Press Ctrl+C in either service window to stop that service."
}
