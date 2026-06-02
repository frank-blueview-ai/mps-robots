param(
    [Parameter(Position = 0)]
    [ValidateSet("start", "stop", "restart", "status")]
    [string]$Action = "start",
    [string]$Addr = $env:ORA_SERVER_ADDR,
    [int]$TimeoutSeconds = 15
)

$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
$runtimeRoot = Join-Path $projectRoot "runtime"
$serverExe = Join-Path $runtimeRoot "ora-server.exe"
$stdout = Join-Path $runtimeRoot "ora-server.out.log"
$stderr = Join-Path $runtimeRoot "ora-server.err.log"

if (-not $Addr) {
    $Addr = "127.0.0.1:8080"
}

function Get-ServerPort {
    param([string]$ListenAddr)

    if ($ListenAddr -match ":(\d+)$") {
        return [int]$matches[1]
    }

    throw "Could not determine port from address $ListenAddr"
}

function Get-ServerUrl {
    param([string]$ListenAddr)

    $port = Get-ServerPort -ListenAddr $ListenAddr
    return "http://127.0.0.1:$port"
}

function Get-GoServerProcess {
    Get-CimInstance Win32_Process |
        Where-Object {
            ($_.Name -eq "ora-server.exe" -and $_.CommandLine -like "*$serverExe*") -or
            ($_.Name -eq "go.exe" -and $_.CommandLine -like "*cmd/ora-server*")
        }
}

function Build-GoServer {
    $go = Get-Command go -ErrorAction SilentlyContinue
    if (-not $go) {
        throw "Go is not installed or is not on PATH. Install it with: winget install GoLang.Go"
    }

    New-Item -ItemType Directory -Force -Path $runtimeRoot | Out-Null
    & $go.Source build -o $serverExe .\cmd\ora-server
}

function Build-Frontend {
    $npm = Get-Command npm -ErrorAction SilentlyContinue
    if (-not $npm) {
        throw "npm is not installed or is not on PATH. Install Node.js, then run npm install."
    }

    & $npm.Source run build:frontend
}

function Stop-GoServer {
    $processes = Get-GoServerProcess
    if (-not $processes) {
        Write-Host "ORA Go server is not running."
        return
    }

    foreach ($process in $processes) {
        Stop-Process -Id $process.ProcessId -Force
        Write-Host "Stopped ORA Go server process $($process.ProcessId)"
    }
}

function Show-GoServerStatus {
    $serverUrl = Get-ServerUrl -ListenAddr $Addr
    $processes = Get-GoServerProcess

    if ($processes) {
        $processIds = ($processes | ForEach-Object { $_.ProcessId }) -join ", "
        Write-Host "ORA Go server process running: $processIds"
    } else {
        Write-Host "ORA Go server process is not running."
    }

    try {
        $response = Invoke-WebRequest -Uri "$serverUrl/api/health" -UseBasicParsing -TimeoutSec 5
        $status = $response.Content | ConvertFrom-Json
        if ($status.ok -eq $true -and $status.server -eq "ora-go-server") {
            Write-Host "ORA Go server API is reachable at $serverUrl"
            Write-Host $response.Content
        } else {
            Write-Host "Port responded at $serverUrl, but it is not the ORA Go server."
        }
    } catch {
        Write-Host "ORA Go server API is not reachable at $serverUrl"
    }
}

function Start-GoServer {
    $serverUrl = Get-ServerUrl -ListenAddr $Addr
    $port = Get-ServerPort -ListenAddr $Addr

    $existing = Get-GoServerProcess | Select-Object -First 1
    if ($existing) {
        Write-Host "ORA Go server is already running as process $($existing.ProcessId)"
    } else {
        $listener = Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue |
            Where-Object { $_.State -eq "Listen" } |
            Select-Object -First 1

        if ($listener) {
            throw "Port $port is already in use. Stop NGINX with .\scripts\stop-nginx.ps1 or choose another -Addr."
        }

        Build-Frontend
        Build-GoServer

        Start-Process -FilePath $serverExe `
            -ArgumentList @("-addr", $Addr, "-project-root", $projectRoot) `
            -WorkingDirectory $projectRoot `
            -RedirectStandardOutput $stdout `
            -RedirectStandardError $stderr `
            -WindowStyle Hidden
    }

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -Uri "$serverUrl/api/health" -UseBasicParsing -TimeoutSec 3
            $status = $response.Content | ConvertFrom-Json
            if ($status.ok -eq $true -and $status.server -eq "ora-go-server") {
                Write-Host "ORA Go server is serving at $serverUrl"
                return
            }
        } catch {
            Start-Sleep -Seconds 1
        }

        Start-Sleep -Seconds 1
    }

    Write-Warning "ORA Go server did not become reachable within $TimeoutSeconds seconds. Check runtime\ora-server.err.log."
}

switch ($Action) {
    "start" {
        Start-GoServer
    }
    "stop" {
        Stop-GoServer
    }
    "restart" {
        Stop-GoServer
        Start-Sleep -Seconds 1
        Start-GoServer
    }
    "status" {
        Show-GoServerStatus
    }
}
