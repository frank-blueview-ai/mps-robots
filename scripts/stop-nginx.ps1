$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
$nginxRoot = Join-Path $projectRoot "tools\nginx-1.30.1"
$nginxExe = Join-Path $nginxRoot "nginx.exe"
$configPath = Join-Path $projectRoot "nginx.conf"

if (Test-Path -LiteralPath $nginxExe) {
    & $nginxExe -p "$nginxRoot\" -c $configPath -s stop
}
