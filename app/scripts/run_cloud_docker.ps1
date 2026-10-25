param(
  [string]$StatePath,
  [ValidateRange(1024,65535)][int]$Port = 8765,
  [string]$ModelKeyFile,
  [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
if (-not $StatePath) { $StatePath = Join-Path $repoRoot 'app/.state/network-live' }
$StatePath = [IO.Path]::GetFullPath($StatePath)
if (Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue) {
  throw "Port $Port is already in use. Stop the current Aurora process before starting its replacement."
}
$keyDirectory = Join-Path $repoRoot 'app/.state/cloud-docker'
$keyFile = Join-Path $keyDirectory 'worker.env'
New-Item -ItemType Directory -Path $keyDirectory -Force | Out-Null
if (-not (Test-Path -LiteralPath $keyFile)) {
  $newKey = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
  [IO.File]::WriteAllText($keyFile,"AURORA_CLOUD_BROWSER_KEY=$newKey`n",[Text.UTF8Encoding]::new($false))
}
$keyMatch = [regex]::Match([IO.File]::ReadAllText($keyFile).Trim(), '^AURORA_CLOUD_BROWSER_KEY=([a-f0-9]{64})$')
if (-not $keyMatch.Success) { throw 'Invalid private worker key file; its value is not printed.' }
$nativeBinary = Join-Path $repoRoot 'app/.build/neko-aurora-computer.exe'
Push-Location $repoRoot
try {
  if (-not $SkipBuild) {
    $goTool = Join-Path $repoRoot 'app/.build/tools/go/bin/go.exe'
    if (-not (Test-Path -LiteralPath $goTool)) { $goTool = (Get-Command go -ErrorAction Stop).Source }
    Push-Location (Join-Path $repoRoot 'app/native')
    try { & $goTool build -o $nativeBinary ./cmd/neko-aurora; if ($LASTEXITCODE -ne 0) { throw 'Aurora build failed' } }
    finally { Pop-Location }
  }
  if (-not (Test-Path -LiteralPath $nativeBinary)) { throw 'Build Aurora before using -SkipBuild' }
  $dockerArguments = @('compose','--env-file',$keyFile,'-f','app/cloud-browser/compose.yaml','-f','app/cloud-browser/compose.local.yaml','up','-d')
  if (-not $SkipBuild) { $dockerArguments += '--build' }
  $dockerArguments += 'browser'
  & docker @dockerArguments
  if ($LASTEXITCODE -ne 0) { throw 'Docker worker startup failed' }
  if ($ModelKeyFile) {
    $modelKey = [IO.File]::ReadAllText([IO.Path]::GetFullPath($ModelKeyFile)).Trim()
    if ($modelKey -notmatch '^[A-Za-z0-9_.-]{24,}$') { throw 'Model key file must contain one token; its content is not printed.' }
    $env:ZAI_API_KEY = $modelKey
  }
  $env:AURORA_CLOUD_BROWSER_DOCKER = '1'
  $env:AURORA_CLOUD_BROWSER_URL = 'http://127.0.0.1:8877'
  $env:AURORA_CLOUD_BROWSER_KEY = $keyMatch.Groups[1].Value
  $env:AURORA_RUNTIME = 'http'
  $env:AURORA_COMPUTER = 'off'
  $env:AURORA_BRIEF_LOOP = 'off'
  $env:AURORA_BACKUP_EVERY = 'off'
  & $nativeBinary --state $StatePath --listen "127.0.0.1:$Port"
  exit $LASTEXITCODE
} finally { Pop-Location }
