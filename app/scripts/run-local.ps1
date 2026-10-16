param([string]$GoExe = "go", [switch]$Desktop, [string]$CoreFixtureConfig = "")
$ErrorActionPreference = "Stop"
$repoDir = Split-Path -Parent $PSScriptRoot
$buildDir = Join-Path $repoDir ".build"
New-Item -ItemType Directory -Path $buildDir -Force | Out-Null
Push-Location (Join-Path $repoDir "native")
try {
    & $GoExe build -o (Join-Path $buildDir "neko-control.exe") ./cmd/neko-control
    if ($LASTEXITCODE -ne 0) { throw "Go build failed" }
} finally { Pop-Location }
Push-Location (Join-Path $repoDir "host")
try {
    cargo build --locked
    if ($LASTEXITCODE -ne 0) { throw "Rust host build failed" }
    Copy-Item -LiteralPath (Join-Path $repoDir "host\target\debug\neko-host.exe") -Destination (Join-Path $buildDir "neko-host.exe")
} finally { Pop-Location }
$serviceArgs=@("--state", (Join-Path $repoDir ".state"), "--listen", "127.0.0.1:8765")
if ($CoreFixtureConfig) { $serviceArgs+=@("--core-fixture-config",(Resolve-Path -LiteralPath $CoreFixtureConfig).Path) }
if ($Desktop) {
    Push-Location (Join-Path $repoDir "desktop")
    try {
        cargo build --locked
        if ($LASTEXITCODE -ne 0) { throw "Desktop build failed" }
    } finally { Pop-Location }
    $service = Start-Process -FilePath (Join-Path $buildDir "neko-control.exe") -ArgumentList $serviceArgs -WindowStyle Hidden -PassThru
    try {
        $ready = $false
        for ($i=0; $i -lt 40; $i++) {
            if ($service.HasExited) { throw "Local service exited; check port or store" }
            try { Invoke-WebRequest -Uri "http://127.0.0.1:8765/" -TimeoutSec 1 | Out-Null; $ready=$true; break } catch { Start-Sleep -Milliseconds 100 }
        }
        if (-not $ready) { throw "Local service did not become ready" }
        $desktopExe = Join-Path $repoDir "desktop\target\debug\neko-desktop.exe"
        $window = Start-Process -FilePath $desktopExe -PassThru
        $window.WaitForExit()
    } finally { if (-not $service.HasExited) { $service.Kill(); $service.WaitForExit() } }
} else {
    Write-Output "Neko Bot local: http://127.0.0.1:8765"
    & (Join-Path $buildDir "neko-control.exe") @serviceArgs
}
