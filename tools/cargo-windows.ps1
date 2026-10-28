param([Parameter(ValueFromRemainingArguments = $true)][string[]]$CargoArgs)
$ErrorActionPreference = 'Stop'
if (-not $IsWindows) { throw 'Use cargo directly on non-Windows hosts.' }
$vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
$installations = @()
if (Test-Path -LiteralPath $vswhere) {
    $installations += & $vswhere -all -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
}
$vsRoot = Join-Path $env:ProgramFiles 'Microsoft Visual Studio'
if (Test-Path -LiteralPath $vsRoot) {
    $installations += Get-ChildItem -LiteralPath $vsRoot -Directory | ForEach-Object { Get-ChildItem -LiteralPath $_.FullName -Directory } | Select-Object -ExpandProperty FullName
}
$msvc = $installations | Select-Object -Unique | ForEach-Object {
    $versions = Join-Path $_ 'VC/Tools/MSVC'
    if (Test-Path -LiteralPath $versions) { Get-ChildItem -LiteralPath $versions -Directory }
} | Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName 'bin/Hostx64/x64/link.exe') } | Sort-Object { [version]$_.Name } -Descending | Select-Object -First 1
if (-not $msvc) { throw 'Microsoft C++ Build Tools x64 linker is required.' }
$kitsRoot = (Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows Kits\Installed Roots' -ErrorAction SilentlyContinue).KitsRoot10
if (-not $kitsRoot) { $kitsRoot = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits/10' }
$sdk = Get-ChildItem -LiteralPath (Join-Path $kitsRoot 'Lib') -Directory | Where-Object {
    (Test-Path -LiteralPath (Join-Path $_.FullName 'um/x64/kernel32.lib')) -and (Test-Path -LiteralPath (Join-Path $_.FullName 'ucrt/x64/ucrt.lib'))
} | Sort-Object { [version]$_.Name } -Descending | Select-Object -First 1
if (-not $sdk) { throw 'Windows SDK x64 libraries are required.' }
$before = @{ PATH = $env:PATH; LIB = $env:LIB; INCLUDE = $env:INCLUDE }
try {
    $env:PATH = (Join-Path $msvc.FullName 'bin/Hostx64/x64') + ';' + (Join-Path $kitsRoot "bin/$($sdk.Name)/x64") + ';' + $env:PATH
    $env:LIB = @((Join-Path $msvc.FullName 'lib/x64'), (Join-Path $sdk.FullName 'um/x64'), (Join-Path $sdk.FullName 'ucrt/x64')) -join ';'
    $env:INCLUDE = @((Join-Path $msvc.FullName 'include'), (Join-Path $kitsRoot "Include/$($sdk.Name)/ucrt"), (Join-Path $kitsRoot "Include/$($sdk.Name)/shared"), (Join-Path $kitsRoot "Include/$($sdk.Name)/um"), (Join-Path $kitsRoot "Include/$($sdk.Name)/winrt")) -join ';'
    Write-Host "MSVC $($msvc.Name); Windows SDK $($sdk.Name)"
    & cargo @CargoArgs
    $cargoResult = $LASTEXITCODE
} finally {
    $env:PATH = $before.PATH; $env:LIB = $before.LIB; $env:INCLUDE = $before.INCLUDE
}
exit $cargoResult
