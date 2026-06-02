param(
    [string]$Url = "http://localhost:8081",
    [ValidateSet("edge", "chrome")]
    [string]$Browser = "edge"
)

$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
$profileRoot = Join-Path $projectRoot "runtime\controller-browser-profile"

New-Item -ItemType Directory -Force -Path $profileRoot | Out-Null

$candidates = if ($Browser -eq "chrome") {
    @(
        "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
        "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe"
    )
} else {
    @(
        "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe",
        "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe"
    )
}

$browserExe = $candidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1

if (-not $browserExe) {
    throw "Could not find $Browser. Open $Url manually in a browser profile with extensions disabled."
}

Start-Process -FilePath $browserExe -ArgumentList @(
    "--disable-extensions",
    "--user-data-dir=$profileRoot",
    "--new-window",
    $Url
)
