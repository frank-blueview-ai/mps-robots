param(
    [Parameter(Position = 0)]
    [ValidateSet("start", "stop", "restart", "status")]
    [string]$Action = "start",
    [string]$OraName = $env:ORA_NAME,
    [string]$OraPassword = $env:ORA_PASSWORD,
    [int]$TimeoutSeconds = 120
)

$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
$runtimeRoot = Join-Path $projectRoot "runtime"
$stdout = Join-Path $runtimeRoot "ora-bridge.out.log"
$stderr = Join-Path $runtimeRoot "ora-bridge.err.log"

function Read-DotEnv {
    param([string]$Path)

    $values = @{}
    if (-not (Test-Path -LiteralPath $Path)) {
        return $values
    }

    foreach ($line in Get-Content -LiteralPath $Path) {
        if ($line -notmatch "^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)\s*$") {
            continue
        }

        $key = $matches[1]
        $value = $matches[2].Trim()

        if (($value.StartsWith('"') -and $value.EndsWith('"')) -or
            ($value.StartsWith("'") -and $value.EndsWith("'"))) {
            $value = $value.Substring(1, $value.Length - 2)
        }

        $values[$key] = $value
    }

    return $values
}

function Get-BridgeProcess {
    Get-CimInstance Win32_Process -Filter "name = 'node.exe'" |
        Where-Object { $_.CommandLine -like "*scripts/ora-bridge-server.js*" }
}

function Stop-Bridge {
    $bridgeProcesses = Get-BridgeProcess

    if (-not $bridgeProcesses) {
        Write-Host "ORA bridge is not running."
        return
    }

    foreach ($process in $bridgeProcesses) {
        Stop-Process -Id $process.ProcessId -Force
        Write-Host "Stopped ORA bridge process $($process.ProcessId)"
    }
}

function Show-BridgeStatus {
    $bridgeProcesses = Get-BridgeProcess
    if (-not $bridgeProcesses) {
        Write-Host "ORA bridge process is not running."
        return
    }

    $processIds = ($bridgeProcesses | ForEach-Object { $_.ProcessId }) -join ", "
    Write-Host "ORA bridge process running: $processIds"

    try {
        $response = Invoke-WebRequest -Uri "http://127.0.0.1:8787/status" -UseBasicParsing -TimeoutSec 5
        $status = $response.Content | ConvertFrom-Json
        if ($status.connected -eq $true) {
            Write-Host "ORA bridge API connected at http://127.0.0.1:8787"
        } else {
            Write-Host "ORA bridge API is reachable but not connected."
        }
    } catch {
        Write-Host "ORA bridge API is not reachable yet."
    }
}

function Start-Bridge {
    New-Item -ItemType Directory -Force -Path $runtimeRoot | Out-Null

    $dotEnv = Read-DotEnv -Path (Join-Path $projectRoot ".env")

    if (-not $OraName) {
        $OraName = $dotEnv["ORA_NAME"]
    }

    if (-not $OraPassword) {
        $OraPassword = $dotEnv["ORA_PASSWORD"]
    }

    if (-not $OraName) {
        $OraName = Read-Host "ORA device name"
    }

    if (-not $OraPassword) {
        $OraPassword = Read-Host "ORA password"
    }

    $env:ORA_NAME = $OraName
    $env:ORA_PASSWORD = $OraPassword

    $existingBridge = Get-BridgeProcess | Select-Object -First 1

    if ($existingBridge) {
        Write-Host "ORA bridge is already running as process $($existingBridge.ProcessId)"
    } else {
        Start-Process -FilePath "node" `
            -ArgumentList @("scripts/ora-bridge-server.js") `
            -WorkingDirectory $projectRoot `
            -RedirectStandardOutput $stdout `
            -RedirectStandardError $stderr `
            -WindowStyle Hidden
    }

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -Uri "http://127.0.0.1:8787/status" -UseBasicParsing -TimeoutSec 5
            $status = $response.Content | ConvertFrom-Json
            if ($status.connected -eq $true) {
                Write-Host "ORA bridge connected at http://127.0.0.1:8787"
                return
            }
        } catch {
            Start-Sleep -Seconds 3
            continue
        }

        Start-Sleep -Seconds 3
    }

    Write-Warning "ORA bridge did not report connected within $TimeoutSeconds seconds. Check runtime\ora-bridge.err.log."
}

switch ($Action) {
    "start" {
        Start-Bridge
    }
    "stop" {
        Stop-Bridge
    }
    "restart" {
        Stop-Bridge
        Start-Sleep -Seconds 2
        Start-Bridge
    }
    "status" {
        Show-BridgeStatus
    }
}
