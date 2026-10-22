param(
    [string]$Manifest = "",
    [string]$StateDirectory = "",
    [string]$CoreFixtureConfig = "",
    [string]$CoreProfileConfig = "",
    [UInt16]$Port = 8765,
    [switch]$Desktop,
    [switch]$VerifyDesktopOnly,
    [switch]$CheckOnly
)
$ErrorActionPreference = "Stop"
if ($Port -eq 0) { throw "Choose a nonzero local port" }
if ($VerifyDesktopOnly -and -not $Desktop) { throw "Desktop verification requires Desktop mode" }
$repoDir = [IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))
if (-not $Manifest) { $Manifest = Join-Path $repoDir ".checkpoints\local-product-release\launch.json" }
$receipt = Get-Content -LiteralPath $Manifest -Raw | ConvertFrom-Json
if ($receipt.version -ne 1 -or $receipt.scope -ne "local-journal" -or $receipt.appCommit -notmatch '^[a-f0-9]{40}$') { throw "Invalid local release receipt" }
$buildRoot = [IO.Path]::GetFullPath((Join-Path $repoDir ".build")) + [IO.Path]::DirectorySeparatorChar
function Resolve-ReleaseBinary($item, [string]$filename) {
    if (-not $item -or $item.sha256 -notmatch '^[a-f0-9]{64}$' -or [IO.Path]::IsPathRooted($item.path)) { throw "Invalid binary receipt: $filename" }
    $resolved = [IO.Path]::GetFullPath((Join-Path $repoDir $item.path))
    if (-not $resolved.StartsWith($buildRoot, [StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($resolved) -ne $filename) { throw "Binary outside reviewed build folder: $filename" }
    $cursor = Get-Item -LiteralPath $resolved
    while ($cursor -and $cursor.FullName -ne $repoDir) {
        if ($cursor.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw "Reparse point in binary path: $filename" }
        if ($cursor -is [IO.FileInfo]) { $cursor = $cursor.Directory } else { $cursor = $cursor.Parent }
    }
    if ((Get-FileHash -LiteralPath $resolved -Algorithm SHA256).Hash.ToLowerInvariant() -ne $item.sha256) { throw "Binary hash changed: $filename" }
    return $resolved
}
$controlExe = Resolve-ReleaseBinary $receipt.control "neko-aurora.exe"
$hostExe = Resolve-ReleaseBinary $receipt.host "neko-host.exe"
if ($receipt.core) { $coreExe = Resolve-ReleaseBinary $receipt.core "neko-core.exe" }
if ($receipt.writerHelper) { $writerHelperExe = Resolve-ReleaseBinary $receipt.writerHelper "neko-writer-lease.exe" }
if ($Desktop) {
    if ($receipt.desktopLaunchContract -ne 1) { throw "Verified desktop launch contract required; no legacy fallback" }
    $desktopExe = Resolve-ReleaseBinary $receipt.desktop "neko-desktop.exe"
}
if ($CheckOnly) { Write-Output "Verified local binaries for $($receipt.appCommit). No service started."; return }
if (-not $StateDirectory) { $StateDirectory = Join-Path $repoDir ".state" }
$statePath = [IO.Path]::GetFullPath($StateDirectory)
if ($statePath.Contains('"')) { throw "Invalid state path" }
$listener = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Parse("127.0.0.1"), $Port)
try { $listener.Start() } catch { throw "Port $Port is occupied. Choose an unused candidate port or close its owner yourself." } finally { $listener.Stop() }
$arguments = @("--state", $statePath, "--listen", "127.0.0.1:$Port", "--host-executable", $hostExe)
if ($Desktop) {
    $instanceReceipt = Join-Path $statePath ("desktop-launch-" + [Guid]::NewGuid().ToString("N") + ".json")
    $arguments += @("--desktop-launch-receipt", $instanceReceipt)
}
if ($CoreFixtureConfig) {
    $fixturePath = (Resolve-Path -LiteralPath $CoreFixtureConfig).Path
    if ($fixturePath.Contains('"')) { throw "Invalid fixture path" }
    $arguments += @("--core-fixture-config", $fixturePath)
}
if ($CoreProfileConfig) {
    $profilePath = (Resolve-Path -LiteralPath $CoreProfileConfig).Path
    if ($profilePath.Contains('"')) { throw "Invalid trusted profile registry path" }
    $arguments += @("--core-profile-config", $profilePath)
}
Write-Output "Neko Aurora local: http://127.0.0.1:$Port; state: $statePath"
if ($CoreProfileConfig) { Write-Output "Core profile inspection and execution require separate owner consent. Existing Core installation is preserved." }
elseif (-not $CoreFixtureConfig) { Write-Output "Runtime not configured: messages are saved and wait for execution. No provider is called." }
if (-not $Desktop) { & $controlExe @arguments; if ($LASTEXITCODE -ne 0) { throw "Local service exited with code $LASTEXITCODE" }; return }
$quotedArguments = $arguments | ForEach-Object { '"' + ($_ -replace '(\\+)$', '$1$1') + '"' }
$service = Start-Process -FilePath $controlExe -ArgumentList $quotedArguments -WindowStyle Hidden -PassThru
try {
    $ready = $false
    for ($i=0; $i -lt 50; $i++) {
        if ($service.HasExited) { throw "Owned service exited before ready; check its state/port" }
        $listeners = @(Get-NetTCPConnection -LocalAddress "127.0.0.1" -LocalPort $Port -State Listen -ErrorAction SilentlyContinue)
        if ($listeners.Where({ $_.OwningProcess -ne $service.Id }).Count) { throw "Port now belongs to another service; refusing to attach desktop" }
        if ($listeners.Where({ $_.OwningProcess -eq $service.Id }).Count -and (Test-Path -LiteralPath $instanceReceipt -PathType Leaf)) {
            $ready=$true; break
        }
        Start-Sleep -Milliseconds 100
    }
    if (-not $ready) { throw "Owned local service did not become ready" }
    $desktopArguments = @("--launch-receipt", $instanceReceipt, "--state", $statePath, "--endpoint", "http://127.0.0.1:$Port", "--control-sha256", $receipt.control.sha256)
    $quotedDesktopArguments = $desktopArguments | ForEach-Object { '"' + ($_ -replace '(\\+)$', '$1$1') + '"' }
    $verification = Start-Process -FilePath $desktopExe -ArgumentList ($quotedDesktopArguments + '"--verify-only"') -WindowStyle Hidden -PassThru
    $verification.WaitForExit()
    if ($verification.ExitCode -ne 0) { throw "Desktop instance binding was rejected" }
    Write-Output "Verified desktop instance on 127.0.0.1:$Port for state: $statePath"
    if (-not $VerifyDesktopOnly) {
        $window = Start-Process -FilePath $desktopExe -ArgumentList $quotedDesktopArguments -WindowStyle Normal -PassThru
        $window.WaitForExit()
        if ($window.ExitCode -ne 0) { throw "Desktop instance closed with a validation/startup error" }
    }
} finally {
    if (-not $service.HasExited) { $service.Kill(); $service.WaitForExit() }
}
