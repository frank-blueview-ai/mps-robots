param(
    [string]$OraName = $env:ORA_NAME,
    [string]$OraPassword = $env:ORA_PASSWORD
)

$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot

& (Join-Path $PSScriptRoot "stop-bridge.ps1")
& (Join-Path $PSScriptRoot "stop-nginx.ps1")

Start-Sleep -Seconds 2

& (Join-Path $PSScriptRoot "start-nginx.ps1")
& (Join-Path $PSScriptRoot "start-bridge.ps1") -OraName $OraName -OraPassword $OraPassword

Write-Host "ORA local app is available at http://localhost:8080"
