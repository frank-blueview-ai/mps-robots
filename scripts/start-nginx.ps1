$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
$nginxRoot = Join-Path $projectRoot "tools\nginx-1.30.1"
$nginxExe = Join-Path $nginxRoot "nginx.exe"
$configPath = Join-Path $projectRoot "nginx.conf"
$runtimeRoot = Join-Path $projectRoot "runtime\nginx"

if (-not (Test-Path -LiteralPath $nginxExe)) {
    throw "NGINX was not found at $nginxExe"
}

@(
    $runtimeRoot,
    (Join-Path $runtimeRoot "client_body_temp"),
    (Join-Path $runtimeRoot "proxy_temp"),
    (Join-Path $runtimeRoot "fastcgi_temp"),
    (Join-Path $runtimeRoot "uwsgi_temp"),
    (Join-Path $runtimeRoot "scgi_temp")
) | ForEach-Object {
    New-Item -ItemType Directory -Force -Path $_ | Out-Null
}

& $nginxExe -p "$nginxRoot\" -c $configPath -t

$existingListener = Get-NetTCPConnection -LocalPort 8080 -ErrorAction SilentlyContinue |
    Where-Object { $_.State -eq "Listen" } |
    Select-Object -First 1

if ($existingListener) {
    Write-Host "NGINX is already serving ORA controls at http://localhost:8080"
    return
}

Start-Process -FilePath $nginxExe -ArgumentList @("-p", "$nginxRoot\", "-c", $configPath) -WindowStyle Hidden
Start-Sleep -Seconds 1

Write-Host "NGINX serving ORA controls at http://localhost:8080"
